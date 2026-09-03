package unpackerr

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/julienschmidt/httprouter"
)

const (
	sessionCookie    = "session"
	headerAPIKey     = "X-Api-Key" //nolint:gosec // HTTP header name, not a secret
	authHeaderBearer = "Bearer "
	loginFailDelay   = 3 * time.Second
	cookieHashBytes  = 32
	cookieBlockBytes = 32
	maxLoginBody     = 4096
)

type ctxKey int

const authCtxKey ctxKey = 1

type authInfo struct {
	Username    string   `json:"username"`
	APIKey      string   `json:"apiKey"`
	Auth        string   `json:"auth"`
	Via         string   `json:"via"`
	Permissions []string `json:"permissions"`
}

type loginRequest struct {
	Name string `json:"name"`
	KDF  string `json:"kdf"`
}

func (w *WebServer) initCookies() {
	if w == nil || w.cookies != nil {
		return
	}

	w.cookies = securecookie.New(
		securecookie.GenerateRandomKey(cookieHashBytes),
		securecookie.GenerateRandomKey(cookieBlockBytes),
	)
}

func (u *Unpackerr) registerAuthRoutes() {
	base := path.Join(u.Webserver.URLBase, "api", "auth")
	u.Webserver.router.POST(path.Join(base, "login"), u.loginHandler)
	u.Webserver.router.POST(path.Join(base, "logout"), u.logoutHandler)
	u.Webserver.router.GET(path.Join(base, "me"), u.requireAuth(u.meHandler))
}

func (u *Unpackerr) requireAuth(next httprouter.Handle) httprouter.Handle {
	return func(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
		info, ok := u.authenticate(request)
		if !ok {
			writeJSON(response, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})

			return
		}

		ctx := context.WithValue(request.Context(), authCtxKey, info)
		next(response, request.WithContext(ctx), params)
	}
}

func (u *Unpackerr) authenticate(request *http.Request) (authInfo, bool) {
	if key := requestAPIKey(request); key != "" {
		if perms := u.Webserver.PermissionsForKey(key); perms != nil {
			return authInfo{
				Username:    u.Webserver.keyName(key),
				APIKey:      key,
				Auth:        u.Webserver.UIPassword.Type().String(),
				Via:         "key",
				Permissions: perms,
			}, true
		}

		return authInfo{}, false
	}

	if info, ok := u.proxyAuth(request); ok {
		return info, true
	}

	if user, ok := u.sessionUser(request); ok {
		return u.sessionAuth(user), true
	}

	return authInfo{}, false
}

func requestAPIKey(request *http.Request) string {
	if key := strings.TrimSpace(request.Header.Get(headerAPIKey)); key != "" {
		return key
	}

	auth := strings.TrimSpace(request.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), strings.ToLower(authHeaderBearer)) {
		return strings.TrimSpace(auth[len(authHeaderBearer):])
	}

	return ""
}

func (u *Unpackerr) proxyAuth(request *http.Request) (authInfo, bool) {
	pass := u.Webserver.UIPassword
	if !pass.Webauth() || !u.Webserver.allow.Contains(request.RemoteAddr) {
		return authInfo{}, false
	}

	user := defaultUIUser

	if pass.Type() == AuthHeader {
		if header := strings.TrimSpace(request.Header.Get(pass.Header())); header != "" {
			user = header
		} else {
			return authInfo{}, false
		}
	}

	return u.sessionAuth(user), true
}

func (u *Unpackerr) sessionAuth(user string) authInfo {
	info := authInfo{
		Username:    user,
		APIKey:      u.Webserver.adminAPIKey(),
		Auth:        u.Webserver.UIPassword.Type().String(),
		Via:         "session",
		Permissions: AllPermissions(),
	}
	if u.Webserver.UIPassword.Webauth() {
		info.Via = u.Webserver.UIPassword.Type().String()
	}

	return info
}

func (w *WebServer) keyName(key string) string {
	for idx := range w.APIKeys {
		if w.APIKeys[idx].Key == key {
			return w.APIKeys[idx].Name
		}
	}

	return ""
}

func (u *Unpackerr) sessionUser(request *http.Request) (string, bool) {
	if u.Webserver.cookies == nil {
		return "", false
	}

	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}

	values := make(map[string]string)
	if err := u.Webserver.cookies.Decode(sessionCookie, cookie.Value, &values); err != nil {
		return "", false
	}

	user := values["username"]
	if user == "" {
		return "", false
	}

	return user, true
}

func (u *Unpackerr) setSession(response http.ResponseWriter, user string) {
	if u.Webserver.cookies == nil {
		return
	}

	encoded, err := u.Webserver.cookies.Encode(sessionCookie, map[string]string{"username": user})
	if err != nil {
		return
	}

	http.SetCookie(response, &http.Cookie{ //nolint:gosec // Secure follows TLS; plaintext LAN HTTP is supported
		Name:     sessionCookie,
		Value:    encoded,
		Path:     u.Webserver.URLBase,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "",
	})
}

func (u *Unpackerr) loginHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	start := time.Now()
	ok := u.handleLogin(response, request)

	if ok || u.Webserver.failDelay <= 0 {
		return
	}

	if wait := u.Webserver.failDelay - time.Since(start); wait > 0 {
		time.Sleep(wait)
	}
}

func (u *Unpackerr) handleLogin(response http.ResponseWriter, request *http.Request) bool {
	if u.Webserver.UIPassword.Webauth() {
		writeJSON(response, http.StatusForbidden, map[string]string{"error": "password login is disabled"})
		return false
	}

	var body loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxLoginBody)).Decode(&body); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = defaultUIUser
	}

	if !u.Webserver.UIPassword.Valid(name, strings.TrimSpace(body.KDF)) {
		writeJSON(response, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}

	u.setSession(response, name)
	writeJSON(response, http.StatusOK, u.sessionAuth(name))

	return true
}

func (u *Unpackerr) logoutHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	http.SetCookie(response, &http.Cookie{ //nolint:gosec // clearing the session cookie; Secure follows TLS
		Name:     sessionCookie,
		Value:    "",
		Path:     u.Webserver.URLBase,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "",
	})
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (u *Unpackerr) meHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	info, _ := request.Context().Value(authCtxKey).(authInfo)
	writeJSON(response, http.StatusOK, info)
}

func writeJSON(response http.ResponseWriter, code int, msg any) {
	body, err := json.Marshal(msg)
	if err != nil {
		http.Error(response, `{"error":"encode"}`, http.StatusInternalServerError)

		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(code)
	_, _ = response.Write(append(body, '\n'))
}

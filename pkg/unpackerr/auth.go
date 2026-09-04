package unpackerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/julienschmidt/httprouter"
)

const (
	sessionCookie        = "session"
	headerAPIKey         = "X-Api-Key" //nolint:gosec // HTTP header name, not a secret
	headerForwardedProto = "X-Forwarded-Proto"
	authHeaderBearer     = "Bearer "
	loginFailDelay       = 3 * time.Second
	loginReadTimeout     = 5 * time.Second
	cookieHashBytes      = 32
	cookieBlockBytes     = 32
	maxLoginBody         = 4096
)

var (
	errSessionCodec   = errors.New("session codec is not initialized")
	errSessionEntropy = errors.New("could not generate session keys")
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

func (w *WebServer) initCookies() error {
	if w == nil || w.cookies != nil {
		return nil
	}

	hash := securecookie.GenerateRandomKey(cookieHashBytes)
	block := securecookie.GenerateRandomKey(cookieBlockBytes)

	if hash == nil || block == nil {
		return errSessionEntropy
	}

	w.cookies = securecookie.New(hash, block)

	return nil
}

func (u *Unpackerr) registerAuthRoutes() {
	base := path.Join(u.Webserver.URLBase, "api", "auth")
	if u.Webserver.cookies != nil {
		u.Webserver.router.POST(path.Join(base, "login"), u.loginHandler)
		u.Webserver.router.POST(path.Join(base, "logout"), u.logoutHandler)
	}

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
	if key := strings.TrimSpace(request.Header.Get(headerAPIKey)); key != "" {
		return u.authAPIKey(key)
	}

	if bearer := requestBearer(request); bearer != "" {
		if info, ok := u.authAPIKey(bearer); ok {
			return info, true
		}
	}

	if info, ok := u.proxyAuth(request); ok {
		return info, true
	}

	if user, ok := u.sessionUser(request); ok {
		return u.sessionAuth(user), true
	}

	return authInfo{}, false
}

func (u *Unpackerr) authAPIKey(key string) (authInfo, bool) {
	if key == "" {
		return authInfo{}, false
	}

	perms := u.Webserver.PermissionsForKey(key)
	if perms == nil {
		return authInfo{}, false
	}

	return authInfo{
		Username:    u.Webserver.keyName(key),
		APIKey:      key,
		Auth:        u.Webserver.UIPassword.Type().String(),
		Via:         "key",
		Permissions: perms,
	}, true
}

func requestAPIKey(request *http.Request) string {
	if key := strings.TrimSpace(request.Header.Get(headerAPIKey)); key != "" {
		return key
	}

	return requestBearer(request)
}

func requestBearer(request *http.Request) string {
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

func (u *Unpackerr) setSession(response http.ResponseWriter, request *http.Request, user string) error {
	if u.Webserver.cookies == nil {
		return errSessionCodec
	}

	encoded, err := u.Webserver.cookies.Encode(sessionCookie, map[string]string{"username": user})
	if err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}

	http.SetCookie(response, u.sessionCookie(request, encoded, 0))

	return nil
}

func (u *Unpackerr) sessionCookie(request *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure follows TLS or a trusted X-Forwarded-Proto
		Name:     sessionCookie,
		Value:    value,
		Path:     u.Webserver.URLBase,
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   u.cookieSecure(request),
	}
}

func (u *Unpackerr) cookieSecure(request *http.Request) bool {
	if u.Webserver != nil && u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "" {
		return true
	}

	if request == nil {
		return false
	}

	if request.TLS != nil {
		return true
	}

	return u.trustedForwardedHTTPS(request)
}

func (u *Unpackerr) trustedForwardedHTTPS(request *http.Request) bool {
	if u.Webserver == nil || strings.LastIndex(request.RemoteAddr, ":") < 0 {
		return false
	}

	if !u.Webserver.allow.Contains(request.RemoteAddr) {
		return false
	}

	return forwardedProto(request) == "https"
}

func forwardedProto(request *http.Request) string {
	proto := strings.TrimSpace(request.Header.Get(headerForwardedProto))
	if proto == "" {
		return ""
	}

	if idx := strings.LastIndexByte(proto, ','); idx >= 0 {
		proto = proto[idx+1:]
	}

	return strings.ToLower(strings.TrimSpace(proto))
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

	ctrl := http.NewResponseController(response)
	_ = ctrl.SetReadDeadline(time.Now().Add(loginReadTimeout))

	var body loginRequest

	err := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxLoginBody)).Decode(&body)
	_ = ctrl.SetReadDeadline(time.Time{})

	if err != nil {
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

	if err := u.setSession(response, request, name); err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": "session"})
		return false
	}

	writeJSON(response, http.StatusOK, u.sessionAuth(name))

	return true
}

func (u *Unpackerr) logoutHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	http.SetCookie(response, u.sessionCookie(request, "", -1))
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
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(code)
	_, _ = response.Write(append(body, '\n'))
}

package unpackerr

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeQbit mimics the parts of qBittorrent 4.3 (Web API 2.8) that Unpackerr uses:
// a cookie session from auth/login, a Referer check on login, 403 without a
// session, 404 for an unknown hash, and 200 "Fails." for bad credentials.
type fakeQbit struct {
	*httptest.Server

	user, pass string
	logins     atomic.Int32
	requests   atomic.Int32
	loginCode  int // 0 = qBittorrent 4.x style 200 "Ok.", or e.g. 204 for 5.2+.

	mu       sync.Mutex
	sessions map[string]bool
	torrents map[string][]qbitFile
}

func newFakeQbit(t *testing.T, base string) *fakeQbit {
	t.Helper()

	fake := &fakeQbit{
		user:     "admin",
		pass:     "s3cret",
		sessions: map[string]bool{},
		torrents: map[string][]qbitFile{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc(base+"api/v2/auth/login", fake.login)
	mux.HandleFunc(base+"api/v2/torrents/files", fake.files)
	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)

	return fake
}

func (f *fakeQbit) login(resp http.ResponseWriter, req *http.Request) {
	f.requests.Add(1)
	f.logins.Add(1)

	if req.Method != http.MethodPost || !strings.HasPrefix(req.Referer(), f.URL) {
		http.Error(resp, "Unauthorized", http.StatusUnauthorized) // CSRF / Referer check.
		return
	}

	if req.PostFormValue("username") != f.user || req.PostFormValue("password") != f.pass {
		_, _ = resp.Write([]byte("Fails."))
		return
	}

	sid := "sid" + req.PostFormValue("username") + time.Now().String()

	f.mu.Lock()
	f.sessions[sid] = true
	f.mu.Unlock()

	//nolint:gosec // like qBittorrent over plain HTTP; a Secure cookie would not come back.
	http.SetCookie(resp, &http.Cookie{Name: "SID", Value: sid, Path: "/", HttpOnly: true})

	if f.loginCode != 0 {
		resp.WriteHeader(f.loginCode)
		return
	}

	_, _ = resp.Write([]byte("Ok."))
}

func (f *fakeQbit) files(resp http.ResponseWriter, req *http.Request) {
	f.requests.Add(1)

	cookie, err := req.Cookie("SID")

	f.mu.Lock()
	authed := err == nil && f.sessions[cookie.Value]
	files, known := f.torrents[req.URL.Query().Get("hash")]
	f.mu.Unlock()

	switch {
	case !authed:
		http.Error(resp, "Forbidden", http.StatusForbidden)
	case !known:
		http.Error(resp, "Not Found", http.StatusNotFound)
	default:
		if err := json.NewEncoder(resp).Encode(files); err != nil {
			http.Error(resp, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (f *fakeQbit) expireSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sessions = map[string]bool{}
}

// add makes testHash a known torrent with these files.
func (f *fakeQbit) add(files ...qbitFile) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.torrents[testHash] = files
}

const testHash = "0123456789abcdef0123456789abcdef01234567"

func TestQbitClientLoginAndFiles(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/qbittorrent/")
	fake.add(qbitFile{Name: "Show/a.rar", Size: 10, Priority: 1})

	client := newQbitClient(fake.URL+"/qbittorrent/", fake.user, fake.pass)

	files, err := client.files(context.Background(), testHash)
	if err != nil || len(files) != 1 || files[0].Name != "Show/a.rar" || files[0].Size != 10 {
		t.Fatalf("files: %+v, %v", files, err)
	}

	if _, err := client.files(context.Background(), testHash); err != nil {
		t.Fatal(err)
	}

	if got := fake.logins.Load(); got != 1 {
		t.Fatalf("logins: %d, want 1 (the session cookie must be reused)", got)
	}

	fake.expireSessions()

	if _, err := client.files(context.Background(), testHash); err != nil {
		t.Fatalf("after session expiry: %v", err)
	}

	if got := fake.logins.Load(); got != 2 {
		t.Fatalf("logins after expiry: %d, want 2", got)
	}
}

func TestQbitClientLogin204(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	fake.loginCode = http.StatusNoContent
	fake.add(qbitFile{Name: "a.mkv", Size: 1, Priority: 1})

	if _, err := newQbitClient(fake.URL+"/", fake.user, fake.pass).files(context.Background(), testHash); err != nil {
		t.Fatal(err)
	}
}

func TestQbitClientRejectedLoginIsNotRetried(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	client := newQbitClient(fake.URL+"/", fake.user, "wrong")

	for range 3 {
		if _, err := client.files(context.Background(), testHash); !errors.Is(err, errQbitLogin) {
			t.Fatalf("err: %v, want errQbitLogin", err)
		}
	}

	if got := fake.logins.Load(); got != 1 {
		t.Fatalf("logins: %d, want 1; a rejected login must not be repeated", got)
	}

	if err := client.login(context.Background()); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("login error must not be nil or contain the password: %v", err)
	}
}

func TestQbitClientUnknownTorrent(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	client := newQbitClient(fake.URL+"/", fake.user, fake.pass)

	for range 2 {
		if _, err := client.files(context.Background(), testHash); !errors.Is(err, errQbitUnknown) {
			t.Fatalf("err: %v, want errQbitUnknown", err)
		}
	}

	if !client.retryAt.IsZero() || client.rejected {
		t.Fatal("an unknown torrent must not pause or disable the client")
	}
}

func TestQbitClientUnreachable(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()
	_ = listener.Close() // nothing listens here now.

	client := newQbitClient("http://"+addr+"/", "admin", "s3cret")

	if _, err := client.files(context.Background(), testHash); err == nil || errors.Is(err, errQbitLogin) {
		t.Fatalf("err: %v, want a connection error", err)
	}

	if _, err := client.files(context.Background(), testHash); !errors.Is(err, errQbitPaused) {
		t.Fatalf("second err: %v, want errQbitPaused", err)
	}
}

func TestNormalizeQbitURL(t *testing.T) {
	t.Parallel()

	cases := []struct{ raw, want string }{
		{raw: "", want: ""},
		{raw: "  https://host/qbittorrent", want: "https://host/qbittorrent/"},
		{raw: "http://127.0.0.1:8080/", want: "http://127.0.0.1:8080/"},
	}

	for _, test := range cases {
		if got, err := normalizeQbitURL(test.raw); err != nil || got != test.want {
			t.Fatalf("%q: got %q, %v; want %q", test.raw, got, err, test.want)
		}
	}

	if _, err := normalizeQbitURL("host:8080"); !errors.Is(err, errQbitURL) {
		t.Fatalf("err: %v, want errQbitURL", err)
	}
}

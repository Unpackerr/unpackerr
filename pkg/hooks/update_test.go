package hooks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/cnfg"
)

func TestWantUpdate(t *testing.T) {
	t.Parallel()

	disabled := false
	enabled := true
	telegram := "https://api.telegram.org/botx/sendMessage"
	discord := "https://discord.com/api/webhooks/1/x"

	tests := []struct {
		name string
		hook *Config
		want bool
	}{
		{name: "nil", hook: nil, want: false},
		{name: "discord default", hook: &Config{URL: discord}, want: true},
		{name: "telegram opt out", hook: &Config{URL: telegram, Update: &disabled}, want: false},
		{name: "discord opt in", hook: &Config{URL: discord, Update: &enabled}, want: true},
		{name: "slack", hook: &Config{URL: "https://hooks.slack.com/services/T/B/X", Update: &enabled}, want: false},
		{name: "ntfy", hook: &Config{URL: "https://ntfy.sh/topic"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.hook.WantUpdate(); got != test.want {
				t.Fatalf("WantUpdate %v want %v", got, test.want)
			}
		})
	}
}

func TestDiscordCreateThenEdit(t *testing.T) {
	t.Parallel()

	capture := &reqCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		capture.add(req)

		if req.Method == http.MethodPost {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"msg-1"}`))

			return
		}

		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ids := map[string]string{}
	hook := updatingHook(t, srv.URL, ProfileDiscord)
	item := updatingItem(hook, extract.QUEUED, ids)

	if drainWorker(t, item) != 1 {
		t.Fatal("create")
	}

	item.Payload = &Payload{Path: "/dl/show", Event: extract.EXTRACTING, IDs: map[string]any{"title": "Show"}}
	if drainWorker(t, item) != 1 {
		t.Fatal("edit")
	}

	methods, paths, queries := capture.snapshot()
	if len(methods) != 2 || methods[0] != http.MethodPost || methods[1] != http.MethodPatch {
		t.Fatalf("methods %v", methods)
	}

	if !strings.Contains(queries[0], "wait=true") {
		t.Fatalf("create query %q", queries[0])
	}

	if !strings.HasSuffix(paths[1], "/messages/msg-1") {
		t.Fatalf("edit path %q", paths[1])
	}

	if ids[hook.Identity()] != "msg-1" {
		t.Fatalf("saved %q", ids[hook.Identity()])
	}
}

func TestDiscordGoneRecreates(t *testing.T) {
	t.Parallel()

	capture := &reqCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		capture.add(req)

		switch req.Method {
		case http.MethodPatch:
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"msg-2"}`))
		}
	}))
	t.Cleanup(srv.Close)

	ids := map[string]string{"hook": "stale"}
	hook := updatingHook(t, srv.URL, ProfileDiscord)
	hook.Name = "hook"
	item := updatingItem(hook, extract.EXTRACTED, ids)

	if drainWorker(t, item) != 1 {
		t.Fatal("after")
	}

	methods, _, _ := capture.snapshot()
	if len(methods) != 2 || methods[0] != http.MethodPatch || methods[1] != http.MethodPost {
		t.Fatalf("methods %v", methods)
	}

	if ids["hook"] != "msg-2" {
		t.Fatalf("saved %q", ids["hook"])
	}
}

func TestTelegramCreateThenEdit(t *testing.T) {
	t.Parallel()

	capture := &reqCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		body := capture.add(req)

		if strings.HasSuffix(req.URL.Path, "/sendMessage") {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))

			return
		}

		if !strings.Contains(string(body), `"message_id":42`) {
			t.Errorf("edit body %s", body)
		}

		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ids := map[string]string{}
	hook := updatingHook(t, srv.URL+"/botTOKEN/sendMessage", ProfileTelegram)
	item := updatingItem(hook, extract.QUEUED, ids)

	if drainWorker(t, item) != 1 {
		t.Fatal("create")
	}

	item.Payload = &Payload{Path: "/dl/show", Event: extract.EXTRACTING, IDs: map[string]any{"title": "Show"}}
	if drainWorker(t, item) != 1 {
		t.Fatal("edit")
	}

	_, paths, _ := capture.snapshot()
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/sendMessage") ||
		!strings.HasSuffix(paths[1], "/editMessageText") {
		t.Fatalf("paths %v", paths)
	}

	if ids[hook.Identity()] != "42" {
		t.Fatalf("saved %q", ids[hook.Identity()])
	}
}

func TestIdentity(t *testing.T) {
	t.Parallel()

	if got := (*Config)(nil).Identity(); got != "" {
		t.Fatalf("nil %q", got)
	}

	if got := (&Config{Name: " discord ", URL: "https://example"}).Identity(); got != "discord" {
		t.Fatalf("name %q", got)
	}

	if got := (&Config{URL: "https://example"}).Identity(); got != "https://example" {
		t.Fatalf("url %q", got)
	}

	if got := (&Config{Command: "/bin/hook"}).Identity(); got != "/bin/hook" {
		t.Fatalf("command %q", got)
	}
}

func TestSendWithLogStaysCreate(t *testing.T) {
	t.Parallel()

	capture := &reqCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		capture.add(req)
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	hook := updatingHook(t, srv.URL, ProfileDiscord)
	payload := &Payload{Path: "/dl", Event: extract.QUEUED, IDs: map[string]any{"title": "Show"}}

	if err := SendWithLog(noopLogger{}, hook, payload); err != nil {
		t.Fatal(err)
	}

	methods, _, queries := capture.snapshot()
	if len(methods) != 1 || methods[0] != http.MethodPost || queries[0] != "" {
		t.Fatalf("sample must POST the original URL, got %v %v", methods, queries)
	}
}

type reqCapture struct {
	mutex   sync.Mutex
	methods []string
	paths   []string
	queries []string
}

func (capture *reqCapture) add(req *http.Request) []byte {
	body, _ := io.ReadAll(req.Body)

	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	capture.methods = append(capture.methods, req.Method)
	capture.paths = append(capture.paths, req.URL.Path)
	capture.queries = append(capture.queries, req.URL.RawQuery)

	return body
}

func (capture *reqCapture) snapshot() ([]string, []string, []string) {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()

	return append([]string{}, capture.methods...),
		append([]string{}, capture.paths...),
		append([]string{}, capture.queries...)
}

func updatingItem(hook *Config, event extract.Status, ids map[string]string) *Item {
	return &Item{
		Config:  hook,
		Payload: &Payload{Path: "/dl/show", Event: event, IDs: map[string]any{"title": "Show"}},
		LookupID: func() string {
			return ids[hook.Identity()]
		},
		SaveID: func(msgID string) {
			ids[hook.Identity()] = msgID
		},
	}
}

func updatingHook(t *testing.T, rawURL, template string) *Config {
	t.Helper()

	hook := &Config{
		Name:     "hook",
		URL:      rawURL,
		TempName: template,
		Nickname: "Unpackerr",
		Silent:   true,
		Timeout:  cnfg.Duration{Duration: time.Second},
	}
	if err := ValidateWebhooks([]*Config{hook}, time.Second); err != nil {
		t.Fatal(err)
	}

	return hook
}

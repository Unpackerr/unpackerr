package configdef

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

type liveRoot struct {
	Debug     bool         `toml:"debug"`
	Interval  liveDuration `toml:"interval"`
	Webserver *liveWeb     `toml:"webserver"`
	Sonarr    []liveStarr  `toml:"sonarr"`
	Folder    []liveFolder `toml:"folder"`
	Webhook   []liveHook   `toml:"webhook"`
}

type liveWeb struct {
	Metrics    bool                `toml:"metrics"`
	ListenAddr string              `toml:"listen_addr"`
	APIKeys    []liveAPIKey        `toml:"api_keys"`
	Roles      map[string]liveRole `toml:"roles"`
}

type liveAPIKey struct {
	Name  string   `toml:"name"`
	Key   string   `toml:"key"`
	Roles []string `toml:"roles"`
}

type liveRole struct {
	Permissions []string `toml:"permissions"`
}

type liveStarr struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}

type liveFolder struct {
	Path        string        `toml:"path"`
	DeleteAfter *liveDuration `toml:"delete_after"`
}

type liveHook struct {
	URL    string       `toml:"url"`
	Token  string       `toml:"token"`
	Events []liveStatus `toml:"events"`
}

type liveStatus uint8

func (s liveStatus) MarshalText() ([]byte, error) {
	return []byte("queued"), nil
}

type liveDuration struct{ time.Duration }

func (d liveDuration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func TestExampleTOMLContainsWebserver(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).ExampleTOML()
	for _, want := range []string{"[webserver]", "listen_addr", "metrics"} {
		if !strings.Contains(body, want) {
			t.Fatalf("example TOML missing %q", want)
		}
	}
}

func TestRenderLiveCommentsDefaults(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Debug:    true,
		Interval: liveDuration{2 * time.Minute},
		Webserver: &liveWeb{
			Metrics:    false,
			ListenAddr: "0.0.0.0:5656",
		},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})

	if !strings.Contains(body, "debug = true") {
		t.Fatalf("expected live debug=true, got:\n%s", snippet(body, "debug"))
	}

	if !strings.Contains(body, "#listen_addr = ") && !strings.Contains(body, "# listen_addr = ") {
		t.Fatalf("default listen_addr should stay commented, got:\n%s", snippet(body, "listen_addr"))
	}

	if !strings.Contains(body, "#[[sonarr]]") && !strings.Contains(body, "# [[sonarr]]") {
		t.Fatalf("empty sonarr list should keep the commented template, got:\n%s", snippet(body, "sonarr"))
	}
}

func TestRenderLiveWritesNonDefaultList(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Sonarr: []liveStarr{{URL: "http://sonarr:8989", APIKey: "0123456789abcdef0123456789abcdef"}},
		Folder: []liveFolder{{Path: "/downloads/watch"}},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "#[[sonarr]]") {
		t.Fatal("configured sonarr instance should not use the commented template header")
	}

	if !strings.Contains(body, "[[sonarr]]") {
		t.Fatalf("missing live [[sonarr]]:\n%s", snippet(body, "sonarr"))
	}

	if !strings.Contains(body, `url = "http://sonarr:8989"`) {
		t.Fatalf("missing live sonarr url:\n%s", snippet(body, "url"))
	}

	if !strings.Contains(body, "[[folder]]") {
		t.Fatalf("missing live [[folder]]:\n%s", snippet(body, "folder"))
	}
}

func TestRenderLiveNilDurationStaysCommented(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).RenderTOML(&liveRoot{
		Folder: []liveFolder{{Path: "/downloads/watch"}},
	}, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "delete_after = ''") {
		t.Fatalf("nil delete_after must not write an empty string:\n%s", snippet(body, "delete_after"))
	}

	if !strings.Contains(body, "# delete_after = ") && !strings.Contains(body, "#delete_after = ") {
		t.Fatalf("nil delete_after should keep the commented default:\n%s", snippet(body, "delete_after"))
	}
}

func TestRenderLiveEventsStayNumeric(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).RenderTOML(&liveRoot{
		Webhook: []liveHook{{
			URL:    "https://example.invalid/hook",
			Token:  "tok",
			Events: []liveStatus{1, 4},
		}},
	}, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, `"queued"`) {
		t.Fatalf("events must not use TextMarshaler strings:\n%s", snippet(body, "events ="))
	}

	if !strings.Contains(body, "events = [1, 4]") && !strings.Contains(body, "events = [1,4]") {
		t.Fatalf("events should be integer IDs:\n%s", snippet(body, "events ="))
	}

	if !strings.Contains(body, `token = "tok"`) {
		t.Fatalf("missing live webhook token:\n%s", snippet(body, "token"))
	}
}

func TestRenderLiveAPIKeysAndRoles(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Webserver: &liveWeb{
			ListenAddr: "127.0.0.1:5656",
			APIKeys: []liveAPIKey{{
				Name:  "home",
				Key:   strings.Repeat("f", 60),
				Roles: []string{"stats"},
			}},
			Roles: map[string]liveRole{
				"stats": {Permissions: []string{"read:system:stats"}},
			},
		},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "api_keys =") {
		t.Fatalf("api_keys must be nested tables, got:\n%s", snippet(body, "api_keys"))
	}

	if strings.Contains(body, "roles = {") || strings.Contains(body, "roles = [stats]") {
		t.Fatalf("roles must be nested tables, got:\n%s", snippet(body, "roles"))
	}

	if !strings.Contains(body, "[[webserver.api_keys]]") {
		t.Fatalf("missing [[webserver.api_keys]]:\n%s", snippet(body, "webserver"))
	}

	if !strings.Contains(body, "[webserver.roles.stats]") {
		t.Fatalf("missing [webserver.roles.stats]:\n%s", snippet(body, "roles"))
	}

	decoded := struct {
		Webserver liveWeb `toml:"webserver"`
	}{}
	if err := toml.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("written TOML must parse: %v\n%s", err, snippet(body, "webserver"))
	}

	if len(decoded.Webserver.APIKeys) != 1 || decoded.Webserver.APIKeys[0].Name != "home" {
		t.Fatalf("decoded keys %+v", decoded.Webserver.APIKeys)
	}

	if decoded.Webserver.Roles["stats"].Permissions[0] != "read:system:stats" {
		t.Fatalf("decoded roles %+v", decoded.Webserver.Roles)
	}
}

func TestFormatTOMLNilCollections(t *testing.T) {
	t.Parallel()

	if got := formatTOML("roles", []string(nil)); got != "[]" {
		t.Fatalf("nil slice: %s", got)
	}

	if got := formatTOML("roles", map[string]string(nil)); got != "{}" {
		t.Fatalf("nil map: %s", got)
	}
}

func TestTOMLKeyQuotesControlChars(t *testing.T) {
	t.Parallel()

	got := tomlKey("\x01")
	if strings.Contains(got, `\x`) {
		t.Fatalf("TOML does not allow Go \\x escapes: %s", got)
	}

	if got != `"\u0001"` {
		t.Fatalf("got %s", got)
	}
}

func TestRenderLiveNilAPIKeyRoles(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Webserver: &liveWeb{
			ListenAddr: "127.0.0.1:5656",
			APIKeys: []liveAPIKey{{
				Name: "home",
				Key:  strings.Repeat("f", 60),
			}},
		},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})
	if strings.Contains(body, "roles = ''") {
		t.Fatalf("nil roles must not stringify:\n%s", snippet(body, "roles"))
	}

	if !strings.Contains(body, "roles = []") {
		t.Fatalf("nil roles must be an array:\n%s", snippet(body, "api_keys"))
	}
}

func TestFormatTOMLDoesNotSprintStructs(t *testing.T) {
	t.Parallel()

	got := formatTOML("api_keys", []liveAPIKey{{Name: "home"}})
	if strings.Contains(got, "{") || strings.Contains(got, "home") {
		t.Fatalf("struct slices must not use fmt.Sprint, got %q", got)
	}
}

func TestFormatTOMLPathQuotes(t *testing.T) {
	t.Parallel()

	if got := strings.TrimSpace(formatTOML("path", "/downloads")); got != "'/downloads'" {
		t.Fatalf("unix path %s", got)
	}

	win := strings.TrimSpace(formatTOML("extract_path", `C:\downloads`))
	if win != `"C:\\downloads"` {
		t.Fatalf("windows path must keep double quotes, got %s", win)
	}

	quoted := strings.TrimSpace(formatTOML("path", `C:\foo"bar`))
	if !strings.HasPrefix(quoted, `"`) {
		t.Fatalf("embedded quote must keep double quotes, got %s", quoted)
	}
}

func TestRenderLivePersist(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Webserver: &liveWeb{ListenAddr: "0.0.0.0:5656"},
	}

	body := schema.RenderTOML(live, RenderOpts{
		Mode:    RenderLive,
		Persist: []string{"webserver.listen_addr"},
	})

	if strings.Contains(body, "# listen_addr = ") || strings.Contains(body, "#listen_addr = ") {
		t.Fatalf("persisted listen_addr should be live, got:\n%s", snippet(body, "listen_addr"))
	}

	if !strings.Contains(body, `listen_addr = "0.0.0.0:5656"`) {
		t.Fatalf("missing persisted listen_addr:\n%s", snippet(body, "listen_addr"))
	}
}

func TestAtomicWriteBackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path := filepath.Join(dir, "unpackerr.conf")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "new\n" {
		t.Fatalf("got %q", got)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}

	if string(bak) != "old\n" {
		t.Fatalf("backup %q", bak)
	}

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && stat.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", stat.Mode().Perm())
	}
}

func MustLoad(t *testing.T) *Config {
	t.Helper()

	schema, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	return schema
}

func snippet(body, key string) string {
	idx := strings.Index(body, key)
	if idx < 0 {
		return body
	}

	start := max(idx-80, 0)
	end := min(idx+160, len(body))

	return body[start:end]
}

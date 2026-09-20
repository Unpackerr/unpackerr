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
	Debug     bool                  `toml:"debug"`
	Interval  liveDuration          `toml:"interval"`
	Webserver *liveWeb              `toml:"webserver"`
	Sonarr    map[string]liveStarr  `toml:"sonarr"`
	Folder    map[string]liveFolder `toml:"folder"`
	Webhook   map[string]liveHook   `toml:"webhook"`
	Hooks     *liveHooks            `toml:"hooks"`
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

type liveHooks struct {
	CustomIDs map[string]string `toml:"custom_ids"`
	Titles    map[string]string `toml:"titles"`
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
	for _, want := range []string{"[webserver]", "[folders]", "[hooks]", "listen_addr", "metrics"} {
		if !strings.Contains(body, want) {
			t.Fatalf("example TOML missing %q", want)
		}
	}

	for _, bad := range []string{"#[webserver]", "#[folders]", "#[hooks]"} {
		if strings.Contains(body, bad) {
			t.Fatalf("singleton header must stay live, found %q", bad)
		}
	}

	var dest map[string]any
	if _, err := toml.Decode(body, &dest); err != nil {
		t.Fatalf("example TOML must parse: %v", err)
	}
}

func TestExampleTOMLCommentsStarrApps(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).ExampleTOML()
	for _, header := range []string{"[sonarr.0]", "[radarr.0]", "[lidarr.0]", "[readarr.0]"} {
		if strings.Contains(body, "\n"+header+"\n") {
			t.Fatalf("example must comment %s so first-run does not warn, got:\n%s",
				header, snippet(body, header))
		}

		if !strings.Contains(body, "#"+header) {
			t.Fatalf("example missing commented %s", header)
		}
	}

	var parsed map[string]any
	if _, err := toml.Decode(body, &parsed); err != nil {
		t.Fatal(err)
	}

	for _, app := range []string{"sonarr", "radarr", "lidarr", "readarr"} {
		if _, ok := parsed[app]; ok {
			t.Fatalf("commented %s header must not decode to a live instance: %#v", app, parsed[app])
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

	if !strings.Contains(body, "#[sonarr.0]") && !strings.Contains(body, "# [sonarr.0]") {
		t.Fatalf("empty sonarr map should keep the commented template, got:\n%s", snippet(body, "sonarr"))
	}
}

func TestRenderLiveWritesNonDefaultList(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Sonarr: map[string]liveStarr{"0": {URL: "http://sonarr:8989", APIKey: "0123456789abcdef0123456789abcdef"}},
		Folder: map[string]liveFolder{"watch": {Path: "/downloads/watch"}},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "#[sonarr.0]") {
		t.Fatal("configured sonarr instance should not use the commented template header")
	}

	if !strings.Contains(body, "[sonarr.0]") {
		t.Fatalf("missing live [sonarr.0]:\n%s", snippet(body, "sonarr"))
	}

	if !strings.Contains(body, `url = "http://sonarr:8989"`) {
		t.Fatalf("missing live sonarr url:\n%s", snippet(body, "url"))
	}

	if !strings.Contains(body, "[folder.watch]") {
		t.Fatalf("missing live [folder.watch]:\n%s", snippet(body, "folder"))
	}
}

func TestRenderLiveNilDurationStaysCommented(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).RenderTOML(&liveRoot{
		Folder: map[string]liveFolder{"0": {Path: "/downloads/watch"}},
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
		Webhook: map[string]liveHook{"discord": {
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
				"stats": {Permissions: []string{"system:stats:read"}},
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

	if decoded.Webserver.Roles["stats"].Permissions[0] != "system:stats:read" {
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

func TestFormatTOMLStringMapIsInlineTable(t *testing.T) {
	t.Parallel()

	got := formatTOML("custom_ids", map[string]string{
		"url":   "https://unpackerr.example",
		"host":  "unpackerr",
		"title": "ignored",
	})
	if strings.Contains(got, "custom_ids =") || strings.Count(got, "=") != 3 {
		t.Fatalf("must be an inline table, got %q", got)
	}

	if !strings.HasPrefix(got, "{ ") || !strings.HasSuffix(got, " }") {
		t.Fatalf("inline braces: %q", got)
	}

	decoded := struct {
		IDs map[string]string `toml:"custom_ids"`
	}{}
	if err := toml.Unmarshal([]byte("custom_ids = "+got+"\n"), &decoded); err != nil {
		t.Fatalf("inline table must parse: %v (%s)", err, got)
	}

	if decoded.IDs["url"] != "https://unpackerr.example" || decoded.IDs["host"] != "unpackerr" {
		t.Fatalf("decoded %+v", decoded.IDs)
	}
}

func TestRenderLiveHookStringMaps(t *testing.T) {
	t.Parallel()

	body := MustLoad(t).RenderTOML(&liveRoot{
		Hooks: &liveHooks{
			CustomIDs: map[string]string{"asdasdas": "asdasd"},
			Titles:    map[string]string{"extracting": "Archive Found"},
		},
	}, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "custom_ids =") || strings.Contains(body, "titles = {") ||
		strings.Contains(body, "custom_ids = asdasdas") || strings.Contains(body, "titles = extracting") {
		t.Fatalf("string maps must be nested tables, not assignments:\n%s", snippet(body, "[hooks]"))
	}

	if !strings.Contains(body, "[hooks.custom_ids]") || !strings.Contains(body, "[hooks.titles]") {
		t.Fatalf("missing nested hook tables:\n%s", snippet(body, "[hooks]"))
	}

	if !strings.Contains(body, `asdasdas = "asdasd"`) || !strings.Contains(body, `extracting = "Archive Found"`) {
		t.Fatalf("missing map rows:\n%s", snippet(body, "[hooks]"))
	}

	decoded := struct {
		Hooks liveHooks `toml:"hooks"`
	}{}
	if err := toml.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("written TOML must parse: %v\n%s", err, snippet(body, "[hooks]"))
	}

	if decoded.Hooks.CustomIDs["asdasdas"] != "asdasd" || decoded.Hooks.Titles["extracting"] != "Archive Found" {
		t.Fatalf("decoded %+v", decoded.Hooks)
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

func TestRenderLiveNestedAPIKeysAndRoles(t *testing.T) {
	t.Parallel()

	schema := MustLoad(t)
	live := &liveRoot{
		Webserver: &liveWeb{
			ListenAddr: "0.0.0.0:5656",
			APIKeys: []liveAPIKey{{
				Name:  "admin",
				Key:   strings.Repeat("k", 60),
				Roles: []string{"admin"},
			}},
			Roles: map[string]liveRole{
				"stats": {Permissions: []string{"system:stats:read"}},
			},
		},
	}

	body := schema.RenderTOML(live, RenderOpts{Mode: RenderLive})

	if strings.Contains(body, "api_keys =") {
		t.Fatalf("struct-slice api_keys must be tables, got:\n%s", snippet(body, "api_keys"))
	}

	if !strings.Contains(body, "[[webserver.api_keys]]") {
		t.Fatalf("missing [[webserver.api_keys]]:\n%s", snippet(body, "api_keys"))
	}

	if !strings.Contains(body, `name = "admin"`) || !strings.Contains(body, `roles = ["admin"]`) {
		t.Fatalf("missing api key fields:\n%s", snippet(body, "[[webserver.api_keys]]"))
	}

	if strings.Contains(body, "roles = {") {
		t.Fatalf("struct-map roles must be tables, got:\n%s", snippet(body, "roles"))
	}

	if !strings.Contains(body, "[webserver.roles.stats]") {
		t.Fatalf("missing [webserver.roles.stats]:\n%s", snippet(body, "roles"))
	}

	if !strings.Contains(body, `permissions = ["system:stats:read"]`) {
		t.Fatalf("missing role permissions:\n%s", snippet(body, "permissions"))
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

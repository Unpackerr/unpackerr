package configdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type liveRoot struct {
	Debug     bool         `toml:"debug"`
	Interval  liveDuration `toml:"interval"`
	Webserver *liveWeb     `toml:"webserver"`
	Sonarr    []liveStarr  `toml:"sonarr"`
	Folder    []liveFolder `toml:"folder"`
}

type liveWeb struct {
	Metrics    bool   `toml:"metrics"`
	ListenAddr string `toml:"listen_addr"`
}

type liveStarr struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}

type liveFolder struct {
	Path string `toml:"path"`
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

	if stat.Mode().Perm() != 0o600 {
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

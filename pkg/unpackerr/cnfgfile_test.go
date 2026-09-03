package unpackerr

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
	"golift.io/cnfgfile"
)

func TestWriteConfigFileRoundTrip(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Config.Debug = true
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	loaded := New()
	if err := cnfgfile.Unmarshal(loaded.Config, unpack.ConfigFile); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !loaded.Config.Debug {
		t.Fatal("expected debug true after decode")
	}

	if _, err := os.Stat(unpack.ConfigFile + ".bak"); err == nil {
		t.Fatal("first write should not create a backup")
	}

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(unpack.ConfigFile + ".bak"); err != nil {
		t.Fatalf("rewrite should keep a backup: %v", err)
	}
}

func TestWriteConfigFilePreservesLiveValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	passFile := filepath.Join(dir, "pass.txt")

	if err := os.WriteFile(passFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := liveWriteUnpackerr(dir, passFile)
	if err := unpack.setPasswords(); err != nil {
		t.Fatal(err)
	}

	if len(unpack.Passwords) != 1 || unpack.Passwords[0] != "secret" {
		t.Fatalf("expanded passwords %q", unpack.Passwords)
	}

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	assertLiveWriteBody(t, string(body), passFile)

	loaded := New()
	if err := cnfgfile.Unmarshal(loaded.Config, unpack.ConfigFile); err != nil {
		t.Fatalf("decode written config: %v\n%s", err, body)
	}

	assertLiveWriteLoaded(t, loaded, passFile)
}

func TestWriteConfigFileRequiresPath(t *testing.T) {
	t.Parallel()

	if err := New().writeConfigFile(); err == nil {
		t.Fatal("expected an error without ConfigFile")
	}
}

func TestConfigTOMLTagsInSchema(t *testing.T) {
	t.Parallel()

	schema, err := configdef.Load()
	if err != nil {
		t.Fatal(err)
	}

	skip := map[string]struct{}{
		"keep_history": {}, // undocumented until the history API
		"path":         {}, // legacy StarrConfig alias for paths
		"http_pass":    {},
		"http_user":    {},
		"username":     {},
		"password":     {},
	}

	missing := missingSchemaTags(reflect.TypeFor[Config](), schema.ParamNames(), skip)
	if len(missing) > 0 {
		t.Fatalf("toml tags missing from definitions.yml: %s", strings.Join(missing, ", "))
	}
}

func liveWriteUnpackerr(dir, passFile string) *Unpackerr {
	unpack := New()
	unpack.ConfigFile = filepath.Join(dir, "unpackerr.conf")
	unpack.Config.Debug = true
	unpack.Passwords = StringSlice{"filepath:" + passFile}
	unpack.Webserver.Pprof = true
	unpack.Folders = []*FolderConfig{{Path: "/downloads/watch"}}
	unpack.Webhook = []*WebhookConfig{{
		URL:    "https://example.invalid/hook",
		Token:  "tok",
		Events: ExtractStatuses{QUEUED, EXTRACTED},
	}}
	unpack.Sonarr = []*SonarrConfig{{
		URL:      "http://127.0.0.1:8989",
		APIKey:   strings.Repeat("a", 32),
		ValidSSL: true,
		Paths:    StringSlice{"/custom"},
	}}

	return unpack
}

func assertLiveWriteBody(t *testing.T, text, passFile string) {
	t.Helper()

	switch {
	case !strings.Contains(text, "filepath:"+passFile):
		t.Fatal("filepath: password source must be preserved")
	case strings.Contains(text, "secret"):
		t.Fatal("expanded password leaked into the config file")
	case strings.Contains(text, "delete_after = ''"):
		t.Fatal("nil delete_after must not write an empty string")
	case strings.Contains(text, `"queued"`):
		t.Fatal("events must stay numeric")
	case !strings.Contains(text, "pprof = true"):
		t.Fatal("missing live pprof")
	case !strings.Contains(text, `token = "tok"`):
		t.Fatal("missing live webhook token")
	case !strings.Contains(text, "valid_ssl = true"):
		t.Fatal("missing live valid_ssl")
	}
}

func assertLiveWriteLoaded(t *testing.T, loaded *Unpackerr, passFile string) {
	t.Helper()

	switch {
	case !loaded.Config.Debug:
		t.Fatal("debug")
	case loaded.Webserver == nil || !loaded.Webserver.Pprof:
		t.Fatal("pprof")
	case len(loaded.Passwords) != 1 || loaded.Passwords[0] != "filepath:"+passFile:
		t.Fatalf("passwords %q", loaded.Passwords)
	case len(loaded.Folders) != 1 || loaded.Folders[0].Path != "/downloads/watch":
		t.Fatal("folder path")
	case loaded.Folders[0].DeleteAfter != nil:
		t.Fatal("nil delete_after should stay unset")
	case len(loaded.Webhook) != 1 || loaded.Webhook[0].Token != "tok":
		t.Fatal("webhook token")
	case len(loaded.Webhook[0].Events) != 2 ||
		loaded.Webhook[0].Events[0] != QUEUED || loaded.Webhook[0].Events[1] != EXTRACTED:
		t.Fatalf("webhook events %v", loaded.Webhook[0].Events)
	case len(loaded.Sonarr) != 1 || !loaded.Sonarr[0].ValidSSL:
		t.Fatal("valid_ssl")
	}
}

func missingSchemaTags(typ reflect.Type, known, skip map[string]struct{}) []string {
	seen := map[reflect.Type]struct{}{}
	found := map[string]struct{}{}

	collectTOMLTags(typ, known, skip, found, seen)

	missing := make([]string, 0, len(found))
	for name := range found {
		missing = append(missing, name)
	}

	return missing
}

func collectTOMLTags(typ reflect.Type, known, skip, found map[string]struct{}, seen map[reflect.Type]struct{}) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	if _, dup := seen[typ]; dup {
		return
	}

	seen[typ] = struct{}{}

	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "-" {
			continue
		}

		if tag != "" {
			if _, skipped := skip[tag]; !skipped {
				if _, ok := known[tag]; !ok {
					found[tag] = struct{}{}
				}
			}
		}

		collectTOMLTags(field.Type, known, skip, found, seen)
	}
}

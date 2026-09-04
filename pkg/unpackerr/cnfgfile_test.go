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
	unpack.snapshotFileConfig()

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
	unpack.snapshotFileConfig()

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

	assertLiveWriteBody(t, string(body))

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

func TestWriteConfigFileKeepsFilepathAfterParse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "sonarr.key")
	secret := strings.Repeat("k", 32)

	if err := os.WriteFile(keyFile, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.ConfigFile = filepath.Join(dir, "unpackerr.conf")
	unpack.Sonarr = []*SonarrConfig{{
		URL:    "http://127.0.0.1:8989",
		APIKey: filePrefix + keyFile,
	}}

	unpack.snapshotFileConfig()

	if _, err := cnfgfile.Parse(unpack.Config, &cnfgfile.Opts{Prefix: filePrefix}); err != nil {
		t.Fatal(err)
	}

	if unpack.Sonarr[0].APIKey != secret {
		t.Fatalf("Parse should expand api_key, got %q", unpack.Sonarr[0].APIKey)
	}

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(body)
	if strings.Contains(text, secret) {
		t.Fatal("expanded api key leaked into the config file")
	}

	loaded := New()
	if err := cnfgfile.Unmarshal(loaded.Config, unpack.ConfigFile); err != nil {
		t.Fatalf("decode: %v\n%s", err, text)
	}

	if len(loaded.Sonarr) != 1 || loaded.Sonarr[0].APIKey != filePrefix+keyFile {
		t.Fatalf("api_key %q", loaded.Sonarr[0].APIKey)
	}
}

func TestUnmarshalConfigDoesNotPersistEnvSecrets(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "unpackerr.conf")
	body := "debug = false\n[webserver]\nlisten_addr = \"127.0.0.1:0\"\nui_password = \"\"\n"

	if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	secret := strings.Repeat("E", 32)

	t.Setenv("UN_DEBUG", "true")
	t.Setenv("UN_SONARR_0_URL", "http://127.0.0.1:8989")
	t.Setenv("UN_SONARR_0_API_KEY", secret)

	unpack := New()
	unpack.ConfigFile = conf

	if _, _, _, err := unpack.unmarshalConfig(); err != nil {
		t.Fatal(err)
	}

	if !unpack.Config.Debug {
		t.Fatal("live config should take UN_DEBUG")
	}

	if len(unpack.Sonarr) != 1 || unpack.Sonarr[0].APIKey != secret {
		t.Fatalf("live sonarr %+v", unpack.Sonarr)
	}

	if unpack.fileConfig == nil || unpack.fileConfig.Debug || len(unpack.fileConfig.Sonarr) != 0 {
		t.Fatalf("file snapshot took env values: %+v", unpack.fileConfig)
	}

	written, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, secret) || strings.Contains(text, "debug = true") {
		t.Fatalf("env values leaked into the config file:\n%s", text)
	}
}

func TestUnmarshalConfigEnvUIPasswordStaysOutOfFile(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "unpackerr.conf")
	body := "[webserver]\nlisten_addr = \"127.0.0.1:0\"\nui_password = \"fileuser:filepass99\"\n"

	if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("UN_WEBSERVER_UI_PASSWORD", "envuser:envsecret1")

	unpack := New()
	unpack.ConfigFile = conf

	if _, _, _, err := unpack.unmarshalConfig(); err != nil {
		t.Fatal(err)
	}

	if !unpack.Webserver.UIPassword.ValidPlain("envuser", "envsecret1") {
		t.Fatal("live password should come from the env overlay")
	}

	stored := unpack.fileConfig.Webserver.UIPassword.Val()
	if stored != "fileuser:filepass99" {
		t.Fatalf("file snapshot password %q", stored)
	}

	written, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, "envuser") || strings.Contains(text, "envsecret1") {
		t.Fatalf("env ui_password leaked into the config file:\n%s", text)
	}

	if !strings.Contains(text, "fileuser:filepass99") {
		t.Fatalf("file ui_password should be unchanged:\n%s", text)
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
		HTTPUser: "basicuser",
		HTTPPass: "basicpass",
		Username: "nativeuser",
		Password: "nativepass",
		ValidSSL: true,
		Paths:    StringSlice{"/custom"},
	}}

	return unpack
}

func assertLiveWriteBody(t *testing.T, text string) {
	t.Helper()

	switch {
	case !strings.Contains(text, `filepath:`):
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
	case !strings.Contains(text, `http_user = "basicuser"`) || !strings.Contains(text, `http_pass = "basicpass"`):
		t.Fatal("missing live http basic auth")
	case !strings.Contains(text, `username = "nativeuser"`) || !strings.Contains(text, `password = "nativepass"`):
		t.Fatal("missing live native auth")
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
	case loaded.Sonarr[0].HTTPUser != "basicuser" || loaded.Sonarr[0].HTTPPass != "basicpass":
		t.Fatal("http basic auth")
	case loaded.Sonarr[0].Username != "nativeuser" || loaded.Sonarr[0].Password != "nativepass":
		t.Fatal("native auth")
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

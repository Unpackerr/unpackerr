package unpackerr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
	"golift.io/starr"
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

func TestWriteConfigFileAPIKeysAndRoles(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("k", apiKeyMinLen)
	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  "home",
		Key:   key,
		Roles: []string{"stats"},
	}}
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.snapshotFileConfig()

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(body)
	if strings.Contains(text, "api_keys =") || strings.Contains(text, "roles = {") ||
		strings.Contains(text, "roles = [stats]") {
		t.Fatalf("auth tables must not be inlined:\n%s", text)
	}

	if !strings.Contains(text, "[[webserver.api_keys]]") || !strings.Contains(text, "[webserver.roles.stats]") {
		t.Fatalf("missing nested auth tables:\n%s", text)
	}

	loaded := New()
	if err := cnfgfile.Unmarshal(loaded.Config, unpack.ConfigFile); err != nil {
		t.Fatalf("decode: %v\n%s", err, text)
	}

	if err := loaded.Webserver.validateAuth(); err != nil {
		t.Fatal(err)
	}

	if !loaded.Webserver.HasPermission(key, PermReadSystemStats) {
		t.Fatal("reloaded key should keep its role")
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

	if unpack.envUsed["DEBUG"] != "true" {
		t.Fatalf("env used DEBUG: %+v", unpack.envUsed)
	}

	if unpack.envUsed["SONARR_0_URL"] != "http://127.0.0.1:8989" {
		t.Fatalf("env used URL: %+v", unpack.envUsed)
	}

	if unpack.envUsed["SONARR_0_API_KEY"] != secret {
		t.Fatalf("env used API key: %+v", unpack.envUsed)
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

func TestUnmarshalConfigAdoptsWhisparrBeforePersist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	conf := filepath.Join(dir, "unpackerr.conf")
	key := strings.Repeat("b", apiKeyMinLength)
	body := "[webserver]\nlisten_addr = \"127.0.0.1:0\"\nui_password = \"\"\n\n" +
		"[[radarr]]\nurl = \"http://radarr:7878\"\napi_key = \"" + strings.Repeat("a", apiKeyMinLength) + "\"\n\n" +
		"[[whisparr]]\nurl = \"http://whisparr:6969\"\napi_key = \"" + key + "\"\n"

	if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.ConfigFile = conf

	if _, _, _, err := unpack.unmarshalConfig(); err != nil {
		t.Fatal(err)
	}

	if len(unpack.Whisparr) != 0 {
		t.Fatalf("live whisparr leftover: %d", len(unpack.Whisparr))
	}

	if unpack.fileConfig == nil || len(unpack.fileConfig.Whisparr) != 0 {
		t.Fatalf("file whisparr leftover: %+v", unpack.fileConfig)
	}

	written, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, "[[whisparr]]") {
		t.Fatalf("password persist dropped [[whisparr]] instead of folding it:\n%s", text)
	}

	if !strings.Contains(text, "http://whisparr:6969") || !strings.Contains(text, "[[radarr]]") {
		t.Fatalf("folded whisparr missing from radarr persist:\n%s", text)
	}
}

func TestWriteConfigFileAdoptsWhisparr(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Radarr = []*RadarrConfig{{
		URL:    "http://radarr:7878",
		APIKey: strings.Repeat("a", apiKeyMinLength),
	}}
	unpack.Whisparr = []*RadarrConfig{{
		URL:    "http://whisparr:6969",
		APIKey: strings.Repeat("b", apiKeyMinLength),
	}}
	unpack.snapshotFileConfig()
	unpack.adoptWhisparr()

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, "[[whisparr]]") {
		t.Fatalf("--reset persist dropped [[whisparr]]:\n%s", text)
	}

	if !strings.Contains(text, "http://whisparr:6969") {
		t.Fatalf("folded whisparr url missing:\n%s", text)
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

func TestUnmarshalConfigENVRolesAndAPIKeys(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "unpackerr.conf")
	body := "[webserver]\nlisten_addr = \"127.0.0.1:0\"\nui_password = \"\"\n"

	if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	key := strings.Repeat("Z", apiKeyMinLen)

	t.Setenv("UN_WEBSERVER_ROLES_stats_PERMISSIONS_0", PermReadSystemStats)
	t.Setenv("UN_WEBSERVER_ROLES_env_only_PERMISSIONS_0", PermReadSystemStats)
	t.Setenv("UN_WEBSERVER_API_KEYS_0_NAME", "envhome")
	t.Setenv("UN_WEBSERVER_API_KEYS_0_KEY", key)
	t.Setenv("UN_WEBSERVER_API_KEYS_0_ROLES_0", "env_only")

	unpack := New()
	unpack.ConfigFile = conf

	if _, _, _, err := unpack.unmarshalConfig(); err != nil {
		t.Fatal(err)
	}

	if unpack.envUsed["WEBSERVER_ROLES_stats_PERMISSIONS_0"] != PermReadSystemStats {
		t.Fatalf("env used mixed-case role key: %+v", unpack.envUsed)
	}

	if unpack.Webserver.Roles["stats"].Permissions[0] != PermReadSystemStats {
		t.Fatalf("stats role %+v", unpack.Webserver.Roles)
	}

	if unpack.Webserver.Roles["env_only"].Permissions[0] != PermReadSystemStats {
		t.Fatalf("env_only role %+v", unpack.Webserver.Roles)
	}

	if !unpack.Webserver.HasPermission(key, PermReadSystemStats) {
		t.Fatal("env api key should get env_only")
	}

	written, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, "\n[[webserver.api_keys]]") ||
		strings.Contains(text, "[webserver.roles.env_only]") ||
		strings.Contains(text, key) || strings.Contains(text, "envhome") {
		t.Fatalf("env auth leaked into the config file:\n%s", text)
	}
}

func TestConfigTOMLTagsInSchema(t *testing.T) {
	t.Parallel()

	schema, err := configdef.Load()
	if err != nil {
		t.Fatal(err)
	}

	skip := map[string]struct{}{
		"path":     {}, // legacy StarrConfig alias for paths
		"key":      {}, // nested [[webserver.api_keys]]; parent api_keys is in the schema
		"whisparr": {}, // accepted on load, folded into [[radarr]]
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

// TestWriteConfigFileFullRoundTrip proves a UI save cannot brick a config: every
// section is populated, rendered to TOML, loaded back through the real loader,
// and compared field for field. Values equal to a definitions.yml default are
// written commented-out and re-filled at startup, so every value here is
// non-default. filepath: values must survive as written.
func TestWriteConfigFileFullRoundTrip(t *testing.T) { //nolint:funlen // one field per line is the point.
	t.Parallel()

	key := strings.Repeat("k", apiKeyMinLen)
	starrKey := strings.Repeat("s", apiKeyMinLength)
	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Config.Debug = true
	unpack.Quiet = true
	unpack.Activity = true
	unpack.Parallel = 3
	unpack.MaxRetries = 7
	unpack.RemnantAction = "delete"
	unpack.LogFile = "/var/log/unpackerr.log"
	unpack.LogFiles = 4
	unpack.LogFileMb = 12
	unpack.FileMode = "0640"
	unpack.DirMode = "0750"
	unpack.KeepHistory = 50
	unpack.Interval = cnfg.Duration{Duration: 3 * time.Minute}
	unpack.StartDelay = cnfg.Duration{Duration: 2 * time.Minute}
	unpack.Passwords = StringSlice{"plain-pass", "filepath:/run/secrets/rarpass"}
	unpack.Webserver.ListenAddr = "127.0.0.1:5656"
	unpack.Webserver.URLBase = "/unpackerr/"
	unpack.Webserver.Metrics = true
	unpack.Webserver.SSLCrtFile = "/etc/ssl/unpackerr.crt"
	unpack.Webserver.SSLKeyFile = "/etc/ssl/unpackerr.key"
	unpack.Webserver.Upstreams = StringSlice{"10.0.0.0/8"}
	unpack.Webserver.UIPassword = "filepath:/run/secrets/ui"
	unpack.Webserver.APIKeys = []APIKey{
		{Name: "admin", Key: key, Roles: []string{RoleAdmin}},
		{Name: "home", Key: strings.Repeat("h", apiKeyMinLen), Roles: []string{"stats"}},
	}
	unpack.Webserver.Roles = map[string]Role{"stats": {Permissions: []string{PermReadSystemStats}}}

	starrConf := func(url string) StarrConfig {
		secret := "filepath:/run/secrets/" + strings.TrimPrefix(url, "http://")

		return StarrConfig{ //nolint:modernize // URL and APIKey are promoted; keeping Config explicit reads better.
			Config: starr.Config{URL: url, APIKey: secret},
			Paths:  StringSlice{"/downloads", "/mnt/dl"}, Protocols: "torrent",
			DeleteOrig: true, Syncthing: true, ValidSSL: true, MaxBytes: "10GB",
			DeleteDelay: cnfg.Duration{Duration: 7 * time.Minute}, Timeout: cnfg.Duration{Duration: 42 * time.Second},
		}
	}

	unpack.Sonarr = []*SonarrConfig{{StarrConfig: starrConf("http://sonarr:8989")}}
	unpack.Radarr = []*RadarrConfig{
		{StarrConfig: starrConf("http://radarr:7878")},
		{StarrConfig: starrConf("http://whisparr:6969")},
	}
	unpack.Lidarr = []*LidarrConfig{{StarrConfig: starrConf("http://lidarr:8686"), SplitFlac: true}}
	unpack.Readarr = []*ReadarrConfig{{StarrConfig: starrConf("http://readarr:8787")}}
	unpack.Readarr[0].APIKey = starrKey
	unpack.Folder.Interval = cnfg.Duration{Duration: 4 * time.Second}
	unpack.Folder.Buffer = 5000
	unpack.Folders = []*FolderConfig{{
		Path: "/watch", ExtractPath: "/extracted", DeleteOrig: true, MoveBack: true, ExtractISOs: true,
		DeleteAfter: &cnfg.Duration{Duration: 11 * time.Minute}, MaxNested: 2, MaxFiles: 99, MaxRatio: 3.5,
		ExcludePaths: []string{"/watch/skip"},
	}}
	unpack.Webhook = []*WebhookConfig{{ //nolint:gosec // filepath: reference, not a credential.
		Name: "discord", URL: "https://discord.example/hook", CType: "text/plain",
		Timeout: cnfg.Duration{Duration: 9 * time.Second}, IgnoreSSL: true, Silent: true,
		Events: ExtractStatuses{EXTRACTED, EXTRACTFAILED}, Exclude: hooks.StringSlice{"lidarr"},
		Nickname: "Bot", Token: "filepath:/run/secrets/hook", Channel: "general",
	}}
	unpack.Cmdhook = []*WebhookConfig{{
		Name: "notify", Command: "/usr/local/bin/notify.sh --flag", Shell: true,
		Timeout: cnfg.Duration{Duration: 5 * time.Second}, Events: ExtractStatuses{IMPORTED},
	}}
	unpack.snapshotFileConfig()

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	loaded := New()
	if err := cnfgfile.Unmarshal(loaded.Config, unpack.ConfigFile); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}

	want, err := json.MarshalIndent(unpack.fileConfig, "", " ")
	if err != nil {
		t.Fatal(err)
	}

	got, err := json.MarshalIndent(cloneConfig(loaded.Config), "", " ")
	if err != nil {
		t.Fatal(err)
	}

	if string(want) != string(got) {
		t.Fatalf("round trip changed the config.\nwant:\n%s\ngot:\n%s\ntoml:\n%s", want, got, body)
	}

	for _, secret := range []string{
		"filepath:/run/secrets/rarpass", "filepath:/run/secrets/ui",
		"filepath:/run/secrets/sonarr:8989", "filepath:/run/secrets/hook",
	} {
		if !strings.Contains(string(body), secret) {
			t.Fatalf("filepath: value %q was not written as-is:\n%s", secret, body)
		}
	}
}

func TestEnvSuffixesAndSecrets(t *testing.T) {
	t.Parallel()

	got := envSuffixes(cnfg.Pairs{
		"UN_DEBUG":                               "true",
		"UN_SONARR_0_API_KEY":                    "k",
		"UN_WEBSERVER_ROLES_stats_PERMISSIONS_0": "system:stats:read",
	}, "UN")
	if got["DEBUG"] != "true" || got["SONARR_0_API_KEY"] != "k" {
		t.Fatalf("%v", got)
	}

	if got["WEBSERVER_ROLES_stats_PERMISSIONS_0"] != "system:stats:read" {
		t.Fatalf("mixed-case suffix lost: %v", got)
	}

	if _, ok := got["WEBSERVER_ROLES_STATS_PERMISSIONS_0"]; ok {
		t.Fatalf("uppercasing collapsed the role key: %v", got)
	}

	app := envSuffixes(cnfg.Pairs{"APP__DEBUG": "true"}, "APP_")
	if app["DEBUG"] != "true" {
		t.Fatalf("prefix APP_ should strip APP__: %v", app)
	}

	if _, ok := app["_DEBUG"]; ok {
		t.Fatalf("trimmed prefix left a leading underscore: %v", app)
	}

	for _, name := range []string{
		"SONARR_0_API_KEY", "PASSWORD", "WEBSERVER_UI_PASSWORD", "PASSWORDS_0",
		"SONARR_0_HTTP_PASS", "WEBSERVER_API_KEYS_0_KEY", "WEBHOOK_0_TOKEN",
	} {
		if !envValueSecret(name) {
			t.Fatalf("expected secret %s", name)
		}
	}

	for _, name := range []string{"DEBUG", "WEBSERVER_SSL_KEY_FILE", "WEBSERVER_API_KEYS_0_NAME"} {
		if envValueSecret(name) {
			t.Fatalf("unexpected secret %s", name)
		}
	}
}

func TestValidateSonarrSkipsShortAPIKey(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{
		URL:    "http://127.0.0.1:8989",
		APIKey: "short",
	}}

	if err := validateStarrList(unpack, &unpack.Sonarr, starr.Sonarr); err != nil {
		t.Fatal(err)
	}

	if len(unpack.Sonarr) != 0 {
		t.Fatalf("short key must skip, got %d", len(unpack.Sonarr))
	}
}

func TestCheckStarrName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "Sportarr", "Sonarr 4K", "Fightarr-2"} {
		if err := checkStarrName(name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}

	for _, name := range []string{
		`Sport"arr`,
		"Sport'arr",
		"Sport`arr",
		`Sport\arr`,
		"Sport{arr}",
		"Sport<arr>",
		"Sport\narr",
	} {
		if err := checkStarrName(name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("%q: got %v want ErrInvalidName", name, err)
		}
	}
}

func TestValidateAppUsesInstanceLabel(t *testing.T) {
	t.Parallel()

	unpack := New()
	key := strings.Repeat("a", apiKeyMinLength)

	conf := &StarrConfig{Name: "Sportarr"}
	conf.URL = "ftp://127.0.0.1:8989"
	conf.APIKey = key

	err := unpack.validateApp(conf, starr.Sonarr)
	if err == nil || !strings.Contains(err.Error(), "Sportarr") {
		t.Fatalf("invalid URL: %v", err)
	}

	conf = &StarrConfig{Name: "Sportarr"}
	conf.URL = "http://127.0.0.1:8989"
	conf.APIKey = "short"

	err = unpack.validateApp(conf, starr.Sonarr)
	if err == nil || !strings.Contains(err.Error(), "Sportarr") {
		t.Fatalf("short key: %v", err)
	}

	conf = &StarrConfig{Name: "Sportarr", MaxBytes: "nope"}
	conf.URL = "http://127.0.0.1:8989"
	conf.APIKey = key

	err = unpack.validateApp(conf, starr.Sonarr)
	if err == nil || !strings.Contains(err.Error(), "Sportarr") {
		t.Fatalf("max bytes: %v", err)
	}
}

func TestValidateStarrListRejectsBadName(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{
		Name:   `Sport"arr`,
		URL:    "http://127.0.0.1:8989",
		APIKey: strings.Repeat("a", apiKeyMinLength),
	}}

	err := validateStarrList(unpack, &unpack.Sonarr, starr.Sonarr)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("got %v want ErrInvalidName", err)
	}

	if len(unpack.Sonarr) != 1 {
		t.Fatalf("bad name must not skip the instance, got %d", len(unpack.Sonarr))
	}
}

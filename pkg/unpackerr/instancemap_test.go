package unpackerr

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"golift.io/cnfg"
)

func instanceMap[T any](items []*T) InstanceMap[T] {
	out := make(InstanceMap[T], len(items))
	for idx, item := range items {
		out[strconv.Itoa(idx)] = item
	}

	return out
}

func TestInstanceMapJSONArrayAndObject(t *testing.T) {
	t.Parallel()

	var fromArray InstanceMap[SonarrConfig]
	if err := json.Unmarshal([]byte(`[{"url":"http://sonarr:8989","name":"TV"}]`), &fromArray); err != nil {
		t.Fatal(err)
	}

	if got := fromArray["0"]; got == nil || got.URL != "http://sonarr:8989" || got.Name != "TV" {
		t.Fatalf("array row: %+v", fromArray)
	}

	var fromObject InstanceMap[SonarrConfig]
	if err := json.Unmarshal([]byte(
		`{"uhd":{"url":"http://sonarr-4k:8989","name":"Starrs & Stripes"}}`,
	), &fromObject); err != nil {
		t.Fatal(err)
	}

	if got := fromObject["uhd"]; got == nil || got.Name != "Starrs & Stripes" {
		t.Fatalf("named: %+v", fromObject)
	}

	if err := json.Unmarshal([]byte(`{"starrs & stripes":{"url":"http://x"}}`), &fromObject); err == nil {
		t.Fatal("expected invalid slug")
	}
}

func TestInstanceMapTOMLArrayAndTable(t *testing.T) {
	t.Parallel()

	type wrap struct {
		Sonarr InstanceMap[SonarrConfig] `toml:"sonarr"`
	}

	var array wrap
	if _, err := toml.Decode("[[sonarr]]\nurl = \"http://sonarr:8989\"\n", &array); err != nil {
		t.Fatal(err)
	}

	if got := array.Sonarr["0"]; got == nil || got.URL != "http://sonarr:8989" {
		t.Fatalf("array: %+v", array.Sonarr)
	}

	var named wrap
	if _, err := toml.Decode(
		"[sonarr.uhd]\nname = \"Starrs & Stripes\"\nurl = \"http://sonarr-4k:8989\"\n", &named,
	); err != nil {
		t.Fatal(err)
	}

	if got := named.Sonarr["uhd"]; got == nil || got.Name != "Starrs & Stripes" {
		t.Fatalf("named: %+v", named.Sonarr)
	}
}

func TestInstanceMapRejectsBadSlug(t *testing.T) {
	t.Parallel()

	for _, slug := range []string{"starrs & stripes", "has space", `quo"te`, "brace{"} {
		raw, err := json.Marshal(map[string]map[string]string{slug: {"url": "http://x"}})
		if err != nil {
			t.Fatal(err)
		}

		var dest InstanceMap[SonarrConfig]
		if err := json.Unmarshal(raw, &dest); err == nil {
			t.Fatalf("accepted %q", slug)
		}
	}
}

func TestPeelInstanceEnvKeepsKeyCase(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[SonarrConfig]()

	key, field, indexes, matched := peelInstanceEnv("SONARR_uhd_API_KEY", "SONARR_", typ)
	if !matched || key != "uhd" || field != "API_KEY" || indexes != nil {
		t.Fatalf("got key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("SONARR_0_URL", "SONARR_", typ)
	if !matched || key != "0" || field != "URL" || indexes != nil {
		t.Fatalf("index: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("SONARR_starrs_stripes_URL", "SONARR_", typ)
	if !matched || key != "starrs_stripes" || field != "URL" || indexes != nil {
		t.Fatalf("underscore: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("SONARR_0_PATHS", "SONARR_", typ)
	if !matched || key != "0" || field != "PATHS" || indexes != nil {
		t.Fatalf("paths: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}
}

func TestPeelInstanceEnvLongestFieldFirst(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[FolderConfig]()

	key, field, indexes, matched := peelInstanceEnv("FOLDER_watch_EXTRACT_PATH", "FOLDER_", typ)
	if !matched || key != "watch" || field != "EXTRACT_PATH" || indexes != nil {
		t.Fatalf("extract_path: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("FOLDER_watch_PATH", "FOLDER_", typ)
	if !matched || key != "watch" || field != "PATH" || indexes != nil {
		t.Fatalf("path: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("FOLDER_watch_DELETE_AFTER", "FOLDER_", typ)
	if !matched || key != "watch" || field != "DELETE_AFTER" || indexes != nil {
		t.Fatalf("delete_after: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}

	key, field, indexes, matched = peelInstanceEnv("FOLDER_watch_EXCLUDE_PATH_0", "FOLDER_", typ)
	if !matched || key != "watch" || field != "EXCLUDE_PATH" || len(indexes) != 1 || indexes[0] != 0 {
		t.Fatalf("exclude_path: key=%q field=%q indexes=%v ok=%v", key, field, indexes, matched)
	}
}

func TestStripEnvFromFoldersPeelsExtractPath(t *testing.T) {
	t.Parallel()

	after := cnfg.Duration{Duration: 10 * time.Minute}
	unpack := New()
	unpack.envUsed = map[string]string{
		"FOLDER_watch_EXTRACT_PATH": "/env/extract",
		"FOLDER_watch_DELETE_AFTER": "5m",
	}

	got := unpack.stripEnvFromFolders(InstanceMap[FolderConfig]{
		"watch": {Path: "/watch", ExtractPath: "/file/extract", DeleteAfter: &after},
	})

	item := got["watch"]
	if item == nil {
		t.Fatal("stripped the folder")
	}

	if item.ExtractPath != "" {
		t.Fatalf("extract_path stayed %q", item.ExtractPath)
	}

	if item.DeleteAfter != nil {
		t.Fatalf("delete_after stayed %+v", item.DeleteAfter)
	}

	if item.Path != "/watch" {
		t.Fatalf("path %q", item.Path)
	}
}

func TestStripEnvFromFoldersKeepsSiblingExcludePaths(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.envUsed = map[string]string{
		"FOLDER_watch_EXCLUDE_PATH_0": "/c",
	}

	got := unpack.stripEnvFromFolders(InstanceMap[FolderConfig]{
		"watch": {Path: "/watch", ExcludePaths: []string{"/a", "/b"}},
	})

	item := got["watch"]
	if item == nil {
		t.Fatal("stripped the folder")
	}

	if len(item.ExcludePaths) != 2 || item.ExcludePaths[0] != "" || item.ExcludePaths[1] != "/b" {
		t.Fatalf("exclude_paths %+v", item.ExcludePaths)
	}

	src := &FolderConfig{ExcludePaths: []string{"/c"}}
	dst := &FolderConfig{Path: "/watch", ExcludePaths: []string{"/a", "/b"}}
	unpack.copyEnvOwnedFields(SectionFolders, "watch", src, dst)

	if len(dst.ExcludePaths) != 2 || dst.ExcludePaths[0] != "/c" || dst.ExcludePaths[1] != "/b" {
		t.Fatalf("live exclude_paths %+v", dst.ExcludePaths)
	}
}

func TestStripEnvFromStarrKeepsFileAPIKeyWhenURLIsEnv(t *testing.T) {
	t.Parallel()

	secret := strings.Repeat("k", apiKeyMinLength)
	unpack := New()
	unpack.envUsed = map[string]string{"SONARR_0_URL": "http://127.0.0.1:8989"}

	app := &SonarrConfig{}
	app.URL = "http://file.invalid:8989"
	app.APIKey = secret
	app.Name = "uhd"
	app.Paths = StringSlice{"/downloads/tv"}

	got := unpack.stripEnvFromStarr[SonarrConfig, *SonarrConfig](SectionSonarr, InstanceMap[SonarrConfig]{
		"0": app,
	})

	item := got["0"]
	if item == nil {
		t.Fatal("stripped mixed file+env sonarr")
	}

	if item.URL != "" || item.APIKey != secret || item.Name != "uhd" || len(item.Paths) != 1 {
		t.Fatalf("file snapshot %+v", item)
	}
}

func TestStripEnvFromStarrDropsNameOnlyWhenURLIsEnv(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.envUsed = map[string]string{"SONARR_0_URL": "http://127.0.0.1:8989"}

	app := &SonarrConfig{}
	app.URL = "http://file.invalid:8989"
	app.Name = "books"

	got := unpack.stripEnvFromStarr[SonarrConfig, *SonarrConfig](SectionSonarr, InstanceMap[SonarrConfig]{
		"0": app,
	})
	if len(got) != 0 {
		t.Fatalf("name-only stub stayed %+v", got)
	}
}

func TestStripEnvFromHooksKeepsTokenWhenURLIsEnv(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.envUsed = map[string]string{"WEBHOOK_0_URL": "http://env.example/hook"}

	got := unpack.stripEnvFromHooks(SectionWebhooks, InstanceMap[WebhookConfig]{
		"0": {URL: "http://file.example/hook", Token: "secret-token", Name: "discord"},
	})

	item := got["0"]
	if item == nil {
		t.Fatal("stripped mixed file+env webhook")
	}

	if item.URL != "" || item.Token != "secret-token" || item.Name != "discord" {
		t.Fatalf("file snapshot %+v", item)
	}
}

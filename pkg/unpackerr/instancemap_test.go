package unpackerr

import (
	"encoding/json"
	"reflect"
	"strconv"
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

	fields := tomlEnvFieldNames(reflect.TypeFor[SonarrConfig]())

	key, field, matched := peelInstanceEnv("SONARR_uhd_API_KEY", "SONARR_", fields)
	if !matched || key != "uhd" || field != "API_KEY" {
		t.Fatalf("got key=%q field=%q ok=%v fields=%v", key, field, matched, fields)
	}

	key, field, matched = peelInstanceEnv("SONARR_0_URL", "SONARR_", fields)
	if !matched || key != "0" || field != "URL" {
		t.Fatalf("index: key=%q field=%q ok=%v", key, field, matched)
	}

	key, field, matched = peelInstanceEnv("SONARR_starrs_stripes_URL", "SONARR_", fields)
	if !matched || key != "starrs_stripes" || field != "URL" {
		t.Fatalf("underscore: key=%q field=%q ok=%v", key, field, matched)
	}

	key, field, matched = peelInstanceEnv("SONARR_0_PATHS_0", "SONARR_", fields)
	if !matched || key != "0" || field != "PATHS" {
		t.Fatalf("indexed paths: key=%q field=%q ok=%v", key, field, matched)
	}
}

func TestPeelInstanceEnvLongestFieldFirst(t *testing.T) {
	t.Parallel()

	fields := tomlEnvFieldNames(reflect.TypeFor[FolderConfig]())

	key, field, matched := peelInstanceEnv("FOLDER_watch_EXTRACT_PATH", "FOLDER_", fields)
	if !matched || key != "watch" || field != "EXTRACT_PATH" {
		t.Fatalf("extract_path: key=%q field=%q ok=%v fields=%v", key, field, matched, fields)
	}

	key, field, matched = peelInstanceEnv("FOLDER_watch_PATH", "FOLDER_", fields)
	if !matched || key != "watch" || field != "PATH" {
		t.Fatalf("path: key=%q field=%q ok=%v", key, field, matched)
	}

	key, field, matched = peelInstanceEnv("FOLDER_watch_DELETE_AFTER", "FOLDER_", fields)
	if !matched || key != "watch" || field != "DELETE_AFTER" {
		t.Fatalf("delete_after: key=%q field=%q ok=%v", key, field, matched)
	}

	key, field, matched = peelInstanceEnv("FOLDER_watch_EXCLUDE_PATHS_0", "FOLDER_", fields)
	if !matched || key != "watch" || field != "EXCLUDE_PATHS" {
		t.Fatalf("exclude_paths: key=%q field=%q ok=%v", key, field, matched)
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

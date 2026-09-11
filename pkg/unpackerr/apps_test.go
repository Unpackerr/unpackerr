package unpackerr

import (
	"testing"

	"golift.io/starr"
)

func TestAdoptWhisparrFoldsIntoRadarr(t *testing.T) {
	t.Parallel()

	u := New()
	u.Radarr = []*RadarrConfig{{
		StarrConfig: StarrConfig{Name: "Movies", Config: starr.Config{URL: "http://radarr:7878"}},
	}}
	u.Whisparr = []*RadarrConfig{
		{StarrConfig: StarrConfig{Config: starr.Config{URL: "http://whisparr:6969"}}},
		{StarrConfig: StarrConfig{Name: "Adult", Config: starr.Config{URL: "http://whisparr2:6969"}}},
	}
	u.snapshotFileConfig()
	u.adoptWhisparr()

	if len(u.Whisparr) != 0 {
		t.Fatalf("live whisparr leftover: %d", len(u.Whisparr))
	}

	if u.fileConfig == nil || len(u.fileConfig.Whisparr) != 0 {
		t.Fatalf("file whisparr leftover: %+v", u.fileConfig)
	}

	if len(u.Radarr) != 3 {
		t.Fatalf("live radarr count: %d", len(u.Radarr))
	}

	if u.Radarr[0].Name != "Movies" {
		t.Fatalf("existing radarr name: %q", u.Radarr[0].Name)
	}

	if u.Radarr[1].Name != string(starr.Whisparr) {
		t.Fatalf("unnamed whisparr name: %q", u.Radarr[1].Name)
	}

	if u.Radarr[2].Name != "Adult" {
		t.Fatalf("named whisparr name: %q", u.Radarr[2].Name)
	}

	if len(u.fileConfig.Radarr) != 3 {
		t.Fatalf("file radarr count: %d", len(u.fileConfig.Radarr))
	}

	if u.fileConfig.Radarr[1].Name != string(starr.Whisparr) || u.fileConfig.Radarr[2].Name != "Adult" {
		t.Fatalf("file names: %q %q", u.fileConfig.Radarr[1].Name, u.fileConfig.Radarr[2].Name)
	}
}

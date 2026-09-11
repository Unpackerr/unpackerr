package unpackerr

import (
	"testing"

	"golift.io/starr"
)

func TestAdoptWhisparrFoldsIntoRadarr(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Radarr = []*RadarrConfig{{
		Name: "Movies",
		URL:  "http://radarr:7878",
	}}
	unpack.Whisparr = []*RadarrConfig{
		{URL: "http://whisparr:6969"},
		{Name: "Adult", URL: "http://whisparr2:6969"},
	}
	unpack.snapshotFileConfig()
	unpack.adoptWhisparr()

	if len(unpack.Whisparr) != 0 {
		t.Fatalf("live whisparr leftover: %d", len(unpack.Whisparr))
	}

	if unpack.fileConfig == nil || len(unpack.fileConfig.Whisparr) != 0 {
		t.Fatalf("file whisparr leftover: %+v", unpack.fileConfig)
	}

	if len(unpack.Radarr) != 3 {
		t.Fatalf("live radarr count: %d", len(unpack.Radarr))
	}

	if unpack.Radarr[0].Name != "Movies" {
		t.Fatalf("existing radarr name: %q", unpack.Radarr[0].Name)
	}

	if unpack.Radarr[1].Name != string(starr.Whisparr) {
		t.Fatalf("unnamed whisparr name: %q", unpack.Radarr[1].Name)
	}

	if unpack.Radarr[2].Name != "Adult" {
		t.Fatalf("named whisparr name: %q", unpack.Radarr[2].Name)
	}

	if len(unpack.fileConfig.Radarr) != 3 {
		t.Fatalf("file radarr count: %d", len(unpack.fileConfig.Radarr))
	}

	if unpack.fileConfig.Radarr[1].Name != string(starr.Whisparr) || unpack.fileConfig.Radarr[2].Name != "Adult" {
		t.Fatalf("file names: %q %q", unpack.fileConfig.Radarr[1].Name, unpack.fileConfig.Radarr[2].Name)
	}
}

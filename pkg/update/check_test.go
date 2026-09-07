package update

import (
	"testing"
	"time"
)

func TestFillUpdateUnknownVersion(t *testing.T) {
	t.Parallel()

	rel := &GitHubReleasesLatest{
		TagName:     "v0.16.1",
		HTMLURL:     "https://example.com",
		PublishedAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	}

	for _, version := range []string{"", " ", "-", "v", "dev"} {
		got := FillUpdate(rel, version)
		if got.Outdate {
			t.Errorf("version %q: Outdate = true, want false", version)
		}

		if got.Version != unknownVersion {
			t.Errorf("version %q: Version = %q, want %q", version, got.Version, unknownVersion)
		}
	}
}

func TestFillUpdateComparesSemver(t *testing.T) {
	t.Parallel()

	rel := &GitHubReleasesLatest{TagName: "v0.16.1"}

	if got := FillUpdate(rel, "0.16.0"); !got.Outdate || got.Version != "v0.16.0" {
		t.Fatalf("older: %+v", got)
	}

	if got := FillUpdate(rel, "v0.16.1"); got.Outdate || got.Version != "v0.16.1" {
		t.Fatalf("same: %+v", got)
	}

	if got := FillUpdate(rel, "0.17.0"); got.Outdate || got.Version != "v0.17.0" {
		t.Fatalf("newer: %+v", got)
	}
}

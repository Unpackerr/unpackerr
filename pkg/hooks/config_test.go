package hooks

import (
	"testing"

	"golift.io/starr"
)

func TestExcludedDialectOrName(t *testing.T) {
	t.Parallel()

	hook := &Config{Exclude: []string{"sonarr", "Sportarr"}}

	tests := []struct {
		name     string
		app      starr.App
		inst     string
		excluded bool
	}{
		{name: "dialect", app: starr.Sonarr, inst: "", excluded: true},
		{name: "named dialect still matches", app: starr.Sonarr, inst: "TV", excluded: true},
		{name: "instance name", app: starr.Radarr, inst: "Sportarr", excluded: true},
		{name: "name case", app: starr.Radarr, inst: "sportarr", excluded: true},
		{name: "other app", app: starr.Lidarr, inst: "Music", excluded: false},
		{name: "empty exclude name", app: starr.Radarr, inst: "", excluded: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := hook.Excluded(test.app, test.inst); got != test.excluded {
				t.Fatalf("Excluded(%s, %q) = %v, want %v", test.app, test.inst, got, test.excluded)
			}
		})
	}
}

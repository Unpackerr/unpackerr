package extract

import (
	"testing"

	"golift.io/starr"
)

func TestExtractLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item *Extract
		want string
	}{
		{name: "nil", item: nil, want: ""},
		{name: "dialect", item: &Extract{App: starr.Sonarr}, want: string(starr.Sonarr)},
		{name: "named", item: &Extract{App: starr.Sonarr, Name: "Sportarr"}, want: "Sportarr"},
		{name: "spaces", item: &Extract{App: starr.Radarr, Name: "  "}, want: string(starr.Radarr)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.item.Label(); got != test.want {
				t.Fatalf("Label() = %q, want %q", got, test.want)
			}
		})
	}
}

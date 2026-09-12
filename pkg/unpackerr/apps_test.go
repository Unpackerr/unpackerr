package unpackerr

import (
	"strings"
	"testing"

	"golift.io/cnfg"
)

func TestWhisparrEnvIsIgnored(t *testing.T) {
	t.Setenv("UN_WHISPARR_0_URL", "http://whisparr:6969")
	t.Setenv("UN_WHISPARR_0_API_KEY", strings.Repeat("W", apiKeyMinLength))

	unpack := New()
	if _, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix); err != nil {
		t.Fatal(err)
	}

	if len(unpack.Radarr) != 0 {
		t.Fatalf("UN_WHISPARR_* must not become radarr: %+v", unpack.Radarr)
	}
}

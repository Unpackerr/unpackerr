package unpackerr

import (
	"fmt"
	"strings"
	"testing"
)

func TestLiveConfigTextSharesRunningDump(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = "/tmp/unpackerr.conf"

	var body strings.Builder

	unpack.writeRunningConfig(func(format string, v ...any) {
		fmt.Fprintf(&body, format+"\n", v...)
	})

	got := body.String()
	live := unpack.liveConfigText()

	if got == "" {
		t.Fatal("running dump is empty")
	}

	if strings.Contains(got, "Live Settings") || strings.Contains(got, "Startup Settings") {
		t.Fatal("running dump must not include live/startup headers")
	}

	if !strings.Contains(live, got) {
		t.Fatalf("live dump missing running body\nlive:\n%s\nbody:\n%s", live, got)
	}

	if !strings.Contains(live, "Live Settings") || !strings.Contains(live, unpack.ConfigFile) {
		t.Fatalf("live dump missing header/config file: %q", live)
	}

	if !strings.Contains(got, "Whisparr Config: 0 servers") {
		t.Fatalf("empty whisparr should print 0 servers: %q", got)
	}
}

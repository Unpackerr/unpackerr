package unpackerr

import (
	"fmt"
	"strings"
	"testing"
)

func dumpRunningConfig(unpack *Unpackerr, auth dumpAuth) string {
	var body strings.Builder

	unpack.writeRunningConfig(func(format string, v ...any) {
		fmt.Fprintf(&body, format+"\n", v...)
	}, auth)

	return body.String()
}

func TestLiveConfigTextSharesRunningDump(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = "/tmp/unpackerr.conf"

	got := dumpRunningConfig(unpack, dumpAuth{})
	live := unpack.liveConfigText(authInfo{Permissions: []string{PermAll}})

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

func TestLiveConfigOmitsWithoutSectionRead(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webhook = []*WebhookConfig{{
		Name: "https://example.com/hook?token=hook-secret",
	}}
	unpack.Cmdhook = []*WebhookConfig{{
		Name:    "cmd",
		Command: "/usr/bin/env token=cmd-secret",
	}}

	got := dumpRunningConfig(unpack, dumpAuth{
		gated: true,
		info:  authInfo{Permissions: []string{PermReadSystemInfo}},
	})

	for _, need := range []string{
		"omitted (need read:config:sonarr)",
		"omitted (need read:config:radarr)",
		"omitted (need read:config:lidarr)",
		"omitted (need read:config:readarr)",
		"omitted (need read:config:whisparr)",
		"omitted (need read:config:folders)",
		"omitted (need read:config:general)",
		"omitted (need read:config:webhooks)",
		"omitted (need read:config:cmdhooks)",
		"omitted (need read:config:webserver)",
	} {
		if !strings.Contains(got, need) {
			t.Errorf("missing %q in:\n%s", need, got)
		}
	}

	if strings.Contains(got, "hook-secret") || strings.Contains(got, "cmd-secret") {
		t.Fatalf("secret leaked in gated dump:\n%s", got)
	}

	if strings.Contains(got, " => Parallel:") {
		t.Fatalf("general details leaked:\n%s", got)
	}
}

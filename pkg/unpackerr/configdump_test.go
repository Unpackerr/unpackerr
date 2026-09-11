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

	if strings.Contains(got, "Default Extract Limits") {
		t.Fatalf("shared extract-limit defaults must not print:\n%s", got)
	}
}

func TestRunningDumpPrintsPerAppMaxBytes(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Sonarr = []*SonarrConfig{{
		URL: "http://sonarr.test",
	}}
	unpack.Radarr = []*RadarrConfig{{
		URL:      "http://radarr.test",
		MaxBytes: "10GB",
	}}
	unpack.Lidarr = []*LidarrConfig{{
		URL: "http://lidarr.test",
	}}
	unpack.Readarr = []*ReadarrConfig{{
		URL:      "http://readarr.test",
		MaxBytes: "0",
	}}
	unpack.Whisparr = []*RadarrConfig{{
		URL: "http://whisparr.test",
	}}
	unpack.Folders = []*FolderConfig{
		{Path: "/watch"},
		{Path: "/capped", MaxBytes: "2GB"},
	}

	got := dumpRunningConfig(unpack, dumpAuth{})
	if strings.Contains(got, "Default Extract Limits") {
		t.Fatalf("shared extract-limit defaults must not print:\n%s", got)
	}

	want := map[string]string{
		"http://sonarr.test":   "max_bytes:" + defaultSonarrMaxBytes,
		"http://radarr.test":   "max_bytes:10GB",
		"http://lidarr.test":   "max_bytes:" + defaultLidarrMaxBytes,
		"http://readarr.test":  "max_bytes:0",
		"http://whisparr.test": "max_bytes:" + defaultWhisparrMaxBytes,
		"Path: /watch":         "max_bytes:uncapped",
		"Path: /capped":        "max_bytes:2GB",
	}

	for needle, maxBytes := range want {
		line := dumpLineContaining(got, needle)
		if line == "" {
			t.Errorf("missing %q in:\n%s", needle, got)
			continue
		}

		if !strings.Contains(line, maxBytes) {
			t.Errorf("%q: want %q in %q", needle, maxBytes, line)
		}
	}
}

func dumpLineContaining(dump, needle string) string {
	for line := range strings.SplitSeq(dump, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}

	return ""
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

	for _, section := range []ConfigSection{
		SectionSonarr, SectionRadarr, SectionLidarr, SectionReadarr, SectionWhisparr,
		SectionFolders, SectionGeneral, SectionWebhooks, SectionCmdhooks, SectionWebserver,
	} {
		need := "omitted (need " + PermReadConfig(section) + ")"
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

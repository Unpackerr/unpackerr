package unpackerr

import (
	"testing"
)

func TestValidateHooksConfig(t *testing.T) {
	t.Parallel()

	if err := validateHooksConfig(&HooksConfig{
		CustomIDs: map[string]string{"url": "https://x"},
		Titles:    HookTitles{Extracting: "Archive Found"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := validateHooksConfig(&HooksConfig{CustomIDs: map[string]string{"bad key": "x"}}); err == nil {
		t.Fatal("expected invalid id key")
	}
}

func TestHookTitlesForStatus(t *testing.T) {
	t.Parallel()

	titles := HookTitles{Extracting: "  Archive Found  "}
	if got := titles.forStatus(EXTRACTING); got != "Archive Found" {
		t.Fatalf("custom %q", got)
	}

	if got := titles.forStatus(EXTRACTED); got != EXTRACTED.Desc() {
		t.Fatalf("builtin %q", got)
	}

	if titles.nonEmpty() != 1 {
		t.Fatalf("count %d", titles.nonEmpty())
	}
}

func TestPayloadCustomIDs(t *testing.T) {
	t.Parallel()

	if got := payloadCustomIDs(nil); got != nil {
		t.Fatalf("nil %v", got)
	}

	got := payloadCustomIDs(map[string]string{"url": "https://x", "": "skip", "empty": ""})
	if len(got) != 1 || got["url"] != "https://x" {
		t.Fatalf("%+v", got)
	}
}

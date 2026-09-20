package unpackerr

import (
	"testing"
)

func TestValidateHooksConfig(t *testing.T) {
	t.Parallel()

	if err := validateHooksConfig(&HooksConfig{
		CustomIDs: map[string]string{"url": "https://x"},
		Titles:    map[string]string{"extracting": "Archive Found"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := validateHooksConfig(&HooksConfig{CustomIDs: map[string]string{"bad key": "x"}}); err == nil {
		t.Fatal("expected invalid id key")
	}

	if err := validateHooksConfig(&HooksConfig{Titles: map[string]string{"nope": "x"}}); err == nil {
		t.Fatal("expected unknown title")
	}

	if err := validateHooksConfig(&HooksConfig{Titles: map[string]string{"2": "x"}}); err == nil {
		t.Fatal("expected numeric title key")
	}

	if err := validateHooksConfig(&HooksConfig{Titles: map[string]string{" extracting ": "x"}}); err == nil {
		t.Fatal("expected padded title key")
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

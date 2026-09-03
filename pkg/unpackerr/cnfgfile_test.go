package unpackerr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigFileRoundTrip(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Config.Debug = true
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(body)
	if !strings.Contains(text, "[webserver]") {
		t.Fatalf("missing [webserver]: %s", text)
	}

	if !strings.Contains(text, "debug = true") {
		t.Fatalf("missing live debug: %s", text)
	}

	if _, err := os.Stat(unpack.ConfigFile + ".bak"); err == nil {
		t.Fatal("first write should not create a backup")
	}

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(unpack.ConfigFile + ".bak"); err != nil {
		t.Fatalf("rewrite should keep a backup: %v", err)
	}
}

func TestWriteConfigFileRequiresPath(t *testing.T) {
	t.Parallel()

	if err := New().writeConfigFile(); err == nil {
		t.Fatal("expected an error without ConfigFile")
	}
}

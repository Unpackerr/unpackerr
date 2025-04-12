package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsEmpty(t *testing.T) {
	t.Parallel()

	emptyDir := t.TempDir()
	if !dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return true on an empty folder")
	}

	f, err := os.Create(filepath.Join(emptyDir, "emptyFile"))
	if err != nil {
		t.Fatalf("Got an error making temp file: %v", err)
	}
	defer f.Close()

	if dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return false when the folder has a file in it")
	}
}

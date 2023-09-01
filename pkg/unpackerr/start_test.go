package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsEmpty(t *testing.T) {
	t.Parallel()

	emptyDir, err := os.MkdirTemp("", "gobuildtestdir")
	if err != nil {
		t.Fatalf("Got an error making temp folder: %v", err)
	}

	if !dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return true on an emty folder")
	}

	f, err := os.Create(filepath.Join(emptyDir, "emptyFile"))
	if err != nil {
		t.Fatalf("Got an error making temp file: %v", err)
	}
	f.Close()

	if dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return false when the folder has a file in it")
	}

}

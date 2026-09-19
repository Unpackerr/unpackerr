package folders

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeWaitExtensions(t *testing.T) {
	t.Parallel()

	got := NormalizeWaitExtensions([]string{" part ", ".CRDOWNLOAD", "part", ".", "", ".part"})
	want := []string{".part", ".CRDOWNLOAD"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestWaitFileInTopDoesNotWalk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")

	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(nested, "movie.zip.part"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := WaitFileInTop(dir, []string{".part"}); got != "" {
		t.Fatalf("nested wait file %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "movie.zip.part"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := WaitFileInTop(dir, []string{"part"}); got != "movie.zip.part" {
		t.Fatalf("top wait file %q", got)
	}
}

func TestCloneListCopiesWaitExtensions(t *testing.T) {
	t.Parallel()

	src := []*FolderConfig{{
		Path:           "/watch",
		WaitExtensions: []string{".part"},
	}}
	cloned := CloneList(src)
	cloned[0].WaitExtensions[0] = ".crdownload"

	if src[0].WaitExtensions[0] != ".part" {
		t.Fatalf("shared wait_extensions slice: %v", src[0].WaitExtensions)
	}
}

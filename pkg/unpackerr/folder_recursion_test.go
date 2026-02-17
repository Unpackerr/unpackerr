package unpackerr

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/xtractr"
)

func TestFolderDisableRecursionHonored(t *testing.T) {
	t.Parallel()

	archivePath := makeNestedZipFixture(t)
	done := runExtraction(t, archivePath, true)

	if done.Error != nil {
		t.Fatalf("expected no extraction error, got: %v", done.Error)
	}

	if containsFileBase(done.Output, "nested-only.txt") {
		t.Fatalf("did not expect nested archive contents when disable_recursion=true: %s; files: %v",
			done.Output, listFiles(done.Output))
	}

	if !containsFileBase(done.Output, "inner.zip") {
		t.Fatalf("expected nested archive file to remain when recursion is disabled: %s; files: %v",
			done.Output, listFiles(done.Output))
	}
}

func TestFolderDisableRecursionFalseExtractsNested(t *testing.T) {
	t.Parallel()

	archivePath := makeNestedZipFixture(t)
	done := runExtraction(t, archivePath, false)

	if done.Error != nil {
		t.Fatalf("expected no extraction error, got: %v", done.Error)
	}

	if !containsFileBase(done.Output, "nested-only.txt") {
		t.Fatalf("expected nested archive contents when disable_recursion=false: %s; files: %v",
			done.Output, listFiles(done.Output))
	}
}

func TestFolderExcludeSuffixesDirectoryDoesNotExcludeAllArchives(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exclude := folderExcludeSuffixes(dir, &FolderConfig{DisableRecursion: true, ExtractISOs: false})

	if !containsString(exclude, ".iso") {
		t.Fatalf("expected .iso exclusion when extract_isos=false; got: %v", exclude)
	}

	if containsString(exclude, ".zip") {
		t.Fatalf("did not expect all archive suffixes for watched folders; got: %v", exclude)
	}
}

func TestFolderExcludeSuffixesArchiveExcludesAllWhenDisableRecursion(t *testing.T) {
	t.Parallel()

	archivePath := makeNestedZipFixture(t)
	exclude := folderExcludeSuffixes(archivePath, &FolderConfig{DisableRecursion: true, ExtractISOs: false})

	if !containsString(exclude, ".zip") {
		t.Fatalf("expected archive suffix exclusions for watched archive file; got: %v", exclude)
	}
}

func runExtraction(t *testing.T, archivePath string, disableRecursion bool) *xtractr.Response {
	t.Helper()

	cfg := &FolderConfig{DisableRecursion: disableRecursion, ExtractISOs: false}
	exclude := folderExcludeSuffixes(archivePath, cfg)

	queue := xtractr.NewQueue(&xtractr.Config{
		Parallel: 1,
		Suffix:   suffix,
		FileMode: defaultFileMode,
		DirMode:  defaultDirMode,
	})

	t.Cleanup(func() { queue.Stop() })

	callbacks := make(chan *xtractr.Response, updateChanBuf)

	_, err := queue.Extract(&xtractr.Xtract{
		Name:             archivePath,
		Filter:           xtractr.Filter{Path: archivePath, ExcludeSuffix: exclude},
		TempFolder:       true,
		DeleteOrig:       false,
		CBChannel:        callbacks,
		DisableRecursion: disableRecursion,
		LogFile:          false,
	})
	if err != nil {
		t.Fatalf("queue.Extract returned error: %v", err)
	}

	timeout := time.NewTimer(90 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case resp := <-callbacks:
			if resp.Done {
				return resp
			}
		case <-timeout.C:
			t.Fatal("timed out waiting for folder extraction callback")
		}
	}
}

func makeNestedZipFixture(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	innerZipBytes, err := buildZip(map[string][]byte{
		"nested-only.txt": []byte("inner payload"),
	})
	if err != nil {
		t.Fatalf("building inner zip fixture: %v", err)
	}

	outerZipBytes, err := buildZip(map[string][]byte{
		"folder1/inner.zip": innerZipBytes,
		"folder2/root.txt":  []byte("outer payload"),
	})
	if err != nil {
		t.Fatalf("building outer zip fixture: %v", err)
	}

	if err := os.WriteFile(archivePath, outerZipBytes, 0o600); err != nil {
		t.Fatalf("writing outer zip fixture: %v", err)
	}

	return archivePath
}

func buildZip(entries map[string][]byte) ([]byte, error) {
	var output bytes.Buffer

	writer := zip.NewWriter(&output)
	for name, data := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("creating zip entry: %w", err)
		}

		if _, err := entry.Write(data); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("creating zip entry: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing zip writer: %w", err)
	}

	return output.Bytes(), nil
}

func containsFileBase(root, base string) bool {
	var found bool

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil //nolint:nilerr
		}

		if filepath.Base(path) == base {
			found = true
		}

		return nil
	})

	return found
}

func listFiles(root string) []string {
	files := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil //nolint:nilerr
		}

		if rel, relErr := filepath.Rel(root, path); relErr == nil {
			files = append(files, rel)
		} else {
			files = append(files, path)
		}

		return nil
	})

	return files
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(item, want) {
			return true
		}
	}

	return false
}

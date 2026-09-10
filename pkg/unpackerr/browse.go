package unpackerr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
)

const (
	maxBrowseBody       = 4096
	windowsDriveFirst   = 'C'
	windowsDriveLast    = 'Z'
	windowsDriveSuffix  = `:\`
	windowsPathSep      = `\`
	errBrowsePathMsg    = "unable to read provided path"
	errBrowseContentMsg = "unable to read content of provided path"
)

var (
	errBrowsePath    = errors.New(errBrowsePathMsg)
	errBrowseContent = errors.New(errBrowseContentMsg)
	errMissingPath   = errors.New("path is required")
)

// BrowseDir is the JSON body for GET/POST /api/browse.
type BrowseDir struct {
	Sep   string   `json:"sep"`
	Path  string   `json:"path"`
	Mom   string   `json:"mom"`
	Dirs  []string `json:"dirs"`
	Files []string `json:"files"`
	Error string   `json:"error"`
}

type browseCreateRequest struct {
	Path string `json:"path"`
}

func (u *Unpackerr) browseHandler(response http.ResponseWriter, request *http.Request) {
	output, err := browseDir(request.URL.Query().Get("dir"))
	if err != nil {
		writeJSON(response, http.StatusNotAcceptable, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, output)
}

func (u *Unpackerr) browseCreateHandler(response http.ResponseWriter, request *http.Request) {
	body, ok := readBrowseCreate(response, request)
	if !ok {
		return
	}

	u.Printf("[user requested] Creating folder: %s", body.Path)

	output, err := mkdirBrowsePath(body.Path)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, output)
}

func readBrowseCreate(response http.ResponseWriter, request *http.Request) (browseCreateRequest, bool) {
	var body browseCreateRequest

	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxBrowseBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return browseCreateRequest{}, false
	}

	switch err := decoder.Decode(&struct{}{}); {
	case errors.Is(err, io.EOF):
		if strings.TrimSpace(body.Path) == "" {
			writeJSON(response, http.StatusBadRequest, map[string]string{"error": errMissingPath.Error()})
			return browseCreateRequest{}, false
		}

		return body, true
	case err != nil:
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return browseCreateRequest{}, false
	default:
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errExtraJSON.Error()})
		return browseCreateRequest{}, false
	}
}

func mkdirBrowsePath(path string) (*BrowseDir, error) {
	path = expandBrowsePath(path)

	if err := os.MkdirAll(path, defaultDirMode); err != nil {
		return nil, fmt.Errorf("unable to create folder: %w", err)
	}

	return browseDir(path)
}

func browseDir(dir string) (*BrowseDir, error) {
	output, err := browsedDirMeta(dir)
	if err != nil {
		return nil, err
	}

	if isWindowsVolumeRoot(output.Path) {
		output.Dirs = windowsDriveRoots()
		output.Files = []string{}
		output.Mom = ""
		output.Path = ""

		return output, nil
	}

	if err := fillBrowseEntries(output); err != nil {
		return browseParentFallback(output, err)
	}

	return output, nil
}

func fillBrowseEntries(output *BrowseDir) error {
	entries, err := os.ReadDir(output.Path)
	if err != nil {
		return fmt.Errorf("%w: %w", errBrowseContent, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			output.Dirs = append(output.Dirs, entry.Name())
			continue
		}

		output.Files = append(output.Files, entry.Name())
	}

	slices.Sort(output.Dirs)
	slices.Sort(output.Files)

	return nil
}

// browseParentFallback lists one parent when the path exists but cannot be
// read (chmod bits, etc.). A Stat fallback already moved Path up; if that
// parent is also unlistable, both failed → 406.
func browseParentFallback(output *BrowseDir, listErr error) (*BrowseDir, error) {
	parent := filepath.Dir(output.Path)
	if parent == output.Path || output.Error != "" {
		return nil, listErr
	}

	listed, err := browsedDirMeta(parent)
	if err != nil {
		return nil, listErr
	}

	if err := fillBrowseEntries(listed); err != nil {
		return nil, listErr
	}

	listed.Error = listErr.Error()

	return listed, nil
}

func browsedDirMeta(dir string) (*BrowseDir, error) {
	dir = expandBrowsePath(dir)

	output := &BrowseDir{
		Path:  dir,
		Dirs:  []string{},
		Files: []string{},
		Sep:   string(filepath.Separator),
		Mom:   filepath.Dir(dir),
	}

	if dir == "" {
		output.Mom = ""
		return output, nil
	}

	info, err := os.Stat(dir) //nolint:gosec // G703: file browser list
	if err != nil {
		output.Error = errBrowsePathMsg + ": " + err.Error()
		output.Path = filepath.Dir(dir)

		if info, err = os.Stat(output.Path); err != nil { //nolint:gosec // G703: file browser list
			return nil, fmt.Errorf("%w: %w", errBrowsePath, err)
		}
	}

	if !info.IsDir() {
		output.Path = filepath.Dir(dir)
	}

	output.Mom = filepath.Dir(output.Path)
	if output.Mom == output.Path {
		output.Mom = ""
	}

	return output, nil
}

func expandBrowsePath(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		if isWindowsVolumeRoot("") {
			return ""
		}

		dir = "~"
	}

	expanded, err := homedir.Expand(dir)
	if err != nil || strings.TrimSpace(expanded) == "" {
		return dir
	}

	return filepath.Clean(expanded)
}

func isWindowsVolumeRoot(path string) bool {
	if runtime.GOOS != windows {
		return false
	}

	return path == "" || path == "/" || path == windowsPathSep
}

func windowsDriveRoots() []string {
	roots := make([]string, 0, windowsDriveLast-windowsDriveFirst+1)

	for letter := windowsDriveFirst; letter <= windowsDriveLast; letter++ {
		root := string(letter) + windowsDriveSuffix
		if _, err := os.Stat(root); err != nil {
			continue
		}

		roots = append(roots, root)
	}

	return roots
}

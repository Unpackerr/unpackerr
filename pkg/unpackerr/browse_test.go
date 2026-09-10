package unpackerr

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBrowseDirListsSortedEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zeta"), defaultDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(dir, "alpha"), defaultDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	got, err := browseDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got.Path != dir {
		t.Fatalf("path %q", got.Path)
	}

	if got.Error != "" {
		t.Fatalf("error %q", got.Error)
	}

	if !slices.Equal(got.Dirs, []string{"alpha", "zeta"}) {
		t.Fatalf("dirs %v", got.Dirs)
	}

	if !slices.Equal(got.Files, []string{"a.txt", "b.txt"}) {
		t.Fatalf("files %v", got.Files)
	}

	if got.Mom != filepath.Dir(dir) {
		t.Fatalf("mom %q", got.Mom)
	}

	if got.Sep != string(filepath.Separator) {
		t.Fatalf("sep %q", got.Sep)
	}
}

func TestBrowseDirFallsBackToParent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")

	got, err := browseDir(missing)
	if err != nil {
		t.Fatal(err)
	}

	if got.Path != dir {
		t.Fatalf("path %q", got.Path)
	}

	if got.Error == "" {
		t.Fatal("expected error for missing path")
	}
}

func TestBrowseDirFileUsesParent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	file := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(file, []byte("x"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	got, err := browseDir(file)
	if err != nil {
		t.Fatal(err)
	}

	if got.Path != dir {
		t.Fatalf("path %q", got.Path)
	}

	if !slices.Contains(got.Files, "log.txt") {
		t.Fatalf("files %v", got.Files)
	}
}

func TestBrowseDirUnixRootHasEmptyMom(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("unix volume root")
	}

	got, err := browseDir("/")
	if err != nil {
		t.Fatal(err)
	}

	if got.Path != "/" {
		t.Fatalf("path %q", got.Path)
	}

	if got.Mom != "" {
		t.Fatalf("mom %q", got.Mom)
	}
}

func TestBrowseDirUnlistableFallsBackToParent(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("directory list bits are not POSIX")
	}

	if os.Geteuid() == 0 {
		t.Skip("root can list mode 0 directories")
	}

	dir := t.TempDir()
	blocked := filepath.Join(dir, "secret")

	if err := os.Mkdir(blocked, defaultDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(blocked, defaultDirMode)
	})

	got, err := browseDir(blocked)
	if err != nil {
		t.Fatal(err)
	}

	if got.Path != dir {
		t.Fatalf("path %q", got.Path)
	}

	if got.Error == "" {
		t.Fatal("expected error for unlistable path")
	}
}

func TestExpandBrowsePathEmptyIsHome(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("empty is the volume-root list on windows")
	}

	got := expandBrowsePath("")
	if got == "" || got == "~" {
		t.Fatalf("empty should expand to home, got %q", got)
	}
}

func TestBrowseRequiresAuth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	if rec := doAuth(t, unpack, http.MethodGet, "/api/browse?dir=/", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d %s", rec.Code, rec.Body.String())
	}
}

func TestBrowseForbiddenWithoutPermission(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	statKey := strings.Repeat("S", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "home",
		Key:   statKey,
		Roles: []string{"stats"},
	})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, statKey)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/browse?dir=/", "", withKey); rec.Code != http.StatusForbidden {
		t.Fatalf("get %d %s", rec.Code, rec.Body.String())
	}

	body := `{"path":"/tmp/x"}`
	if rec := doAuth(t, unpack, http.MethodPost, "/api/browse", body, withKey); rec.Code != http.StatusForbidden {
		t.Fatalf("post %d %s", rec.Code, rec.Body.String())
	}
}

func TestBrowseListsTempDir(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), defaultDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	target := "/api/browse?dir=" + url.QueryEscape(dir)

	rec := doAuth(t, unpack, http.MethodGet, target, "", withAdminKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("browse %d %s", rec.Code, rec.Body.String())
	}

	got := decodeBrowse(t, rec.Body.Bytes())
	if got.Path != dir {
		t.Fatalf("path %q", got.Path)
	}

	if !slices.Equal(got.Dirs, []string{"sub"}) || !slices.Equal(got.Files, []string{"f.txt"}) {
		t.Fatalf("dirs %v files %v", got.Dirs, got.Files)
	}
}

func TestBrowseCreateDir(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	dir := t.TempDir()
	folder := filepath.Join(dir, "nested", "newdir")

	folderRec := doAuth(t, unpack, http.MethodPost, "/api/browse",
		`{"path":`+jsonString(t, folder)+`}`, withAdminKey)
	if folderRec.Code != http.StatusOK {
		t.Fatalf("mkdir %d %s", folderRec.Code, folderRec.Body.String())
	}

	if _, err := os.Stat(folder); err != nil {
		t.Fatal(err)
	}

	got := decodeBrowse(t, folderRec.Body.Bytes())
	if got.Path != folder {
		t.Fatalf("path %q", got.Path)
	}

	again := doAuth(t, unpack, http.MethodPost, "/api/browse",
		`{"path":`+jsonString(t, folder)+`}`, withAdminKey)
	if again.Code != http.StatusOK {
		t.Fatalf("exists %d %s", again.Code, again.Body.String())
	}
}

func TestBrowseCreateRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	dir := t.TempDir()
	body := `{"path":` + jsonString(t, filepath.Join(dir, "x")) + `,"dir":true}`

	if rec := doAuth(t, unpack, http.MethodPost, "/api/browse", body, withAdminKey); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown %d %s", rec.Code, rec.Body.String())
	}
}

func TestBrowseCreateForbiddenWithoutWrite(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	readKey := strings.Repeat("R", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"browser": {Permissions: []string{PermReadSystemBrowse}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "reader",
		Key:   readKey,
		Roles: []string{"browser"},
	})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	}

	dir := t.TempDir()
	target := "/api/browse?dir=" + url.QueryEscape(dir)

	if rec := doAuth(t, unpack, http.MethodGet, target, "", withKey); rec.Code != http.StatusOK {
		t.Fatalf("get %d %s", rec.Code, rec.Body.String())
	}

	if rec := doAuth(t, unpack, http.MethodPost, "/api/browse",
		`{"path":`+jsonString(t, filepath.Join(dir, "x"))+`}`, withKey); rec.Code != http.StatusForbidden {
		t.Fatalf("post %d %s", rec.Code, rec.Body.String())
	}
}

func TestBrowseCreateRejectsEmptyPath(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	body := `{"path":"  "}`

	if rec := doAuth(t, unpack, http.MethodPost, "/api/browse", body, withAdminKey); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d %s", rec.Code, rec.Body.String())
	}
}

func withAdminKey(req *http.Request) {
	req.Header.Set(headerAPIKey, strings.Repeat("A", apiKeyMinLen))
}

func decodeBrowse(t *testing.T, body []byte) BrowseDir {
	t.Helper()

	var got BrowseDir
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}

	return got
}

func jsonString(t *testing.T, value string) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

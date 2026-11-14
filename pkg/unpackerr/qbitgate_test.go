package unpackerr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

func TestCheckTorrentFiles(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	download := filepath.Join(parent, "Show.S01")
	writeSized(t, filepath.Join(download, "E01", "e01.rar"), 10)
	writeSized(t, filepath.Join(download, "E01", "e01.r00"), 7) // short: still arriving.
	writeSized(t, filepath.Join(download, "show.nfo"), 3)

	files := []qbitFile{
		{Name: "Show.S01/E01/e01.rar", Size: 10, Priority: 1},
		{Name: "Show.S01/E01/e01.r00", Size: 10, Priority: 1},
		{Name: "Show.S01/E02/e02.rar", Size: 10, Priority: 1},
		{Name: "Show.S01/show.nfo", Size: 3, Priority: 1},
		{Name: "Show.S01/sample.mkv", Size: 99, Priority: 0}, // deselected: never arrives.
	}

	check, mapped := checkTorrentFiles(download, files)
	if !mapped || check.files != 4 || check.missing != 2 || check.firstMissing != "Show.S01/E01/e01.r00" {
		t.Fatalf("partial: %+v mapped=%v", check, mapped)
	}

	if check.have != 13 || check.want != 33 {
		t.Fatalf("bytes: have %d want %d", check.have, check.want)
	}

	writeSized(t, filepath.Join(download, "E01", "e01.r00"), 10)
	writeSized(t, filepath.Join(download, "E02", "e02.rar"), 10)
	writeSized(t, filepath.Join(download, "E02", ".syncthing.e02.r00.tmp"), 10) // a file not in the list yet.

	check, mapped = checkTorrentFiles(download, files)
	if !mapped || check.missing != 0 || !strings.HasSuffix(check.temp, ".syncthing.e02.r00.tmp") {
		t.Fatalf("temp file in a subfolder: %+v mapped=%v", check, mapped)
	}

	if err := os.Remove(filepath.Join(download, "E02", ".syncthing.e02.r00.tmp")); err != nil {
		t.Fatal(err)
	}

	if check, mapped = checkTorrentFiles(download, files); !mapped || check.missing != 0 || check.temp != "" {
		t.Fatalf("complete: %+v mapped=%v", check, mapped)
	}

	// A list whose root folder is not the download folder cannot be mapped.
	if _, ok := checkTorrentFiles(download, []qbitFile{{Name: "Other/e01.rar", Size: 10, Priority: 1}}); ok {
		t.Fatal("mismatched root folder must not be trusted")
	}

	if _, ok := checkTorrentFiles(download, []qbitFile{{Name: "../escape", Size: 1, Priority: 1}}); ok {
		t.Fatal("a name outside the save path must not be trusted")
	}

	if _, ok := checkTorrentFiles(download, []qbitFile{{Name: "Show.S01/x", Size: 1, Priority: 0}}); ok {
		t.Fatal("a list with nothing selected has nothing to check")
	}
}

// A single-file torrent's file name has no root folder; the download path is the file.
func TestCheckTorrentFilesSingleFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "Movie.2026.mkv")
	list := []qbitFile{{Name: "Movie.2026.mkv", Size: 20, Priority: 1}}

	writeSized(t, file, 12)

	if check, ok := checkTorrentFiles(file, list); !ok || check.missing != 1 {
		t.Fatalf("short single file: %+v ok=%v", check, ok)
	}

	writeSized(t, file, 20)

	if check, ok := checkTorrentFiles(file, list); !ok || check.missing != 0 {
		t.Fatalf("complete single file: %+v ok=%v", check, ok)
	}
}

func TestTorrentHash(t *testing.T) {
	t.Parallel()

	if got := torrentHash(strings.ToUpper(testHash)); got != testHash {
		t.Fatalf("upper-case hash: %q", got)
	}

	for _, bad := range []any{nil, "", "SABnzbd_nzo_abc123", strings.Repeat("z", 40), 42} {
		if got := torrentHash(bad); got != "" {
			t.Fatalf("%v: got %q, want empty", bad, got)
		}
	}
}

// qbitTestItem is a completed Sonarr item whose download folder holds the first
// volume of a three-volume set, with a qBittorrent instance configured for it.
func qbitTestItem(t *testing.T, qbitURL string) (*Unpackerr, *Extract) {
	t.Helper()

	const sonarrURL = "http://sonarr:8989"

	parent := t.TempDir()
	download := filepath.Join(parent, "Show.S01")
	writeSized(t, filepath.Join(download, "show.rar"), 100)

	unpack := New()
	unpack.StartDelay.Duration = 0
	unpack.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: 1, Suffix: suffix, Logger: unpack.Logger, FileMode: defaultFileMode, DirMode: defaultDirMode,
	})
	t.Cleanup(unpack.Stop)

	unpack.Sonarr = instanceMap([]*SonarrConfig{{
		URL:      sonarrURL,
		Timeout:  cnfg.Duration{Duration: 5 * time.Second},
		QbitURL:  qbitURL,
		QbitUser: "admin",
		QbitPass: "s3cret",
	}})

	item := &Extract{
		App:       starr.Sonarr,
		URL:       sonarrURL,
		Path:      download,
		Status:    WAITING,
		Updated:   time.Now().Add(-time.Hour),
		Syncthing: true,
		Size:      300,
		IDs:       map[string]any{"downloadId": strings.ToUpper(testHash)},
		XProg:     &ExtractProgress{},
	}
	item.XProg.Extract = item
	unpack.Map["Show.S01"] = item

	return unpack, item
}

func threeVolumes() []qbitFile {
	return []qbitFile{
		{Name: "Show.S01/show.rar", Size: 100, Priority: 1},
		{Name: "Show.S01/show.r00", Size: 100, Priority: 1},
		{Name: "Show.S01/show.r01", Size: 100, Priority: 1},
	}
}

// Missing files keep the item waiting with no timeout, then it queues once they land.
func TestQbitCheckWaitsWithoutTimeout(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/qbittorrent/")
	fake.add(threeVolumes()...)

	unpack, item := qbitTestItem(t, fake.URL+"/qbittorrent/")
	now := time.Now()

	for _, at := range []time.Time{now, now.Add(syncthingStall), now.Add(24 * time.Hour)} {
		unpack.extractCompletedDownload("Show.S01", at, item)

		if item.Status != WAITING || item.Note != noteWaitingFiles {
			t.Fatalf("at +%v: status %s note %q, want waiting for files", at.Sub(now), item.Status, item.Note)
		}
	}

	// Reported size reached, but a volume is still syncing: wait for the temp file to go.
	writeSized(t, filepath.Join(item.Path, "show.r00"), 100)
	writeSized(t, filepath.Join(item.Path, "show.r01"), 100)
	writeSized(t, filepath.Join(item.Path, "Sub", ".syncthing.extra.tmp"), 1)

	unpack.extractCompletedDownload("Show.S01", now.Add(25*time.Hour), item)

	if item.Status != WAITING {
		t.Fatalf("temp file present: status %s, want waiting", item.Status)
	}

	if err := os.RemoveAll(filepath.Join(item.Path, "Sub")); err != nil {
		t.Fatal(err)
	}

	unpack.extractCompletedDownload("Show.S01", now.Add(26*time.Hour), item)

	if item.Status != QUEUED {
		t.Fatalf("all files present: status %s, want queued", item.Status)
	}
}

// qBittorrent unreachable: the size check applies, including its stall fallback.
func TestQbitCheckUnreachableFallsBackToSize(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	url := fake.URL + "/"
	fake.Close() // gone.

	unpack, item := qbitTestItem(t, url)
	now := time.Now()

	unpack.extractCompletedDownload("Show.S01", now, item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("status %s note %q, want the size check waiting", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.S01", now.Add(syncthingStall), item)

	if item.Status != QUEUED {
		t.Fatalf("status %s, want queued by the stall fallback", item.Status)
	}
}

// qBittorrent does not know the torrent (removed after seeding): same fallback.
func TestQbitCheckUnknownTorrentFallsBackToSize(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	unpack, item := qbitTestItem(t, fake.URL+"/")

	unpack.extractCompletedDownload("Show.S01", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("status %s note %q, want the size check waiting", item.Status, item.Note)
	}
}

// A good qBittorrent answer resets the size check, so a later outage gets a full stall window.
func TestQbitCheckResetsStallWindow(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	fake.add(threeVolumes()...)

	unpack, item := qbitTestItem(t, fake.URL+"/")
	start := time.Now()
	item.SyncSeen, item.SyncSeenAt = 100, start.Add(-2*time.Hour)

	unpack.extractCompletedDownload("Show.S01", start, item)

	if !item.SyncSeenAt.IsZero() {
		t.Fatal("a qBittorrent answer must reset the size check's stall clock")
	}

	unpack.Sonarr["0"].QbitURL = "http://127.0.0.1:1/" // now unreachable.
	unpack.extractCompletedDownload("Show.S01", start.Add(time.Minute), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("status %s note %q, want the size check waiting, not stalled", item.Status, item.Note)
	}
}

// Without qbit_url the qBittorrent check is not used at all.
func TestQbitCheckUnconfigured(t *testing.T) {
	t.Parallel()

	unpack, item := qbitTestItem(t, "")

	if got := unpack.qbitCheck("Show.S01", item, time.Now()); got != qbitUnused {
		t.Fatalf("state %d, want unused", got)
	}
}

func TestRedactLiveSecretsQbitPass(t *testing.T) {
	t.Parallel()

	cfg := &Config{Sonarr: instanceMap([]*SonarrConfig{{QbitURL: "http://q/", QbitUser: "u", QbitPass: "s3cret"}})}
	redactLiveSecrets(cfg)

	if conf := cfg.Sonarr["0"]; conf.QbitPass != "" || conf.QbitUser != "u" {
		t.Fatalf("pass %q user %q: the password must be blank, the user kept", conf.QbitPass, conf.QbitUser)
	}
}

// The qBittorrent settings survive a config write (the file is rendered from
// definitions.yml), and qbit_pass accepts filepath: like the other secrets.
func TestQbitSettingsPersistAndExpand(t *testing.T) {
	t.Parallel()

	secret := filepath.Join(t.TempDir(), "qbit.pass")
	if err := os.WriteFile(secret, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	unpack := New()
	unpack.ConfigFile = filepath.Join(dir, "unpackerr.conf")
	unpack.Sonarr = instanceMap([]*SonarrConfig{{
		URL: "http://sonarr:8989", APIKey: "0123456789abcdef0123456789abcdef",
		QbitURL: "https://seedbox/qbittorrent/", QbitUser: "admin", QbitPass: filePrefix + secret,
	}})
	unpack.snapshotFileConfig()

	if err := unpack.writeConfigFile(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	wants := []string{
		`qbit_url = "https://seedbox/qbittorrent/"`,
		`qbit_user = "admin"`,
		`qbit_pass = "` + filePrefix + secret + `"`,
	}

	for _, want := range wants {
		if !strings.Contains(string(body), want) {
			t.Fatalf("rendered config lacks %s", want)
		}
	}

	live := cloneStarrMap[SonarrConfig, *SonarrConfig](unpack.Sonarr)
	if err := expandFilepaths(&live); err != nil {
		t.Fatal(err)
	}

	if got := live["0"].QbitPass; got != "s3cret" {
		t.Fatal("qbit_pass filepath: was not expanded")
	}
}

// Progress for a waiting item is logged when it starts waiting and then every
// qbitLogEvery, not on every check.
func TestQbitCheckLogRate(t *testing.T) {
	t.Parallel()

	fake := newFakeQbit(t, "/")
	fake.add(threeVolumes()...)

	unpack, item := qbitTestItem(t, fake.URL+"/")
	start := time.Now()

	for minute := range 25 {
		unpack.extractCompletedDownload("Show.S01", start.Add(time.Duration(minute)*time.Minute), item)
	}

	if want := start.Add(20 * time.Minute); !item.QbitLogAt.Equal(want) {
		t.Fatalf("last log at +%v, want +20m (logged at 0, 10 and 20 minutes)", item.QbitLogAt.Sub(start))
	}
}

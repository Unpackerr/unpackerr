package unpackerr

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

const (
	noteNoExtractable    = "no extractable files"
	noteWaitingSyncthing = "waiting for syncthing"
)

// StarrConfig is the shared config items for all starr apps.
type StarrConfig struct {
	starr.Config
	Name           string        `json:"name"            toml:"name"            xml:"name"            yaml:"name"`
	Paths          []string      `json:"paths"           toml:"paths"           xml:"paths"           yaml:"paths"`
	Protocols      string        `json:"protocols"       toml:"protocols"       xml:"protocols"       yaml:"protocols"`
	DeleteOrig     bool          `json:"delete_orig"     toml:"delete_orig"     xml:"delete_orig"     yaml:"delete_orig"`
	DeleteDelay    cnfg.Duration `json:"delete_delay"    toml:"delete_delay"    xml:"delete_delay"    yaml:"delete_delay"`
	Syncthing      bool          `json:"syncthing"       toml:"syncthing"       xml:"syncthing"       yaml:"syncthing"`
	SyncthingStall cnfg.Duration `json:"syncthing_stall" toml:"syncthing_stall" xml:"syncthing_stall" yaml:"syncthing_stall"` //nolint:lll
	ValidSSL       bool          `json:"valid_ssl"       toml:"valid_ssl"       xml:"valid_ssl"       yaml:"valid_ssl"`
	Timeout        cnfg.Duration `json:"timeout"         toml:"timeout"         xml:"timeout"         yaml:"timeout"`
	// MaxBytes overrides the app default (Sonarr 20GB, Radarr 75GB, Lidarr 4GB, Readarr 1GB).
	// Empty uses that default. `0` or `0B` is unlimited.
	MaxBytes string `json:"maxBytes" toml:"max_bytes" xml:"max_bytes" yaml:"maxBytes"`
	maxBytes uint64
	// Last poll snapshot for GET /api/stats starrQueues. Published under History.mu.
	lastQueued    int
	lastRetrieved int
	lastPolled    time.Time
	lastPollErr   string
	polled        bool // true after a successful GetQueue in this process (Queue != nil).
}

func (c *StarrConfig) takePoll(old *StarrConfig) {
	if c == nil || old == nil {
		return
	}

	c.lastQueued = old.lastQueued
	c.lastRetrieved = old.lastRetrieved
	c.lastPolled = old.lastPolled
	c.lastPollErr = old.lastPollErr
	c.polled = old.polled
}

func (c *StarrConfig) hasPolled() bool { return c != nil && c.polled }

// Label is the human-facing instance name, or app when Name is empty.
func (c *StarrConfig) Label(app starr.App) string {
	if c != nil {
		if name := strings.TrimSpace(c.Name); name != "" {
			return name
		}
	}

	return string(app)
}

// checkQueueChanges checks each item for state changes from the app queues.
func (u *Unpackerr) checkQueueChanges(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for name, data := range u.Map {
		switch {
		case data.App == FolderString:
			continue // folders are handled in folder.go.
		case !u.haveQitem(name, data.App) && !u.queueSnapshotReady(data):
			// Queue == nil is "never fetched", not "empty"; wait for a successful poll.
			u.Debugf("%s: Queue not polled yet; not treating as imported: %s", data.Label(), name)
		case !u.haveQitem(name, data.App):
			// This fires when an items becomes missing (imported/deleted) from the application queue.
			switch elapsed := now.Sub(data.Updated); {
			case data.Status == WAITING && !hasUnpacked(data):
				// A waiting item just fell out of the queue. We never extracted it. Remove it and move on.
				// One that waits to retry a late archive has earlier output, and takes the import path below.
				u.deleteExtract(name)
				u.Printf("[%v] Imported: %v (not extracted, removing from history)", data.Label(), name)
				u.notifyQueueLocked()
			case data.Status > IMPORTED:
				u.Debugf("Already imported? %s", name)
			case data.Status == IMPORTED:
				u.Debugf("%v: Awaiting Delete Delay (%v remains): %v",
					data.Label(), data.DeleteDelay-elapsed.Round(time.Second), name)
			default:
				u.updateQueueStatus(&newStatus{Name: name, Status: IMPORTED, Resp: data.Resp}, now, true)
				u.Printf("[%v] Imported: %v (delete in %v)", data.Label(), name, data.DeleteDelay)
			}
		case data.Status == IMPORTED:
			// The item fell out of the app queue and came back. Reset it.
			u.Printf("%s: Extraction Not Imported: %s - De-queued and returned.", data.Label(), name)
			data.Status = EXTRACTED

			u.notifyQueueLocked()
		case data.Status > IMPORTED:
			// The item fell out of the app queue and came back. Reset it.
			u.Printf("%s: Extraction Restarting: %s - Deleted Item De-queued and returned.", data.Label(), name)
			data.Status = WAITING
			data.Updated = now

			u.notifyQueueLocked()
		}

		u.Printf("[%s] Status: %s (%v, elapsed: %v) %s", data.Label(), name, data.Status.Desc(),
			now.Sub(data.Updated).Round(time.Second), data.XProg)
	}
}

// extractCompletedDownloads process each download and checks if it needs to be extracted.
// This is called from the main go routine in start.go and it only processes starr apps, not folders.
func (u *Unpackerr) extractCompletedDownloads(now time.Time) {
	type pending struct {
		name string
		item *Extract
	}

	u.lockHistory()

	jobs := make([]pending, 0)

	for name, item := range u.Map {
		if item.App == FolderString || !u.queueSnapshotReady(item) {
			continue
		}

		// EXTRACTED stays in this loop while Syncthing can still deliver
		// archives that were not on disk for the first extract.
		if item.Status < QUEUED || u.rescans(name, item) {
			jobs = append(jobs, pending{name: name, item: item})
		}
	}

	u.unlockHistory()

	for _, job := range jobs {
		u.extractCompletedDownload(job.name, now, job.item)
	}
}

// extractCompletedDownload checks if a completed starr download needs to be queued for extraction.
// This is called by extractCompletedDownloads() via the main routine in start.go.
func (u *Unpackerr) extractCompletedDownload(name string, now time.Time, item *Extract) {
	// A finished Syncthing download stays EXTRACTED until Starr imports it.
	// Archives that arrive after the first extract are queued from here.
	rescan := item.Status == EXTRACTED && hasUnpacked(item)
	if item.Status >= QUEUED && !rescan {
		return
	}

	if !rescan && u.waitingForStartDelay(name, item, now) {
		return
	}

	files, scan := findDownloadArchives(item)
	if len(files) == 0 {
		u.noteNothingToExtract(name, item, now)
		return
	}

	if item.Syncthing && u.syncthingWait(name, item, scan, files, now) {
		return
	}

	u.queueDownloadExtract(name, item, files, now)
}

func (u *Unpackerr) waitingForStartDelay(name string, item *Extract, now time.Time) bool {
	if d := u.StartDelay.Duration - now.Sub(item.Updated); d > time.Second { // wiggle room.
		u.Printf("[%s] Waiting for Start Delay: %v (%v remains)", item.Label(), name, d.Round(time.Second))
		return true
	}

	return false
}

// findDownloadArchives lists the archives to extract for this download. The
// suffix filter is the one the extract uses, so a type a Starr download is not
// extracted as is not listed here either. An item with earlier extracts leaves
// out their archives and their output, in every status.
func findDownloadArchives(item *Extract) (xtractr.ArchiveList, *syncScan) {
	scan := &syncScan{unpacked: newUnpackedSet(item)}
	filter := xtractr.Filter{
		Path:          item.Path,
		ExcludeSuffix: xtractr.AllExcept(starrArchiveTypes(item)...),
	}

	if item.Syncthing {
		filter.Observe = scan.observe // fills the scan during the archive search; no second walk.
	}

	if scan.unpacked != nil {
		filter.Accept = scan.unpacked.accept
	}

	return xtractr.FindCompressedFiles(filter), scan
}

func (u *Unpackerr) noteNothingToExtract(name string, item *Extract, now time.Time) {
	if hasUnpacked(item) {
		u.nothingNewToExtract(name, item, now)
		return
	}

	if _, err := os.Stat(item.Path); err != nil {
		u.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s (stat err: %v)",
			item.Label(), name, item.Path, err)
	} else {
		u.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s (%s Activity Queue status: %v)",
			item.Label(), name, item.Path, item.Label(), item.IDs["reason"])
	}

	u.setItemNote(name, item, noteNoExtractable)
}

// nothingNewToExtract handles a search that found no archive beyond those
// already unpacked. The earlier extract still stands and its output is ready
// to import, so an item that went back to WAITING returns to EXTRACTED.
func (u *Unpackerr) nothingNewToExtract(name string, item *Extract, now time.Time) {
	if item.Status == EXTRACTED {
		u.clearSyncthingNote(name, item)
		return
	}

	u.lockHistory()
	defer u.unlockHistory()

	if u.Map[name] != item {
		return
	}

	// The failed extract is over, and so is its snapshot. A file Syncthing
	// delivered since then is not in it, and the next late archive must not
	// take that file for a leftover.
	item.PreFiles = nil

	u.Printf("[%s] Nothing new to extract, the earlier extract stands: %s", item.Label(), name)
	u.updateQueueStatus(&newStatus{Name: name, Status: EXTRACTED, Resp: withoutError(item.Resp)}, now, false)
}

func (u *Unpackerr) queueDownloadExtract(name string, item *Extract, files xtractr.ArchiveList, now time.Time) {
	// One snapshot covers an extract and its retries: a retry must not
	// recapture leftovers that failed to clear into download content. A
	// finished Syncthing extract drops it, so the next extract of that item
	// starts from the folder as it is then, earlier output included.
	snap, err := keepDirSnapshot(item.PreFiles, archiveSnapshotPaths(item.Path, files)...)
	if err != nil && hasUnpacked(item) {
		// Leftovers of this extract could not be told from the files already
		// here. The item keeps its place, and the next pass tries again.
		u.Errorf("[%s] Snapshot dests for remnant check, not queueing yet: %s: %v", item.Label(), name, err)
		return
	}

	u.markItemQueued(name, item, snap, err, now)

	xtract := &xtractr.Xtract{
		Password:       u.getPasswordFromPath(item.Path),
		Passwords:      u.Passwords,
		Name:           name,
		Path:           item.Path,
		ExcludeSuffix:  xtractr.AllExcept(starrArchiveTypes(item)...),
		APEOpts:        apeOpts(item),
		MaxBytes:       item.MaxBytes,
		MaxFiles:       defaultMaxFiles,
		MaxRatio:       defaultMaxRatio,
		MaxNested:      defaultMaxNested,
		ExtrasMaxDepth: defaultExtrasMaxDepth,
		TempFolder:     false,
		DeleteOrig:     false,
		CBChannel:      u.updates,
		Progress:       u.progressUpdateCallback(item),
	}

	if item.Syncthing {
		xtract.Accept = pinArchives(files)
	}

	// This queues the extraction. Which may start right away.
	queueSize, _ := u.Extract(xtract)

	u.logQueuedDownload(queueSize, item, files)
}

func (u *Unpackerr) clearSyncthingNote(name string, item *Extract) {
	if item.Note != noteWaitingSyncthing {
		return
	}

	u.setItemNote(name, item, "")
}

func (u *Unpackerr) markItemQueued(
	name string,
	item *Extract,
	snap map[string]os.FileInfo,
	snapErr error,
	now time.Time,
) {
	u.lockHistory()
	defer u.unlockHistory()

	if snapErr != nil {
		u.Errorf("[%s] Snapshot dests for remnant check: %v", item.Label(), snapErr)
	} else {
		item.PreFiles = snap
	}

	if item.Status == EXTRACTED {
		// A late archive is a new extract with a retry budget of its own.
		// The retries an earlier extract used do not count against it.
		item.Retries = 0
		item.NoRetry = false
	}

	item.Status = QUEUED
	item.Updated = now
	item.Note = ""
	u.maybeRecordHistory(name, item)
	u.notifyQueueLocked()
}

// starrArchiveTypes is what a Starr download may be extracted as.
// xtractr recognizes more formats; watched folders use that wider set.
// .cue is a FLAC or APE cue sheet.
func starrArchiveTypes(item *Extract) []string {
	types := []string{
		".rar", ".r00", ".zip", ".7z", ".7z.001", ".gz", ".tgz",
		".tar", ".tar.gz", ".bz2", ".tbz2",
	}

	if item != nil && item.SplitFlac {
		types = append(types, ".cue")
	}

	return types
}

func apeOpts(item *Extract) xtractr.APEOpts {
	if item == nil {
		return xtractr.APEOpts{}
	}

	return xtractr.APEOpts{
		Compression: item.APECompression,
		Output:      item.APEFormat,
	}
}

func lidarrImportsTracks(item *Extract) bool {
	return item != nil && item.SplitFlac
}

func (u *Unpackerr) logQueuedDownload(queueSize int, item *Extract, files xtractr.ArchiveList) {
	count := fmt.Sprint("1 archive: ", files.Random()[0])
	if fileCount := files.Count(); fileCount > 1 {
		count = fmt.Sprintf("%v archives in %d folders", fileCount, len(files))
	}

	u.Printf("[%s] Extraction Queued: %s, retries: %d, %s, delete orig: %v, queue size: %d",
		item.Label(), item.Path, item.Retries, count, item.DeleteOrig, queueSize)
}

func (u *Unpackerr) setItemNote(name string, item *Extract, note string) {
	u.lockHistory()
	defer u.unlockHistory()

	if item.Note == note {
		return
	}

	item.Note = note
	u.stampQueueDue(name, item)

	if u.hub != nil {
		u.hub.notifyProgress(u.queueFromExtract(name, item))
	}
}

func (u *Unpackerr) getPasswordFromPath(path string) string {
	start, end := strings.Index(path, "{{"), strings.Index(path, "}}")

	if start == -1 || end == -1 || start > end {
		return ""
	}

	u.Debugf("Found password in Path: %s", path[start+2:end])

	return path[start+2 : end]
}

// checkExtractDone checks if an extracted and imported item needs to be deleted.
// Or if an extraction failed and needs to be restarted.
// This runs at a short interval to check for extraction state changes, and should return quickly.
//
//nolint:cyclop,wsl
func (u *Unpackerr) checkExtractDone(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for name, item := range u.Map {
		switch elapsed := now.Sub(item.Updated); {
		case item.Status == DELETED && elapsed >= item.DeleteDelay:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			u.deleteExtract(name)
			u.Printf("[%s] Finished, Removed History: %v", item.Label(), name)
			u.notifyQueueLocked()
		case item.App == FolderString:
			continue // folders are handled in folder.go.
		case item.Status == EXTRACTFAILED && item.NoRetry:
			// Stay EXTRACTFAILED. DELETED is > IMPORTED, so checkQueueChanges
			// would bounce a still-completed Starr item back to WAITING.
		case item.Status == EXTRACTFAILED && elapsed >= u.RetryDelay.Duration &&
			item.Retries < u.maxRetries():
			u.Retries++
			item.Retries++
			item.Status = WAITING
			item.Updated = now
			u.Printf("[%s] Extract failed %v ago, triggering restart (%d/%d): %v",
				item.Label(), elapsed.Round(time.Second), item.Retries, u.maxRetries(), name)
			u.notifyQueueLocked()
		case item.Status == EXTRACTFAILED && item.Retries >= u.maxRetries():
			// Stay EXTRACTFAILED. DELETED is > IMPORTED, so checkQueueChanges
			// would bounce a still-completed Starr item back to WAITING.
			item.NoRetry = true
			u.updateQueueStatus(&newStatus{Name: name, Status: EXTRACTFAILED, Resp: item.Resp}, now, true)
			u.Printf("[%s] Retries exhausted (%d/%d), giving up: %v",
				item.Label(), item.Retries, u.maxRetries(), name)
		case (item.Status == EXTRACTED || item.Status == EXTRACTING || item.Status == QUEUED) &&
			elapsed >= staleItemTimeout && !u.rescans(name, item):
			// Safety net: items stuck at intermediate states for too long are cleaned up
			// to prevent unbounded map growth (e.g. Starr app never imports the item).
			// A finished Syncthing download the Starr app still lists is searched on
			// every pass for late archives. It keeps its place for as long as it is listed.
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: item.Resp}, now, true)
			u.Printf("[%s] Stale item removed after %v at status %s: %v",
				item.Label(), elapsed.Round(time.Second), item.Status.Desc(), name)
		case item.Status == IMPORTED && elapsed >= item.DeleteDelay &&
			(!u.queueSnapshotReady(item) || u.haveQitem(name, item.App)):
			// Still in Starr, or we have not polled this process; checkQueueChanges will
			// reset IMPORTED→EXTRACTED when the title is back in the queue.
			u.Debugf("%s: not deleting imported item still queued or unpolled: %s", item.Label(), name)
		case item.Status == IMPORTED && elapsed >= item.DeleteDelay:
			var webhook bool

			if item.DeleteOrig {
				u.queueDelete(&fileDeleteReq{Paths: []string{item.Path}})
				webhook = true //nolint:wsl_v5
			} else if item.Resp != nil && len(item.Resp.NewFiles) > 0 && item.DeleteDelay >= 0 {
				// Delete extracted files and purge empty parents up to and including the download path.
				// A Syncthing item names the files inside its extracted folders, so a file
				// Syncthing delivered into one of them is left where it is.
				u.queueDelete(&fileDeleteReq{
					Paths:            item.Resp.NewFiles,
					Exact:            hasUnpacked(item),
					Files:            slices.Clone(item.OutputFiles),
					PurgeEmptyParent: true,
					PurgeEmptyRoot:   item.Path,
				})
				webhook = true //nolint:wsl_v5
			}

			if webhook {
				// The extracted files are going away. A download the Starr app
				// lists again after this is extracted from the start.
				item.Unpacked = nil
				item.OutputFiles = nil
			}

			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: item.Resp}, now, webhook)
		}
	}
}

// handleXtractrCallback handles callbacks from the xtractr library for starr apps (not folders).
// This takes the provided info and logs it then sends it the queue update method.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) { //nolint:funlen
	now := resp.Started.Add(resp.Elapsed)

	u.lockHistory()

	item := u.Map[resp.X.Name]
	if item == nil {
		u.unlockHistory()
		return
	}

	if !resp.Done {
		resetExtractProgress(item, resp.Archives.Count()+resp.Extras.Count())

		u.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		// An item with earlier extracts keeps their files and archives on
		// record while this one runs: a restart or an import in the middle
		// of it still has to clean them up. Any other item reports this
		// start alone, with nothing left over from a failed attempt.
		u.updateQueueStatus(
			&newStatus{Name: resp.X.Name, Status: EXTRACTING, Resp: keepUnpacked(item, resp, false)}, now, true)
		u.unlockHistory()

		return
	}

	app := item.App
	url := item.URL
	preFiles := item.PreFiles
	retries := item.Retries
	syncthing := item.Syncthing

	// What the extract wrote, listed while it was still in the temp folder.
	var staged map[string]struct{}
	if item.XProg != nil {
		staged = item.XProg.TempFiles
	}

	u.unlockHistory()
	u.updateMetrics(resp, app, url)

	remnantStatus, remnants := u.handleRemnants(resp, preFiles, retries)
	finished := !remnants && resp.Error == nil

	var files, wrote []string
	if finished {
		files = fileList(resp.X.Path)
	}

	if syncthing {
		wrote = outputFiles(resp, finished, staged)
	}

	u.lockHistory()
	defer u.unlockHistory()

	item = u.Map[resp.X.Name]
	if item == nil {
		return
	}

	// This run's counts are logged below. For an item with earlier extracts
	// the stored response keeps what they recorded, so delete still removes
	// their output. Only an extract that finished adds its archives to the
	// ones a later search skips.
	stored := keepUnpacked(item, resp, finished)
	recordOutputFiles(item, wrote, finished)

	switch {
	case remnants && remnantStatus == WAITING:
		// Every blocker was cleared; re-extract into the empty destination.
		// This case wins over resp.Error, so log a password/corrupt-archive
		// error here the way the folder callback logs before remnant cleanup.
		if resp.Error != nil {
			u.Errorf("[%s] Extraction Failed: %s: %v", item.Label(), resp.X.Name, resp.Error)
		}

		u.Retries++
		item.Retries++
		item.Status = WAITING
		item.Updated = now
		item.Resp = stored
		u.Printf("[%s] Cleared interrupted-extraction remnant(s), restarting extraction: %s", item.Label(), resp.X.Name)
		u.notifyQueueLocked()
	case remnants:
		if remnantAction(u.RemnantAction) == "off" {
			item.NoRetry = true
		}

		u.Errorf("[%s] Extraction blocked by interrupted-extraction remnant(s): %s", item.Label(), resp.X.Name)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: stored}, now, true)
	case resp.Error != nil:
		if xtractr.IsLimitError(resp.Error) {
			item.NoRetry = true
		}

		u.Errorf("Extraction Failed: %s: %v", resp.X.Name, resp.Error)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: stored}, now, true)
	default:
		u.Printf("Extraction Finished: %s => elapsed: %v, archives: %d, extra archives: %d, "+
			"files extracted: %d, wrote: %sB", resp.X.Name, resp.Elapsed.Round(time.Second),
			resp.Archives.Count(), resp.Extras.Count(), len(resp.NewFiles), bytefmt.ByteSize(resp.Size))
		u.Debugf("Extraction Finished: %d files in path: %s", len(files), files)
		recordUnpacked(item, resp)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTED, Resp: stored}, now, true)

		if item.App == starr.Lidarr && lidarrImportsTracks(item) && resp.Size > 0 {
			go u.importSplitFlacTracks(item, u.lidarrServerByURL(item.URL))
		}
	}
}

// Looking for a message that looks like:
// "No files found are eligible for import in /downloads/Downloading/Space.Warriors.S99E88.GrOuP.1080p.WEB.x264".
func (u *Unpackerr) getDownloadPath(outputPath, label, title string, paths []string) string {
	var errs []error

	// Try all the user provided paths.
	for _, path := range paths {
		path = filepath.Join(path, title)

		switch _, err := os.Stat(path); err {
		default:
			errs = append(errs, err)
		case nil:
			return path
		}
	}

	// Print the errors for each user-provided path.
	u.Debugf("%s: Errors encountered looking for %s path: %q", label, title, errs)

	// The title often differs from the actual folder name (e.g. torrent names include genre tags).
	// Try the folder name from outputPath against configured paths — the folder name is the real
	// directory on disk. This also handles cross-platform setups where outputPath is a UNC/Windows
	// path but the configured paths are local Linux mounts of the same share.
	if outputPath != "" {
		outputFolder := crossPlatformBase(outputPath)
		if outputFolder != "" && outputFolder != "." && outputFolder != title {
			for _, path := range paths {
				candidate := filepath.Join(path, outputFolder)

				if _, err := os.Stat(candidate); err == nil {
					u.Debugf("%s: Resolved via outputPath folder name: %s -> %s", label, outputPath, candidate)
					return candidate
				}
			}
		}

		u.Debugf("%s: Configured paths do not exist; trying 'outputPath': %s", label, outputPath)

		return outputPath
	}

	u.Debugf("%s: Configured paths do not exist and 'outputPath' is empty for: %s", label, title)

	return filepath.Join(paths[0], title) // useless, but return something. :(
}

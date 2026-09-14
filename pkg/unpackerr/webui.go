package unpackerr

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/version"
	"golift.io/xtractr"
)

type webStatusSnapshot struct {
	GeneratedAt    string              `json:"generatedAt"`
	Uptime         string              `json:"uptime"`
	Stats          *Stats              `json:"stats"`
	Buffers        webStatusBuffers    `json:"buffers"`
	Counters       webStatusCounters   `json:"counters"`
	ActiveCount    int                 `json:"activeCount"`
	CompletedCount int                 `json:"completedCount"`
	Items          []webStatusItem     `json:"items"`
	dismissed      map[string]struct{} `json:"-"`
}

type webStatusBuffers struct {
	Deletes       int `json:"deletes"`
	FolderEvents  int `json:"folderEvents"`
	FolderUpdates int `json:"folderUpdates"`
	Hooks         int `json:"hooks"`
	XtractUpdates int `json:"xtractUpdates"`
}

type webStatusCounters struct {
	CmdFail  uint `json:"cmdFail"`
	CmdOK    uint `json:"cmdOK"`
	Finished uint `json:"finished"`
	HookFail uint `json:"hookFail"`
	HookOK   uint `json:"hookOK"`
	Retries  uint `json:"retries"`
}

type webStatusItem struct {
	ID          string             `json:"id"`
	Key         string             `json:"-"`
	Active      bool               `json:"active"`
	Completed   bool               `json:"completed"`
	App         string             `json:"app"`
	CurrentFile string             `json:"currentFile,omitempty"`
	DeleteAt    string             `json:"deleteAt,omitempty"`
	DeleteIn    string             `json:"deleteIn,omitempty"`
	Elapsed     string             `json:"elapsed"`
	Details     *webStatusDetails  `json:"details,omitempty"`
	Error       string             `json:"error,omitempty"`
	Name        string             `json:"name"`
	Path        string             `json:"path"`
	Progress    *webStatusProgress `json:"progress,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	Status      string             `json:"status"`
	StatusText  string             `json:"statusText"`
	UpdatedAt   string             `json:"updatedAt"`
}

type webStatusDetails struct {
	Title     string            `json:"title,omitempty"`
	Bytes     string            `json:"bytes,omitempty"`
	Elapsed   string            `json:"elapsed,omitempty"`
	Output    string            `json:"output,omitempty"`
	StartedAt string            `json:"startedAt,omitempty"`
	Queue     int               `json:"queue,omitempty"`
	Archives  []string          `json:"archives,omitempty"`
	Files     []string          `json:"files,omitempty"`
	IDs       map[string]string `json:"ids,omitempty"`
}

type webStatusProgress struct {
	Archive      string  `json:"archive"`
	ArchiveCount int     `json:"archiveCount"`
	ArchiveIndex int     `json:"archiveIndex"`
	ETA          string  `json:"eta,omitempty"`
	Percent      float64 `json:"percent"`
	Speed        string  `json:"speed,omitempty"`
	Summary      string  `json:"summary"`
	TotalBytes   string  `json:"totalBytes"`
	WrittenBytes string  `json:"writtenBytes"`
}

const (
	webStatusRankExtracting = iota
	webStatusRankQueued
	webStatusRankWaiting
	webStatusRankFailed
	webStatusRankExtracted
	webStatusRankImported
	webStatusRankDeleted
	webStatusRankExtractedNothing
	webStatusRankDefault
)

func (u *Unpackerr) refreshWebState(now time.Time) {
	u.webState.Store(u.buildWebState(now))
}

func (u *Unpackerr) buildWebState(now time.Time) *webStatusSnapshot {
	prev := u.webState.Load()
	stats := u.currentStats()
	dismissed := webStatusDismissedItems(prev)
	items, currentKeys := u.buildTrackedWebItems(now, dismissed)

	items = u.buildWaitingFolderWebItems(items, currentKeys, now)
	items = mergeRetainedCompletedWebItems(items, prev, currentKeys, dismissed, now)
	sortWebStatusItems(items)

	buffers := u.currentWebBuffers()
	activeCount, completedCount := webStatusItemCounts(items)

	return &webStatusSnapshot{
		GeneratedAt:    now.Format(time.RFC3339),
		Uptime:         formatDuration(now.Sub(version.Started)),
		Stats:          stats,
		Buffers:        buffers,
		ActiveCount:    activeCount,
		CompletedCount: completedCount,
		Counters: webStatusCounters{
			CmdFail:  stats.CmdFail,
			CmdOK:    stats.CmdOK,
			Finished: u.Finished,
			HookFail: stats.HookFail,
			HookOK:   stats.HookOK,
			Retries:  u.Retries,
		},
		Items:     items,
		dismissed: dismissed,
	}
}

func (u *Unpackerr) buildTrackedWebItems(
	now time.Time, dismissed map[string]struct{},
) ([]webStatusItem, map[string]struct{}) {
	items := make([]webStatusItem, 0, len(u.Map))
	currentKeys := make(map[string]struct{}, len(u.Map))

	for name, item := range u.Map {
		var folderItem *Folder
		if u.folders != nil {
			folderItem = u.folders.Folders[name]
		}

		webItem := buildWebStatusItem(name, item, folderItem, now)

		currentKeys[webItem.Key] = struct{}{}
		if _, skip := dismissed[webItem.Key]; skip && webItem.Completed {
			continue
		}

		items = append(items, webItem)
	}

	return items, currentKeys
}

func (u *Unpackerr) buildWaitingFolderWebItems(
	items []webStatusItem, currentKeys map[string]struct{}, now time.Time,
) []webStatusItem {
	if u.folders == nil {
		return items
	}

	for name, folder := range u.folders.Folders {
		if _, ok := u.Map[name]; ok || folder == nil || folder.Status != WAITING {
			continue
		}

		if !folderHasExtractableContent(name, folder.Config) {
			continue
		}

		item := buildWaitingFolderStatusItem(name, folder, now)
		currentKeys[item.Key] = struct{}{}
		items = append(items, item)
	}

	return items
}

func mergeRetainedCompletedWebItems(
	items []webStatusItem,
	prev *webStatusSnapshot,
	currentKeys map[string]struct{},
	dismissed map[string]struct{},
	now time.Time,
) []webStatusItem {
	if prev == nil {
		return items
	}

	retainedKeys := make(map[string]struct{}, len(prev.Items))
	for _, item := range prev.Items {
		if !item.Completed {
			continue
		}

		if _, ok := currentKeys[item.Key]; ok {
			continue
		}

		if _, ok := retainedKeys[item.Key]; ok {
			continue
		}

		if _, skip := dismissed[item.Key]; skip {
			continue
		}

		item.Elapsed = webStatusElapsed(item.UpdatedAt, now)
		retainedKeys[item.Key] = struct{}{}
		items = append(items, item)
	}

	return items
}

func sortWebStatusItems(items []webStatusItem) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.Completed != right.Completed {
			return !left.Completed
		}

		if leftRank, rightRank := webStatusRank(left.Status), webStatusRank(right.Status); leftRank != rightRank {
			return leftRank < rightRank
		}

		return left.UpdatedAt > right.UpdatedAt
	})
}

func folderHasExtractableContent(path string, cfg *FolderConfig) bool {
	if path == "" {
		return false
	}

	if xtractr.IsArchiveFile(path) {
		return true
	}

	exclude := folderExcludeSuffixes(path, cfg)

	return xtractr.FindCompressedFiles(xtractr.Filter{
		Path:          path,
		ExcludeSuffix: exclude,
		AllowSymlinks: cfg != nil && cfg.AllowSymlinks,
	}).Count() > 0
}

func (u *Unpackerr) currentWebBuffers() webStatusBuffers {
	buffers := webStatusBuffers{
		Deletes:       len(u.delChan),
		XtractUpdates: len(u.updates),
	}
	if u.hookWorker != nil {
		buffers.Hooks = u.hookWorker.Len()
	}
	if u.folders == nil {
		return buffers
	}

	buffers.FolderEvents = len(u.folders.Events)
	buffers.FolderUpdates = len(u.folders.Updates)

	return buffers
}

func webStatusItemCounts(items []webStatusItem) (int, int) {
	var (
		activeCount    int
		completedCount int
	)

	for _, item := range items {
		if item.Completed {
			completedCount++
			continue
		}

		activeCount++
	}

	return activeCount, completedCount
}

func buildWebStatusItem(name string, item *Extract, folder *Folder, now time.Time) webStatusItem {
	output := webStatusItem{
		ID:         webStatusItemID(name, item.Status, item.Updated),
		Key:        webStatusItemKey(name, item.Path, string(item.App)),
		Active:     !webStatusIsCompleted(item.Status.String()),
		Completed:  webStatusIsCompleted(item.Status.String()),
		App:        string(item.App),
		Elapsed:    now.Sub(item.Updated).Round(time.Second).String(),
		Details:    buildWebStatusDetails(item),
		Name:       webStatusDisplayName(name, item),
		Path:       item.Path,
		Status:     item.Status.String(),
		StatusText: webStatusText(item.Status, string(item.App)),
		UpdatedAt:  item.Updated.Format(time.RFC3339),
	}

	if item.Resp != nil && item.Resp.Error != nil {
		output.Error = item.Resp.Error.Error()
	}

	if reason, ok := item.IDs["reason"]; ok {
		output.Reason = fmt.Sprint(reason)
	}

	if item.XProg != nil {
		output.Progress = buildWebStatusProgress(item.XProg, now, item.Status == EXTRACTING)
		if output.Progress != nil {
			output.CurrentFile = output.Progress.Archive
		}
	}

	output.DeleteIn, output.DeleteAt = webStatusDeleteTiming(item, folder, now)

	return output
}

func buildWaitingFolderStatusItem(name string, folder *Folder, now time.Time) webStatusItem {
	reason := ""
	if folder.Config != nil && folder.Config.Path != "" {
		reason = "Watching folder: " + folder.Config.Path
	}

	return webStatusItem{
		ID:         webStatusItemID(name, folder.Status, folder.Updated),
		Key:        webStatusItemKey(name, name, FolderString),
		Active:     true,
		Completed:  false,
		App:        FolderString,
		Elapsed:    now.Sub(folder.Updated).Round(time.Second).String(),
		Name:       webStatusLabel(name),
		Path:       name,
		Reason:     reason,
		Status:     folder.Status.String(),
		StatusText: webStatusText(folder.Status, FolderString),
		UpdatedAt:  folder.Updated.Format(time.RFC3339),
	}
}

func buildWebStatusDetails(item *Extract) *webStatusDetails {
	details := &webStatusDetails{}
	populateWebStatusIdentityDetails(details, item)

	if item.Resp != nil {
		populateWebStatusResponseDetails(details, item.Resp)
	}

	if webStatusDetailsEmpty(details) {
		return nil
	}

	return details
}

func populateWebStatusIdentityDetails(details *webStatusDetails, item *Extract) {
	if title, ok := item.IDs["title"]; ok {
		details.Title = fmt.Sprint(title)
	}

	if details.Title == "" || details.Title == item.Path {
		details.Title = webStatusDisplayName(item.Path, item)
	}

	if len(item.IDs) == 0 {
		return
	}

	details.IDs = make(map[string]string, len(item.IDs))
	for key, value := range item.IDs {
		if key == "title" && details.Title != "" {
			details.IDs[key] = details.Title
			continue
		}

		details.IDs[key] = fmt.Sprint(value)
	}
}

func populateWebStatusResponseDetails(details *webStatusDetails, response *xtractr.Response) {
	if response.Started.Unix() > 0 {
		details.StartedAt = response.Started.Format(time.RFC3339)
	}

	if response.Output != "" {
		details.Output = response.Output
	}

	if response.Size > 0 {
		details.Bytes = bytefmt.ByteSize(response.Size) + "B"
	}

	if response.Elapsed > 0 {
		details.Elapsed = response.Elapsed.Round(time.Second).String()
	}

	details.Queue = response.Queued

	for _, archiveGroup := range response.Archives {
		details.Archives = append(details.Archives, archiveGroup...)
	}

	for _, extraGroup := range response.Extras {
		details.Archives = append(details.Archives, extraGroup...)
	}

	for _, file := range response.NewFiles {
		if webStatusShouldHideFile(file) {
			continue
		}

		details.Files = append(details.Files, file)
	}
}

func webStatusDetailsEmpty(details *webStatusDetails) bool {
	return details.Title == "" &&
		details.Bytes == "" &&
		details.Elapsed == "" &&
		details.Output == "" &&
		details.StartedAt == "" &&
		details.Queue == 0 &&
		len(details.Archives) == 0 &&
		len(details.Files) == 0 &&
		len(details.IDs) == 0
}

func webStatusDisplayName(name string, item *Extract) string {
	if item != nil {
		if title, ok := item.IDs["title"]; ok {
			display := strings.TrimSpace(fmt.Sprint(title))
			if display != "" && display != item.Path {
				return display
			}
		}

		if item.Path != "" {
			return webStatusLabel(item.Path)
		}
	}

	return webStatusLabel(name)
}

func webStatusText(status ExtractStatus, app string) string {
	if app == FolderString && status == EXTRACTED {
		return "Extracted"
	}

	return status.Desc()
}

func webStatusDeleteTiming(item *Extract, folder *Folder, now time.Time) (string, string) {
	switch {
	case item == nil:
		return "", ""
	case item.App == FolderString:
		if item.Status != EXTRACTED || folder == nil || folder.Config == nil || folder.Config.DeleteAfter == nil {
			return "", ""
		}

		return webStatusDeleteWindow(item.Updated, folder.Config.DeleteAfter.Duration, now)
	case item.Status == IMPORTED:
		return webStatusDeleteWindow(item.Updated, item.DeleteDelay, now)
	default:
		return "", ""
	}
}

func webStatusDeleteWindow(updated time.Time, delay time.Duration, now time.Time) (string, string) {
	if delay <= 0 || updated.IsZero() {
		return "", ""
	}

	deleteAt := updated.Add(delay)

	remaining := deleteAt.Sub(now).Round(time.Second)
	if remaining <= 0 {
		return "", deleteAt.Format(time.RFC3339)
	}

	return remaining.String(), deleteAt.Format(time.RFC3339)
}

func webStatusLabel(value string) string {
	value = filepath.Clean(value)

	label := filepath.Base(value)
	if label == "." || label == string(filepath.Separator) || label == "" {
		return value
	}

	return label
}

func webStatusShouldHideFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == "unpackerred.txt" || base == "_unpackerred.txt" {
		return true
	}

	return strings.HasPrefix(base, "_unpackerred.") && strings.HasSuffix(base, ".txt")
}

func buildWebStatusProgress(progress *ExtractProgress, now time.Time, live bool) *webStatusProgress {
	if progress == nil || progress.Progress == nil {
		return nil
	}

	wrote, total := progress.Bytes()

	basePath := ""
	if progress.Extract != nil {
		basePath = progress.Path
	}

	archive := ""
	if progress.XFile != nil {
		archive = strings.TrimLeft(strings.TrimPrefix(progress.XFile.FilePath, basePath), `/\`)
	}

	summary := noProgressText
	if progress.XFile != nil && progress.Extract != nil {
		summary = progress.String()
	} else if progress.XFile != nil {
		summary = fmt.Sprintf("on archive: %d/%d @ %sB/%sB (%.0f%%): %s",
			progress.Extracted+1, progress.Archives, bytefmt.ByteSize(wrote), bytefmt.ByteSize(total),
			progress.Percent(), archive)
	}

	output := &webStatusProgress{
		Archive:      archive,
		ArchiveCount: progress.Archives,
		ArchiveIndex: progress.Extracted + 1,
		Percent:      progress.Percent(),
		Summary:      summary,
		TotalBytes:   bytefmt.ByteSize(total) + "B",
		WrittenBytes: bytefmt.ByteSize(wrote) + "B",
	}

	if speed, ok := progress.Speed(now); live && ok {
		output.Speed = bytefmt.ByteSize(speed) + "B/s"
	}

	if eta, ok := progress.ETA(now); live && ok {
		output.ETA = eta.String()
	}

	return output
}

func webStatusRank(status string) int {
	switch status {
	case EXTRACTING.String():
		return webStatusRankExtracting
	case QUEUED.String():
		return webStatusRankQueued
	case WAITING.String():
		return webStatusRankWaiting
	case EXTRACTFAILED.String():
		return webStatusRankFailed
	case EXTRACTED.String():
		return webStatusRankExtracted
	case IMPORTED.String():
		return webStatusRankImported
	case DELETED.String(), DELETING.String():
		return webStatusRankDeleted
	case EXTRACTEDNOTHING.String():
		return webStatusRankExtractedNothing
	default:
		return webStatusRankDefault
	}
}

func webStatusItemID(name string, status ExtractStatus, updated time.Time) string {
	return fmt.Sprintf("%s|%s|%s", name, status.String(), updated.Format(time.RFC3339Nano))
}

func webStatusItemKey(name, path, app string) string {
	if path != "" {
		return app + "|" + path
	}

	return app + "|" + name
}

func webStatusIsCompleted(status string) bool {
	switch status {
	case EXTRACTED.String(), IMPORTED.String(), DELETING.String(), DELETED.String(), EXTRACTEDNOTHING.String():
		return true
	default:
		return false
	}
}

func webStatusElapsed(updatedAt string, now time.Time) string {
	if updatedAt == "" {
		return ""
	}

	lastUpdated, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return ""
	}

	return now.Sub(lastUpdated).Round(time.Second).String()
}

func webStatusDismissedItems(snapshot *webStatusSnapshot) map[string]struct{} {
	if snapshot == nil || len(snapshot.dismissed) == 0 {
		return make(map[string]struct{})
	}

	output := make(map[string]struct{}, len(snapshot.dismissed))
	for item := range snapshot.dismissed {
		output[item] = struct{}{}
	}

	return output
}

func (u *Unpackerr) snapshotStats() *Stats {
	if snap := u.webState.Load(); snap != nil && snap.Stats != nil {
		stats := *snap.Stats
		return &stats
	}

	return &Stats{}
}

func (u *Unpackerr) currentStats() *Stats {
	stats := &Stats{}
	stats.HookOK, stats.HookFail = u.WebhookCounts()
	stats.CmdOK, stats.CmdFail = u.CmdhookCounts()

	for name := range u.Map {
		addStatusCount(stats, u.Map[name].Status)
	}

	if u.folders != nil {
		for name, folder := range u.folders.Folders {
			if _, ok := u.Map[name]; ok || folder == nil {
				continue
			}

			addStatusCount(stats, folder.Status)
		}
	}

	return stats
}

func addStatusCount(stats *Stats, status ExtractStatus) {
	switch status {
	case WAITING:
		stats.Waiting++
	case QUEUED:
		stats.Queued++
	case EXTRACTING:
		stats.Extracting++
	case DELETEFAILED, EXTRACTFAILED:
		stats.Failed++
	case EXTRACTED:
		stats.Extracted++
	case DELETED, DELETING:
		stats.Deleted++
	case IMPORTED:
		stats.Imported++
	}
}

func (u *Unpackerr) webIndex(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := statusPageTemplate().Execute(writer, nil); err != nil {
		u.Errorf("rendering web status page: %v", err)
	}
}

func (u *Unpackerr) webStatusAPI(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	snapshot := u.webState.Load()
	if snapshot == nil {
		snapshot = &webStatusSnapshot{Stats: &Stats{}}
	}

	encodeWebStatusJSON(writer, snapshot, u.Errorf)
}


func (u *Unpackerr) webClearCompletedAPI(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	snapshot := u.webState.Load()
	if snapshot == nil {
		snapshot = &webStatusSnapshot{Stats: &Stats{}, dismissed: make(map[string]struct{})}
	}

	next := *snapshot
	next.dismissed = webStatusDismissedItems(snapshot)
	next.Items = make([]webStatusItem, 0, len(snapshot.Items))
	next.ActiveCount = 0
	next.CompletedCount = 0

	for _, item := range snapshot.Items {
		if item.Completed {
			next.dismissed[item.Key] = struct{}{}
			continue
		}

		next.Items = append(next.Items, item)
		next.ActiveCount++
	}

	u.webState.Store(&next)
	encodeWebStatusJSON(writer, &next, u.Errorf)
}

func encodeWebStatusJSON(writer http.ResponseWriter, payload any, logFunc func(string, ...any)) {
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		logFunc("encoding web status payload: %v", err)
	}
}

func statusPageTemplate() *template.Template {
	return template.Must(template.New("status").Parse(statusPageHTML))
}

const statusPageHTML = `<!doctype html>
	<html lang="en">
	<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Unpackerr Status</title>
  <link
    rel="icon"
    type="image/png"
    href="https://cdn.jsdelivr.net/gh/selfhst/icons@main/png/unpackerr.png"
  >
  <style>
    :root {
      color-scheme: dark;
      --bg: #0d0d0d;
      --panel: #141414;
      --panel-strong: #191919;
      --panel-soft: rgba(255, 255, 255, 0.045);
      --panel-softer: rgba(255, 255, 255, 0.025);
      --text: #f2f2f2;
      --heading: #ffffff;
      --muted: #a9a9a9;
      --accent: #ff4b2f;
      --accent-strong: #ff7a45;
      --accent-cool: #b9c0ca;
      --good: #16c784;
      --warn: #f6c453;
      --bad: #ff4d5e;
      --border: #2d2d2d;
      --border-strong: #555555;
      --shadow: none;
      --font: "JetBrains Mono", "Cascadia Mono", Consolas, "SFMono-Regular", monospace;
      --mono: "JetBrains Mono", "Cascadia Mono", Consolas, "SFMono-Regular", monospace;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: var(--font);
      background:
        linear-gradient(rgba(255, 255, 255, 0.055) 1px, transparent 1px),
        linear-gradient(90deg, rgba(255, 255, 255, 0.055) 1px, transparent 1px),
        var(--bg);
      background-size: 40px 40px, 40px 40px, auto;
      color: var(--text);
      transition: color 160ms ease;
    }
	    .shell {
	      max-width: 1480px;
	      margin: 0 auto;
	      padding: 42px 28px 72px;
	    }
	    .hero {
	      display: grid;
	      gap: 28px;
	      margin-bottom: 34px;
	      padding: 34px;
      border: 1px solid var(--border);
      border-radius: 0;
      background: rgba(20, 20, 20, 0.96);
      box-shadow: var(--shadow);
      position: relative;
      overflow: hidden;
    }
    .hero::before {
      content: "";
      position: absolute;
      inset: 0;
      border-top: 1px solid var(--border-strong);
      pointer-events: none;
    }
    .hero::after {
      content: "";
      position: absolute;
      inset: auto 0 0;
      height: 1px;
      background: var(--border-strong);
      pointer-events: none;
    }
	    .headline {
	      display: flex;
	      flex-wrap: wrap;
	      gap: 24px;
      align-items: flex-start;
      justify-content: space-between;
      position: relative;
      z-index: 1;
    }
	    .hero-copy {
	      display: grid;
	      gap: 18px;
	      max-width: 780px;
	    }
	    .title-row {
	      display: flex;
	      flex-wrap: wrap;
	      align-items: center;
	      align-content: center;
	      gap: 14px;
	    }
	    .repo-link {
	      display: inline-flex;
	      align-items: center;
	      justify-content: center;
	      width: 44px;
	      height: 44px;
	      align-self: center;
	      margin-top: 0;
	      transform: translateY(-2px);
	      border-radius: 0;
	      border: 1px solid var(--border);
	      background: #111111;
	      color: var(--text);
	      text-decoration: none;
	      box-shadow: none;
	      transition: transform 140ms ease, border-color 140ms ease, background 140ms ease, box-shadow 140ms ease;
	    }
	    .repo-link:hover {
	      transform: translateY(-3px);
	      border-color: var(--accent);
	      background: #1c1210;
	      box-shadow: none;
	    }
	    .repo-link svg {
	      width: 21px;
	      height: 21px;
	      fill: currentColor;
	    }
	    .headline-actions {
	      display: flex;
	      flex-wrap: wrap;
	      gap: 16px;
	      align-items: center;
	      justify-content: flex-end;
	    }
	    h1 {
	      margin: 0;
	      font-size: clamp(2rem, 4vw, 3.1rem);
	      line-height: 1;
	      letter-spacing: 0;
	      color: var(--heading);
	      display: flex;
	      align-items: center;
	    }
    .subtle {
      color: var(--muted);
      font-size: 1rem;
      line-height: 1.65;
    }
	    .stamp-chip {
	      display: inline-flex;
	      align-items: center;
	      gap: 10px;
	      padding: 13px 18px;
	      min-height: 48px;
	      border-radius: 0;
      border: 1px solid var(--border);
      background: #121212;
      color: var(--text);
      box-shadow: none;
    }
    .stamp-chip::before {
      content: "";
      width: 10px;
      height: 10px;
      border-radius: 50%;
      background: var(--good);
      box-shadow: 0 0 10px rgba(22, 199, 132, 0.7);
      flex: 0 0 auto;
    }
	    .action-button {
      appearance: none;
      border: 1px solid var(--border-strong);
      background: #151515;
      color: var(--text);
      border-radius: 0;
	      padding: 12px 18px;
      font: inherit;
      font-size: 0.82rem;
      font-weight: 700;
      text-transform: uppercase;
      cursor: pointer;
      box-shadow: none;
      transition: transform 140ms ease, border-color 140ms ease, background 140ms ease, box-shadow 140ms ease;
    }
    .action-button:hover {
      transform: translateY(-1px);
      border-color: var(--accent);
      background: #241411;
      box-shadow: none;
    }
    .action-button:disabled {
      opacity: 0.45;
      cursor: default;
      transform: none;
      box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
    }
	    .grid {
	      display: grid;
	      gap: 22px;
	      grid-template-columns: repeat(auto-fit, minmax(132px, 1fr));
	    }
    .card, .table-card {
      background: rgba(18, 18, 18, 0.96);
      border: 1px solid var(--border);
      border-radius: 0;
      box-shadow: var(--shadow);
    }
	    .card {
	      padding: 24px;
	      position: relative;
	      overflow: hidden;
	    }
    .card::before {
      content: "";
      position: absolute;
      inset: 0 auto 0 0;
      width: 3px;
      background: var(--muted);
      opacity: 0.72;
    }
    .card .label {
      color: var(--muted);
      font-size: 0.74rem;
      text-transform: uppercase;
      letter-spacing: 0;
    }
	    .card .value {
	      margin-top: 16px;
      color: var(--heading);
      font-size: 2rem;
      font-weight: 800;
      letter-spacing: 0;
      line-height: 1;
    }
    .card.tone-extracting::before {
      background: var(--accent);
    }
    .card.tone-queued::before,
    .card.tone-waiting::before {
      background: var(--warn);
    }
    .card.tone-failed::before {
      background: var(--bad);
    }
    .card.tone-extracted::before,
    .card.tone-imported::before,
    .card.tone-deleted::before {
      background: var(--good);
    }
	    .rail {
	      display: grid;
	      gap: 16px;
	      margin-top: 6px;
	      position: relative;
	      z-index: 1;
	      grid-template-columns: minmax(0, 1fr);
	    }
	    .pill {
	      display: flex;
	      align-items: center;
	      gap: 6px;
	      flex-wrap: wrap;
	      background: #141414;
	      border: 1px solid var(--border);
	      color: var(--muted);
	      border-radius: 0;
	      padding: 12px 16px;
	      font-size: 0.9rem;
	      min-width: 0;
	      overflow-wrap: anywhere;
	      white-space: normal;
	    }
    .pill strong {
	      color: var(--text);
	      font-weight: 700;
	      flex: 0 0 auto;
	    }
	    .table-card {
	      overflow: hidden;
	      margin-top: 26px;
	    }
	    .table-head {
	      display: flex;
	      justify-content: space-between;
	      gap: 18px;
	      align-items: center;
	      padding: 28px 28px 18px;
	    }
	    .table-wrap {
	      overflow-x: auto;
	      padding: 0 18px 18px;
	    }
	    table {
      width: 100%;
      border-collapse: collapse;
      table-layout: fixed;
      min-width: 980px;
    }
	    th, td {
	      text-align: left;
	      padding: 22px 18px;
	      border-top: 1px solid var(--border);
	      vertical-align: top;
	    }
    th {
      position: relative;
      color: var(--muted);
      font-size: 0.74rem;
      text-transform: uppercase;
      letter-spacing: 0;
      font-weight: 600;
    }
    .column-resizer {
      position: absolute;
      top: 0;
      right: -5px;
      width: 10px;
      height: 100%;
      cursor: col-resize;
      touch-action: none;
      z-index: 2;
    }
    .column-resizer::after {
      content: "";
      position: absolute;
      top: 24%;
      bottom: 24%;
      left: 4px;
      width: 2px;
      background: var(--border-strong);
      transition: background 120ms ease, box-shadow 120ms ease;
    }
    .column-resizer:hover::after,
    .column-resizer:focus-visible::after,
    body.is-resizing-columns .column-resizer.is-active::after {
      background: var(--accent);
      box-shadow: 0 0 8px rgba(255, 75, 47, 0.65);
    }
    .column-resizer:focus-visible {
      outline: none;
    }
    body.is-resizing-columns {
      cursor: col-resize;
      user-select: none;
    }
    tr:hover td {
      background: rgba(255, 75, 47, 0.06);
    }
    .items-row {
      cursor: pointer;
    }
    .items-row.is-selected td {
      background: rgba(255, 75, 47, 0.1);
    }
    .items-cell-item {
      min-width: 0;
    }
    .item-title {
      line-height: 1.45;
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    .items-cell-status,
    .items-cell-progress,
    .items-cell-updated,
    .items-cell-delete,
    .items-cell-path {
      font-size: 0.86rem;
      line-height: 1.45;
    }
    .status {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      background: transparent;
      border: 1px solid transparent;
      border-radius: 0;
      padding: 6px 0;
      white-space: nowrap;
      font-size: inherit;
    }
    .status::before {
      content: "";
      width: 9px;
      height: 9px;
      border-radius: 50%;
      background: currentColor;
      box-shadow: 0 0 10px currentColor;
    }
    .status.extracting {
      color: var(--accent);
      background: transparent;
      border-color: transparent;
    }
    .status.queued,
    .status.waiting {
      color: var(--warn);
      background: transparent;
      border-color: transparent;
    }
    .status.extractfailed,
    .status.deletefailed {
      color: var(--bad);
      background: transparent;
      border-color: transparent;
    }
    .status.extracted,
    .status.imported,
    .status.deleted,
    .status.deleting {
      color: var(--good);
      background: transparent;
      border-color: transparent;
    }
    .status.extractednothing {
      color: var(--muted);
      background: transparent;
      border-color: transparent;
    }
    .muted {
      color: var(--muted);
    }
    .path {
      font-family: var(--mono);
      font-size: inherit;
      color: var(--text);
      word-break: break-all;
      line-height: inherit;
    }
	    .progress {
	      display: grid;
	      gap: 8px;
	      min-width: 0;
	    }
    .progress-bar {
      position: relative;
      width: 100%;
      height: 12px;
      border-radius: 0;
      background: #0f0f0f;
      border: 1px solid var(--border);
      overflow: hidden;
    }
    .progress-bar > span {
      position: relative;
      display: block;
      height: 100%;
      background: var(--accent);
      border-radius: 0;
      transition: width 360ms cubic-bezier(0.22, 1, 0.36, 1);
    }
    .progress-bar > span::after {
      content: none;
    }
	    .empty {
	      padding: 48px 22px 54px;
	      text-align: center;
	      color: var(--muted);
	    }
    .detail-card {
      padding-bottom: 18px;
    }
	    .detail-grid {
	      display: grid;
	      gap: 18px;
	      grid-template-columns: minmax(0, 1.45fr) minmax(280px, 0.75fr);
	      padding: 12px;
	    }
	    .detail-section {
	      border: 1px solid var(--border);
	      border-radius: 0;
	      padding: 18px;
	      background: #141414;
	      min-width: 0;
	    }
    .detail-section-wide {
      grid-column: 1 / -1;
    }
    .detail-section-title {
      color: var(--heading);
      font-size: 1rem;
      font-weight: 700;
      line-height: 1.45;
      word-break: break-word;
    }
    .detail-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 10px 20px;
      margin-top: 14px;
    }
    .detail-meta-item {
      min-width: 120px;
    }
    .detail-label,
    .detail-field .label {
      color: var(--muted);
      font-size: 0.78rem;
      letter-spacing: 0;
      text-transform: uppercase;
      margin-bottom: 8px;
    }
    .detail-value {
      color: var(--text);
      line-height: 1.5;
      word-break: break-word;
    }
    .detail-paths {
      display: grid;
      gap: 14px;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      margin-top: 18px;
    }
    .detail-path-row {
      min-width: 0;
    }
    .detail-path-row-wide {
      grid-column: 1 / -1;
    }
    .detail-path-value {
      font-family: var(--mono);
      font-size: 0.86rem;
      line-height: 1.5;
      color: var(--text);
      word-break: break-all;
    }
    .detail-lines {
      display: grid;
      gap: 12px;
    }
	    .detail-field {
	      border: 1px solid var(--border);
	      border-radius: 0;
	      padding: 18px;
	      background: #141414;
	    }
    .detail-field .value {
      color: var(--text);
      line-height: 1.55;
      word-break: break-word;
    }
	    .detail-lists {
	      display: grid;
	      gap: 18px;
	      padding: 12px;
	    }
	    .detail-list {
	      border: 1px solid var(--border);
	      border-radius: 0;
	      background: #141414;
	      padding: 18px;
	    }
    .detail-list strong {
      display: block;
      margin-bottom: 10px;
      color: var(--heading);
    }
    .detail-list ul {
      margin: 0;
      padding-left: 18px;
    }
    .detail-list li {
      margin: 0 0 8px;
      line-height: 1.45;
      word-break: break-word;
    }
    .detail-list code {
      font-family: var(--mono);
      font-size: 0.82rem;
    }
		    @media (max-width: 1080px) {
	      .headline-actions {
	        justify-content: flex-start;
	      }
	      .table-head {
	        display: grid;
	        grid-template-columns: minmax(0, 1fr);
	      }
	      .table-wrap {
	        overflow-x: visible;
	        padding: 0 14px 14px;
	      }
	      table {
	        display: block;
	        min-width: 0;
	        width: 100%;
	      }
	      tbody {
	        display: block;
	        width: 100%;
	      }
	      thead {
	        display: none;
	      }
	      .column-reset,
	      .column-resizer {
	        display: none;
	      }
	      #items {
	        display: grid;
	        gap: 14px;
	        width: 100%;
	      }
	      .items-row,
	      .empty-row {
	        display: block;
	        width: 100%;
	      }
	      .items-row {
	        border: 1px solid var(--border);
	        border-radius: 0;
	        background: rgba(18, 18, 18, 0.96);
	        box-shadow: none;
	        overflow: hidden;
	      }
	      .items-row td {
	        display: grid;
	        gap: 8px;
	        padding: 15px 16px 16px;
	        border-top: 1px solid var(--border);
	        background: transparent;
	      }
	      .items-row td:first-child {
	        border-top: 0;
	        padding-top: 18px;
	      }
	      .items-row td::before {
	        content: attr(data-label);
	        color: var(--muted);
	        font-size: 0.72rem;
	        font-weight: 700;
	        letter-spacing: 0;
	        line-height: 1.2;
	        text-transform: uppercase;
	      }
	      .items-row td:first-child::before {
	        color: var(--heading);
	      }
	      .items-row.is-selected {
	        border-color: var(--accent);
	        box-shadow: none;
	      }
	      .items-row:hover td,
	      .items-row.is-selected td {
	        background: transparent;
	      }
	      .progress {
	        min-width: 0;
	      }
	      .detail-grid {
	        grid-template-columns: minmax(0, 1fr);
	      }
	      .detail-paths {
	        grid-template-columns: minmax(0, 1fr);
	      }
	      .detail-section-wide {
	        grid-column: auto;
	      }
	      .empty-row td {
	        display: block;
	        border-top: 0;
	        padding: 34px 18px 38px;
	      }
	      .empty-row td::before {
	        content: none;
	      }
		    }
		    @media (max-width: 720px) {
		      .shell {
		        padding: 24px 16px 48px;
		      }
		      .hero {
		        padding: 22px;
		      }
		    }
	    @media (min-width: 760px) {
	      .rail {
	        grid-template-columns: repeat(2, minmax(0, 1fr));
	      }
	    }
	    @media (min-width: 1120px) {
	      .grid {
	        grid-template-columns: repeat(7, minmax(0, 1fr));
	      }
	      .rail {
	        grid-template-columns: repeat(3, minmax(0, 1fr));
	      }
	    }
	    .auth-panel {
	      max-width: 420px;
	      margin: 48px auto 0;
	      padding: 24px;
	      border: 1px solid var(--border);
	      border-radius: 14px;
	      background: var(--panel);
	    }
	    .auth-panel h2 { margin: 0 0 8px; }
	    .auth-panel form { display: grid; gap: 14px; margin-top: 20px; }
	    .auth-panel label { display: grid; gap: 6px; color: var(--muted); font-size: 0.82rem; }
	    .auth-panel input {
	      padding: 10px 12px;
	      border: 1px solid var(--border);
	      border-radius: 8px;
	      color: var(--text);
	      background: var(--bg);
	      font: inherit;
	    }
	    .auth-error { min-height: 1.2em; color: var(--bad); font-size: 0.82rem; }
	  </style>
</head>
<body>
	  <section class="auth-panel" id="auth-panel" hidden>
	    <h2>Sign in to UnpackUI</h2>
	    <div class="subtle">Use the web UI credentials printed at startup or stored in your configuration.</div>
	    <form id="auth-form">
	      <label>Username <input id="auth-name" name="username" autocomplete="username" value="admin"></label>
	      <label>Password
	        <input id="auth-password" name="password" type="password" autocomplete="current-password" required>
	      </label>
	      <button class="action-button" id="auth-submit" type="submit">Sign in</button>
	      <div class="auth-error" id="auth-error" role="alert"></div>
	    </form>
	  </section>
	  <div class="shell" id="status-shell">
	    <section class="hero">
	      <div class="headline">
	        <div class="hero-copy">
	          <div class="title-row">
	            <h1>Unpackerr Status</h1>
		            <a
		              class="repo-link"
		              href="https://github.com/TheBadFella/UnpackUI"
		              target="_blank"
		              rel="noreferrer"
		              aria-label="Open the UnpackUI repository on GitHub"
		            >
		              <svg viewBox="0 0 16 16" aria-hidden="true">
		                <path
		                  d="M8 0C3.58 0 0 3.73 0 8.33c0 3.68 2.29 6.8 5.47 7.9
		                  .4.08.55-.18.55-.4 0-.2-.01-.86-.01-1.56-2.01.45-2.53-.51-2.69-.98
		                  -.09-.25-.48-.99-.82-1.19-.28-.16-.68-.57-.01-.58.63-.01 1.08.6
		                  1.23.85.72 1.28 1.87.92 2.33.7.07-.54.28-.92.51-1.13
		                  -1.78-.21-3.64-.92-3.64-4.08 0-.9.31-1.64.82-2.22-.08-.21-.36-1.06
		                  .08-2.21 0 0 .67-.22 2.2.85a7.3 7.3 0 0 1 4 0c1.53-1.08 2.2-.85
		                  2.2-.85.44 1.15.16 2 .08 2.21.51.58.82 1.31.82 2.22 0 3.17-1.87 3.87
		                  -3.65 4.08.29.26.54.76.54 1.53 0 1.11-.01 2-.01 2.28 0 .22.14.49.55.4
		                  A8.34 8.34 0 0 0 16 8.33C16 3.73 12.42 0 8 0Z"
		                />
		              </svg>
		            </a>
	          </div>
	          <div class="subtle">Live extraction progress.</div>
	        </div>
        <div class="headline-actions">
          <div class="stamp-chip" id="stamp">Loading...</div>
        </div>
      </div>
      <div class="grid" id="stat-grid"></div>
      <div class="rail" id="meta-rail"></div>
    </section>
	    <section class="table-card">
	      <div class="table-head">
	        <div>
	          <strong>Tracked Items</strong>
	          <div class="subtle">Current queue plus completed items that stay visible until you clear them.</div>
	        </div>
	        <div class="headline-actions">
	          <button class="action-button column-reset" id="reset-columns" type="button">Reset columns</button>
	          <button class="action-button" id="clear-completed" type="button">Clear completed</button>
	          <div class="subtle" id="item-count"></div>
	        </div>
	      </div>
	      <div class="table-wrap">
	        <table id="items-table">
	          <colgroup id="items-cols"></colgroup>
	          <thead>
	            <tr id="items-head">
	              <th>Item</th>
	              <th>Status</th>
	              <th>Progress</th>
	              <th>Updated</th>
	              <th>Path</th>
	            </tr>
	          </thead>
          <tbody id="items">
            <tr><td colspan="5" class="empty">Waiting for status data...</td></tr>
          </tbody>
	        </table>
	      </div>
	    </section>
	    <section class="table-card detail-card" id="detail-card" hidden>
	      <div class="table-head">
	        <div>
	          <strong>Task Details</strong>
	          <div class="subtle" id="detail-subtitle">Click a task row to inspect its details.</div>
	        </div>
	        <div class="headline-actions">
	          <button class="action-button" id="detail-close" type="button">Close</button>
	        </div>
	      </div>
	      <div class="table-wrap">
	        <div class="detail-grid" id="detail-grid"></div>
	        <div class="detail-lists" id="detail-lists"></div>
	      </div>
	    </section>
	  </div>
	  <script>
	    const statOrder = [
	      ['extracting', 'Extracting'],
	      ['queued', 'Queued'],
	      ['waiting', 'Waiting'],
	      ['failed', 'Failed'],
	      ['extracted', 'Extracted'],
	      ['imported', 'Imported'],
	      ['deleted', 'Deleted']
	    ];

	    const statGrid = document.getElementById('stat-grid');
	    const metaRail = document.getElementById('meta-rail');
	    const stamp = document.getElementById('stamp');
	    const clearCompletedButton = document.getElementById('clear-completed');
	    const resetColumnsButton = document.getElementById('reset-columns');
	    const itemCount = document.getElementById('item-count');
	    const itemsTable = document.getElementById('items-table');
	    const itemsCols = document.getElementById('items-cols');
	    const itemsHead = document.getElementById('items-head');
	    const itemsBody = document.getElementById('items');
	    const detailCard = document.getElementById('detail-card');
	    const detailSubtitle = document.getElementById('detail-subtitle');
	    const detailGrid = document.getElementById('detail-grid');
	    const detailLists = document.getElementById('detail-lists');
	    const detailClose = document.getElementById('detail-close');
	    const statusUrl = new URL('api/status', window.location.href);
	    const clearCompletedUrl = new URL('api/status/clear-completed', window.location.href);
	    const loginUrl = new URL('api/auth/login', window.location.href);
	    const authPanel = document.getElementById('auth-panel');
	    const authForm = document.getElementById('auth-form');
	    const authName = document.getElementById('auth-name');
	    const authPassword = document.getElementById('auth-password');
	    const authSubmit = document.getElementById('auth-submit');
	    const authError = document.getElementById('auth-error');
	    const statusShell = document.getElementById('status-shell');
	    const activeRefreshMs = 2000;
	    const idleRefreshMs = 30000;
	    const errorRefreshMs = 10000;
	    const columnWidthStorageKey = 'unpackerr.status.column-widths.v1';
	    let lastSnapshot = { items: [] };
	    let selectedItemId = '';
	    let refreshTimer = 0;
	    let renderedColumnSignature = '';
	    let activeColumnResize = null;

	    function showLogin(message = '') {
	      window.clearTimeout(refreshTimer);
	      statusShell.hidden = true;
	      authPanel.hidden = false;
	      authError.textContent = message;
	      authPassword.focus();
	    }

	    function showStatus() {
	      authPanel.hidden = true;
	      statusShell.hidden = false;
	      authError.textContent = '';
	    }

	    async function deriveKDF(username, password) {
	      if (!window.crypto?.subtle) {
	        throw new Error('Password login requires HTTPS or localhost.');
	      }

	      const encoder = new TextEncoder();
	      const key = await window.crypto.subtle.importKey(
	        'raw', encoder.encode(password), 'PBKDF2', false, ['deriveBits']
	      );
	      const bits = await window.crypto.subtle.deriveBits({
	        name: 'PBKDF2',
	        hash: 'SHA-256',
	        salt: encoder.encode('unpackerr:' + username),
	        iterations: 210000
	      }, key, 256);

	      return Array.from(new Uint8Array(bits), (byte) => byte.toString(16).padStart(2, '0')).join('');
	    }

	    authForm.addEventListener('submit', async (event) => {
	      event.preventDefault();
	      authSubmit.disabled = true;
	      authError.textContent = '';

	      try {
	        const name = authName.value.trim() || 'admin';
	        const kdf = await deriveKDF(name, authPassword.value);
	        const response = await fetch(loginUrl, {
	          method: 'POST',
	          cache: 'no-store',
	          headers: { 'Content-Type': 'application/json' },
	          body: JSON.stringify({ name, kdf })
	        });
	        if (!response.ok) {
	          const message = response.status === 401
	            ? 'Invalid username or password.'
	            : 'Sign in failed (HTTP ' + response.status + ').';
	          throw new Error(message);
	        }

	        authPassword.value = '';
	        showStatus();
	        await refresh();
	      } catch (error) {
	        showLogin(error.message);
	      } finally {
	        authSubmit.disabled = false;
	      }
	    });

	    function escapeHtml(value) {
	      return String(value ?? '').replace(/[&<>"']/g, (char) => ({
	        '&': '&amp;',
	        '<': '&lt;',
	        '>': '&gt;',
	        '"': '&quot;',
	        "'": '&#39;'
	      })[char]);
	    }

	    function formatRemaining(deleteAt) {
	      const target = new Date(deleteAt);
	      if (Number.isNaN(target.getTime())) {
	        return '';
	      }

	      let totalSeconds = Math.max(0, Math.round((target.getTime() - Date.now()) / 1000));
	      const days = Math.floor(totalSeconds / 86400);
	      totalSeconds -= days * 86400;
	      const hours = Math.floor(totalSeconds / 3600);
	      totalSeconds -= hours * 3600;
	      const minutes = Math.floor(totalSeconds / 60);
	      const seconds = totalSeconds - (minutes * 60);

	      const parts = [];
	      if (days) parts.push(days + 'd');
	      if (hours || days) parts.push(hours + 'h');
	      if (minutes || hours || days) parts.push(minutes + 'm');
	      parts.push(seconds + 's');

	      return parts.join('');
	    }

	    function updateDeleteCountdowns() {
	      document.querySelectorAll('[data-delete-at]').forEach((node) => {
	        const formatted = formatRemaining(node.dataset.deleteAt);
	        const prefix = node.dataset.prefix || '';
	        const fallback = node.dataset.empty || '';
	        node.textContent = formatted ? prefix + formatted : fallback;
	      });
	    }

	    function renderStats(stats = {}) {
	      statGrid.innerHTML = statOrder.map(([key, label]) => (
	        '<article class="card tone-' + key + '">' +
	          '<div class="label">' + label + '</div>' +
	          '<div class="value">' + (stats[key.charAt(0).toUpperCase() + key.slice(1)] ?? 0) + '</div>' +
	        '</article>'
	      )).join('');
	    }

		    function renderMeta(data) {
		      const counters = data.counters ?? {};
		      const buffers = data.buffers ?? {};
		      const bufferSummary =
		        'fs ' + (buffers.folderEvents ?? 0) +
		        ', updates ' + ((buffers.xtractUpdates ?? 0) + (buffers.folderUpdates ?? 0)) +
		        ', hooks ' + (buffers.hooks ?? 0) +
		        ', deletes ' + (buffers.deletes ?? 0);
		      const metaItems = [
		        ['Uptime', escapeHtml(data.uptime ?? '0s')],
		        ['Finished', counters.finished ?? 0],
		        ['Retries', counters.retries ?? 0],
		        ['Webhooks', (counters.hookOK ?? 0) + ' ok / ' + (counters.hookFail ?? 0) + ' failed'],
		        ['Cmdhooks', (counters.cmdOK ?? 0) + ' ok / ' + (counters.cmdFail ?? 0) + ' failed'],
		        ['Buffers', bufferSummary]
		      ];
		      metaRail.innerHTML = metaItems.map(([label, value]) => (
		        '<span class="pill"><strong>' + escapeHtml(label) + '</strong>' +
		          escapeHtml(value) + '</span>'
		      )).join('');
		    }

		    function renderDetailField(label, value, monospace) {
		      return '<div class="detail-field">' +
		        '<div class="label">' + escapeHtml(label) + '</div>' +
		        '<div class="value' + (monospace ? ' path' : '') + '">' + escapeHtml(value) + '</div>' +
		      '</div>';
		    }

		    function renderDetailMeta(label, value) {
		      if (!value) {
		        return '';
		      }

		      return '<div class="detail-meta-item">' +
		        '<div class="detail-label">' + escapeHtml(label) + '</div>' +
		        '<div class="detail-value">' + escapeHtml(value) + '</div>' +
		      '</div>';
		    }

		    function renderDetailPath(label, value) {
		      if (!value) {
		        return '';
		      }

		      return '<div class="detail-path-row">' +
		        '<div class="detail-label">' + escapeHtml(label) + '</div>' +
		        '<div class="detail-path-value">' + escapeHtml(value) + '</div>' +
		      '</div>';
		    }

		    function renderDetailPathWide(label, value) {
		      if (!value) {
		        return '';
		      }

		      return '<div class="detail-path-row detail-path-row-wide">' +
		        '<div class="detail-label">' + escapeHtml(label) + '</div>' +
		        '<div class="detail-path-value">' + escapeHtml(value) + '</div>' +
		      '</div>';
		    }

		    function renderDetailSection(title, content, wide) {
		      if (!content) {
		        return '';
		      }

		      return '<div class="detail-section' + (wide ? ' detail-section-wide' : '') + '">' +
		        (title ? '<div class="detail-label">' + escapeHtml(title) + '</div>' : '') +
		        content +
		      '</div>';
		    }

		    function normalizePath(value) {
		      return String(value ?? '').replaceAll('\\', '/').replace(/\/+$/, '');
		    }

		    function pathBase(value) {
		      const normalized = normalizePath(value);
		      if (!normalized) {
		        return '';
		      }

		      const parts = normalized.split('/');
		      return parts[parts.length - 1] || normalized;
		    }

		    function pathDir(value) {
		      const text = String(value ?? '');
		      const index = Math.max(text.lastIndexOf('\\'), text.lastIndexOf('/'));
		      return index > -1 ? text.slice(0, index) : '';
		    }

		    function relativePath(value, root) {
		      const normalizedValue = normalizePath(value);
		      const normalizedRoot = normalizePath(root);
		      if (!normalizedValue || !normalizedRoot || !normalizedValue.startsWith(normalizedRoot + '/')) {
		        return pathBase(value) || value;
		      }

		      return normalizedValue.slice(normalizedRoot.length + 1);
		    }

		    function sameTime(first, second) {
		      const firstDate = new Date(first);
		      const secondDate = new Date(second);
		      if (Number.isNaN(firstDate.getTime()) || Number.isNaN(secondDate.getTime())) {
		        return false;
		      }

		      return Math.abs(firstDate.getTime() - secondDate.getTime()) < 1000;
		    }

		    function renderItemCell(label, content, className) {
		      return '<td data-label="' + escapeHtml(label) + '"' +
		        (className ? ' class="' + className + '"' : '') +
		        '>' + content + '</td>';
		    }

	    function renderDetailList(title, values, monospace) {
	      const renderedValues = values.map((value) => {
	        const content = monospace ? '<code>' + escapeHtml(value) + '</code>' : escapeHtml(value);
	        return '<li>' + content + '</li>';
		      }).join('');

		      return '<div class="detail-list">' +
		        '<strong>' + escapeHtml(title) + '</strong>' +
		        '<ul>' +
		          renderedValues +
		        '</ul>' +
		      '</div>';
		    }

	    function renderTaskDetails(item) {
	      if (!item) {
	        detailCard.hidden = true;
	        return;
	      }

	      const details = item.details ?? {};
		      const progress = item.progress ?? null;
		      const updatedAt = new Date(item.updatedAt).toLocaleString();
		      const showStarted = details.startedAt && !sameTime(details.startedAt, item.updatedAt);
		      const startedAt = showStarted ? new Date(details.startedAt).toLocaleString() : '';
		      const deleteAt = item.deleteAt ? new Date(item.deleteAt).toLocaleString() : '';
		      const title = details.title || item.name;
		      const location = pathDir(item.path);
		      const progressSummary = progress
		        ? (progress.percent || 0).toFixed(0) + '% - ' +
		          (progress.archiveIndex || 1) + ' of ' + (progress.archiveCount || 1) +
		          ((progress.archiveCount || 1) === 1 ? ' archive' : ' archives') +
		          ' - ' + (progress.writtenBytes || details.bytes || '0B') + ' / ' +
		          (progress.totalBytes || details.bytes || '0B')
		        : '';
		      const overviewMeta = [
		        renderDetailMeta('Status', item.statusText),
		        renderDetailMeta('Updated', updatedAt),
		        !item.completed ? renderDetailMeta('Age', item.elapsed) : '',
		        item.error ? renderDetailMeta('Error', item.error) : ''
		      ].join('');
		      const overviewPaths = [
		        renderDetailPath('Location', location),
		        item.reason ? renderDetailPath('Reason', item.reason) : '',
		        renderDetailPathWide('Source archive', pathBase(item.path)),
		        details.output ? renderDetailPathWide('Output folder', pathBase(details.output)) : '',
		        item.currentFile ? renderDetailPathWide('Current archive', pathBase(item.currentFile)) : ''
		      ].join('');
		      const overview = '<div class="detail-section-title">' + escapeHtml(title) + '</div>' +
		        '<div class="detail-meta">' + overviewMeta + '</div>' +
		        '<div class="detail-paths">' + overviewPaths + '</div>';
		      const extractionLines = [
		        progressSummary ? '<div class="detail-value">' + escapeHtml(progressSummary) + '</div>' : '',
		        progress && progress.speed ? renderDetailMeta('Speed', progress.speed) : '',
		        progress && progress.eta ? renderDetailMeta('ETA', progress.eta) : '',
		        details.elapsed ? renderDetailMeta('Elapsed', details.elapsed) : '',
		        startedAt ? renderDetailMeta('Started', startedAt) : '',
		        details.queue ? renderDetailMeta('Queue at start', details.queue) : ''
		      ].join('');
		      const extraction = extractionLines
		        ? '<div class="detail-lines">' + extractionLines + '</div>'
		        : '<div class="detail-value">No extraction details yet.</div>';
		      const cleanup = item.deleteAt
		        ? '<div class="detail-lines">' +
		            '<div class="detail-meta-item">' +
		              '<div class="detail-label">Deletes in</div>' +
		              '<div class="detail-value" data-delete-at="' + escapeHtml(item.deleteAt) +
		                '" data-empty="due now">' + escapeHtml(item.deleteIn || '') + '</div>' +
		            '</div>' +
		            renderDetailMeta('Scheduled for', deleteAt) +
		          '</div>'
		        : (item.deleteIn ? '<div class="detail-value">' + escapeHtml(item.deleteIn) + '</div>' : '');
		      const sections = [
		        renderDetailSection('Overview', overview, true),
		        renderDetailSection('Extraction', extraction, false),
		        renderDetailSection('Cleanup', cleanup, false)
		      ];

	      const lists = [];
	      const archives = details.archives ?? [];
	      const files = details.files ?? [];
	      const duplicateSingleArchive = archives.length === 1 &&
	        normalizePath(archives[0]) === normalizePath(item.path);
	      if (archives.length && !duplicateSingleArchive) {
	        const archiveNames = archives.map((archive) => relativePath(archive, location));
	        lists.push(renderDetailList('Archives (' + archives.length + ')', archiveNames, true));
	      }
	      if (files.length) {
	        const fileNames = files.map((file) => relativePath(file, details.output || location));
	        lists.push(renderDetailList('Extracted files (' + files.length + ')', fileNames, true));
	      }

	      detailGrid.innerHTML = sections.join('');
	      detailLists.innerHTML = lists.join('');
	      detailSubtitle.textContent = item.completed
	        ? 'Completed task summary.'
	        : 'Live task details and extraction context.';
		      detailCard.hidden = false;
		    }

	    function renderUpdatedCellContent(item) {
	      const updatedAt = escapeHtml(new Date(item.updatedAt).toLocaleString());
	      if (item.completed) {
	        return '<div>' + updatedAt + '</div>' +
	          '<div class="muted">Last event</div>';
	      }

	      return '<div>' + escapeHtml(item.elapsed) + '</div>' +
	        '<div class="muted">' + updatedAt + '</div>';
	    }

	    function tableColumns(showDeleteIn) {
	      const columns = [
	        { key: 'item', label: 'Item', width: showDeleteIn ? 35 : 42, min: 180 },
	        { key: 'status', label: 'Status', width: showDeleteIn ? 13 : 14, min: 110 },
	        { key: 'progress', label: 'Progress', width: showDeleteIn ? 16 : 15, min: 150 }
	      ];
	      if (showDeleteIn) {
	        columns.push({ key: 'delete', label: 'Deletes In', width: 10, min: 110 });
	      }
	      columns.push(
	        { key: 'updated', label: 'Updated', width: showDeleteIn ? 12 : 13, min: 130 },
	        { key: 'path', label: 'Path', width: showDeleteIn ? 14 : 16, min: 160 }
	      );

	      return columns;
	    }

	    function loadColumnWidths() {
	      try {
	        const widths = JSON.parse(window.localStorage.getItem(columnWidthStorageKey) || '{}');
	        return widths && typeof widths === 'object' ? widths : {};
	      } catch (_) {
	        return {};
	      }
	    }

	    function saveColumnWidths() {
	      const widths = {};
	      Array.from(itemsCols.children).forEach((column) => {
	        const header = itemsHead.querySelector('[data-column-key="' + column.dataset.columnKey + '"]');
	        if (header) {
	          widths[column.dataset.columnKey] = Math.round(header.getBoundingClientRect().width);
	        }
	      });

	      try {
	        window.localStorage.setItem(columnWidthStorageKey, JSON.stringify(widths));
	      } catch (_) {
	        // Resizing still works when browser storage is unavailable.
	      }
	    }

	    function renderTableColumns(showDeleteIn) {
	      const columns = tableColumns(showDeleteIn);
	      const signature = columns.map((column) => column.key).join('|');
	      if (signature === renderedColumnSignature) {
	        return;
	      }

	      renderedColumnSignature = signature;
	      const savedWidths = loadColumnWidths();
	      const baseWidth = Math.max(980, itemsTable.parentElement.clientWidth);
	      const hasSavedWidths = columns.some((column) => Number(savedWidths[column.key]) >= column.min);
	      let totalWidth = 0;

	      itemsCols.innerHTML = columns.map((column) => {
	        const savedWidth = Number(savedWidths[column.key]);
	        const width = savedWidth >= column.min ? savedWidth : Math.round(baseWidth * column.width / 100);
	        totalWidth += width;
	        return '<col data-column-key="' + column.key + '" style="width:' +
	          (hasSavedWidths ? width + 'px' : column.width + '%') + '">';
	      }).join('');

	      itemsHead.innerHTML = columns.map((column) => (
	        '<th data-column-key="' + column.key + '">' + escapeHtml(column.label) +
	          '<span class="column-resizer" role="separator" tabindex="0"' +
	            ' aria-label="Resize ' + escapeHtml(column.label) + ' column"' +
	            ' aria-orientation="vertical" aria-valuemin="' + column.min + '"' +
	            ' title="Drag to resize ' + escapeHtml(column.label) + ' column"></span>' +
	        '</th>'
	      )).join('');

	      itemsTable.style.width = hasSavedWidths ? Math.max(baseWidth, totalWidth) + 'px' : '';
	    }

	    function materializeColumnWidths() {
	      const headers = Array.from(itemsHead.children);
	      const columns = Array.from(itemsCols.children);
	      const widths = headers.map((header) => header.getBoundingClientRect().width);
	      widths.forEach((width, index) => {
	        columns[index].style.width = Math.round(width) + 'px';
	      });
	      itemsTable.style.width = Math.round(itemsTable.getBoundingClientRect().width) + 'px';

	      return { columns, widths };
	    }

	    function beginColumnResize(event) {
	      const handle = event.target.closest('.column-resizer');
	      if (!handle || window.matchMedia('(max-width: 1080px)').matches) {
	        return;
	      }

	      const header = handle.closest('th[data-column-key]');
	      const column = tableColumns((lastSnapshot.items ?? []).some((item) => Boolean(item.deleteIn)))
	        .find((candidate) => candidate.key === header.dataset.columnKey);
	      const index = Array.from(itemsHead.children).indexOf(header);
	      const layout = materializeColumnWidths();

	      event.preventDefault();
	      handle.classList.add('is-active');
	      document.body.classList.add('is-resizing-columns');
	      activeColumnResize = {
	        handle,
	        index,
	        startX: event.clientX,
	        startWidth: layout.widths[index],
	        startTableWidth: itemsTable.getBoundingClientRect().width,
	        minWidth: column ? column.min : 80,
	        columns: layout.columns
	      };
	    }

	    function updateColumnResize(event) {
	      if (!activeColumnResize) {
	        return;
	      }

	      const resizedWidth = Math.max(
	        activeColumnResize.minWidth,
	        activeColumnResize.startWidth + event.clientX - activeColumnResize.startX
	      );
	      const delta = resizedWidth - activeColumnResize.startWidth;
	      activeColumnResize.columns[activeColumnResize.index].style.width = Math.round(resizedWidth) + 'px';
	      itemsTable.style.width = Math.round(Math.max(
	        itemsTable.parentElement.clientWidth,
	        activeColumnResize.startTableWidth + delta
	      )) + 'px';
	      activeColumnResize.handle.setAttribute('aria-valuenow', Math.round(resizedWidth));
	    }

	    function finishColumnResize() {
	      if (!activeColumnResize) {
	        return;
	      }

	      activeColumnResize.handle.classList.remove('is-active');
	      document.body.classList.remove('is-resizing-columns');
	      activeColumnResize = null;
	      saveColumnWidths();
	    }

	    function resizeColumnWithKeyboard(event) {
	      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') {
	        return;
	      }

	      const handle = event.target.closest('.column-resizer');
	      if (!handle || window.matchMedia('(max-width: 1080px)').matches) {
	        return;
	      }

	      event.preventDefault();
	      const header = handle.closest('th[data-column-key]');
	      const definitions = tableColumns((lastSnapshot.items ?? []).some((item) => Boolean(item.deleteIn)));
	      const definition = definitions.find((column) => column.key === header.dataset.columnKey);
	      const index = Array.from(itemsHead.children).indexOf(header);
	      const layout = materializeColumnWidths();
	      const tableWidth = itemsTable.getBoundingClientRect().width;
	      const delta = event.key === 'ArrowRight' ? 12 : -12;
	      const width = Math.max(definition ? definition.min : 80, layout.widths[index] + delta);

	      layout.columns[index].style.width = Math.round(width) + 'px';
	      itemsTable.style.width = Math.round(Math.max(
	        itemsTable.parentElement.clientWidth,
	        tableWidth + width - layout.widths[index]
	      )) + 'px';
	      handle.setAttribute('aria-valuenow', Math.round(width));
	      saveColumnWidths();
	    }

	    function renderItems(data) {
	      const items = data.items ?? [];
	      const activeCount = data.activeCount ?? 0;
	      const completedCount = data.completedCount ?? 0;
	      const showDeleteIn = items.some((item) => Boolean(item.deleteIn));
	      itemCount.textContent = activeCount + ' active / ' + completedCount + ' completed';
	      clearCompletedButton.disabled = completedCount === 0;
	      renderTableColumns(showDeleteIn);

			      if (!items.length) {
			        itemsBody.innerHTML =
			          '<tr class="empty-row"><td colspan="' + (showDeleteIn ? '6' : '5') +
			          '" class="empty">Nothing is queued or being tracked right now.</td></tr>';
			        detailCard.hidden = true;
			        return;
			      }

		      itemsBody.innerHTML = items.map((item) => {
		        const notes = renderItemNotes(item);
		        const progressHtml = renderProgress(item);
		        const deleteInHtml = renderDeleteInCell(item, showDeleteIn);
		        const rowClass = selectedItemId === item.id ? ' is-selected' : '';
		        const rowId = escapeHtml(item.id);
		        const statusClass = escapeHtml(item.status);
		        const statusText = escapeHtml(item.statusText);

				        return (
				          '<tr class="items-row' + rowClass + '" data-item-id="' + rowId + '">' +
				            renderItemCell('Item', '' +
				              '<div class="item-title"><strong>' + escapeHtml(item.name) + '</strong></div>' +
			              notes, 'items-cell items-cell-item') +
				            renderItemCell(
				              'Status',
				              '<span class="status ' + statusClass + '">' + statusText + '</span>',
				              'items-cell items-cell-status'
				            ) +
				            renderItemCell('Progress', progressHtml, 'items-cell items-cell-progress') +
				            deleteInHtml +
			            renderItemCell('Updated', renderUpdatedCellContent(item), 'items-cell items-cell-updated') +
		            renderItemCell(
		              'Path',
		              '<div class="path">' + escapeHtml(pathDir(item.path) || item.path) + '</div>',
		              'items-cell items-cell-path'
		            ) +
		          '</tr>'
			        );
			      }).join('');
			    }

		    function renderProgress(item) {
		      const progress = item.progress;
		      if (!progress) {
		        return '<span class="muted">' +
		          (item.completed ? 'Click for extracted-file details' : 'No byte-level progress yet') +
		          '</span>';
		      }

		      const percentText = escapeHtml((progress.percent || 0).toFixed(0));
		      const archiveIndex = escapeHtml(progress.archiveIndex || 1);
		      const archiveCount = progress.archiveCount || 1;
		      const archiveCountText = escapeHtml(archiveCount);
		      const archiveLabel = archiveCount === 1 ? 'archive' : 'archives';
		      const width = Math.max(0, Math.min(100, progress.percent || 0));
		      const byteSummary =
		        percentText + '% - ' +
		        escapeHtml(progress.writtenBytes || '0B') + ' / ' +
		        escapeHtml(progress.totalBytes || '0B');
		      const progressStats = [
		        progress.speed ? 'Speed ' + escapeHtml(progress.speed) : '',
		        progress.eta ? 'ETA ' + escapeHtml(progress.eta) : ''
		      ].filter(Boolean).join(' | ');

		      return '<div class="progress">' +
		        '<div><strong>' + percentText + '%</strong> / ' + archiveIndex +
		          ' of ' + archiveCountText + ' ' + archiveLabel + '</div>' +
		        '<div class="progress-bar"><span style="width:' + width + '%"></span></div>' +
		        '<div class="muted">' + byteSummary + '</div>' +
		        (progressStats ? '<div class="muted">' + progressStats + '</div>' : '') +
		        (progress.archive ? '<div class="path">' + escapeHtml(progress.archive) + '</div>' : '') +
		      '</div>';
		    }

		    function renderItemNotes(item) {
		      return [
		        item.reason
		          ? '<div class="muted"><strong>Reason</strong> ' + escapeHtml(item.reason) + '</div>'
		          : '',
		        item.error
		          ? '<div class="muted"><strong>Error</strong> ' + escapeHtml(item.error) + '</div>'
		          : ''
		      ].join('');
		    }

			    function renderDeleteInCell(item, showDeleteIn) {
			      if (!showDeleteIn) {
			        return '';
			      }

			      if (item.deleteAt) {
			        return renderItemCell(
			          'Deletes In',
			          '<div data-delete-at="' + escapeHtml(item.deleteAt) +
			            '" data-empty="-">' + escapeHtml(item.deleteIn || '-') + '</div>',
			          'items-cell items-cell-delete'
			        );
			      }

			      if (item.deleteIn) {
			        return renderItemCell(
			          'Deletes In',
			          '<div>' + escapeHtml(item.deleteIn) + '</div>',
			          'items-cell items-cell-delete'
			        );
			      }

			      return renderItemCell(
			        'Deletes In',
			        '<span class="muted">-</span>',
			        'items-cell items-cell-delete'
			      );
			    }

	    function renderSnapshot(data) {
	      lastSnapshot = data ?? { items: [] };
	      renderStats(lastSnapshot.stats);
	      renderMeta(lastSnapshot);
	      renderItems(lastSnapshot);

	      if (selectedItemId) {
	        const selectedItem = (lastSnapshot.items ?? []).find((item) => item.id === selectedItemId);
	        if (selectedItem) {
	          renderTaskDetails(selectedItem);
	        } else {
	          selectedItemId = '';
	          detailCard.hidden = true;
	        }
	      }

	      updateDeleteCountdowns();
	    }

	    function hasLiveActivity(data) {
	      const items = data?.items ?? [];
	      return items.some((item) => !item.completed || Boolean(item.deleteAt));
	    }

	    function scheduleRefresh(delay) {
	      window.clearTimeout(refreshTimer);
	      refreshTimer = window.setTimeout(refresh, delay);
	    }

	    async function refresh() {
	      try {
	        const response = await fetch(statusUrl, { cache: 'no-store' });
	        if (response.status === 401) {
	          showLogin();
	          return;
	        }
	        if (!response.ok) throw new Error('HTTP ' + response.status);
	        const data = await response.json();
	        showStatus();
	        renderSnapshot(data);
	        const generated = data.generatedAt ? new Date(data.generatedAt).toLocaleString() : 'just now';
	        stamp.textContent = 'Updated ' + generated;
	        scheduleRefresh(hasLiveActivity(data) ? activeRefreshMs : idleRefreshMs);
	      } catch (error) {
	        stamp.textContent = 'Status unavailable: ' + error.message;
	        scheduleRefresh(errorRefreshMs);
	      }
	    }

	    detailClose.addEventListener('click', () => {
	      selectedItemId = '';
	      detailCard.hidden = true;
	      renderItems(lastSnapshot);
	    });

	    clearCompletedButton.addEventListener('click', async () => {
	      try {
	        const response = await fetch(clearCompletedUrl, { method: 'POST', cache: 'no-store' });
	        if (!response.ok) throw new Error('HTTP ' + response.status);
	        const data = await response.json();
	        if (selectedItemId && !(data.items ?? []).some((item) => item.id === selectedItemId)) {
	          selectedItemId = '';
	          detailCard.hidden = true;
	        }
	        renderSnapshot(data);
	        stamp.textContent = 'Cleared completed items';
	        scheduleRefresh(hasLiveActivity(data) ? activeRefreshMs : idleRefreshMs);
	      } catch (error) {
	        stamp.textContent = 'Unable to clear completed items: ' + error.message;
	      }
	    });

	    resetColumnsButton.addEventListener('click', () => {
	      try {
	        window.localStorage.removeItem(columnWidthStorageKey);
	      } catch (_) {
	        // Reset the current layout even when browser storage is unavailable.
	      }
	      renderedColumnSignature = '';
	      renderTableColumns((lastSnapshot.items ?? []).some((item) => Boolean(item.deleteIn)));
	    });

	    itemsHead.addEventListener('pointerdown', beginColumnResize);
	    itemsHead.addEventListener('keydown', resizeColumnWithKeyboard);
	    document.addEventListener('pointermove', updateColumnResize);
	    document.addEventListener('pointerup', finishColumnResize);
	    document.addEventListener('pointercancel', finishColumnResize);

	    itemsBody.addEventListener('click', (event) => {
	      const row = event.target.closest('tr[data-item-id]');
	      if (!row) {
	        return;
	      }

	      const item = (lastSnapshot.items ?? []).find((candidate) => candidate.id === row.dataset.itemId);
	      if (!item) {
	        return;
	      }

	      if (selectedItemId === item.id) {
	        selectedItemId = '';
	        detailCard.hidden = true;
	        renderItems(lastSnapshot);
	        return;
	      }

	      selectedItemId = item.id;
	      renderItems(lastSnapshot);
	      renderTaskDetails(item);
	    });

	    refresh();
	    setInterval(updateDeleteCountdowns, 1000);
	  </script>
</body>
</html>`

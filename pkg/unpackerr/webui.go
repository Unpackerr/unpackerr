package unpackerr

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/version"
	"golift.io/xtractr"
)

//go:embed webui.html
var statusPageHTML string

var statusPageParsed = sync.OnceValue(func() *template.Template {
	return template.Must(template.New("status").Parse(statusPageHTML))
})

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
	maxRetainedWebCompleted = 100
)

func (u *Unpackerr) refreshWebState(now time.Time) {
	u.webState.Store(u.buildWebState(now))
}

func (u *Unpackerr) buildWebState(now time.Time) *webStatusSnapshot {
	prev := u.webState.Load()
	stats := u.currentStats()
	dismissed := webStatusDismissedItems(prev)
	items, currentKeys, currentIDs := u.buildTrackedWebItems(now, dismissed)

	items = u.buildWaitingFolderWebItems(items, currentKeys, now)
	items = mergeRetainedCompletedWebItems(items, prev, currentKeys, dismissed, now)
	sortWebStatusItems(items)
	items = capCompletedWebItems(items, maxRetainedWebCompleted)
	dismissed = pruneWebStatusDismissed(dismissed, items, currentIDs)

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
) ([]webStatusItem, map[string]struct{}, map[string]struct{}) {
	items := make([]webStatusItem, 0, len(u.Map))
	currentKeys := make(map[string]struct{}, len(u.Map))
	currentIDs := make(map[string]struct{}, len(u.Map))

	for name, item := range u.Map {
		var folderItem *Folder
		if u.folders != nil {
			folderItem = u.folders.Folders[name]
		}

		webItem := buildWebStatusItem(name, item, folderItem, now)

		currentKeys[webItem.Key] = struct{}{}
		currentIDs[webItem.ID] = struct{}{}
		if !webItem.Completed {
			delete(dismissed, webItem.ID)
		}

		if _, skip := dismissed[webItem.ID]; skip && webItem.Completed {
			continue
		}

		items = append(items, webItem)
	}

	return items, currentKeys, currentIDs
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

		if _, skip := dismissed[item.ID]; skip {
			continue
		}

		item.Elapsed = webStatusElapsed(item.UpdatedAt, now)
		retainedKeys[item.Key] = struct{}{}
		items = append(items, item)
	}

	return items
}

func capCompletedWebItems(items []webStatusItem, maxCompleted int) []webStatusItem {
	if maxCompleted <= 0 {
		return items
	}

	completed := 0
	for _, item := range items {
		if item.Completed {
			completed++
		}
	}

	if completed <= maxCompleted {
		return items
	}

	return items[:len(items)-(completed-maxCompleted)]
}

func pruneWebStatusDismissed(
	dismissed map[string]struct{}, items []webStatusItem, currentIDs map[string]struct{},
) map[string]struct{} {
	if len(dismissed) == 0 {
		return dismissed
	}

	shown := make(map[string]struct{}, len(items))
	for _, item := range items {
		shown[item.ID] = struct{}{}
	}

	kept := make(map[string]struct{})
	for id := range dismissed {
		if _, live := currentIDs[id]; !live {
			continue
		}

		if _, visible := shown[id]; visible {
			continue
		}

		kept[id] = struct{}{}
	}

	return kept
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

func (u *Unpackerr) currentStats() *Stats {
	stats := &Stats{}
	u.fillQueueStats(stats)

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
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Frame-Options", "DENY")
	writer.Header().Set("Referrer-Policy", "no-referrer")

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

func (u *Unpackerr) webClearCompletedAPI(writer http.ResponseWriter, request *http.Request) {
	var snapshot *webStatusSnapshot

	err := u.onMainLoop(request.Context(), func() error {
		snapshot = u.clearCompletedWebItems(time.Now())
		return nil
	})
	if err != nil {
		writeQueueActionError(writer, err)
		return
	}

	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	encodeWebStatusJSON(writer, snapshot, u.Errorf)
}

func (u *Unpackerr) clearCompletedWebItems(now time.Time) *webStatusSnapshot {
	prev := u.webState.Load()
	if prev == nil {
		u.refreshWebState(now)
		return u.webState.Load()
	}

	dismissed := webStatusDismissedItems(prev)
	for _, item := range prev.Items {
		if item.Completed {
			dismissed[item.ID] = struct{}{}
		}
	}

	held := *prev
	held.dismissed = dismissed
	u.webState.Store(&held)
	u.refreshWebState(now)

	return u.webState.Load()
}

func encodeWebStatusJSON(writer http.ResponseWriter, payload any, logFunc func(string, ...any)) {
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		logFunc("encoding web status payload: %v", err)
	}
}

func statusPageTemplate() *template.Template {
	return statusPageParsed()
}

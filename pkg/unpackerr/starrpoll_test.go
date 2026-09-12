package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/starr/lidarr"
	"golift.io/starr/radarr"
	"golift.io/starr/readarr"
	"golift.io/starr/sonarr"
	"golift.io/xtractr"
)

func TestQueueViewsIDs(t *testing.T) {
	t.Parallel()

	son := &SonarrConfig{Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
		Title: "Show", DownloadID: "d1", SeriesID: 10, EpisodeID: 20,
	}}}}
	rad := &RadarrConfig{Queue: &radarr.Queue{Records: []*radarr.QueueRecord{{
		Title: "Movie", DownloadID: "d2", MovieID: 30,
	}}}}
	lid := &LidarrConfig{Queue: &lidarr.Queue{Records: []*lidarr.QueueRecord{{
		Title: "Album", DownloadID: "d3", ArtistID: 40, AlbumID: 50,
	}}}}
	rea := &ReadarrConfig{Queue: &readarr.Queue{Records: []*readarr.QueueRecord{{
		Title: "Book", DownloadID: "d4", AuthorID: 60, BookID: 70,
	}}}}

	if got := son.queueViews()[0].IDs["seriesId"]; got != int64(10) {
		t.Fatalf("sonarr seriesId: got %v", got)
	}

	if got := rad.queueViews()[0].IDs["movieId"]; got != int64(30) {
		t.Fatalf("radarr movieId: got %v", got)
	}

	if got := lid.queueViews()[0].IDs["artistId"]; got != int64(40) {
		t.Fatalf("lidarr artistId: got %v", got)
	}

	if got := rea.queueViews()[0].IDs["bookId"]; got != int64(70) {
		t.Fatalf("readarr bookId: got %v", got)
	}

	if haveStarrQitem([]*SonarrConfig{son}, "Show") != true {
		t.Fatal("expected haveStarrQitem true for Show")
	}

	if haveStarrQitem([]*SonarrConfig{son}, "Nope") {
		t.Fatal("expected haveStarrQitem false for Nope")
	}
}

func TestCheckStarrQueueLidarrTweak(t *testing.T) {
	t.Parallel()

	const (
		title      = "Album"
		outputPath = `/lidarr/host/Album`
	)

	mappedRoot := t.TempDir()
	mappedPath := filepath.Join(mappedRoot, title)

	if err := os.Mkdir(mappedPath, 0o750); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.Lidarr = []*LidarrConfig{{
		Protocols: defaultProtocol,
		Paths:     StringSlice{mappedRoot},
		SplitFlac: true,
		Queue: &lidarr.Queue{Records: []*lidarr.QueueRecord{{
			Title:      title,
			Status:     "completed",
			Protocol:   starr.Protocol("torrent"),
			OutputPath: outputPath,
		}}},
	}}

	checkStarrQueue(unpack, unpack.Lidarr, starr.Lidarr, time.Now())

	item, ok := unpack.Map[title]
	if !ok {
		t.Fatal("expected Lidarr queue item in extract map")
	}

	if !item.SplitFlac {
		t.Fatal("expected SplitFlac from Lidarr tweakExtract")
	}

	if item.OutputPath != outputPath {
		t.Fatalf("OutputPath: got %q want original Starr path %q", item.OutputPath, outputPath)
	}

	if item.Path == outputPath {
		t.Fatal("Path should be the host-mapped download dir, not the Starr OutputPath")
	}

	if item.Path != mappedPath {
		t.Fatalf("Path: got %q want mapped %q", item.Path, mappedPath)
	}
}

func TestCheckStarrQueueSetsName(t *testing.T) {
	t.Parallel()

	const title = "Show"

	mappedRoot := t.TempDir()
	mappedPath := filepath.Join(mappedRoot, title)

	if err := os.Mkdir(mappedPath, 0o750); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{
		Name:      "Sportarr",
		Protocols: defaultProtocol,
		Paths:     StringSlice{mappedRoot},
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title:    title,
			Status:   "completed",
			Protocol: starr.Protocol("torrent"),
		}}},
	}}

	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, time.Now())

	item, ok := unpack.Map[title]
	if !ok {
		t.Fatal("expected Sonarr queue item in extract map")
	}

	if item.App != starr.Sonarr {
		t.Fatalf("App: got %q want %s", item.App, starr.Sonarr)
	}

	if item.Name != "Sportarr" {
		t.Fatalf("Name: got %q want Sportarr", item.Name)
	}

	if item.Label() != "Sportarr" {
		t.Fatalf("Label: got %q want Sportarr", item.Label())
	}

	unpack.Sonarr[0].Name = "Fightarr"
	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, time.Now())

	if unpack.Map[title].Name != "Fightarr" {
		t.Fatalf("Name after rename: got %q want Fightarr", unpack.Map[title].Name)
	}
}

func TestCheckStarrQueueKeepsForeignName(t *testing.T) {
	t.Parallel()

	const title = "Show"

	unpack := New()
	unpack.Map[title] = &Extract{
		App:  starr.Radarr,
		Name: "Movies",
		URL:  "http://radarr:7878",
	}
	unpack.Sonarr = []*SonarrConfig{{
		Name:      "Sportarr",
		URL:       "http://sonarr:8989",
		Protocols: defaultProtocol,
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title:    title,
			Status:   "downloading",
			Protocol: starr.Protocol("torrent"),
		}}},
	}}

	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, time.Now())

	if unpack.Map[title].Name != "Movies" {
		t.Fatalf("Name: got %q want Movies", unpack.Map[title].Name)
	}
}

func TestStarrConfigLabel(t *testing.T) {
	t.Parallel()

	if got := (*StarrConfig)(nil).Label(starr.Sonarr); got != string(starr.Sonarr) {
		t.Fatalf("nil Label: %q", got)
	}

	cfg := &StarrConfig{Name: "  Sportarr "}
	if got := cfg.Label(starr.Sonarr); got != "Sportarr" {
		t.Fatalf("named Label: %q", got)
	}
}

func TestTallyQueueViews(t *testing.T) {
	t.Parallel()

	got := tallyQueueViews([]queueView{
		{Status: "completed", Protocol: "torrent"},
		{Status: "Completed", Protocol: "usenet", TrackedStatus: "error"},
		{Status: "failed"},
		{Status: "downloading"},
		{Status: "paused", TrackedState: "downloadFailed"},
		{Status: "queued"},
		{Status: "warning"},
		{Status: "completed", TrackedStatus: "warning"},
	}, "torrent")
	if got.complete != 3 || got.match != 1 || got.issues != 5 || got.downloading != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestStarrQueueStatsCountsRecords(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{
		Name: "Sportarr", Protocols: "torrent", lastQueued: 6, lastRetrieved: 4,
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{
			{Status: "completed", Protocol: "torrent"},
			{Status: "completed", Protocol: "usenet", TrackedDownloadStatus: "error"},
			{Status: "failed"},
			{Status: "downloading"},
			{Status: "warning"},
		}},
	}}

	stats := &Stats{}
	unpack.fillQueueStats(stats)

	got := stats.StarrQueues[0]
	if got.Queued != 6 || got.Retrieved != 4 || got.Complete != 2 || got.Match != 1 ||
		got.Issues != 3 || got.Downloading != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestFillQueueStatsConfigCounts(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{}, {}}
	unpack.Radarr = []*RadarrConfig{{}}
	unpack.Folders = []*FolderConfig{{Path: "/watch"}, {Path: "/other"}}
	unpack.Finished = 9
	unpack.Map["live"] = &Extract{Status: IMPORTED, Updated: time.Now()}
	unpack.Map["out"] = &Extract{Status: EXTRACTED, Updated: time.Now()}

	stats := &Stats{}
	unpack.fillQueueStats(stats)

	if stats.Starrs != 3 {
		t.Fatalf("starrs %d", stats.Starrs)
	}

	if stats.Folders != 2 {
		t.Fatalf("folders %d", stats.Folders)
	}

	unpack.Webhook = []*WebhookConfig{{}}

	stats = &Stats{}
	unpack.fillQueueStats(stats)

	if stats.Webhooks != 1 || stats.Cmdhooks != 0 {
		t.Fatalf("hooks configured webhook:%d cmd:%d", stats.Webhooks, stats.Cmdhooks)
	}

	if stats.Imported != 1 {
		t.Fatalf("live imported %d", stats.Imported)
	}

	if stats.Extracted != 1 || stats.Finished != 9 {
		t.Fatalf("extracted %d finished %d", stats.Extracted, stats.Finished)
	}

	if len(stats.StarrQueues) != 3 {
		t.Fatalf("starr queues %d", len(stats.StarrQueues))
	}
}

func TestFillStackDepthsSplitsChannels(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.folders.Events = make(chan *eventData, 8)

	unpack.folders.Updates = make(chan *xtractr.Response, 8)
	unpack.folders.Events <- &eventData{}

	unpack.folders.Events <- &eventData{}

	unpack.folders.Updates <- &xtractr.Response{}

	unpack.updates <- &xtractr.Response{}

	unpack.delChan <- &fileDeleteReq{}

	unpack.taskChan <- nil

	stats := &Stats{}
	unpack.fillStackDepths(stats)

	if stats.StackFS != (BufferStat{Len: 2, Cap: 8}) ||
		stats.StackFolder != (BufferStat{Len: 1, Cap: 8}) ||
		stats.StackXtractr != (BufferStat{Len: 1, Cap: updateChanBuf}) ||
		stats.StackDel != (BufferStat{Len: 1, Cap: updateChanBuf}) ||
		stats.StackTask != (BufferStat{Len: 1, Cap: updateChanBuf}) ||
		stats.StackHook != (BufferStat{Len: 0, Cap: updateChanBuf}) {
		t.Fatalf("%+v", stats)
	}

	if stats.stackTotal() != 6 {
		t.Fatalf("stack total %d", stats.stackTotal())
	}
}

func TestStarrQueueStatsShowsPollCounts(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Sonarr = []*SonarrConfig{{}}
	unpack.Sonarr[0].Name = "Sportarr"
	unpack.Sonarr[0].URL = "http://127.0.0.1:8989"
	unpack.Sonarr[0].lastQueued = 12
	unpack.Sonarr[0].lastRetrieved = 8
	unpack.Sonarr[0].lastPollErr = "timeout"

	stats := &Stats{}
	unpack.fillQueueStats(stats)

	if len(stats.StarrQueues) != 1 {
		t.Fatalf("starr queues %d", len(stats.StarrQueues))
	}

	got := stats.StarrQueues[0]
	if got.Name != "Sportarr" || got.Queued != 12 || got.Retrieved != 8 || got.Error != "timeout" {
		t.Fatalf("%+v", got)
	}
}

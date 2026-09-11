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

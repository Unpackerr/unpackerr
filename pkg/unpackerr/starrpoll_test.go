package unpackerr

import (
	"testing"

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

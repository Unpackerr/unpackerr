package starr

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

// RadarHistory is the /api/history endpoint.
type RadarHistory struct {
	Page          int            `json:"page"`
	PageSize      int            `json:"pageSize"`
	SortKey       string         `json:"sortKey"`
	SortDirection string         `json:"sortDirection"`
	TotalRecords  int64          `json:"totalRecords"`
	Records       []*RadarRecord `json:"Records"`
}

// RadarRecord is a record in Radarr History
type RadarRecord struct {
	EpisodeID   int64  `json:"episodeId"`
	MovieID     int64  `json:"movieId"`
	SeriesID    int64  `json:"seriesId"`
	SourceTitle string `json:"sourceTitle"`
	Quality     struct {
		Quality struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"quality"`
		Revision struct {
			Version int64 `json:"version"`
			Real    int64 `json:"real"`
		} `json:"revision"`
	} `json:"quality"`
	QualityCutoffNotMet bool      `json:"qualityCutoffNotMet"`
	Date                time.Time `json:"date"`
	DownloadID          string    `json:"downloadId"`
	EventType           string    `json:"eventType"`
	Data                struct {
		Indexer         string    `json:"indexer"`
		NzbInfoURL      string    `json:"nzbInfoUrl"`
		ReleaseGroup    string    `json:"releaseGroup"`
		Age             string    `json:"age"`
		AgeHours        string    `json:"ageHours"`
		AgeMinutes      string    `json:"ageMinutes"`
		PublishedDate   time.Time `json:"publishedDate"`
		DownloadClient  string    `json:"downloadClient"`
		Size            string    `json:"size"`
		DownloadURL     string    `json:"downloadUrl"`
		GUID            string    `json:"guid"`
		TvdbID          string    `json:"tvdbId"`
		TvRageID        string    `json:"tvRageId"`
		Protocol        string    `json:"protocol"`
		TorrentInfoHash []string  `json:"torrentInfoHash"`
	} `json:"data"`
	Movie struct {
		Title      string    `json:"title"`
		SortTitle  string    `json:"sortTitle"`
		SizeOnDisk int64     `json:"sizeOnDisk"`
		Status     string    `json:"status"`
		Overview   string    `json:"overview"`
		InCinemas  time.Time `json:"inCinemas"`
		Images     []struct {
			CoverType string `json:"coverType"`
			URL       string `json:"url"`
		} `json:"images"`
		Website          string    `json:"website"`
		Downloaded       bool      `json:"downloaded"`
		Year             int       `json:"year"`
		HasFile          bool      `json:"hasFile"`
		YouTubeTrailerID string    `json:"youTubeTrailerId"`
		Studio           string    `json:"studio"`
		Path             string    `json:"path"`
		ProfileID        int       `json:"profileId"`
		Monitored        bool      `json:"monitored"`
		Runtime          int       `json:"runtime"`
		LastInfoSync     time.Time `json:"lastInfoSync"`
		CleanTitle       string    `json:"cleanTitle"`
		ImdbID           string    `json:"imdbId"`
		TmdbID           int64     `json:"tmdbId"`
		TitleSlug        string    `json:"titleSlug"`
		Genres           []string  `json:"genres"`
		Tags             []string  `json:"tags"`
		Added            time.Time `json:"added"`
		Ratings          struct {
			Votes int64   `json:"votes"`
			Value float64 `json:"value"`
		} `json:"ratings"`
		AlternativeTitles []string `json:"alternativeTitles"`
		QualityProfileID  int      `json:"qualityProfileId"`
		ID                int64    `json:"id"`
	} `json:"movie"`
	ID int `json:"id"`
}

// RadarQueue is the /api/queue endpoint.
type RadarQueue struct {
	Movie struct {
		Title             string `json:"title"`
		AlternativeTitles []struct {
			SourceType string `json:"sourceType"`
			MovieID    int64  `json:"movieId"`
			Title      string `json:"title"`
			SourceID   int64  `json:"sourceId"`
			Votes      int64  `json:"votes"`
			VoteCount  int64  `json:"voteCount"`
			Language   string `json:"language"`
			ID         int64  `json:"id"`
		} `json:"alternativeTitles"`
		SecondaryYearSourceID int       `json:"secondaryYearSourceId"`
		SortTitle             string    `json:"sortTitle"`
		SizeOnDisk            int64     `json:"sizeOnDisk"`
		Status                string    `json:"status"`
		Overview              string    `json:"overview"`
		InCinemas             time.Time `json:"inCinemas"`
		PhysicalRelease       time.Time `json:"physicalRelease"`
		Images                []struct {
			CoverType string `json:"coverType"`
			URL       string `json:"url"`
		} `json:"images"`
		Website             string    `json:"website"`
		Downloaded          bool      `json:"downloaded"`
		Year                int       `json:"year"`
		HasFile             bool      `json:"hasFile"`
		YouTubeTrailerID    string    `json:"youTubeTrailerId"`
		Studio              string    `json:"studio"`
		Path                string    `json:"path"`
		ProfileID           int       `json:"profileId"`
		PathState           string    `json:"pathState"`
		Monitored           bool      `json:"monitored"`
		MinimumAvailability string    `json:"minimumAvailability"`
		IsAvailable         bool      `json:"isAvailable"`
		FolderName          string    `json:"folderName"`
		Runtime             int       `json:"runtime"`
		LastInfoSync        time.Time `json:"lastInfoSync"`
		CleanTitle          string    `json:"cleanTitle"`
		ImdbID              string    `json:"imdbId"`
		TmdbID              int64     `json:"tmdbId"`
		TitleSlug           string    `json:"titleSlug"`
		Genres              []string  `json:"genres"`
		Tags                []string  `json:"tags"`
		Added               time.Time `json:"added"`
		Ratings             struct {
			Votes int64   `json:"votes"`
			Value float64 `json:"value"`
		} `json:"ratings"`
		QualityProfileID int64 `json:"qualityProfileId"`
		ID               int64 `json:"id"`
	} `json:"movie"`
	Quality struct {
		Quality struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"quality"`
		Revision struct {
			Version int64 `json:"version"`
			Real    int64 `json:"real"`
		} `json:"revision"`
	} `json:"quality"`
	Size                    float64   `json:"size"`
	Title                   string    `json:"title"`
	Sizeleft                float64   `json:"sizeleft"`
	Timeleft                string    `json:"timeleft"`
	EstimatedCompletionTime time.Time `json:"estimatedCompletionTime"`
	Status                  string    `json:"status"`
	TrackedDownloadStatus   string    `json:"trackedDownloadStatus"`
	StatusMessages          []struct {
		Title    string   `json:"title"`
		Messages []string `json:"messages"`
	} `json:"statusMessages"`
	DownloadID string `json:"downloadId"`
	Protocol   string `json:"protocol"`
	ID         int64  `json:"id"`
}

// RadarrHistory returns the Radarr History (grabs/failures/completed)
func RadarrHistory(c Config) ([]*RadarRecord, error) {
	var h *RadarHistory
	if params, err := url.ParseQuery("sortKey=date&sortDir=asc&page=1&pageSize=0"); err != nil {
		return nil, errors.Wrap(err, "url.ParseQuery")
	} else if rawJSON, err := c.Req("history", params); err != nil {
		return nil, errors.Wrap(err, "c.Req(queue)")
	} else if err = json.Unmarshal(rawJSON, &h); err != nil {
		return nil, errors.Wrap(err, "json.Unmarshal(response)")
	}
	// This isn't used, but it's included because I wasted my time playing with it.
	return h.Records, nil
}

// RadarrQueue returns the Radarr Queue
func RadarrQueue(c Config) ([]*RadarQueue, error) {
	var h []*RadarQueue
	if params, err := url.ParseQuery("sort_by=timeleft&order=asc"); err != nil {
		return nil, errors.Wrap(err, "url.ParseQuery")
	} else if rawJSON, err := c.Req("queue", params); err != nil {
		return nil, errors.Wrap(err, "c.Req(queue)")
	} else if err = json.Unmarshal(rawJSON, &h); err != nil {
		return nil, errors.Wrap(err, "json.Unmarshal(response)")
	}
	return h, nil
}

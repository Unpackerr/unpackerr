package starr

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

// SonarQueue is the /api/queue endpoint.
type SonarQueue struct {
	Series struct {
		Title       string `json:"title"`
		SortTitle   string `json:"sortTitle"`
		SeasonCount int    `json:"seasonCount"`
		Status      string `json:"status"`
		Overview    string `json:"overview"`
		Network     string `json:"network"`
		AirTime     string `json:"airTime"`
		Images      []struct {
			CoverType string `json:"coverType"`
			URL       string `json:"url"`
		} `json:"images"`
		Seasons []struct {
			SeasonNumber int  `json:"seasonNumber"`
			Monitored    bool `json:"monitored"`
		} `json:"seasons"`
		Year              int       `json:"year"`
		Path              string    `json:"path"`
		ProfileID         int       `json:"profileId"`
		SeasonFolder      bool      `json:"seasonFolder"`
		Monitored         bool      `json:"monitored"`
		UseSceneNumbering bool      `json:"useSceneNumbering"`
		Runtime           int       `json:"runtime"`
		TvdbID            int64     `json:"tvdbId"`
		TvRageID          int64     `json:"tvRageId"`
		TvMazeID          int64     `json:"tvMazeId"`
		FirstAired        time.Time `json:"firstAired"`
		LastInfoSync      time.Time `json:"lastInfoSync"`
		SeriesType        string    `json:"seriesType"`
		CleanTitle        string    `json:"cleanTitle"`
		ImdbID            string    `json:"imdbId"`
		TitleSlug         string    `json:"titleSlug"`
		Certification     string    `json:"certification"`
		Genres            []string  `json:"genres"`
		Tags              []string  `json:"tags"`
		Added             time.Time `json:"added"`
		Ratings           struct {
			Votes int64   `json:"votes"`
			Value float64 `json:"value"`
		} `json:"ratings"`
		QualityProfileID int `json:"qualityProfileId"`
		ID               int `json:"id"`
	} `json:"series"`
	Episode struct {
		SeriesID                 int       `json:"seriesId"`
		EpisodeFileID            int       `json:"episodeFileId"`
		SeasonNumber             int       `json:"seasonNumber"`
		EpisodeNumber            int       `json:"episodeNumber"`
		Title                    string    `json:"title"`
		AirDate                  string    `json:"airDate"`
		AirDateUtc               time.Time `json:"airDateUtc"`
		Overview                 string    `json:"overview"`
		HasFile                  bool      `json:"hasFile"`
		Monitored                bool      `json:"monitored"`
		AbsoluteEpisodeNumber    int       `json:"absoluteEpisodeNumber"`
		UnverifiedSceneNumbering bool      `json:"unverifiedSceneNumbering"`
		ID                       int64     `json:"id"`
	} `json:"episode"`
	Quality struct {
		Quality struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"quality"`
		Revision struct {
			Version int `json:"version"`
			Real    int `json:"real"`
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

// SonarrQueue returns the Sonarr Queue
func SonarrQueue(c Config) ([]*SonarQueue, error) {
	var q []*SonarQueue
	if params, err := url.ParseQuery("sort_by=timeleft&order=asc"); err != nil {
		return q, errors.Wrap(err, "url.ParseQuery")
	} else if rawJSON, err := c.Req("queue", params); err != nil {
		return q, errors.Wrap(err, "c.Req(queue)")
	} else if err = json.Unmarshal(rawJSON, &q); err != nil {
		return q, errors.Wrap(err, "json.Unmarshal(response)")
	}
	return q, nil
}

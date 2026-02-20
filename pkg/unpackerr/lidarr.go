package unpackerr

import (
	"errors"
	"strings"
	"time"

	"golift.io/starr"
	"golift.io/starr/lidarr"
)

// LidarrConfig represents the input data for a Lidarr server.
type LidarrConfig struct {
	StarrConfig
	SplitFlac      bool          `json:"split_flac" toml:"split_flac" xml:"split_flac" yaml:"split_flac"`
	Queue          *lidarr.Queue `json:"-"          toml:"-"          xml:"-"          yaml:"-"`
	*lidarr.Lidarr `json:"-"          toml:"-"          xml:"-"          yaml:"-"`
}

func (u *Unpackerr) validateLidarr() error {
	tmp := u.Lidarr[:0]

	for idx := range u.Lidarr {
		if err := u.validateApp(&u.Lidarr[idx].StarrConfig, starr.Lidarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		u.Lidarr[idx].Lidarr = lidarr.New(&u.Lidarr[idx].Config)
		tmp = append(tmp, u.Lidarr[idx])
	}

	u.Lidarr = tmp

	return nil
}

func (u *Unpackerr) logLidarr() {
	if count := len(u.Lidarr); count == 1 {
		u.Printf(" => Lidarr Config: 1 server: "+starrLogLine+", split_flac:%v",
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout.String(),
			u.Lidarr[0].ValidSSL, u.Lidarr[0].Protocols, u.Lidarr[0].Syncthing,
			u.Lidarr[0].DeleteOrig, u.Lidarr[0].DeleteDelay.String(), u.Lidarr[0].Paths,
			u.Lidarr[0].SplitFlac)
	} else {
		u.Printf(" => Lidarr Config: %d servers", count)

		for _, f := range u.Lidarr {
			u.Printf(starrLogPfx+starrLogLine+", split_flac:%v",
				f.URL, f.APIKey != "", f.Timeout.String(), f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.String(), f.Paths,
				f.SplitFlac)
		}
	}
}

// getLidarrQueue saves the Lidarr Queue(s).
func (u *Unpackerr) getLidarrQueue(server *LidarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Lidarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, DefaultQueuePageSize)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Lidarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Lidarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Lidarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkLidarrQueue saves completed Lidarr-queued downloads to u.Map.
func (u *Unpackerr) checkLidarrQueue(now time.Time) {
	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Lidarr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Map[record.Title] = &Extract{
					App:         starr.Lidarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					SplitFlac:   server.SplitFlac,
					Path:        u.getDownloadPath(record.OutputPath, starr.Lidarr, record.Title, server.Paths),
					IDs: map[string]any{
						"title":      record.Title,
						"artistId":   record.ArtistID,
						"albumId":    record.AlbumID,
						"downloadId": record.DownloadID,
						"reason":     buildStatusReason(record.Status, record.StatusMessages),
					},
				}
				u.Map[record.Title].XProg = &ExtractProgress{Extract: u.Map[record.Title]}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Lidarr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveLidarrQitem(name string) bool {
	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			if record.Title == name {
				return true
			}
		}
	}

	return false
}

// lidarrServerByURL returns the Lidarr server config that matches the given URL, or nil.
func (u *Unpackerr) lidarrServerByURL(url string) *LidarrConfig {
	for _, server := range u.Lidarr {
		if server.URL == url {
			return server
		}
	}

	return nil
}

// extractionHasFlacFiles returns true if any path in files has a .flac extension.
// Used to only trigger manual import after a FLAC+CUE split, not for e.g. zip-of-mp3s.
func extractionHasFlacFiles(files []string) bool {
	for _, p := range files {
		if strings.HasSuffix(strings.ToLower(p), ".flac") {
			return true
		}
	}

	return false
}

// importSplitFlacTracks runs in a goroutine after a Lidarr FLAC+CUE split extraction completes.
// It asks Lidarr for the manual import list for the extract folder and sends the ManualImport command
// so Lidarr imports the split track files.
func (u *Unpackerr) importSplitFlacTracks(item *Extract, server *LidarrConfig) {
	if server == nil {
		u.Printf("[Lidarr] No Lidarr server found for manual import, this might be a bug: %s", item.Path)
		return
	}

	downloadID, _ := item.IDs["downloadId"].(string)
	artistID, _ := item.IDs["artistId"].(int64)

	params := &lidarr.ManualImportParams{
		Folder:               item.Path,
		DownloadID:           downloadID,
		ArtistID:             artistID,
		FilterExistingFiles:  false,
		ReplaceExistingFiles: true,
	}

	outputs, err := server.ManualImport(params)
	if err != nil {
		u.Errorf("[Lidarr] Manual import list failed for %s: %v", item.Path, err)
		return
	}

	if len(outputs) == 0 {
		u.Printf("[Lidarr] No files returned for manual import (folder: %s); import manually in Lidarr", item.Path)
		return
	}

	cmd := lidarr.ManualImportCommandFromOutputs(outputs, true)
	if cmd == nil {
		u.Printf("[Lidarr] No importable files for manual import: %s", item.Path)
		return
	}

	_, err = server.SendManualImportCommand(cmd)
	if err != nil {
		u.Errorf("[Lidarr] Manual import command failed for %s: %v", item.Path, err)
		return
	}

	u.Printf("[Lidarr] Manual import triggered for %d files: %s", len(cmd.Files), item.Path)
}

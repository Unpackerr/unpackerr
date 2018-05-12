package deluge

import (
	"encoding/json"

	"github.com/davidnewhall/unpacker-poller/exp"
)

// Deluge methods.
const (
	AuthLogin      = "auth.login"
	AddMagnet      = "core.add_torrent_magnet"
	AddTorrentURL  = "core.add_torrent_url"
	AddTorrentFile = "core.add_torrent_file"
	GetTorrentStat = "core.get_torrent_status"
	GetAllTorrents = "core.get_torrents_status"
)

// Config is the data needed to poll Deluge.
type Config struct {
	URL      string  `json:"url" toml:"url" xml:"url" yaml:"url"`
	Password string  `json:"password" toml:"password" xml:"password" yaml:"password"`
	HTTPPass string  `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string  `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Timeout  exp.Dur `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
}

// Response from Deluge
type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// XferStatus represents a transfer in Deluge.
type XferStatus struct {
	Comment             string  `json:"comment"`
	ActiveTime          int64   `json:"active_time"`
	IsSeed              bool    `json:"is_seed"`
	Hash                string  `json:"hash"`
	UploadPayloadRate   int64   `json:"upload_payload_rate"`
	MoveCompletedPath   string  `json:"move_completed_path"`
	Private             bool    `json:"private"`
	TotalPayloadUpload  int64   `json:"total_payload_upload"`
	Paused              bool    `json:"paused"`
	SeedRank            int64   `json:"seed_rank"`
	SeedingTime         int64   `json:"seeding_time"`
	MaxUploadSlots      int64   `json:"max_upload_slots"`
	PrioritizeFirstLast bool    `json:"prioritize_first_last"`
	DistributedCopies   float64 `json:"distributed_copies"`
	DownloadPayloadRate int64   `json:"download_payload_rate"`
	Message             string  `json:"message"`
	NumPeers            int64   `json:"num_peers"`
	MaxDownloadSpeed    int64   `json:"max_download_speed"`
	MaxConnections      int64   `json:"max_connections"`
	Compact             bool    `json:"compact"`
	Ratio               float64 `json:"ratio"`
	TotalPeers          int64   `json:"total_peers"`
	TotalSize           int64   `json:"total_size"`
	TotalWanted         int64   `json:"total_wanted"`
	State               string  `json:"state"`
	FilePriorities      []int   `json:"file_priorities"`
	Label               string  `json:"label"`
	MaxUploadSpeed      int64   `json:"max_upload_speed"`
	RemoveAtRatio       bool    `json:"remove_at_ratio"`
	Tracker             string  `json:"tracker"`
	SavePath            string  `json:"save_path"`
	Progress            float64 `json:"progress"`
	TimeAdded           float64 `json:"time_added"`
	TrackerHost         string  `json:"tracker_host"`
	TotalUploaded       int64   `json:"total_uploaded"`
	Files               []struct {
		Index  int64  `json:"index"`
		Path   string `json:"path"`
		Offset int64  `json:"offset"`
		Size   int64  `json:"size"`
	} `json:"files"`
	TotalDone           int64         `json:"total_done"`
	NumPieces           int64         `json:"num_pieces"`
	TrackerStatus       string        `json:"tracker_status"`
	TotalSeeds          int64         `json:"total_seeds"`
	MoveOnCompleted     bool          `json:"move_on_completed"`
	NextAnnounce        int64         `json:"next_announce"`
	StopAtRatio         bool          `json:"stop_at_ratio"`
	FileProgress        []float64     `json:"file_progress"`
	MoveCompleted       bool          `json:"move_completed"`
	PieceLength         int64         `json:"piece_length"`
	AllTimeDownload     int64         `json:"all_time_download"`
	MoveOnCompletedPath string        `json:"move_on_completed_path"`
	NumSeeds            int64         `json:"num_seeds"`
	Peers               []interface{} `json:"peers"`
	Name                string        `json:"name"`
	Trackers            []struct {
		SendStats    bool        `json:"send_stats"`
		Fails        int64       `json:"fails"`
		Verified     bool        `json:"verified"`
		MinAnnounce  interface{} `json:"min_announce"`
		URL          string      `json:"url"`
		FailLimit    int64       `json:"fail_limit"`
		NextAnnounce interface{} `json:"next_announce"`
		CompleteSent bool        `json:"complete_sent"`
		Source       int64       `json:"source"`
		StartSent    bool        `json:"start_sent"`
		Tier         int64       `json:"tier"`
		Updating     bool        `json:"updating"`
	} `json:"trackers"`
	TotalPayloadDownload int64       `json:"total_payload_download"`
	IsAutoManaged        bool        `json:"is_auto_managed"`
	SeedsPeersRatio      float64     `json:"seeds_peers_ratio"`
	Queue                int64       `json:"queue"`
	NumFiles             int64       `json:"num_files"`
	Eta                  json.Number `json:"eta"`
	StopRatio            float64     `json:"stop_ratio"`
	IsFinished           bool        `json:"is_finished"`
}

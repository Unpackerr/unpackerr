package unpackerr

import (
	"github.com/Unpackerr/unpackerr/pkg/folders"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/starr/lidarr"
	"golift.io/starr/radarr"
	"golift.io/starr/readarr"
	"golift.io/starr/sonarr"
)

// starrApp is what putStarrList, clone/carry, and the poll helpers need from each
// Starr config type. P is the pointer type (*SonarrConfig), T the struct.
type starrApp[T any] interface {
	*T
	conf() *StarrConfig
	connect()         // build the API client from conf.
	takeQueue(old *T) // keep the last polled queue from a matching old entry.
	stripRuntime()    // nil the queue and client on a file-shaped clone.
	// pollQueue fetches without publishing. The returned bind assigns Queue and
	// must run under History.mu with lastQueued/lastRetrieved/lastPollErr.
	pollQueue() (bind func(), total, retrieved int, err error)
	queueViews() []queueView
	hasQueueTitle(name string) bool
	tweakExtract(item *Extract, rec queueView)
	logExtra() string
}

func (s *SonarrConfig) conf() *StarrConfig { return &s.StarrConfig }
func (s *SonarrConfig) connect()           { s.Sonarr = sonarr.New(&s.Config) }
func (s *SonarrConfig) takeQueue(o *SonarrConfig) {
	s.Queue = o.Queue
	s.takePoll(&o.StarrConfig)
}
func (s *SonarrConfig) stripRuntime() { s.Queue, s.Sonarr = nil, nil }

func (r *RadarrConfig) conf() *StarrConfig { return &r.StarrConfig }
func (r *RadarrConfig) connect()           { r.Radarr = radarr.New(&r.Config) }
func (r *RadarrConfig) takeQueue(o *RadarrConfig) {
	r.Queue = o.Queue
	r.takePoll(&o.StarrConfig)
}
func (r *RadarrConfig) stripRuntime() { r.Queue, r.Radarr = nil, nil }

func (l *LidarrConfig) conf() *StarrConfig { return &l.StarrConfig }
func (l *LidarrConfig) connect()           { l.Lidarr = lidarr.New(&l.Config) }
func (l *LidarrConfig) takeQueue(o *LidarrConfig) {
	l.Queue = o.Queue
	l.takePoll(&o.StarrConfig)
}
func (l *LidarrConfig) stripRuntime() { l.Queue, l.Lidarr = nil, nil }

func (r *ReadarrConfig) conf() *StarrConfig { return &r.StarrConfig }
func (r *ReadarrConfig) connect()           { r.Readarr = readarr.New(&r.Config) }
func (r *ReadarrConfig) takeQueue(o *ReadarrConfig) {
	r.Queue = o.Queue
	r.takePoll(&o.StarrConfig)
}
func (r *ReadarrConfig) stripRuntime() { r.Queue, r.Readarr = nil, nil }

func cloneConfig(src *Config) *Config {
	dst := *src
	dst.Passwords = append(StringSlice(nil), src.Passwords...)
	dst.Webserver = cloneWebserver(src.Webserver)
	dst.Lidarr = cloneStarrList(src.Lidarr)
	dst.Radarr = cloneStarrList(src.Radarr)
	dst.Readarr = cloneStarrList(src.Readarr)
	dst.Sonarr = cloneStarrList(src.Sonarr)
	dst.Folders = cloneFolderList(src.Folders)
	dst.Webhook = hooks.CloneList(src.Webhook)
	dst.Cmdhook = hooks.CloneList(src.Cmdhook)

	return &dst
}

func cloneWebserver(src *WebServer) *WebServer {
	if src == nil {
		return &WebServer{}
	}

	dst := *src
	dst.Upstreams = append(StringSlice(nil), src.Upstreams...)
	dst.APIKeys = cloneAPIKeys(src.APIKeys)
	dst.Roles = cloneRoles(src.Roles)
	dst.router = nil
	dst.server = nil
	dst.keyPerms = nil
	dst.cookies = nil

	return &dst
}

func cloneAPIKeys(src []APIKey) []APIKey {
	if src == nil {
		return nil
	}

	out := make([]APIKey, len(src))
	for idx, key := range src {
		out[idx] = APIKey{
			Name:  key.Name,
			Key:   key.Key,
			Roles: append([]string(nil), key.Roles...),
		}
	}

	return out
}

func cloneRoles(src map[string]Role) map[string]Role {
	if src == nil {
		return nil
	}

	out := make(map[string]Role, len(src))
	for name, role := range src {
		out[name] = Role{Permissions: append([]string(nil), role.Permissions...)}
	}

	return out
}

// cloneStarrList copies a Starr list into its file shape: config only,
// no queue, no client. Nil in, nil out so the TOML writer omits the table.
func cloneStarrList[T any, P starrApp[T]](src []P) []P {
	if src == nil {
		return nil
	}

	out := make([]P, len(src))

	for idx, app := range src {
		cloned := *app
		out[idx] = &cloned

		out[idx].conf().Paths = append(StringSlice(nil), app.conf().Paths...)
		out[idx].stripRuntime()
	}

	return out
}

func cloneFolderList(src []*FolderConfig) []*FolderConfig {
	return folders.CloneList(src)
}

// cloneHookList copies hooks without the mutex, counters, client, or template.
func cloneHookList(src []*WebhookConfig) []*WebhookConfig {
	return hooks.CloneList(src)
}

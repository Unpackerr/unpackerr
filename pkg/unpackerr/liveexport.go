package unpackerr

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"golift.io/starr"
	"golift.io/version"
)

func exportLinef(buf *strings.Builder, format string, v ...any) {
	fmt.Fprintf(buf, format+"\n", v...)
}

// liveConfigText is the same rundown as the startup log: what's actually running,
// with secrets omitted (API keys as present/absent, UI password as auth type).
func (u *Unpackerr) liveConfigText() string {
	var buf strings.Builder

	exportLinef(&buf, "==> %s <==", helpLink)
	exportLinef(&buf, "==> Live Settings <==")
	exportLinef(&buf, " => Version: v%s-%s (%s/%s)", version.Version, version.Revision, runtime.GOOS, runtime.GOARCH)

	if strings.TrimSpace(u.ConfigFile) != "" {
		exportLinef(&buf, " => Using Config File: %s", u.ConfigFileWithAge())
	} else {
		exportLinef(&buf, " => Using env variables only. Config file not found.")
	}

	u.appendStarrExport(&buf, starr.Sonarr, u.Sonarr)
	u.appendStarrExport(&buf, starr.Radarr, u.Radarr)
	u.appendLidarrExport(&buf)
	u.appendStarrExport(&buf, starr.Readarr, u.Readarr)
	u.appendStarrExport(&buf, starr.Whisparr, u.Whisparr)
	u.appendFoldersExport(&buf)

	exportLinef(&buf, " => Parallel: %d", u.Parallel)
	exportLinef(&buf, " => Default Extract Limits: Sonarr/Whisparr %s, Radarr %s, Lidarr %s, Readarr %s; "+
		"%d files, %g:1, %d nested, extras depth %d; folders uncapped",
		defaultSonarrMaxBytes, defaultRadarrMaxBytes, defaultLidarrMaxBytes, defaultReadarrMaxBytes,
		defaultMaxFiles, defaultMaxRatio, defaultMaxNested, defaultExtrasMaxDepth)
	exportLinef(&buf, " => Passwords: %d (rar/7z)", len(u.Passwords))
	exportLinef(&buf, " => Interval / Progress: %s/%s", u.Interval.String(), u.Progress.String())
	exportLinef(&buf, " => Start/Delete Delay: %s/%s", u.StartDelay.String(), u.DeleteDelay.String())
	exportLinef(&buf, " => Retry Delay: %v, max: %d", u.RetryDelay, u.maxRetries())
	exportLinef(&buf, " => Remnant Action: %s", u.RemnantAction)
	exportLinef(&buf, " => GUI / StdErr: %v / %v", ui.HasGUI(), u.ErrorStdErr)
	exportLinef(&buf, " => Debug / Quiet: %v / %v", u.Config.Debug, u.Quiet)
	exportLinef(&buf, " => Activity / Queues: %v / %s", u.Activity, u.LogQueues.String())

	if runtime.GOOS != windows {
		exportLinef(&buf, " => Directory & File Modes: %s & %s", u.DirMode, u.FileMode)
	}

	if u.LogFile != "" {
		msg := "no rotation"
		if u.LogFiles > 0 {
			msg = fmt.Sprintf("%d @ %dMb", u.LogFiles, u.LogFileMb)
		}

		exportLinef(&buf, " => Log File: %s (%s, mode: %s)", u.LogFile, msg, u.LogFileMode)
	}

	u.appendWebhookExport(&buf)
	u.appendCmdhookExport(&buf)
	u.appendWebserverExport(&buf)

	return buf.String()
}

type starrExporter interface {
	conf() *StarrConfig
}

func (u *Unpackerr) appendStarrExport[T any, P interface {
	*T
	starrExporter
}](buf *strings.Builder, app starr.App, list []P) {
	count := len(list)
	if count == 1 {
		c := list[0].conf()
		exportLinef(buf, " => %s Config: 1 server: "+starrLogLine,
			app, c.URL, c.APIKey != "", c.Timeout.String(),
			c.ValidSSL, c.Protocols, c.Syncthing,
			c.DeleteOrig, c.DeleteDelay.String(), c.Paths)

		return
	}

	exportLinef(buf, " => %s Config: %d servers", app, count)

	for _, item := range list {
		c := item.conf()
		exportLinef(buf, starrLogPfx+starrLogLine,
			c.URL, c.APIKey != "", c.Timeout.String(), c.ValidSSL, c.Protocols,
			c.Syncthing, c.DeleteOrig, c.DeleteDelay.String(), c.Paths)
	}
}

func (u *Unpackerr) appendLidarrExport(buf *strings.Builder) {
	count := len(u.Lidarr)
	if count == 1 {
		c := u.Lidarr[0]
		exportLinef(buf, " => Lidarr Config: 1 server: "+starrLogLine+", split_flac:%v",
			c.URL, c.APIKey != "", c.Timeout.String(),
			c.ValidSSL, c.Protocols, c.Syncthing,
			c.DeleteOrig, c.DeleteDelay.String(), c.Paths, c.SplitFlac)

		return
	}

	exportLinef(buf, " => Lidarr Config: %d servers", count)

	for _, c := range u.Lidarr {
		exportLinef(buf, starrLogPfx+starrLogLine+", split_flac:%v",
			c.URL, c.APIKey != "", c.Timeout.String(), c.ValidSSL, c.Protocols,
			c.Syncthing, c.DeleteOrig, c.DeleteDelay.String(), c.Paths, c.SplitFlac)
	}
}

func (u *Unpackerr) appendFoldersExport(buf *strings.Builder) {
	count := len(u.Folders)
	if count == 1 {
		folder := u.Folders[0]

		epath := ""
		if folder.ExtractPath != "" {
			epath = ", extract to: " + folder.ExtractPath
		}

		exportLinef(buf, " => Folder Config: 1 path: %s%s; delete_after:%v delete_orig:%v delete_files:%v "+
			"log_file:%v move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v event_buffer:%d",
			folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
			!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
			folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks, u.Folder.Buffer)

		return
	}

	exportLinef(buf, " => Folder Config: %d paths, event_buffer:%d ", count, u.Folder.Buffer)

	for _, folder := range u.Folders {
		epath := ""
		if folder.ExtractPath != "" {
			epath = " extract to: " + folder.ExtractPath
		}

		exportLinef(buf, " =>    Path: %s%s; delete_after:%v delete_orig:%v delete_files:%v log_file:%v "+
			"move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v",
			folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
			!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
			folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks)
	}
}

func (u *Unpackerr) appendWebhookExport(buf *strings.Builder) {
	if len(u.Webhook) != 1 {
		exportLinef(buf, " => Webhook Configs: %d URLs", len(u.Webhook))
	}

	prefix := " =>    URL"
	if len(u.Webhook) == 1 {
		prefix = " => Webhook Config: 1 URL"
	}

	for _, hook := range u.Webhook {
		vars := ""
		if hook.TmplPath != "" {
			vars = ", template: " + hook.TmplPath + ", content_type: " + hook.CType
		}

		if hook.Channel != "" {
			vars += ", channel: " + hook.Channel
		}

		if hook.Nickname != "" {
			vars += ", nickname: " + hook.Nickname
		}

		if len(hook.Exclude) > 0 {
			vars += ", exclude: \"" + strings.Join(hook.Exclude, "; ") + `"`
		}

		exportLinef(buf, "%s: %s, timeout: %v, ignore ssl: %v, silent: %v%s, events: %q",
			prefix, hook.Name, hook.Timeout, hook.IgnoreSSL, hook.Silent, vars, logEvents(hook.Events))
	}
}

func (u *Unpackerr) appendCmdhookExport(buf *strings.Builder) {
	prefix := " =>    Command"
	if len(u.Cmdhook) == 1 {
		prefix = " => Command Hook Config: 1 cmd"
	} else {
		exportLinef(buf, " => Command Hook Configs: %d commands", len(u.Cmdhook))
	}

	for _, hook := range u.Cmdhook {
		exportLinef(buf, "%s: %s, timeout: %v, silent: %v, events: %v, shell: %v, cmd: %s",
			prefix, hook.Name, hook.Timeout, hook.Silent, logEvents(hook.Events), hook.Shell, hook.Command)
	}
}

func (u *Unpackerr) appendWebserverExport(buf *strings.Builder) {
	if u.Webserver == nil || !u.Webserver.Enabled() {
		exportLinef(buf, " => Webserver Disabled")
		return
	}

	ssl := ""
	if u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "" {
		ssl = "s"
	}

	exportLinef(buf, " => Starting webserver. Listen address: http%s://%v%s (%d upstreams) auth:%s",
		ssl, u.Webserver.bindAddr(), u.Webserver.URLBase, len(u.Webserver.Upstreams), u.uiPassword().Type())

	if u.Webserver.Metrics {
		exportLinef(buf, " => Prometheus metrics enabled at %s (API key required)",
			path.Join(u.Webserver.URLBase, "metrics"))
	}
}

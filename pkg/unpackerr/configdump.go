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

// configLine is Printf-shaped so the startup log and live text export share one dump.
type configLine func(format string, v ...any)

// dumpAuth gates live GET /api/system/export sections. Startup logs leave gated false.
type dumpAuth struct {
	gated bool
	info  authInfo
}

func (d dumpAuth) omit(printf configLine, section ConfigSection, label string) bool {
	if !d.gated || d.info.allows(PermReadConfig(section)) {
		return false
	}

	printf(" => %s: omitted (need %s)", label, PermReadConfig(section))

	return true
}

// liveConfigText is the same rundown as the startup log: what's actually running,
// with secrets omitted (API keys as present/absent, UI password as auth type).
// Per-section details require config:{section}:read (or *); missing sections say so.
func (u *Unpackerr) liveConfigText(info authInfo) string {
	var buf strings.Builder

	printf := func(format string, v ...any) {
		fmt.Fprintf(&buf, format+"\n", v...)
	}

	printf("==> %s <==", helpLink)
	printf("==> Live Settings <==")
	printf(" => Version: v%s-%s (%s/%s)", version.Version, version.Revision, runtime.GOOS, runtime.GOARCH)

	if strings.TrimSpace(u.ConfigFile) != "" {
		printf(" => Using Config File: %s", u.ConfigFileWithAge())
	} else {
		printf(" => Using env variables only. Config file not found.")
	}

	u.writeRunningConfig(printf, dumpAuth{gated: true, info: info})

	return buf.String()
}

// writeRunningConfig prints the shared body of the startup / live settings dump.
// It does not mutate config; callers that need a normalized URL base do that first.
func (u *Unpackerr) writeRunningConfig(printf configLine, auth dumpAuth) {
	if !auth.omit(printf, SectionSonarr, "Sonarr Config") {
		logStarr(printf, starr.Sonarr, u.Sonarr)
	}

	if !auth.omit(printf, SectionRadarr, "Radarr Config") {
		logStarr(printf, starr.Radarr, u.Radarr)
	}

	if !auth.omit(printf, SectionLidarr, "Lidarr Config") {
		logStarr(printf, starr.Lidarr, u.Lidarr)
	}

	if !auth.omit(printf, SectionReadarr, "Readarr Config") {
		logStarr(printf, starr.Readarr, u.Readarr)
	}

	if !auth.omit(printf, SectionWhisparr, "Whisparr Config") {
		logStarr(printf, starr.Whisparr, u.Whisparr)
	}

	if !auth.omit(printf, SectionFolders, "Folder Config") {
		u.logFolders(printf)
	}

	if !auth.omit(printf, SectionGeneral, "General Config") {
		u.logGeneral(printf)
	}

	if !auth.omit(printf, SectionWebhooks, "Webhook Config") {
		u.logWebhook(printf)
	}

	if !auth.omit(printf, SectionCmdhooks, "Command Hook Config") {
		u.logCmdhook(printf)
	}

	if !auth.omit(printf, SectionWebserver, "Webserver Config") {
		u.logWebserver(printf)
	}
}

func (u *Unpackerr) logGeneral(printf configLine) {
	printf(" => Parallel: %d", u.Parallel)
	printf(" => Default Extract Limits: Sonarr/Whisparr %s, Radarr %s, Lidarr %s, Readarr %s; "+
		"%d files, %g:1, %d nested, extras depth %d; folders uncapped",
		defaultSonarrMaxBytes, defaultRadarrMaxBytes, defaultLidarrMaxBytes, defaultReadarrMaxBytes,
		defaultMaxFiles, defaultMaxRatio, defaultMaxNested, defaultExtrasMaxDepth)
	printf(" => Passwords: %d (rar/7z)", len(u.Passwords))
	printf(" => Interval / Progress: %s/%s", u.Interval.String(), u.Progress.String())
	printf(" => Start/Delete Delay: %s/%s", u.StartDelay.String(), u.DeleteDelay.String())
	printf(" => Retry Delay: %v, max: %d", u.RetryDelay, u.maxRetries())
	printf(" => Remnant Action: %s", u.RemnantAction)
	printf(" => GUI / StdErr: %v / %v", ui.HasGUI(), u.ErrorStdErr)
	printf(" => Debug / Quiet: %v / %v", u.Config.Debug, u.Quiet)
	printf(" => Activity / Queues: %v / %s", u.Activity, u.LogQueues.String())

	if runtime.GOOS != windows {
		printf(" => Directory & File Modes: %s & %s", u.DirMode, u.FileMode)
	}

	if u.LogFile != "" {
		msg := "no rotation"
		if u.LogFiles > 0 {
			msg = fmt.Sprintf("%d @ %dMb", u.LogFiles, u.LogFileMb)
		}

		printf(" => Log File: %s (%s, mode: %s)", u.LogFile, msg, u.LogFileMode)
	}
}

func logStarr[T any, P starrApp[T]](printf configLine, app starr.App, list []P) {
	count := len(list)
	if count == 1 {
		item := list[0]
		c := item.conf()
		printf(" => %s Config: 1 server: "+starrLogLine+"%s",
			app, c.URL, c.APIKey != "", c.Timeout.String(),
			c.ValidSSL, c.Protocols, c.Syncthing,
			c.DeleteOrig, c.DeleteDelay.String(), c.Paths, item.logExtra())

		return
	}

	printf(" => %s Config: %d servers", app, count)

	for _, item := range list {
		c := item.conf()
		printf(starrLogPfx+starrLogLine+"%s",
			c.URL, c.APIKey != "", c.Timeout.String(), c.ValidSSL, c.Protocols,
			c.Syncthing, c.DeleteOrig, c.DeleteDelay.String(), c.Paths, item.logExtra())
	}
}

func (u *Unpackerr) logFolders(printf configLine) {
	if epath, count := "", len(u.Folders); count == 1 {
		folder := u.Folders[0]
		if folder.ExtractPath != "" {
			epath = ", extract to: " + folder.ExtractPath
		}

		printf(" => Folder Config: 1 path: %s%s; delete_after:%v delete_orig:%v delete_files:%v "+
			"log_file:%v move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v event_buffer:%d",
			folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
			!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
			folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks, u.Folder.Buffer)
	} else {
		printf(" => Folder Config: %d paths, event_buffer:%d ", count, u.Folder.Buffer)

		for _, folder := range u.Folders {
			if epath = ""; folder.ExtractPath != "" {
				epath = " extract to: " + folder.ExtractPath
			}

			printf(" =>    Path: %s%s; delete_after:%v delete_orig:%v delete_files:%v log_file:%v "+
				"move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v",
				folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
				!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
				folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks)
		}
	}
}

func (u *Unpackerr) logWebhook(printf configLine) {
	var vars, prefix string

	if len(u.Webhook) == 1 {
		prefix = " => Webhook Config: 1 URL"
	} else {
		printf(" => Webhook Configs: %d URLs", len(u.Webhook))
		prefix = " =>    URL" //nolint:wsl_v5
	}

	for _, hook := range u.Webhook {
		if vars = ""; hook.TmplPath != "" {
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

		printf("%s: %s, timeout: %v, ignore ssl: %v, silent: %v%s, events: %q",
			prefix, hook.Name, hook.Timeout, hook.IgnoreSSL, hook.Silent, vars, logEvents(hook.Events))
	}
}

func (u *Unpackerr) logCmdhook(printf configLine) {
	var prefix string

	if len(u.Cmdhook) == 1 {
		prefix = " => Command Hook Config: 1 cmd"
	} else {
		printf(" => Command Hook Configs: %d commands", len(u.Cmdhook))
		prefix = " =>    Command" //nolint:wsl_v5
	}

	for _, hook := range u.Cmdhook {
		printf("%s: %s, timeout: %v, silent: %v, events: %v, shell: %v, cmd: %s",
			prefix, hook.Name, hook.Timeout, hook.Silent, logEvents(hook.Events), hook.Shell, hook.Command)
	}
}

func (u *Unpackerr) logWebserver(printf configLine) {
	if u.Webserver == nil || !u.Webserver.Enabled() {
		printf(" => Webserver Disabled")
		return
	}

	ssl := ""
	if u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "" {
		ssl = "s"
	}

	printf(" => Starting webserver. Listen address: http%s://%v%s (%d upstreams) auth:%s",
		ssl, u.Webserver.bindAddr(), u.Webserver.URLBase, len(u.Webserver.Upstreams), u.uiPassword().Type())

	if u.Webserver.Metrics {
		printf(" => Prometheus metrics enabled at %s (API key required)",
			path.Join(u.Webserver.URLBase, "metrics"))
	}
}

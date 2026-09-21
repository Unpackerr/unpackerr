package hooks

import (
	"net/url"
	"strings"
)

// Built-in webhook template names. Unknown URLs resolve to notifiarr.
const (
	ProfileNotifiarr  = "notifiarr"
	ProfileDiscord    = "discord"
	ProfileTelegram   = "telegram"
	ProfileSlack      = "slack"
	ProfilePushover   = "pushover"
	ProfileGotify     = "gotify"
	ProfileNtfy       = "ntfy"
	ProfileApprise    = "apprise"
	ProfileMattermost = "mattermost"
	ProfileCustom     = "custom"
)

// Profile is the resolved webhook template and whether the destination can edit a prior message.
type Profile struct {
	Name      string
	CanUpdate bool
}

// Detect picks a built-in template from a named template, custom file, or URL.
// Named builtins win. template_path wins when the name is empty or unknown.
// Everything else sniffs the URL; unknown URLs use the Notifiarr JSON payload.
func Detect(name, rawURL, tmplPath string) Profile {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "default" {
		name = ProfileNotifiarr
	}

	if profile, ok := namedProfile(name); ok {
		return profile
	}

	if strings.TrimSpace(tmplPath) != "" {
		return Profile{Name: ProfileCustom}
	}

	return sniffURL(rawURL)
}

// DetectTransport picks headers and content-type from the named template or URL.
// A custom template file still uses the destination (ntfy Bearer, Pushover form).
func DetectTransport(name, rawURL string) Profile {
	return Detect(name, rawURL, "")
}

func namedProfile(name string) (Profile, bool) {
	switch name {
	case ProfileNotifiarr:
		return Profile{Name: ProfileNotifiarr}, true
	case ProfileDiscord:
		return Profile{Name: ProfileDiscord, CanUpdate: true}, true
	case ProfileTelegram:
		return Profile{Name: ProfileTelegram, CanUpdate: true}, true
	case ProfileSlack:
		return Profile{Name: ProfileSlack}, true
	case ProfilePushover:
		return Profile{Name: ProfilePushover}, true
	case ProfileGotify:
		return Profile{Name: ProfileGotify}, true
	case ProfileNtfy:
		return Profile{Name: ProfileNtfy}, true
	case ProfileApprise:
		return Profile{Name: ProfileApprise}, true
	case ProfileMattermost:
		return Profile{Name: ProfileMattermost}, true
	default:
		return Profile{}, false
	}
}

func sniffURL(raw string) Profile {
	lower := strings.ToLower(raw)
	host, path := hookHostPath(raw)

	switch {
	case strings.Contains(lower, "discordnotifier.com"), strings.Contains(lower, "notifiarr.com"):
		return Profile{Name: ProfileNotifiarr}
	case strings.Contains(lower, "discord.com"), strings.Contains(lower, "discordapp.com"):
		return Profile{Name: ProfileDiscord, CanUpdate: true}
	case strings.Contains(lower, "api.telegram.org"):
		return Profile{Name: ProfileTelegram, CanUpdate: true}
	case strings.Contains(lower, "hooks.slack.com"):
		return Profile{Name: ProfileSlack}
	case strings.Contains(lower, "pushover.net"):
		return Profile{Name: ProfilePushover}
	case strings.Contains(lower, "gotify"):
		return Profile{Name: ProfileGotify}
	case host == "ntfy.sh" || strings.Contains(host, "ntfy"):
		return Profile{Name: ProfileNtfy}
	case strings.Contains(host, "apprise") || strings.Contains(path, "/notify"):
		return Profile{Name: ProfileApprise}
	case strings.Contains(path, "/hooks/"):
		return Profile{Name: ProfileMattermost}
	default:
		return Profile{Name: ProfileNotifiarr}
	}
}

func hookHostPath(raw string) (string, string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}

	return strings.ToLower(parsed.Hostname()), strings.ToLower(parsed.Path)
}

package hooks

import (
	"testing"
)

func TestDetectNamedAndURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, url, path, want string
		update                bool
	}{
		{name: "discord", url: "https://notifiarr.com/api", want: ProfileDiscord, update: true},
		{name: "default", url: "https://example.com", want: ProfileNotifiarr},
		{name: "nope", path: "/tmp/custom.tmpl", want: ProfileCustom},
		{url: "https://notifiarr.com/api/v1/notification/unpackerr", want: ProfileNotifiarr},
		{url: "https://discordnotifier.com/api", want: ProfileNotifiarr},
		{url: "https://discord.com/api/webhooks/1/token", want: ProfileDiscord, update: true},
		{url: "https://discordapp.com/api/webhooks/1/token", want: ProfileDiscord, update: true},
		{url: "https://api.telegram.org/bot123:abc/sendMessage", want: ProfileTelegram, update: true},
		{url: "https://hooks.slack.com/services/T/B/X", want: ProfileSlack},
		{url: "https://api.pushover.net/1/messages.json", want: ProfilePushover},
		{url: "https://gotify.home.lan/message", want: ProfileGotify},
		{url: "https://ntfy.sh/unpackerr", want: ProfileNtfy},
		{url: "https://ntfy.home.lan/media", want: ProfileNtfy},
		{url: "https://apprise.home.lan/notify/key", want: ProfileApprise},
		{url: "http://192.168.1.10:8000/notify/", want: ProfileApprise},
		{url: "https://mattermost.home.lan/hooks/abc", want: ProfileMattermost},
		{url: "https://example.com/hooks/incoming", want: ProfileMattermost},
		{url: "https://hooks.slack.com/services/T/B/X", want: ProfileSlack},
		{url: "https://requestbin.com/r/abc", want: ProfileNotifiarr},
	}

	for _, test := range tests {
		t.Run(test.want+":"+test.url+test.name+test.path, func(t *testing.T) {
			t.Parallel()

			got := Detect(test.name, test.url, test.path)
			if got.Name != test.want {
				t.Fatalf("name %q", got.Name)
			}

			if got.CanUpdate != test.update {
				t.Fatalf("CanUpdate %v want %v", got.CanUpdate, test.update)
			}
		})
	}
}

func TestDetectNamedBeatsTemplatePath(t *testing.T) {
	t.Parallel()

	got := Detect("telegram", "https://discord.com/api/webhooks/1/x", "/tmp/custom.tmpl")
	if got.Name != ProfileTelegram || !got.CanUpdate {
		t.Fatalf("%+v", got)
	}
}

func TestDetectTransportIgnoresTemplatePath(t *testing.T) {
	t.Parallel()

	if got := Detect("", "https://ntfy.sh/unpackerr", "/tmp/custom.tmpl"); got.Name != ProfileCustom {
		t.Fatalf("body %q", got.Name)
	}

	if got := DetectTransport("", "https://ntfy.sh/unpackerr"); got.Name != ProfileNtfy {
		t.Fatalf("transport %q", got.Name)
	}

	if got := DetectTransport("discord", "https://ntfy.sh/unpackerr"); got.Name != ProfileDiscord {
		t.Fatalf("named %q", got.Name)
	}
}

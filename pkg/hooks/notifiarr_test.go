package hooks

import (
	"strings"
	"testing"
	"time"
)

func TestRewriteNotifiarrURL(t *testing.T) {
	t.Parallel()

	const key = "00000000-0000-4000-8000-000000000000"

	tests := []struct {
		in, url, key string
	}{
		{
			in:  "https://notifiarr.com/api/v1/notification/unpackerr/" + key,
			url: "https://notifiarr.com/api/v1/notification/unpackerr",
			key: key,
		},
		{
			in:  "https://notifiarr.com/api/v1/notification/unpackerr/" + key + "/",
			url: "https://notifiarr.com/api/v1/notification/unpackerr",
			key: key,
		},
		{
			in:  "https://www.notifiarr.com/api/v1/notification/unpackerr/" + key,
			url: "https://www.notifiarr.com/api/v1/notification/unpackerr",
			key: key,
		},
		{
			in:  "https://notifiarr.com/api/v1/notification/unpackerr/" + key + "?foo=1",
			url: "https://notifiarr.com/api/v1/notification/unpackerr?foo=1",
			key: key,
		},
		{
			in:  "https://notifiarr.com/api/v1/notification/unpackerr/",
			url: "https://notifiarr.com/api/v1/notification/unpackerr/",
		},
		{
			in:  "https://notifiarr.com/api/v1/notification/unpackerr/" + key + "/extra",
			url: "https://notifiarr.com/api/v1/notification/unpackerr/" + key + "/extra",
		},
		{
			in:  "https://discord.com/api/webhooks/1/" + key,
			url: "https://discord.com/api/webhooks/1/" + key,
		},
		{
			in:  "https://evilnotifiarr.com/api/v1/notification/unpackerr/" + key,
			url: "https://evilnotifiarr.com/api/v1/notification/unpackerr/" + key,
		},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			t.Parallel()

			url, key := rewriteNotifiarrURL(test.in)
			if url != test.url || key != test.key {
				t.Fatalf("got %q %q want %q %q", url, key, test.url, test.key)
			}
		})
	}
}

func TestNormalizeNotifiarr(t *testing.T) {
	t.Parallel()

	const key = "00000000-0000-4000-8000-000000000000"

	hook := &Config{
		URL: "https://notifiarr.com/api/v1/notification/unpackerr/" + key,
	}
	if err := ValidateWebhooks([]*Config{hook}, time.Second); err != nil {
		t.Fatal(err)
	}

	if hook.URL != "https://notifiarr.com/api/v1/notification/unpackerr" {
		t.Fatalf("url %q", hook.URL)
	}

	if strings.Contains(hook.Name, key) {
		t.Fatalf("name leaked key %q", hook.Name)
	}

	if got := headerValue(hook.Headers, HeaderAPIKey); got != key {
		t.Fatalf("X-Api-Key %q", got)
	}
}

func TestNormalizeNotifiarrKeepsExistingHeader(t *testing.T) {
	t.Parallel()

	hook := &Config{
		URL: "https://notifiarr.com/api/v1/notification/unpackerr/00000000-0000-4000-8000-000000000000",
		Headers: map[string]string{
			"x-api-key": "already",
		},
	}
	NormalizeNotifiarr(hook)

	if hook.URL != "https://notifiarr.com/api/v1/notification/unpackerr" {
		t.Fatalf("url %q", hook.URL)
	}

	if got := headerValue(hook.Headers, HeaderAPIKey); got != "already" {
		t.Fatalf("X-Api-Key %q", got)
	}

	if _, ok := hook.Headers["x-api-key"]; !ok {
		t.Fatalf("headers %+v", hook.Headers)
	}
}

func TestNormalizeNotifiarrNil(t *testing.T) {
	t.Parallel()

	NormalizeNotifiarr(nil)
}

package hooks

import (
	"net/url"
	"regexp"
	"strings"
)

const notifiarrUnpackerrPath = "/api/v1/notification/unpackerr"

var notifiarrAPIKey = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeNotifiarr strips a Notifiarr client API key off the webhook path
// and stores it as X-Api-Key. Existing header values win.
func NormalizeNotifiarr(hook *Config) {
	if hook == nil {
		return
	}

	next, key := rewriteNotifiarrURL(hook.URL)
	hook.URL = next

	if key == "" || strings.TrimSpace(headerValue(hook.Headers, HeaderAPIKey)) != "" {
		return
	}

	hook.Headers = setHeader(hook.Headers, HeaderAPIKey, key)
}

func rewriteNotifiarrURL(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)

	parsed, err := url.Parse(trimmed)
	if err != nil || !notifiarrHost(parsed.Hostname()) {
		return raw, ""
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	if !strings.EqualFold(path, notifiarrUnpackerrPath) &&
		!strings.HasPrefix(strings.ToLower(path), notifiarrUnpackerrPath+"/") {
		return raw, ""
	}

	if strings.EqualFold(path, notifiarrUnpackerrPath) {
		return raw, ""
	}

	rest := path[len(notifiarrUnpackerrPath)+1:]
	if rest == "" || strings.Contains(rest, "/") || !notifiarrAPIKey.MatchString(rest) {
		return raw, ""
	}

	parsed.Path = notifiarrUnpackerrPath
	parsed.RawPath = ""

	return parsed.String(), rest
}

func notifiarrHost(host string) bool {
	host = strings.ToLower(host)

	return host == "notifiarr.com" || strings.HasSuffix(host, ".notifiarr.com")
}

// Package hooks runs webhook HTTP posts and command hooks.
package hooks

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/cnfg"
	"golift.io/starr"
)

// Errors produced by this package.
var (
	ErrInvalidStatus = errors.New("invalid HTTP status reply")
	ErrWebhookNoURL  = errors.New("webhook without a URL configured; fix it")
	ErrCmdhookNoCmd  = errors.New("cmdhook without a command configured; fix it")
	ErrNilConfig     = errors.New("nil config entry")
)

// Logger is the logging surface hooks need from the daemon.
type Logger interface {
	Printf(msg string, v ...any)
	Errorf(msg string, v ...any)
	Debugf(msg string, v ...any)
}

// Config defines a webhook or command hook.
type Config struct {
	Name       string        `json:"name"         toml:"name"          xml:"name"                    yaml:"name"`
	URL        string        `json:"url"          toml:"url"           xml:"url,omitempty"           yaml:"url"`
	Command    string        `json:"command"      toml:"command"       xml:"command,omitempty"       yaml:"command"`
	CType      string        `json:"contentType"  toml:"content_type"  xml:"content_type,omitempty"  yaml:"contentType"`
	TmplPath   string        `json:"templatePath" toml:"template_path" xml:"template_path,omitempty" yaml:"templatePath"`
	TempName   string        `json:"template"     toml:"template"      xml:"template,omitempty"      yaml:"template"`
	Timeout    cnfg.Duration `json:"timeout"      toml:"timeout"       xml:"timeout"                 yaml:"timeout"`
	Shell      bool          `json:"shell"        toml:"shell"         xml:"shell"                   yaml:"shell"`
	IgnoreSSL  bool          `json:"ignoreSsl"    toml:"ignore_ssl"    xml:"ignore_ssl,omitempty"    yaml:"ignoreSsl"`
	Silent     bool          `json:"silent"       toml:"silent"        xml:"silent"                  yaml:"silent"`
	Events     Statuses      `json:"events"       toml:"events"        xml:"events"                  yaml:"events"`
	Exclude    StringSlice   `json:"exclude"      toml:"exclude"       xml:"exclude"                 yaml:"exclude"`
	Nickname   string        `json:"nickname"     toml:"nickname"      xml:"nickname,omitempty"      yaml:"nickname"`
	Token      string        `json:"token"        toml:"token"         xml:"token,omitempty"         yaml:"token"`
	Channel    string        `json:"channel"      toml:"channel"       xml:"channel,omitempty"       yaml:"channel"`
	client     *http.Client
	fails      uint
	posts      uint
	sync.Mutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// Statuses allows us to create a custom environment variable unmarshaller.
type Statuses []extract.Status

// UnmarshalENV turns environment variables into extraction statuses.
func (statuses *Statuses) UnmarshalENV(tag, envval string) error {
	if envval == "" {
		return nil
	}

	envval = strings.Trim(envval, `["',] `)
	vals := strings.Split(envval, ",")
	*statuses = make(Statuses, len(vals))

	for idx, val := range vals {
		intVal, err := strconv.ParseUint(strings.TrimSpace(val), 10, 8)
		if err != nil {
			return fmt.Errorf("converting tag %s value '%s' to number: %w", tag, envval, err)
		}

		(*statuses)[idx] = extract.Status(intVal)
	}

	return nil
}

func (statuses *Statuses) MarshalENV(tag string) (map[string]string, error) {
	vals := make([]string, len(*statuses))

	for idx, status := range *statuses {
		vals[idx] = status.String()
	}

	return map[string]string{tag: strings.Join(vals, ",")}, nil
}

// StringSlice allows a special environment variable unmarshaller for a lot of strings.
type StringSlice []string

// UnmarshalENV turns environment variables into a string slice.
func (slice *StringSlice) UnmarshalENV(_, envval string) error {
	if envval == "" {
		return nil
	}

	envval = strings.Trim(envval, `["',] `)
	vals := strings.Split(envval, ",")
	*slice = make(StringSlice, len(vals))

	for idx, val := range vals {
		(*slice)[idx] = strings.TrimSpace(val)
	}

	return nil
}

func (slice *StringSlice) MarshalENV(tag string) (map[string]string, error) {
	return map[string]string{tag: strings.Join(*slice, ",")}, nil
}

func applyDefaults(hook *Config, defaultTimeout time.Duration) {
	if hook.Timeout.Duration == 0 {
		hook.Timeout.Duration = defaultTimeout
	}

	if len(hook.Events) == 0 {
		hook.Events = Statuses{extract.WAITING}
	}
}

func (w *Config) ensureClient() {
	if w.client != nil {
		return
	}

	w.client = &http.Client{
		Timeout: w.Timeout.Duration,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: w.IgnoreSSL, //nolint:gosec
		}},
	}
}

// Excluded returns true if an app is in the Exclude slice.
func (w *Config) Excluded(app starr.App) bool {
	for _, exclude := range w.Exclude {
		if strings.EqualFold(exclude, string(app)) {
			return true
		}
	}

	return false
}

// HasEvent returns true if a status event is in the Events slice.
// Also returns true if the Events slice has only one value of WAITING.
func (w *Config) HasEvent(e extract.Status) bool {
	for _, status := range w.Events {
		if (status == extract.WAITING && len(w.Events) == 1) || status == e {
			return true
		}
	}

	return false
}

// Counts returns the total count of requests and failures for a webhook.
func (w *Config) Counts() (uint, uint) {
	w.Lock()
	defer w.Unlock()

	return w.posts, w.fails
}

// CountAll returns the total count of requests and errors for all hooks.
func CountAll(hooks []*Config) (uint, uint) {
	var total, fails uint

	for _, hook := range hooks {
		if hook == nil {
			continue
		}

		posts, failures := hook.Counts()
		total += posts
		fails += failures
	}

	return total, fails
}

// LogEvents formats events for printing.
func LogEvents(events []extract.Status) string {
	if len(events) == 1 && events[0] == extract.WAITING {
		return "all"
	}

	var output string

	for _, event := range events {
		if len(output) > 0 {
			output += "; "
		}

		output += event.String()
	}

	return output
}

// CloneList copies hooks without the mutex, counters, client, or template.
func CloneList(src []*Config) []*Config {
	if src == nil {
		return nil
	}

	out := make([]*Config, len(src))
	for idx, hook := range src {
		out[idx] = &Config{
			Name:      hook.Name,
			URL:       hook.URL,
			Command:   hook.Command,
			CType:     hook.CType,
			TmplPath:  hook.TmplPath,
			TempName:  hook.TempName,
			Timeout:   hook.Timeout,
			Shell:     hook.Shell,
			IgnoreSSL: hook.IgnoreSSL,
			Silent:    hook.Silent,
			Events:    append(Statuses(nil), hook.Events...),
			Exclude:   append(StringSlice(nil), hook.Exclude...),
			Nickname:  hook.Nickname,
			Token:     hook.Token,
			Channel:   hook.Channel,
		}
	}

	return out
}

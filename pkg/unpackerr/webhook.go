package unpackerr

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"golift.io/cnfg"
	"golift.io/version"
)

// WebhookConfig defines the data to send webhooks to a server.
type WebhookConfig struct {
	Name       string          `json:"name" toml:"name" xml:"name" yaml:"name"`
	URL        string          `json:"url" toml:"url" xml:"url" yaml:"url"`
	CType      string          `json:"content_type" toml:"content_type" xml:"content_type" yaml:"content_type"`
	Timeout    cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL  bool            `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent     bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events     []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude    []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	client     *http.Client
	fails      uint
	posts      uint
	sync.Mutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// DiscordPayload - https://leovoel.github.io/embed-visualizer/
type DiscordPayload struct {
	Content   string                 `json:"content"`
	Username  string                 `json:"username"`
	AvatarURL string                 `json:"avatar_url"`
	Embeds    []*DiscordPayloadEmbed `json:"embeds"`
}

type DiscordPayloadEmbed struct {
	Title       string                      `json:"title"`
	Description string                      `json:"description"`
	URL         string                      `json:"url"`
	Color       int                         `json:"color"`
	Fields      []*DiscordPayloadEmbedField `json:"fields"`
	Author      *DiscordPayloadEmbedAuthor  `json:"author"`
}

type DiscordPayloadEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type DiscordPayloadEmbedAuthor struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
}

// NotifiarrPayload defines the data sent to notifarr.com webhooks.
type NotifiarrPayload struct {
	Path  string                 `json:"path"`                // Path for the extracted item.
	App   string                 `json:"app"`                 // Application Triggering Event
	IDs   map[string]interface{} `json:"ids,omitempty"`       // Arbitrary IDs from each app.
	Event ExtractStatus          `json:"unpackerr_eventtype"` // The type of the event.
	Time  time.Time              `json:"time"`                // Time of this event.
	Data  *XtractPayload         `json:"data,omitempty"`      // Payload from extraction process.
	// Application Metadata.
	Go       string    `json:"go_version"` // Version of go compiled with
	OS       string    `json:"os"`         // Operating system: linux, windows, darwin
	Arch     string    `json:"arch"`       // Architecture: amd64, armhf
	Version  string    `json:"version"`    // Application Version
	Revision string    `json:"revision"`   // Application Revision
	Branch   string    `json:"branch"`     // Branch built from.
	Started  time.Time `json:"started"`    // App start time.
}

// XtractPayload is a rewrite of xtractr.Response.
type XtractPayload struct {
	Error    string    `json:"error,omitempty"`      // error only during extractfailed
	Archives []string  `json:"archives,omitempty"`   // list of all archive files extracted
	Files    []string  `json:"files,omitempty"`      // list of all files extracted
	Start    time.Time `json:"start,omitempty"`      // start time of extraction
	Output   string    `json:"tmp_folder,omitempty"` // temporary items folder
	Bytes    int64     `json:"bytes,omitempty"`      // Bytes written
	Elapsed  float64   `json:"elapsed,omitempty"`    // Duration in seconds
}

// Errors produced by this file.
var (
	ErrInvalidStatus = fmt.Errorf("invalid HTTP status reply")
)

func (u *Unpackerr) sendWebhooks(i *Extract) {
	if i.Status == IMPORTED && i.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := &NotifiarrPayload{
		Path:  i.Path,
		App:   i.App,
		IDs:   i.IDs,
		Time:  i.Updated,
		Data:  nil,
		Event: i.Status,
		// Application Metadata.
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
	}

	if i.Status <= EXTRACTED && i.Resp != nil {
		payload.Data = &XtractPayload{
			Archives: append(i.Resp.Extras, i.Resp.Archives...),
			Files:    i.Resp.NewFiles,
			Start:    i.Resp.Started,
			Output:   i.Resp.Output,
			Bytes:    i.Resp.Size,
			Elapsed:  i.Resp.Elapsed.Seconds(),
		}

		if i.Resp.Error != nil {
			payload.Data.Error = i.Resp.Error.Error()
		}
	}

	for _, hook := range u.Webhook {
		if !hook.HasEvent(i.Status) || hook.Excluded(i.App) {
			continue
		}

		go u.sendWebhookWithLog(hook, payload)
	}
}

func (u *Unpackerr) sendWebhookWithLog(hook *WebhookConfig, payload *NotifiarrPayload) {
	var (
		body []byte
		err  error
	)

	switch url := strings.ToLower(hook.URL); {
	default:
		u.Printf("[WARN] Webhook URL is unknown; not notifiarr or discord: %v", hook.Name)
		fallthrough
	case strings.Contains(url, "discordnotifier.com") || strings.Contains(url, "notifiarr.com"):
		body, err = json.Marshal(payload)
	case strings.Contains(url, "discord.com"):
		discord := &DiscordPayload{
			Username:  "Unpackerr",
			AvatarURL: "https://github.com/davidnewhall/unpackerr/wiki/images/logo.png",
			Embeds: []*DiscordPayloadEmbed{{
				Title: "**" + payload.IDs["title"].(string) + "**", // welp.
				Author: &DiscordPayloadEmbedAuthor{
					Name:    "Unpackerr: " + payload.Event.Desc(),
					IconURL: "https://github.com/davidnewhall/unpackerr/wiki/images/logo.png",
				},
				Fields: []*DiscordPayloadEmbedField{
					{Name: "**Path**", Value: payload.Path},
					{Name: "**App**", Value: payload.App, Inline: true},
				},
			}},
		}

		if payload.Data != nil {
			discord.Embeds[0].Fields = append(discord.Embeds[0].Fields,
				&DiscordPayloadEmbedField{Name: "**Size**", Value: HumanBytes(payload.Data.Bytes), Inline: true},
				&DiscordPayloadEmbedField{Name: "**Elapsed**", Value: time.Since(payload.Data.Start).String(), Inline: true})
			if payload.Data.Error != "" {
				discord.Embeds[0].Color = 9383736
				discord.Embeds[0].Fields = append(discord.Embeds[0].Fields,
					&DiscordPayloadEmbedField{Name: "**Error**", Value: payload.Data.Error})
			}
		}

		body, err = json.Marshal(discord)
	}

	if err != nil {
		u.Printf("[ERROR] Webhook Payload (%s = %s): %v", payload.Path, payload.Event, err)
	}

	if body, err := hook.Send(body); err != nil {
		u.Printf("[ERROR] Webhook (%s = %s): %v", payload.Path, payload.Event, err)
	} else if !hook.Silent {
		u.Printf("[Webhook] Posted Payload (%s = %s): %s: OK", payload.Path, payload.Event, hook.Name)
		u.Debugf("[DEBUG] Webhook Response: %s", string(bytes.ReplaceAll(body, []byte{'\n'}, []byte{' '})))
	}
}

// Send marshals an interface{} into json and POSTs it to a URL.
func (w *WebhookConfig) Send(body []byte) ([]byte, error) {
	w.Lock()
	defer w.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout.Duration+time.Second)
	defer cancel()

	b, err := w.send(ctx, body)
	if err != nil {
		w.fails++
	}

	w.posts++

	return b, err
}

func (w *WebhookConfig) send(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", w.URL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("creating request '%s': %w", w.Name, err)
	}

	if w.CType == "" {
		req.Header.Set("content-type", "application/json")
	} else {
		req.Header.Set("content-type", w.CType)
	}

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload '%s': %w", w.Name, err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	reply, _ := ioutil.ReadAll(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		return nil, fmt.Errorf("%w (%s) '%s': %s", ErrInvalidStatus, res.Status, w.Name, reply)
	}

	return reply, nil
}

func (u *Unpackerr) validateWebhook() {
	for i := range u.Webhook {
		if u.Webhook[i].Name == "" {
			u.Webhook[i].Name = u.Webhook[i].URL
		}

		if u.Webhook[i].Timeout.Duration == 0 {
			u.Webhook[i].Timeout.Duration = u.Timeout.Duration
		}

		if len(u.Webhook[i].Events) == 0 {
			u.Webhook[i].Events = []ExtractStatus{WAITING}
		}

		if u.Webhook[i].client == nil {
			u.Webhook[i].client = &http.Client{
				Timeout: u.Webhook[i].Timeout.Duration,
				Transport: &http.Transport{TLSClientConfig: &tls.Config{
					InsecureSkipVerify: u.Webhook[i].IgnoreSSL, // nolint: gosec
				}},
			}
		}
	}
}

func (u *Unpackerr) logWebhook() {
	if c := len(u.Webhook); c == 1 {
		u.Printf(" => Webhook Config: 1 URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
			u.Webhook[0].Name, u.Webhook[0].Timeout, u.Webhook[0].IgnoreSSL, u.Webhook[0].Silent, logEvents(u.Webhook[0].Events))
	} else {
		u.Print(" => Webhook Configs:", c, "URLs")

		for _, f := range u.Webhook {
			u.Printf(" =>    URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
				f.Name, f.Timeout, f.IgnoreSSL, f.Silent, logEvents(f.Events))
		}
	}
}

// logEvents is only used in logWebhook to format events for printing.
func logEvents(events []ExtractStatus) (s string) {
	if len(events) == 1 && events[0] == WAITING {
		return "all"
	}

	for _, e := range events {
		if len(s) > 0 {
			s += "; "
		}

		s += e.String()
	}

	return s
}

// Excluded returns true if an app is in the Exclude slice.
func (w *WebhookConfig) Excluded(app string) bool {
	for _, a := range w.Exclude {
		if strings.EqualFold(a, app) {
			return true
		}
	}

	return false
}

// HasEvent returns true if a status event is in the Events slice.
// Also returns true if the Events slice has only one value of WAITING.
func (w *WebhookConfig) HasEvent(e ExtractStatus) bool {
	for _, h := range w.Events {
		if (h == WAITING && len(w.Events) == 1) || h == e {
			return true
		}
	}

	return false
}

// WebhookCounts returns the total count of requests and errors for all webhooks.
func (u *Unpackerr) WebhookCounts() (total uint, fails uint) {
	for _, hook := range u.Webhook {
		t, f := hook.Counts()
		total += t
		fails += f
	}

	return total, fails
}

// Counts returns the total count of requests and failures for a webhook.
func (w *WebhookConfig) Counts() (uint, uint) {
	w.Lock()
	defer w.Unlock()

	return w.posts, w.fails
}

// HumanBytes is from https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
// This converts an int to a human readable byte string.
func HumanBytes(b int64) string {
	const unit = 1024

	if b < unit {
		return fmt.Sprintf("%dB", b)
	}

	div, exp := int64(unit), 0

	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

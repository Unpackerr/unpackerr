package unpackerr

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	URL        string          `json:"url" toml:"url" xml:"url,omitempty" yaml:"url"`
	Command    string          `json:"command" toml:"command" xml:"command,omitempty" yaml:"command"`
	CType      string          `json:"contentType" toml:"content_type" xml:"content_type,omitempty" yaml:"contentType"`
	TmplPath   string          `json:"templatePath" toml:"template_path" xml:"template_path,omitempty" yaml:"templatePath"`
	TempName   string          `json:"template" toml:"template" xml:"template,omitempty" yaml:"template"`
	Timeout    cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	Shell      bool            `json:"shell" toml:"shell" xml:"shell" yaml:"shell"`
	IgnoreSSL  bool            `json:"ignoreSsl" toml:"ignore_ssl" xml:"ignore_ssl,omitempty" yaml:"ignoreSsl"`
	Silent     bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events     []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude    []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	Nickname   string          `json:"nickname" toml:"nickname" xml:"nickname,omitempty" yaml:"nickname"`
	Token      string          `json:"token" toml:"token" xml:"token,omitempty" yaml:"token"`
	Channel    string          `json:"channel" toml:"channel" xml:"channel,omitempty" yaml:"channel"`
	client     *http.Client
	fails      uint
	posts      uint
	sync.Mutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type hookQueueItem struct {
	*WebhookConfig
	*WebhookPayload
}

// Errors produced by this file.
var (
	ErrInvalidStatus = fmt.Errorf("invalid HTTP status reply")
	ErrWebhookNoURL  = fmt.Errorf("webhook without a URL configured; fix it")
)

// runAllHooks sends webhooks and executes command hooks.
func (u *Unpackerr) runAllHooks(i *Extract) {
	if i.Status == IMPORTED && i.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := &WebhookPayload{
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
			Files:   i.Resp.NewFiles,
			Start:   i.Resp.Started,
			Output:  i.Resp.Output,
			Bytes:   i.Resp.Size,
			Queue:   i.Resp.Queued,
			Elapsed: cnfg.Duration{Duration: i.Resp.Elapsed},
		}

		for _, v := range i.Resp.Archives {
			payload.Data.Archives = append(payload.Data.Archives, v...)
		}

		for _, v := range i.Resp.Extras {
			payload.Data.Archives = append(payload.Data.Archives, v...)
		}

		if i.Resp.Error != nil {
			payload.Data.Error = i.Resp.Error.Error()
		}
	}

	for _, hook := range u.Webhook {
		if hook.HasEvent(i.Status) && !hook.Excluded(i.App) {
			u.hookChan <- &hookQueueItem{WebhookConfig: hook, WebhookPayload: payload}
		}
	}

	for _, hook := range u.Cmdhook {
		if hook.HasEvent(i.Status) && !hook.Excluded(i.App) {
			u.hookChan <- &hookQueueItem{WebhookConfig: hook, WebhookPayload: payload}
		}
	}
}

func (u *Unpackerr) sendWebhookWithLog(hook *WebhookConfig, payload *WebhookPayload) {
	var body bytes.Buffer

	if tmpl, err := hook.Template(); err != nil {
		u.Errorf("Webhook Template (%s = %s): %v", payload.Path, payload.Event, err)
		return
	} else if err = tmpl.Execute(&body, payload); err != nil {
		u.Errorf("Webhook Payload (%s = %s): %v", payload.Path, payload.Event, err)
		return
	}

	b := body.String() //nolint:ifshort

	if reply, err := hook.Send(&body); err != nil {
		u.Debugf("Webhook Payload: %s", b)
		u.Errorf("Webhook (%s = %s): %s: %v", payload.Path, payload.Event, hook.Name, err)
		u.Debugf("Webhook Response: %s", string(reply))
	} else if !hook.Silent {
		u.Debugf("Webhook Payload: %s", b)
		u.Printf("[Webhook] Posted Payload (%s = %s): %s: OK", payload.Path, payload.Event, hook.Name)
	}
}

// Send marshals an interface{} into json and POSTs it to a URL.
func (w *WebhookConfig) Send(body io.Reader) ([]byte, error) {
	w.Lock()
	defer w.Unlock()
	w.posts++

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout.Duration+time.Second)
	defer cancel()

	b, err := w.send(ctx, body)
	if err != nil {
		w.fails++
	}

	return b, err
}

func (w *WebhookConfig) send(ctx context.Context, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("content-type", w.CType)

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload: %w", err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	reply, _ := io.ReadAll(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		return nil, fmt.Errorf("%w (%s): %s", ErrInvalidStatus, res.Status, reply)
	}

	return reply, nil
}

func (u *Unpackerr) validateWebhook() error { //nolint:cyclop
	for i := range u.Webhook {
		u.Webhook[i].Command = ""

		if u.Webhook[i].URL == "" {
			return ErrWebhookNoURL
		}

		if u.Webhook[i].Name == "" {
			u.Webhook[i].Name = u.Webhook[i].URL
		}

		if u.Webhook[i].Nickname == "" && u.Webhook[i].TmplPath == "" &&
			!strings.Contains(u.Webhook[i].URL, "pushover.net") {
			u.Webhook[i].Nickname = "Unpackerr"
		}

		if u.Webhook[i].CType == "" {
			u.Webhook[i].CType = "application/json"
			if strings.Contains(u.Webhook[i].URL, "pushover.net") {
				u.Webhook[i].CType = "application/x-www-form-urlencoded"
			}
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
					InsecureSkipVerify: u.Webhook[i].IgnoreSSL, //nolint:gosec
				}},
			}
		}
	}

	return nil
}

func (u *Unpackerr) logWebhook() {
	var ex, pfx string

	if len(u.Webhook) == 1 {
		pfx = " => Webhook Config: 1 URL"
	} else {
		u.Printf(" => Webhook Configs: %d URLs", len(u.Webhook))
		pfx = " =>    URL"
	}

	for _, f := range u.Webhook {
		if ex = ""; f.TmplPath != "" {
			ex = fmt.Sprintf(", template: %s, content_type: %s", f.TmplPath, f.CType)
		}

		u.Printf("%s: %s, timeout: %v, ignore ssl: %v, silent: %v%s, events: %v, channel: %s, nickname: %s",
			pfx, f.Name, f.Timeout, f.IgnoreSSL, f.Silent, ex, logEvents(f.Events), f.Channel, f.Nickname)
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

package unpackerr

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	TmplPath   string          `json:"template_path" toml:"template_path" xml:"template_path" yaml:"template_path"`
	Timeout    cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL  bool            `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent     bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events     []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude    []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	Nickname   string          `json:"nickname" toml:"nickname" xml:"nickname" yaml:"nickname"`
	Channel    string          `json:"channel" toml:"channel" xml:"channel" yaml:"channel"`
	client     *http.Client
	fails      uint
	posts      uint
	sync.Mutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// Errors produced by this file.
var (
	ErrInvalidStatus = fmt.Errorf("invalid HTTP status reply")
	ErrNoURL         = fmt.Errorf("webhook without a URL configured; fix it")
)

func (u *Unpackerr) sendWebhooks(i *Extract) {
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
			Archives: append(i.Resp.Extras, i.Resp.Archives...),
			Files:    i.Resp.NewFiles,
			Start:    i.Resp.Started,
			Output:   i.Resp.Output,
			Bytes:    i.Resp.Size,
			Queue:    i.Resp.Queued,
			Elapsed:  cnfg.Duration{Duration: i.Resp.Elapsed},
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

func (u *Unpackerr) sendWebhookWithLog(hook *WebhookConfig, payload *WebhookPayload) {
	var body bytes.Buffer

	if tmpl, err := hook.Template(); err != nil {
		u.Printf("[ERROR] Webhook Template (%s = %s): %v", payload.Path, payload.Event, err)
		return
	} else if err = tmpl.Execute(&body, payload); err != nil {
		u.Printf("[ERROR] Webhook Payload (%s = %s): %v", payload.Path, payload.Event, err)
		return
	}

	b := body.String() // nolint: ifshort

	if reply, err := hook.Send(&body); err != nil {
		u.Debugf("Webhook Payload: %s", b)
		u.Printf("[ERROR] Webhook (%s = %s): %s: %v", payload.Path, payload.Event, hook.Name, err)
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
	req, err := http.NewRequestWithContext(ctx, "POST", w.URL, body)
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
	reply, _ := ioutil.ReadAll(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		return nil, fmt.Errorf("%w (%s): %s", ErrInvalidStatus, res.Status, reply)
	}

	return reply, nil
}

func (u *Unpackerr) validateWebhook() error {
	for i := range u.Webhook {
		if u.Webhook[i].URL == "" {
			return ErrNoURL
		}

		if u.Webhook[i].Name == "" {
			u.Webhook[i].Name = u.Webhook[i].URL
		}

		if u.Webhook[i].Nickname == "" {
			u.Webhook[i].Nickname = "Unpackerr"
		} else if len(u.Webhook[i].Nickname) > 20 { //nolint:gomnd // be reasonable
			u.Webhook[i].Nickname = u.Webhook[i].Nickname[:20]
		}

		if u.Webhook[i].CType == "" {
			u.Webhook[i].CType = "application/json"
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

	return nil
}

func (u *Unpackerr) logWebhook() {
	var ex, pfx string

	if len(u.Webhook) == 1 {
		pfx = " => Webhook Config: 1 URL"
	} else {
		u.Print(" => Webhook Configs:", len(u.Webhook), "URLs")
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

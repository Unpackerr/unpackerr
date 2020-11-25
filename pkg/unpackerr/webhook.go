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

type WebhookConfig struct {
	Name       string          `json:"name" toml:"name" xml:"name" yaml:"name"`
	URL        string          `json:"url" toml:"url" xml:"url" yaml:"url"`
	Timeout    cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL  bool            `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent     bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events     []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude    []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	client     *http.Client    `json:"-"`
	fails      uint            `json:"-"`
	posts      uint            `json:"-"`
	sync.Mutex `json:"-"`
}

// WebhookPayload defines the data sent to outbound webhooks.
type WebhookPayload struct {
	Path  string                 `json:"path"`
	App   string                 `json:"app"`
	IDs   map[string]interface{} `json:"ids,omitempty"`
	Event ExtractStatus          `json:"unpackerr_eventtype"`
	Time  time.Time              `json:"time"`
	Data  *XtractPayload         `json:"data,omitempty"`
	// Application Metadata.
	Go       string    `json:"go_version"`
	OS       string    `json:"os"`
	Arch     string    `json:"arch"`
	Version  string    `json:"version"`
	Revision string    `json:"revision"`
	Branch   string    `json:"branch"`
	Started  time.Time `json:"started"`
}

type XtractPayload struct {
	Error    string    `json:"error,omitempty"`
	Archives []string  `json:"archives,omitempty"`
	Files    []string  `json:"files,omitempty"`
	Start    time.Time `json:"start,omitempty"`
	Output   string    `json:"tmp_folder,omitempty"`
	Bytes    int64     `json:"bytes,omitempty"`
	Elapsed  float64   `json:"elapsed,omitempty"`
}

var ErrInvalidStatus = fmt.Errorf("invalid HTTP status reply")

func (u *Unpackerr) sendWebhooks(i *Extract) {
	if i.Status == IMPORTED && i.App == "Folder" {
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
			Error:    i.Resp.Error.Error(),
			Archives: append(i.Resp.Extras, i.Resp.Archives...),
			Files:    i.Resp.NewFiles,
			Start:    i.Resp.Started,
			Output:   i.Resp.Output,
			Bytes:    i.Resp.Size,
			Elapsed:  i.Resp.Elapsed.Seconds(),
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
	if body, err := hook.Send(payload); err != nil {
		u.Logf("[ERROR] Webhook (%s = %s): %v", payload.Path, payload.Event, err)
	} else if !hook.Silent {
		u.Logf("[Webhook] Posted Payload (%s = %s): %s: 200 OK", payload.Path, payload.Event, hook.Name)
		u.Debug("[DEBUG] Webhook Response: %s", string(bytes.ReplaceAll(body, []byte{'\n'}, []byte{' '})))
	}
}

// Sends marshals an interface{} into json and POSTs it to a URL.
func (w *WebhookConfig) Send(i interface{}) ([]byte, error) {
	w.Lock()
	defer w.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout.Duration+time.Second)
	defer cancel()

	b, err := w.send(ctx, i)
	if err != nil {
		w.fails++
	}

	w.posts++

	return b, err
}

func (w *WebhookConfig) send(ctx context.Context, i interface{}) ([]byte, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload '%s': %w", w.Name, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", w.URL, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("creating request '%s': %w", w.Name, err)
	}

	req.Header.Set("content-type", "application/json")

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload '%s': %w", w.Name, err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	body, _ := ioutil.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w (%s) '%s': %s", ErrInvalidStatus, res.Status, w.Name, body)
	}

	return body, nil
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
		u.Logf(" => Webhook Config: 1 URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
			u.Webhook[0].Name, u.Webhook[0].Timeout, u.Webhook[0].IgnoreSSL, u.Webhook[0].Silent, logEvents(u.Webhook[0].Events))
	} else {
		u.Log(" => Webhook Configs:", c, "URLs")

		for _, f := range u.Webhook {
			u.Logf(" =>    URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
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

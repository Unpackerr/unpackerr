package unpacker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golift.io/cnfg"
	"golift.io/xtractr"
)

type WebhookConfig struct {
	Name      string          `json:"name" toml:"name" xml:"name" yaml:"name"`
	URL       string          `json:"url" toml:"url" xml:"url" yaml:"url"`
	Timeout   cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL bool            `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent    bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events    []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude   []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	client    *http.Client    `json:"-"`
	fails     uint            `json:"-"`
	posts     uint            `json:"-"`
}

var ErrInvalidStatus = fmt.Errorf("invalid HTTP status reply")

func (u *Unpackerr) sendWebhooks(i *Extracts) {
	for _, hook := range u.Webhook {
		if !hook.HasEvent(i.Status) || hook.Excluded(i.App) {
			continue
		}

		go func(hook *WebhookConfig) {
			ctx, cancel := context.WithTimeout(context.Background(), hook.Timeout.Duration+time.Second)
			defer cancel()

			// We cannot read some of the data in the response until it is done.
			// Otherwise we may have a race condition and crash.
			if j := i; !i.Resp.Done {
				i = &Extracts{
					Path:    j.Path,
					App:     j.App,
					Status:  j.Status,
					Updated: j.Updated,
				}
				if j.Resp != nil {
					i.Resp = &xtractr.Response{
						Done:     false,
						Started:  j.Resp.Started,
						Archives: j.Resp.Archives,
						Output:   j.Resp.Output,
						X:        j.Resp.X,
					}
				}
			}

			if body, err := u.sendWebhook(ctx, hook, i); err != nil {
				u.Logf("[ERROR] Webhook: %v", err)
				hook.fails++
			} else if !hook.Silent {
				u.Logf("[Webhook] Posted Payload: %s: 200 OK", hook.Name)
				u.Debug("[DEBUG] Webhook Response: %s", string(bytes.ReplaceAll(body, []byte{'\n'}, []byte{' '})))
				hook.posts++
			}
		}(hook)
	}
}

func (u *Unpackerr) sendWebhook(ctx context.Context, hook *WebhookConfig, i interface{}) ([]byte, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload '%s': %w", hook.Name, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("creating request '%s': %w", hook.Name, err)
	}

	res, err := hook.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload '%s': %w", hook.Name, err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	body, _ := ioutil.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w (%s) '%s': %s", ErrInvalidStatus, res.Status, hook.Name, body)
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
			u.Webhook[0].Name, u.Webhook[0].Timeout, u.Webhook[0].IgnoreSSL, u.Webhook[0].Silent, u.Webhook[0].Events)
	} else {
		u.Log(" => Webhook Configs:", c, "URLs")

		for _, f := range u.Webhook {
			u.Logf(" =>    URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
				f.Name, f.Timeout, f.IgnoreSSL, f.Silent, f.Events)
		}
	}
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

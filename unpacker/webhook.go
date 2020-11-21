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
	URL       string        `json:"url" toml:"url" xml:"url" yaml:"url"`
	Timeout   cnfg.Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL bool          `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent    bool          `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events    []string      `json:"events" toml:"events" xml:"events" yaml:"events"`
	client    *http.Client  `json:"-"`
}

type WebhookPayload struct {
	Time  time.Time         `json:"time"`
	Name  string            `json:"name"`
	App   string            `json:"app"`
	Event string            `json:"unpackerr_eventtype"`
	Data  *xtractr.Response `json:"data"`
}

func (u *Unpackerr) sendWebhooks(i *Extracts) {
	for _, hook := range u.Webhook {
		if !hook.HasEvent(i.Status) {
			continue
		}

		go func(hook *WebhookConfig) {
			ctx, cancel := context.WithTimeout(context.Background(), hook.Timeout.Duration)
			defer cancel()

			err := u.sendWebhook(ctx, hook, &WebhookPayload{
				Time:  i.Updated,
				Name:  i.Path,
				App:   i.App,
				Event: i.Status.String(),
				Data:  i.Resp,
			})
			if err != nil {
				u.Logf("[ERROR] Webhook: %v", err)

				return
			}
		}(hook)
	}
}

func (u *Unpackerr) sendWebhook(ctx context.Context, hook *WebhookConfig, i interface{}) error {
	b, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("marshaling payload '%s': %w", hook.URL, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("creating request '%s': %w", hook.URL, err)
	}

	res, err := hook.client.Do(req)
	if err != nil {
		return fmt.Errorf("POSTing payload '%s': %w", hook.URL, err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	body, err := ioutil.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POSTing payload, bad status (%s) '%s' (body err: %w) %s",
			res.Status, hook.URL, err, body)
	}

	if !hook.Silent {
		u.Logf("[Webhook] Posted Payload: %s: %s", hook.URL, res.Status)
		u.Debug("[DEBUG] Webhook Response (len:%s): %s", res.Header.Get("content-length"),
			string(bytes.ReplaceAll(body, []byte{'\n'}, []byte{' '})))
	}

	return nil
}

func (u *Unpackerr) validateWebhook() {
	for i := range u.Webhook {
		if u.Webhook[i].Timeout.Duration == 0 {
			u.Webhook[i].Timeout.Duration = u.Timeout.Duration
		}

		if len(u.Webhook[i].Events) == 0 {
			u.Webhook[i].Events = []string{"all"}
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
			u.Webhook[0].URL, u.Webhook[0].Timeout, u.Webhook[0].IgnoreSSL, u.Webhook[0].Silent, u.Webhook[0].Events)
	} else {
		u.Log(" => Webhook Configs:", c, "URLs")

		for _, f := range u.Webhook {
			u.Logf(" =>    URL: %s (timeout: %v, ignore ssl: %v, silent: %v, events: %v)",
				f.URL, f.Timeout, f.IgnoreSSL, f.Silent, f.Events)
		}
	}
}

func (w *WebhookConfig) HasEvent(e fmt.Stringer) bool {
	for _, h := range w.Events {
		if strings.EqualFold(h, "all") || strings.EqualFold(h, e.String()) {
			return true
		}
	}

	return false
}

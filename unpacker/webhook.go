package unpacker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golift.io/cnfg"
)

type WebhookConfig struct {
	URL       string        `json:"url" toml:"url" xml:"url" yaml:"url"`
	Timeout   cnfg.Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	IgnoreSSL bool          `json:"ignore_ssl" toml:"ignore_ssl" xml:"ignore_ssl" yaml:"ignore_ssl"`
	Silent    bool          `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Events    []string      `json:"events" toml:"events" xml:"events" yaml:"events"`
	client    *http.Client
}

func (u *Unpackerr) sendWebhooks(i interface{}) {
	for _, hook := range u.Webhook {
		go func(hook *WebhookConfig) {
			res, err := u.sendWebhook(hook, i)
			if err != nil {
				u.Logf("[ERROR] Webhook: %v", err)

				return
			}
			defer res.Body.Close()

			// The error is mostly ignored because we don't care about the body.
			// Read it in to avoid a memopry leak. Used in the if-stanza below.
			body, err := ioutil.ReadAll(res.Body)

			if res.StatusCode != http.StatusOK {
				u.Logf("[ERROR] Webhook: POSTing payload, bad status (%s) '%s': (body err: %v) %s",
					res.Status, hook.URL, err, body)

				return
			}

			if !hook.Silent {
				u.Logf("[Webhook] Posted Payload: %s: %s", hook.URL, res.Status)
				u.Debug("[DEBUG] Webhook Response (len:%s): %s", res.Header.Get("content-length"),
					string(bytes.ReplaceAll(body, []byte{'\n'}, []byte{' '})))
			}
		}(hook)
	}
}

func (u *Unpackerr) sendWebhook(hook *WebhookConfig, i interface{}) (*http.Response, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload '%s': %w", hook.URL, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hook.Timeout.Duration)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("creating request '%s': %w", hook.URL, err)
	}

	res, err := hook.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload '%s': %w", hook.URL, err)
	}

	return res, nil
}

func (u *Unpackerr) validateWebhook() {
	for i := range u.Webhook {
		if u.Webhook[i].Timeout.Duration == 0 {
			u.Webhook[i].Timeout.Duration = time.Minute
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
		u.Logf(" => Webhook Config: 1 URL: %s (timeout: %v, ignore ssl: %v)",
			u.Webhook[0].URL, u.Webhook[0].Timeout, u.Webhook[0].IgnoreSSL)
	} else {
		u.Log(" => Webhook Configs:", c, "URLs")

		for _, f := range u.Webhook {
			u.Logf(" =>    URL: %s (timeout: %v, ignore ssl: %v)", f.URL, f.Timeout, f.IgnoreSSL)
		}
	}
}

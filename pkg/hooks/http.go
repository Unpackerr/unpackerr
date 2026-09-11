package hooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Send marshals an any into json and POSTs it to a URL.
func (w *Config) Send(body io.Reader) ([]byte, error) {
	if w.URL == "" {
		return nil, ErrWebhookNoURL
	}

	w.Lock()
	defer w.Unlock()

	w.posts++

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout.Duration+time.Second)
	defer cancel()

	resp, err := w.send(ctx, body)
	if err != nil {
		w.fails++
	}

	return resp, err
}

func (w *Config) send(ctx context.Context, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", w.CType)

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POSTing payload: %w", err)
	}
	defer res.Body.Close()

	// The error is mostly ignored because we don't care about the body.
	// Read it in to avoid a memory leak. Used in the if-stanza below.
	reply, _ := io.ReadAll(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		return nil, fmt.Errorf("%w (%s): %s", ErrInvalidStatus, res.Status, reply)
	}

	return reply, nil
}

// SendWithLog renders and POSTs a webhook payload.
func SendWithLog(log Logger, hook *Config, payload *Payload) {
	var body bytes.Buffer

	if tmpl, err := hook.Template(); err != nil {
		log.Errorf("Webhook Template (%s = %s): %v", payload.Path, payload.Event, err)
		return
	} else if err = tmpl.Execute(&body, payload); err != nil {
		log.Errorf("Webhook Payload (%s = %s): %v", payload.Path, payload.Event, err)
		return
	}

	bodyStr := body.String()

	if reply, err := hook.Send(&body); err != nil {
		log.Debugf("Webhook Payload: %s", bodyStr)
		log.Errorf("Webhook (%s = %s): %s: %v", payload.Path, payload.Event, hook.Name, err)
		log.Debugf("Webhook Response: %s", string(reply))
	} else if !hook.Silent {
		log.Debugf("Webhook Payload: %s", bodyStr)
		log.Printf("[Webhook] Posted Payload (%s = %s): %s: OK", payload.Path, payload.Event, hook.Name)
	}
}

package hooks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	w.setRequestHeaders(req)

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

func (w *Config) setRequestHeaders(req *http.Request) {
	for name, value := range w.Headers {
		name = strings.TrimSpace(name)
		if name == "" || skipRequestHeader(name) ||
			strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			continue
		}

		req.Header.Set(name, value)
	}

	req.Header.Set("Content-Type", w.CType)

	if DetectTransport(w.TempName, w.URL).Name != ProfileNtfy {
		return
	}

	if token := strings.TrimSpace(w.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// SendWithLog renders and POSTs a webhook payload. Sample -w stays a create.
func SendWithLog(log Logger, hook *Config, payload *Payload) error {
	return deliverWebhook(log, hook, payload, "", nil)
}

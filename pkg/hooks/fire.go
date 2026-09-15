package hooks

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

// Fire delivers one webhook POST or command-hook run and returns the remote
// reply or command output. Used by the UI test; does not use the hook worker.
func Fire(ctx context.Context, hook *Config, payload *Payload) (string, error) {
	if hook == nil {
		return "", ErrNilConfig
	}

	if hook.Command != "" {
		out, err := runCmd(ctx, hook, payload)
		if out == nil {
			return "", err
		}

		return strings.TrimSpace(out.String()), err
	}

	if hook.URL == "" {
		return "", ErrWebhookNoURL
	}

	var body bytes.Buffer

	tmpl, err := hook.Template()
	if err != nil {
		return "", fmt.Errorf("webhook template: %w", err)
	}

	if err = tmpl.Execute(&body, payload); err != nil {
		return "", fmt.Errorf("webhook payload: %w", err)
	}

	hook.ensureClient()

	httpCtx, cancel := context.WithTimeout(ctx, hook.Timeout.Duration+time.Second)
	defer cancel()

	reply, err := hook.send(httpCtx, &body)

	return strings.TrimSpace(string(reply)), err
}

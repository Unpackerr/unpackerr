package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type discordMessage struct {
	ID string `json:"id"`
}

type telegramMessage struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

func deliverWebhook(log Logger, hook *Config, payload *Payload, msgID string, save func(string)) error {
	body, err := renderWebhook(hook, payload)
	if err != nil {
		log.Errorf("Webhook Template (%s = %s): %v", payload.Path, payload.Event, err)
		return err
	}

	bodyStr := string(body)
	profile := Detect(hook.TempName, hook.URL, hook.TmplPath)

	var reply []byte

	switch {
	case save != nil && profile.Name == ProfileDiscord:
		reply, err = hook.deliverDiscord(body, msgID, save)
	case save != nil && profile.Name == ProfileTelegram:
		reply, err = hook.deliverTelegram(body, msgID, save)
	default:
		reply, err = hook.Send(bytes.NewReader(body))
	}

	if err != nil {
		log.Debugf("Webhook Payload: %s", bodyStr)
		log.Errorf("Webhook (%s = %s): %s: %v", payload.Path, payload.Event, hook.Name, err)
		log.Debugf("Webhook Response: %s", string(reply))

		return err
	}

	if !hook.Silent {
		log.Debugf("Webhook Payload: %s", bodyStr)
		log.Printf("[Webhook] Posted Payload (%s = %s): %s: OK", payload.Path, payload.Event, hook.Name)
	}

	return nil
}

func renderWebhook(hook *Config, payload *Payload) ([]byte, error) {
	tmpl, err := hook.Template()
	if err != nil {
		return nil, fmt.Errorf("webhook template: %w", err)
	}

	var body bytes.Buffer
	if err = tmpl.Execute(&body, payload); err != nil {
		return nil, fmt.Errorf("webhook payload: %w", err)
	}

	return body.Bytes(), nil
}

func (w *Config) deliverDiscord(body []byte, msgID string, save func(string)) ([]byte, error) {
	if msgID != "" {
		reply, err := w.doSend(http.MethodPatch, discordMessageURL(w.URL, msgID), body)
		if err == nil || !messageGone(err, reply) {
			return reply, err
		}
	}

	reply, err := w.doSend(http.MethodPost, discordWaitURL(w.URL), body)
	if err != nil {
		return reply, err
	}

	var parsed discordMessage
	if json.Unmarshal(reply, &parsed) == nil && parsed.ID != "" && save != nil {
		save(parsed.ID)
	}

	return reply, nil
}

func (w *Config) deliverTelegram(body []byte, msgID string, save func(string)) ([]byte, error) {
	if msgID != "" {
		editBody, err := telegramEditBody(body, msgID)
		if err != nil {
			return nil, err
		}

		reply, err := w.doSend(http.MethodPost, telegramMethodURL(w.URL, "editMessageText"), editBody)
		if err == nil || !messageGone(err, reply) {
			return reply, err
		}
	}

	reply, err := w.doSend(http.MethodPost, telegramMethodURL(w.URL, "sendMessage"), body)
	if err != nil {
		return reply, err
	}

	var parsed telegramMessage
	if json.Unmarshal(reply, &parsed) == nil && parsed.Result.MessageID != 0 && save != nil {
		save(strconv.FormatInt(parsed.Result.MessageID, 10))
	}

	return reply, nil
}

func (w *Config) doSend(method, rawURL string, body []byte) ([]byte, error) {
	if rawURL == "" {
		return nil, ErrWebhookNoURL
	}

	w.ensureClient()

	w.Lock()
	defer w.Unlock()

	w.posts++

	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout.Duration+time.Second)
	defer cancel()

	reply, err := w.sendTo(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		w.fails++
	}

	return reply, err
}

func (w *Config) sendTo(ctx context.Context, method, rawURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	w.setRequestHeaders(req)

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending payload: %w", err)
	}
	defer res.Body.Close()

	reply, _ := io.ReadAll(res.Body)

	if res.StatusCode == http.StatusNotFound {
		return reply, fmt.Errorf("%w (%s): %s", ErrMessageGone, res.Status, reply)
	}

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		err := fmt.Errorf("%w (%s): %s", ErrInvalidStatus, res.Status, reply)
		if telegramMessageGone(res.StatusCode, reply) {
			return reply, fmt.Errorf("%w: %w", ErrMessageGone, err)
		}

		return reply, err
	}

	return reply, nil
}

func messageGone(err error, reply []byte) bool {
	if errors.Is(err, ErrMessageGone) {
		return true
	}

	return telegramMessageGone(0, reply)
}

func telegramMessageGone(status int, reply []byte) bool {
	lower := strings.ToLower(string(reply))
	if strings.Contains(lower, "message to edit not found") ||
		strings.Contains(lower, "message not found") {
		return true
	}

	return status == http.StatusBadRequest && strings.Contains(lower, "message to edit")
}

func discordWaitURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	query := parsed.Query()
	query.Set("wait", "true")
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func discordMessageURL(raw, msgID string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/messages/" + url.PathEscape(msgID)
	parsed.RawQuery = ""

	return parsed.String()
}

func telegramMethodURL(raw, method string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/sendMessage"), strings.HasSuffix(path, "/editMessageText"):
		path = path[:strings.LastIndex(path, "/")]
	}

	parsed.Path = path + "/" + method

	return parsed.String()
}

func telegramEditBody(body []byte, msgID string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("telegram edit payload: %w", err)
	}

	id, err := strconv.ParseInt(msgID, 10, 64)
	if err != nil {
		payload["message_id"] = msgID
	} else {
		payload["message_id"] = id
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("telegram edit payload: %w", err)
	}

	return out, nil
}

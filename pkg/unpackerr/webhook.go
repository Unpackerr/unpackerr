package unpackerr

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/version"
)

// WebhookConfig defines the data to send webhooks to a server.
type WebhookConfig struct {
	Name       string          `json:"name"         toml:"name"          xml:"name"                    yaml:"name"`
	URL        string          `json:"url"          toml:"url"           xml:"url,omitempty"           yaml:"url"`
	Command    string          `json:"command"      toml:"command"       xml:"command,omitempty"       yaml:"command"`
	CType      string          `json:"contentType"  toml:"content_type"  xml:"content_type,omitempty"  yaml:"contentType"`
	TmplPath   string          `json:"templatePath" toml:"template_path" xml:"template_path,omitempty" yaml:"templatePath"`
	TempName   string          `json:"template"     toml:"template"      xml:"template,omitempty"      yaml:"template"`
	Timeout    cnfg.Duration   `json:"timeout"      toml:"timeout"       xml:"timeout"                 yaml:"timeout"`
	Shell      bool            `json:"shell"        toml:"shell"         xml:"shell"                   yaml:"shell"`
	IgnoreSSL  bool            `json:"ignoreSsl"    toml:"ignore_ssl"    xml:"ignore_ssl,omitempty"    yaml:"ignoreSsl"`
	Silent     bool            `json:"silent"       toml:"silent"        xml:"silent"                  yaml:"silent"`
	Events     ExtractStatuses `json:"events"       toml:"events"        xml:"events"                  yaml:"events"`
	Exclude    StringSlice     `json:"exclude"      toml:"exclude"       xml:"exclude"                 yaml:"exclude"`
	Nickname   string          `json:"nickname"     toml:"nickname"      xml:"nickname,omitempty"      yaml:"nickname"`
	Token      string          `json:"token"        toml:"token"         xml:"token,omitempty"         yaml:"token"`
	Channel    string          `json:"channel"      toml:"channel"       xml:"channel,omitempty"       yaml:"channel"`
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
	ErrInvalidStatus = errors.New("invalid HTTP status reply")
	ErrWebhookNoURL  = errors.New("webhook without a URL configured; fix it")
)

// ExtractStatuses allows us to create a custom environment variable unmarshaller.
type ExtractStatuses []ExtractStatus

// UnmarshalENV turns environment variables into extraction statuses.
func (statuses *ExtractStatuses) UnmarshalENV(tag, envval string) error {
	if envval == "" {
		return nil
	}

	envval = strings.Trim(envval, `["',] `)
	vals := strings.Split(envval, ",")
	*statuses = make(ExtractStatuses, len(vals))

	for idx, val := range vals {
		intVal, err := strconv.ParseUint(strings.TrimSpace(val), 10, 8)
		if err != nil {
			return fmt.Errorf("converting tag %s value '%s' to number: %w", tag, envval, err)
		}

		(*statuses)[idx] = ExtractStatus(intVal)
	}

	return nil
}

func (statuses *ExtractStatuses) MarshalENV(tag string) (map[string]string, error) {
	vals := make([]string, len(*statuses))

	for idx, status := range *statuses {
		vals[idx] = status.String()
	}

	return map[string]string{tag: strings.Join(vals, ",")}, nil
}

// runAllHooks sends webhooks and executes command hooks.
func (u *Unpackerr) runAllHooks(item *Extract) {
	if item.Status == IMPORTED && item.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := &WebhookPayload{
		Path:  item.Path,
		App:   item.App,
		IDs:   item.IDs,
		Time:  item.Updated,
		Data:  nil,
		Event: item.Status,
		// Application Metadata.
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
	}

	if item.Status <= EXTRACTED && item.Resp != nil {
		payload.Data = &XtractPayload{
			Files:   item.Resp.NewFiles,
			File:    item.Resp.NewFiles,
			Start:   item.Resp.Started,
			Output:  item.Resp.Output,
			Bytes:   item.Resp.Size,
			Queue:   item.Resp.Queued,
			Elapsed: cnfg.Duration{Duration: item.Resp.Elapsed},
		}

		for _, v := range item.Resp.Archives {
			payload.Data.Archives = append(payload.Data.Archives, v...)
			payload.Data.Archive = append(payload.Data.Archive, v...)
		}

		for _, v := range item.Resp.Extras {
			payload.Data.Archives = append(payload.Data.Archives, v...)
			payload.Data.Archive = append(payload.Data.Archive, v...)
		}

		if item.Resp.Error != nil {
			payload.Data.Error = item.Resp.Error.Error()
		}
	}

	for _, hook := range u.Webhook {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App) {
			u.hookChan <- &hookQueueItem{WebhookConfig: hook, WebhookPayload: payload}
		}
	}

	for _, hook := range u.Cmdhook {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App) {
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

	bodyStr := body.String()

	if reply, err := hook.Send(&body); err != nil {
		u.Debugf("Webhook Payload: %s", bodyStr)
		u.Errorf("Webhook (%s = %s): %s: %v", payload.Path, payload.Event, hook.Name, err)
		u.Debugf("Webhook Response: %s", string(reply))
	} else if !hook.Silent {
		u.Debugf("Webhook Payload: %s", bodyStr)
		u.Printf("[Webhook] Posted Payload (%s = %s): %s: OK", payload.Path, payload.Event, hook.Name)
	}
}

// Send marshals an any into json and POSTs it to a URL.
func (w *WebhookConfig) Send(body io.Reader) ([]byte, error) {
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

func (w *WebhookConfig) send(ctx context.Context, body io.Reader) ([]byte, error) {
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
	// Read it in to avoid a memopry leak. Used in the if-stanza below.
	reply, _ := io.ReadAll(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode > http.StatusNoContent {
		return nil, fmt.Errorf("%w (%s): %s", ErrInvalidStatus, res.Status, reply)
	}

	return reply, nil
}

func (u *Unpackerr) validateWebhook() error { //nolint:cyclop
	for idx := range u.Webhook {
		u.Webhook[idx].Command = ""

		if u.Webhook[idx].URL == "" {
			return ErrWebhookNoURL
		}

		if u.Webhook[idx].Name == "" {
			u.Webhook[idx].Name = u.Webhook[idx].URL
		}

		if u.Webhook[idx].Nickname == "" && u.Webhook[idx].TmplPath == "" &&
			!strings.Contains(u.Webhook[idx].URL, "pushover.net") {
			u.Webhook[idx].Nickname = "Unpackerr"
		}

		if u.Webhook[idx].CType == "" {
			u.Webhook[idx].CType = "application/json"
			if strings.Contains(u.Webhook[idx].URL, "pushover.net") {
				u.Webhook[idx].CType = "application/x-www-form-urlencoded"
			}
		}

		if u.Webhook[idx].Timeout.Duration == 0 {
			u.Webhook[idx].Timeout.Duration = u.Timeout.Duration
		}

		if len(u.Webhook[idx].Events) == 0 {
			u.Webhook[idx].Events = []ExtractStatus{WAITING}
		}

		if u.Webhook[idx].client == nil {
			u.Webhook[idx].client = &http.Client{
				Timeout: u.Webhook[idx].Timeout.Duration,
				Transport: &http.Transport{TLSClientConfig: &tls.Config{
					InsecureSkipVerify: u.Webhook[idx].IgnoreSSL, //nolint:gosec
				}},
			}
		}
	}

	return nil
}

func (u *Unpackerr) logWebhook() {
	var vars, prefix string

	if len(u.Webhook) == 1 {
		prefix = " => Webhook Config: 1 URL"
	} else {
		u.Printf(" => Webhook Configs: %d URLs", len(u.Webhook))
		prefix = " =>    URL" //nolint:wsl
	}

	for _, hook := range u.Webhook {
		if vars = ""; hook.TmplPath != "" {
			vars = ", template: " + hook.TmplPath + ", content_type: " + hook.CType
		}

		if hook.Channel != "" {
			vars += ", channel: " + hook.Channel
		}

		if hook.Nickname != "" {
			vars += ", nickname: " + hook.Nickname
		}

		if len(hook.Exclude) > 0 {
			vars += ", exclude: \"" + strings.Join(hook.Exclude, "; ") + `"`
		}

		u.Printf("%s: %s, timeout: %v, ignore ssl: %v, silent: %v%s, events: %q",
			prefix, hook.Name, hook.Timeout, hook.IgnoreSSL, hook.Silent, vars, logEvents(hook.Events))
	}
}

// logEvents is only used in logWebhook to format events for printing.
func logEvents(events []ExtractStatus) string {
	if len(events) == 1 && events[0] == WAITING {
		return "all"
	}

	var output string

	for _, event := range events {
		if len(output) > 0 {
			output += "; "
		}

		output += event.String()
	}

	return output
}

// Excluded returns true if an app is in the Exclude slice.
func (w *WebhookConfig) Excluded(app starr.App) bool {
	for _, exclude := range w.Exclude {
		if strings.EqualFold(exclude, string(app)) {
			return true
		}
	}

	return false
}

// HasEvent returns true if a status event is in the Events slice.
// Also returns true if the Events slice has only one value of WAITING.
func (w *WebhookConfig) HasEvent(e ExtractStatus) bool {
	for _, status := range w.Events {
		if (status == WAITING && len(w.Events) == 1) || status == e {
			return true
		}
	}

	return false
}

// WebhookCounts returns the total count of requests and errors for all webhooks.
func (u *Unpackerr) WebhookCounts() (uint, uint) {
	var total, fails uint

	for _, hook := range u.Webhook {
		posts, failures := hook.Counts()
		total += posts
		fails += failures
	}

	return total, fails
}

// Counts returns the total count of requests and failures for a webhook.
func (w *WebhookConfig) Counts() (uint, uint) {
	w.Lock()
	defer w.Unlock()

	return w.posts, w.fails
}

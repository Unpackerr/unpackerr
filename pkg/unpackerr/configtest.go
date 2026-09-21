package unpackerr

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/cnfg"
	"golift.io/starr"
)

const (
	maxTestTimeout = time.Minute
	maxHookReply   = 2048
	defaultTestTO  = 10 * time.Second
)

var errSectionNotTestable = errors.New("this section cannot be tested")

// configTestRequest is POST /api/config/{section}/test.
// Starr uses url/apiKey/valid_ssl/timeout. Hooks use event/app and hook fields.
// Hook shell/ignoreSsl are pointers so omitted JSON keeps the live values.
type configTestRequest struct {
	Slug         string            `json:"slug"`
	URL          string            `json:"url"`
	APIKey       string            `json:"apiKey"`
	ValidSSL     bool              `json:"valid_ssl"`
	Timeout      cnfg.Duration     `json:"timeout"`
	Event        string            `json:"event"`
	App          string            `json:"app"`
	Command      string            `json:"command"`
	Token        string            `json:"token"`
	ContentType  string            `json:"contentType"`
	Template     string            `json:"template"`
	TemplatePath string            `json:"templatePath"`
	Shell        *bool             `json:"shell"`
	IgnoreSSL    *bool             `json:"ignoreSsl"`
	Nickname     string            `json:"nickname"`
	Channel      string            `json:"channel"`
	Name         string            `json:"name"`
	Headers      map[string]string `json:"headers"`
}

type starrTestResult struct {
	Queued    int           `json:"queued"`
	Retrieved int           `json:"retrieved"`
	Torrents  int           `json:"torrents"`
	Nzbs      int           `json:"nzbs"`
	Other     int           `json:"other,omitempty"`
	Elapsed   cnfg.Duration `json:"elapsed"`
}

type hookTestResult struct {
	Status  string        `json:"status"`
	Reply   string        `json:"reply,omitempty"`
	Elapsed cnfg.Duration `json:"elapsed"`
}

type configTestError struct {
	Error   string        `json:"error"`
	Elapsed cnfg.Duration `json:"elapsed"`
}

type starrProbe struct {
	URL, APIKey, HTTPUser, HTTPPass, Username, Password string
	Timeout                                             time.Duration
}

func (u *Unpackerr) configTestHandler(response http.ResponseWriter, request *http.Request) {
	section := ConfigSection(request.PathValue("section"))

	raw, err := decodeJSONBody(response, request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var body configTestRequest
	if err := unmarshalStrict(raw, &body); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	switch section {
	case SectionSonarr, SectionRadarr, SectionLidarr, SectionReadarr:
		u.testStarrSection(response, section, body)
	case SectionWebhooks, SectionCmdhooks:
		u.testHookSection(request.Context(), response, section, body)
	default:
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errSectionNotTestable.Error()})
	}
}

func (u *Unpackerr) testStarrSection(response http.ResponseWriter, section ConfigSection, body configTestRequest) {
	app, cfg, err := u.starrTestConfig(section, body)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	started := time.Now()
	result, err := probeStarrQueue(app, cfg)
	result.Elapsed = sinceTest(started)

	if err != nil {
		writeJSON(response, http.StatusFailedDependency, configTestError{Error: err.Error(), Elapsed: result.Elapsed})
		return
	}

	u.Printf("[%s] Test (%s): %d queued, %d retrieved, %d torrents, %d nzbs, elapsed %s",
		app, cfg.URL, result.Queued, result.Retrieved, result.Torrents, result.Nzbs, result.Elapsed)
	writeJSON(response, http.StatusOK, result)
}

func (u *Unpackerr) starrTestConfig(section ConfigSection, body configTestRequest) (starr.App, StarrConfig, error) {
	app, ok := sectionStarrApp(section)
	if !ok {
		return "", StarrConfig{}, errSectionNotTestable
	}

	probe, global := u.liveStarrProbe(section, strings.TrimSpace(body.Slug))
	url := firstNonEmpty(strings.TrimSpace(body.URL), probe.URL)
	key := firstNonEmpty(strings.TrimSpace(body.APIKey), probe.APIKey)
	timeout := clampTestTimeout(body.Timeout.Duration, probe.Timeout, global)

	cfg := StarrConfig{
		URL:      url,
		APIKey:   key,
		HTTPUser: probe.HTTPUser,
		HTTPPass: probe.HTTPPass,
		Username: probe.Username,
		Password: probe.Password,
		Client:   starr.Client(timeout, body.ValidSSL),
		ValidSSL: body.ValidSSL,
		Timeout:  cnfg.Duration{Duration: timeout},
	}

	if err := expandFilepaths(&cfg); err != nil {
		return "", StarrConfig{}, err
	}

	if err := requireStarrTestAccess(cfg); err != nil {
		return "", StarrConfig{}, err
	}

	return app, cfg, nil
}

func requireStarrTestAccess(cfg StarrConfig) error {
	if cfg.URL == "" {
		return ErrInvalidURL
	}

	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return ErrInvalidURL
	}

	if len(cfg.APIKey) < apiKeyMinLength {
		return ErrInvalidKey
	}

	return nil
}

func (u *Unpackerr) liveStarrProbe(section ConfigSection, slug string) (starrProbe, time.Duration) {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	global := u.Timeout.Duration
	if slug == "" {
		return starrProbe{}, global
	}

	var cfg *StarrConfig

	switch section {
	case SectionSonarr:
		if item := u.Sonarr[slug]; item != nil {
			cfg = &item.StarrConfig
		}
	case SectionRadarr:
		if item := u.Radarr[slug]; item != nil {
			cfg = &item.StarrConfig
		}
	case SectionLidarr:
		if item := u.Lidarr[slug]; item != nil {
			cfg = &item.StarrConfig
		}
	case SectionReadarr:
		if item := u.Readarr[slug]; item != nil {
			cfg = &item.StarrConfig
		}
	}

	if cfg == nil {
		return starrProbe{}, global
	}

	return starrProbe{
		URL:      cfg.URL,
		APIKey:   cfg.APIKey,
		HTTPUser: cfg.HTTPUser,
		HTTPPass: cfg.HTTPPass,
		Username: cfg.Username,
		Password: cfg.Password,
		Timeout:  cfg.Timeout.Duration,
	}, global
}

func probeStarrQueue(app starr.App, cfg StarrConfig) (starrTestResult, error) {
	switch app {
	case starr.Sonarr:
		return runStarrProbe[SonarrConfig, *SonarrConfig](cfg)
	case starr.Radarr:
		return runStarrProbe[RadarrConfig, *RadarrConfig](cfg)
	case starr.Lidarr:
		return runStarrProbe[LidarrConfig, *LidarrConfig](cfg)
	case starr.Readarr:
		return runStarrProbe[ReadarrConfig, *ReadarrConfig](cfg)
	default:
		return starrTestResult{}, errSectionNotTestable
	}
}

func runStarrProbe[T any, P starrApp[T]](cfg StarrConfig) (starrTestResult, error) {
	var zero T

	server := asStarr[T, P](&zero)
	*server.conf() = cfg
	server.connect()

	bind, total, retrieved, err := server.pollQueue()
	if err != nil {
		return starrTestResult{}, err
	}

	bind()

	out := starrTestResult{Queued: total, Retrieved: retrieved}

	for _, rec := range server.queueViews() {
		switch rec.Protocol {
		case starr.ProtocolTorrent:
			out.Torrents++
		case starr.ProtocolUsenet:
			out.Nzbs++
		default:
			out.Other++
		}
	}

	return out, nil
}

func (u *Unpackerr) testHookSection(
	ctx context.Context, response http.ResponseWriter, section ConfigSection, body configTestRequest,
) {
	hook, event, app, err := u.hookTestConfig(section, body)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	payload := hooks.SamplePayload()
	if err := hooks.PrepareSample(payload, event); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	payload.App = app
	u.decorateSamplePayload(payload, event)

	started := time.Now()
	reply, err := hooks.Fire(ctx, hook, payload)
	elapsed := sinceTest(started)

	if err != nil {
		writeJSON(response, http.StatusFailedDependency, configTestError{Error: err.Error(), Elapsed: elapsed})
		return
	}

	kind := "Webhook"
	if section == SectionCmdhooks {
		kind = "Cmdhook"
	}

	u.Printf("[%s] Test (%s = %s): %s: OK (%s)", kind, payload.Path, payload.Event, hook.Name, elapsed)
	writeJSON(response, http.StatusOK, hookTestResult{Status: "ok", Reply: clipReply(reply), Elapsed: elapsed})
}

func (u *Unpackerr) hookTestConfig(
	section ConfigSection, body configTestRequest,
) (*hooks.Config, extract.Status, starr.App, error) {
	hook, global := u.liveHookClone(section, strings.TrimSpace(body.Slug))
	if hook == nil {
		hook = &hooks.Config{}
	}

	overlayHook(hook, body)
	u.overlayEnvHookHeaders(section, strings.TrimSpace(body.Slug), hook)

	if section != SectionCmdhooks {
		if err := hooks.ValidateHeaders(hook.Headers); err != nil {
			return nil, 0, "", fmt.Errorf("validating headers: %w", err)
		}
	}

	if err := expandFilepaths(hook); err != nil {
		return nil, 0, "", err
	}

	timeout := clampTestTimeout(body.Timeout.Duration, hook.Timeout.Duration, global)
	hook.Timeout.Duration = timeout

	if section == SectionCmdhooks {
		hook.URL = ""

		hook.Command = strings.TrimSpace(expandHomedir(hook.Command))
		if hook.Command == "" {
			return nil, 0, "", hooks.ErrCmdhookNoCmd
		}
	} else {
		hook.Command = ""
		if strings.TrimSpace(hook.URL) == "" {
			return nil, 0, "", hooks.ErrWebhookNoURL
		}
	}

	if hook.Name == "" {
		hook.Name = "test"
	}

	if hook.CType == "" {
		hook.CType = hooks.DefaultContentType(hook.TempName, hook.URL)
	}

	event, err := parseTestEvent(body.Event)
	if err != nil {
		return nil, 0, "", err
	}

	return hook, event, testHookApp(body.App), nil
}

func overlayHook(hook *hooks.Config, body configTestRequest) {
	if v := strings.TrimSpace(body.URL); v != "" {
		hook.URL = v
	}

	if v := strings.TrimSpace(body.Command); v != "" {
		hook.Command = v
	}

	if v := strings.TrimSpace(body.Token); v != "" {
		hook.Token = v
	}

	if v := strings.TrimSpace(body.ContentType); v != "" {
		hook.CType = v
	}

	if v := strings.TrimSpace(body.Template); v != "" {
		hook.TempName = v
	}

	if v := strings.TrimSpace(body.TemplatePath); v != "" {
		hook.TmplPath = v
	}

	if v := strings.TrimSpace(body.Nickname); v != "" {
		hook.Nickname = v
	}

	if v := strings.TrimSpace(body.Channel); v != "" {
		hook.Channel = v
	}

	if v := strings.TrimSpace(body.Name); v != "" {
		hook.Name = v
	}

	if body.Shell != nil {
		hook.Shell = *body.Shell
	}

	if body.IgnoreSSL != nil {
		hook.IgnoreSSL = *body.IgnoreSSL
	}

	if body.Headers != nil {
		hook.Headers = maps.Clone(body.Headers)
	}
}

// overlayEnvHookHeaders puts UN_WEBHOOK_<slug>_HEADERS_* (and cmdhook) children
// back after a posted headers map replaced the live clone wholesale.
func (u *Unpackerr) overlayEnvHookHeaders(section ConfigSection, slug string, hook *hooks.Config) {
	if hook == nil || slug == "" {
		return
	}

	var tag string

	switch section {
	case SectionWebhooks:
		tag = "WEBHOOK"
	case SectionCmdhooks:
		tag = "CMDHOOK"
	default:
		return
	}

	prefix := tag + "_" + slug + "_HEADERS_"

	for key, val := range u.envUsed {
		name, ok := strings.CutPrefix(key, prefix)
		if !ok || name == "" {
			continue
		}

		if hook.Headers == nil {
			hook.Headers = make(hooks.HeaderMap)
		}

		hook.Headers[name] = val
	}
}

func (u *Unpackerr) liveHookClone(section ConfigSection, slug string) (*hooks.Config, time.Duration) {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	global := u.Timeout.Duration
	if slug == "" {
		return nil, global
	}

	var src *hooks.Config

	switch section {
	case SectionWebhooks:
		src = u.Webhook[slug]
	case SectionCmdhooks:
		src = u.Cmdhook[slug]
	}

	if src == nil {
		return nil, global
	}

	return hooks.CloneList([]*hooks.Config{src})[0], global
}

func parseTestEvent(raw string) (extract.Status, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return extract.EXTRACTED, nil
	}

	var event extract.Status
	if err := event.UnmarshalText([]byte(raw)); err != nil {
		return 0, fmt.Errorf("event: %w", err)
	}

	return event, nil
}

func testHookApp(name string) starr.App {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "sonarr":
		return starr.Sonarr
	case "radarr":
		return starr.Radarr
	case "lidarr":
		return starr.Lidarr
	case "readarr":
		return starr.Readarr
	case "folder":
		return FolderString
	default:
		return starr.App(strings.TrimSpace(name))
	}
}

func sectionStarrApp(section ConfigSection) (starr.App, bool) {
	switch section {
	case SectionSonarr:
		return starr.Sonarr, true
	case SectionRadarr:
		return starr.Radarr, true
	case SectionLidarr:
		return starr.Lidarr, true
	case SectionReadarr:
		return starr.Readarr, true
	default:
		return "", false
	}
}

func clampTestTimeout(explicit, live, global time.Duration) time.Duration {
	out := explicit
	if out <= 0 {
		out = live
	}

	if out <= 0 {
		out = global
	}

	if out <= 0 {
		out = defaultTestTO
	}

	if out > maxTestTimeout {
		return maxTestTimeout
	}

	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func sinceTest(started time.Time) cnfg.Duration {
	return cnfg.Duration{Duration: time.Since(started).Round(time.Millisecond)}
}

func clipReply(reply string) string {
	if utf8.RuneCountInString(reply) <= maxHookReply {
		return reply
	}

	runes := []rune(reply)

	return string(runes[:maxHookReply]) + "…"
}

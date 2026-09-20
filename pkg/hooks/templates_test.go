package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/starr"
)

func TestNotifiarrTemplateCustomIDs(t *testing.T) {
	t.Parallel()

	payload := &Payload{
		Path:       "/dl",
		App:        starr.Sonarr,
		IDs:        map[string]any{"title": "Show"},
		CustomIDs:  map[string]string{"url": "https://unpackerr.example"},
		Event:      extract.EXTRACTED,
		Retries:    3,
		EventTitle: "Archive Found",
		Time:       time.Unix(0, 0).UTC(),
	}

	body := renderHookTemplate(t, "notifiarr", payload)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, body)
	}

	ids, _ := parsed["ids"].(map[string]any)
	if ids["title"] != "Show" {
		t.Fatalf("ids %+v", ids)
	}

	if _, ok := ids["url"]; ok {
		t.Fatalf("custom id merged into ids: %+v", ids)
	}

	custom, _ := parsed["customIDs"].(map[string]any)
	if custom["url"] != "https://unpackerr.example" {
		t.Fatalf("customIDs %+v body=%s", custom, body)
	}

	if parsed["retries"] != float64(3) {
		t.Fatalf("retries %+v", parsed["retries"])
	}

	if parsed["eventTitle"] != "Archive Found" {
		t.Fatalf("eventTitle %+v", parsed["eventTitle"])
	}
}

func TestBuiltinTemplatesEncodeApp(t *testing.T) {
	t.Parallel()

	const app = "Sports & News+"

	payload := &Payload{
		Path:  "/dl/show",
		App:   starr.App(app),
		IDs:   map[string]any{"title": "Episode"},
		Event: extract.EXTRACTED,
		Time:  time.Unix(0, 0).UTC(),
		Data:  &XtractPayload{},
	}

	for _, name := range []string{"notifiarr", "discord", "telegram", "slack", "pushover", "gotify"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := renderHookTemplate(t, name, payload)

			switch name {
			case "pushover":
				assertPushoverApp(t, body, app)
			case "telegram":
				if strings.Contains(body, app) {
					t.Fatalf("telegram HTML left %q unescaped", app)
				}

				if !strings.Contains(body, "Sports &amp; News+") {
					t.Fatalf("telegram missing html-escaped app:\n%s", body)
				}
			default:
				assertJSONApp(t, name, body, app)
			}
		})
	}
}

func renderHookTemplate(t *testing.T, name string, payload *Payload) string {
	t.Helper()

	tmpl, err := (&Config{
		TempName: name,
		Nickname: "bot",
		Token:    "tok",
		Channel:  "chan",
	}).Template()
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, payload); err != nil {
		t.Fatal(err)
	}

	return buf.String()
}

func TestBuiltinTemplatesEncodeEventTitle(t *testing.T) {
	t.Parallel()

	const title = `Done "now" & <gone>`

	payload := &Payload{
		Path:       "/dl",
		App:        "Sonarr",
		IDs:        map[string]any{"title": "Show"},
		Event:      extract.EXTRACTED,
		EventTitle: title,
		Time:       time.Unix(0, 0).UTC(),
		Data:       &XtractPayload{},
	}

	for _, name := range []string{"notifiarr", "discord", "telegram", "slack", "gotify"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := renderHookTemplate(t, name, payload)

			var parsed any
			if err := json.Unmarshal([]byte(body), &parsed); err != nil {
				t.Fatalf("invalid json: %v\n%s", err, body)
			}

			if name != "telegram" {
				if !strings.Contains(fmt.Sprint(parsed), `Done "now"`) {
					t.Fatalf("missing title:\n%s", body)
				}

				return
			}

			text, _ := parsed.(map[string]any)["text"].(string)
			if strings.Contains(text, "<gone>") {
				t.Fatalf("telegram HTML left raw: %q", text)
			}

			if !strings.Contains(text, "&lt;gone&gt;") {
				t.Fatalf("telegram missing escaped title: %q", text)
			}
		})
	}
}

func TestBuiltinTemplatesEmptyEventTitleFallsBack(t *testing.T) {
	t.Parallel()

	want := extract.EXTRACTED.Desc()
	payload := &Payload{
		Path:  "/dl",
		App:   "Sonarr",
		IDs:   map[string]any{"title": "Show"},
		Event: extract.EXTRACTED,
		Time:  time.Unix(0, 0).UTC(),
		Data:  &XtractPayload{},
	}

	if payload.Title() != want {
		t.Fatalf("Title() %q", payload.Title())
	}

	for _, name := range []string{"notifiarr", "discord", "telegram", "slack", "gotify", "pushover"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := renderHookTemplate(t, name, payload)
			if name == "pushover" {
				if !strings.Contains(body, url.QueryEscape(want)) {
					t.Fatalf("pushover missing fallback %q:\n%s", want, body)
				}

				return
			}

			if !strings.Contains(body, want) {
				t.Fatalf("missing fallback %q:\n%s", want, body)
			}
		})
	}
}

func TestBuiltinTemplatesRenderDataBytes(t *testing.T) {
	t.Parallel()

	const size uint64 = 2048

	payload := &Payload{
		Path:  "/dl",
		App:   "Sonarr",
		IDs:   map[string]any{"title": "Show"},
		Event: extract.EXTRACTED,
		Time:  time.Unix(0, 0).UTC(),
		Data:  &XtractPayload{Bytes: size},
	}

	want := humanbytes(size)

	for _, name := range []string{"notifiarr", "discord", "telegram", "slack", "gotify", "pushover"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := renderHookTemplate(t, name, payload)
			if name == "notifiarr" {
				if !strings.Contains(body, "2048") {
					t.Fatalf("missing bytes:\n%s", body)
				}

				return
			}

			if !strings.Contains(body, want) {
				t.Fatalf("missing %q:\n%s", want, body)
			}
		})
	}
}

func assertPushoverApp(t *testing.T, body, app string) {
	t.Helper()

	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatal(err)
	}

	msg := values.Get("message")
	if msg == "" {
		t.Fatalf("pushover message empty; keys=%v body=%q", values, body)
	}

	if strings.Contains(msg, app) {
		t.Fatalf("pushover HTML left %q unescaped: %q", app, msg)
	}

	if !strings.Contains(msg, "Sports &amp; News+") {
		t.Fatalf("pushover missing html-escaped app: %q", msg)
	}
}

func assertJSONApp(t *testing.T, name, body, app string) {
	t.Helper()

	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("%s invalid json: %v\n%s", name, err, body)
	}

	if !strings.Contains(fmt.Sprint(parsed), app) {
		t.Fatalf("%s missing app %q:\n%s", name, app, body)
	}
}

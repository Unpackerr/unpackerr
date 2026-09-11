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

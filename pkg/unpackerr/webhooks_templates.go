package unpackerr

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"golift.io/cnfg"
)

// WebhookPayload defines the data sent to notifarr.com (and other) webhooks.
type WebhookPayload struct {
	Path  string                 `json:"path"`                // Path for the extracted item.
	App   string                 `json:"app"`                 // Application Triggering Event
	IDs   map[string]interface{} `json:"ids,omitempty"`       // Arbitrary IDs from each app.
	Event ExtractStatus          `json:"unpackerr_eventtype"` // The type of the event.
	Time  time.Time              `json:"time"`                // Time of this event.
	Data  *XtractPayload         `json:"data,omitempty"`      // Payload from extraction process.
	// Application Metadata.
	Go       string    `json:"go_version"` // Version of go compiled with
	OS       string    `json:"os"`         // Operating system: linux, windows, darwin
	Arch     string    `json:"arch"`       // Architecture: amd64, armhf
	Version  string    `json:"version"`    // Application Version
	Revision string    `json:"revision"`   // Application Revision
	Branch   string    `json:"branch"`     // Branch built from.
	Started  time.Time `json:"started"`    // App start time.
}

// XtractPayload is a rewrite of xtractr.Response.
type XtractPayload struct {
	Error    string        `json:"error,omitempty"`      // error only during extractfailed
	Archives []string      `json:"archives,omitempty"`   // list of all archive files extracted
	Files    []string      `json:"files,omitempty"`      // list of all files extracted
	Start    time.Time     `json:"start,omitempty"`      // start time of extraction
	Output   string        `json:"tmp_folder,omitempty"` // temporary items folder
	Bytes    int64         `json:"bytes,omitempty"`      // Bytes written
	Elapsed  cnfg.Duration `json:"elapsed,omitempty"`    // Duration as a string: 5m32s
	Queue    int           `json:"queue,omitempty"`      // Extraction Queue Size
}

// WebhookTemplateNotifiarr is the default template
// when not using discord.com (below), or a custom template file.
const WebhookTemplateNotifiarr = `{
  "path": "{{.Path}}",
  "app": "{{.App}}",
  "ids": {
    {{$s := separator ",\n"}}{{range $key, $value := .IDs}}{{call $s}}"{{$key}}": "{{$value}}"{{end}}
  },
  "unpackerr_eventtype": "{{.Event}}",
  "time": "{{.Time}}",
{{ if .Data }}    "data": {
    "error": "{{.Data.Error}}",
    "archives": [{{$s := separator ","}}{{range $index, $value := .Data.Archives}}{{call $s}}"{{$value}}"{{end}}],
    "files": [{{$s := separator ","}}{{range $index, $value := .Data.Files}}{{call $s}}"{{$value}}"{{end}}],
    "start": "{{.Data.Start}}",
    "tmp_folder": "{{.Data.Output}}",
    "bytes": "{{.Data.Bytes}}",
    "elapsed": "{{.Data.Elapsed}}"
    },
{{ end }}    "go_version": "{{.Go}}",
  "os": "{{.OS}}",
  "arch": "{{.Arch}}",
  "version": "{{.Version}}",
  "revision": "{{.Revision}}",
  "branch": "{{.Branch}}",
  "started": "{{.Started}}"
}
`

// WebhookTemplateDiscord is used when sending a webhook to discord.com.
const WebhookTemplateDiscord = `{
  "username": "{{nickname}}",
  "avatar_url": "https://raw.githubusercontent.com/wiki/davidnewhall/unpackerr/images/logo.png",
  "embeds": [
    {
     "title": "{{index .IDs "title"}}",
     "timestamp": "{{timestamp .Time}}",
     "author": {
       "name": "Unpackerr: {{.Event.Desc}}",
       "icon_url": "https://raw.githubusercontent.com/wiki/davidnewhall/unpackerr/images/logo.png"
     },
     "fields": [
       {"name": "Path", "value": "{{.Path}}", "inline": false},
       {"name": "App", "value": "{{.App}}", "inline": true}{{ if .Data }},
       {"name": "Elapsed", "value": "{{.Data.Elapsed}}", "inline": true},
       {"name": "Archives", "value": "{{len .Data.Archives}}", "inline": true},
       {"name": "Files", "value": "{{len .Data.Files}}", "inline": true},
       {"name": "Size", "value": "{{humanbytes .Data.Bytes}}", "inline": true},
       {"name": "Queue", "value": "{{.Data.Queue}}", "inline": true}{{- if .Data.Error }},
       {"name": "Error", "value": "{{.Data.Error}}", "inline": false}{{ end }}{{ end }}
     ],
     "footer": {"text": "v{{.Version}}-{{.Revision}} ({{.OS}}/{{.Arch}})"}
   }
  ]
}
`

// WebhookTemplateSlack is a built-in template for sending a message to Slack.
const WebhookTemplateSlack = `
{
  "username": "{{nickname}}",
  {{if channel}}"channel": "{{channel}}",{{end}}
  "icon_url": "https://raw.githubusercontent.com/wiki/davidnewhall/unpackerr/images/logo.png",
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "Unpackerr: {{.Event.Desc}}"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": ":star: *{{index .IDs "title"}}*"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Path*: {{.Path}}"
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Version*\nv{{.Version}}-{{.Revision}}"
        },
        {
          "type": "mrkdwn",
          "text": "*OS (Go)*\n{{.OS}} ({{.Arch}}/{{.Go}})"
        },
        {
          "type": "mrkdwn",
          "text": "*App*\n{{.App}}"
        }{{ if .Data }},
        {
          "type": "mrkdwn",
          "text": "*Size*\n{{humanbytes .Data.Bytes}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Archives*\n{{len .Data.Archives}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Files*\n{{len .Data.Files}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Queue*\n{{.Data.Queue}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Elapsed*\n{{.Data.Elapsed}}"
        }{{end}}
      ]
    }{{if .Data}}{{if .Data.Error}},
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Error*: {{.Data.Error}}"
      }
    }{{end}}{{end}}
  ]
}
`

const byteUnit = 1024

// Template returns a template specific to this webhook.
func (w *WebhookConfig) Template() (*template.Template, error) {
	separator := func(s string) func() string {
		var i bool

		return func() string {
			if !i {
				i = true
				return ""
			}

			return s
		}
	}

	humanbytes := func(size int64) string {
		// This is from https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
		// This func converts an int to a human readable byte string.
		if size < byteUnit {
			return fmt.Sprintf("%dB", size)
		}

		div, exp := int64(byteUnit), 0

		for n := size / byteUnit; n >= byteUnit; n /= byteUnit {
			div *= byteUnit
			exp++
		}

		return fmt.Sprintf("%.1f%ciB", float64(size)/float64(div), "KMGTPE"[exp])
	}

	template := template.New("payload").Funcs(template.FuncMap{
		"separator":  separator,
		"humanbytes": humanbytes,
		"nickname":   func() string { return w.Nickname },
		"channel":    func() string { return w.Channel },
		"timestamp":  func(t time.Time) string { return t.Format(time.RFC3339) },
	})

	// Figure out which template to use based on URL or template_path.
	switch url := strings.ToLower(w.URL); {
	default:
		fallthrough
	case strings.Contains(url, "discordnotifier.com") || strings.Contains(url, "notifiarr.com"):
		return template.Parse(WebhookTemplateNotifiarr)
	case w.TmplPath != "":
		return template.ParseFiles(w.TmplPath)
	case strings.Contains(url, "discord.com"):
		return template.Parse(WebhookTemplateDiscord)
	case strings.Contains(url, "hooks.slack.com"):
		return template.Parse(WebhookTemplateSlack)
	}
}

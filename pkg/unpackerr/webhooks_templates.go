package unpackerr

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
)

// WebhookPayload defines the data sent to notifarr.com (and other) webhooks.
type WebhookPayload struct {
	Path   string         `json:"path"`                // Path for the extracted item.
	App    starr.App      `json:"app"`                 // Application Triggering Event
	IDs    map[string]any `json:"ids,omitempty"`       // Arbitrary IDs from each app.
	Event  ExtractStatus  `json:"unpackerr_eventtype"` // The type of the event.
	Time   time.Time      `json:"time"`                // Time of this event.
	Data   *XtractPayload `json:"data,omitempty"`      // Payload from extraction process.
	Config *WebhookConfig `json:"-"`                   // Payload from extraction process.
	// Application Metadata.
	Go       string    `json:"go"`       // Version of go compiled with
	OS       string    `json:"os"`       // Operating system: linux, windows, darwin
	Arch     string    `json:"arch"`     // Architecture: amd64, armhf
	Version  string    `json:"version"`  // Application Version
	Revision string    `json:"revision"` // Application Revision
	Branch   string    `json:"branch"`   // Branch built from.
	Started  time.Time `json:"started"`  // App start time.
}

// XtractPayload is a rewrite of xtractr.Response.
type XtractPayload struct {
	Error    string        `json:"error,omitempty"`    // error only during extractfailed
	Archive  []string      `json:"archive,omitempty"`  // list of all archive files extracted
	Archives StringSlice   `json:"archives,omitempty"` // list of all archive files extracted
	Files    StringSlice   `json:"files,omitempty"`    // list of all files extracted
	File     []string      `json:"file,omitempty"`     // list of all files extracted
	Start    time.Time     `json:"start,omitempty"`    // start time of extraction
	Output   string        `json:"output,omitempty"`   // temporary items folder
	Bytes    int64         `json:"bytes,omitempty"`    // Bytes written
	Elapsed  cnfg.Duration `json:"elapsed,omitempty"`  // Duration as a string: 5m32s
	Queue    int           `json:"queue,omitempty"`    // Extraction Queue Size
}

// WebhookTemplateNotifiarr is the default template
// when not using discord.com (below), or a custom template file.
const WebhookTemplateNotifiarr = `{
  "path": {{encode .Path}},
  "app": "{{.App}}",
  "ids": {
    {{$s := separator ",\n"}}{{range $key, $value := .IDs}}{{call $s}}"{{$key}}": {{encode $value}}{{end}}
  },
  "unpackerr_eventtype": "{{.Event}}",
  "time": "{{.Time}}",
{{ if .Data }}    "data": {
    "error": {{encode .Data.Error}},
    "archives": [{{$s := separator ","}}{{range $index, $value := .Data.Archives}}{{call $s}}"{{$value}}"{{end}}],
    "files": [{{$s := separator ","}}{{range $index, $value := .Data.Files}}{{call $s}}"{{$value}}"{{end}}],
    "start": "{{.Data.Start}}",
    "tmp_folder": {{encode .Data.Output}},
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

const WebhookTemplateTelegram = `{
  "chat_id": "{{nickname}}",
  "parse_mode": "HTML",
  "text": "<b><a href=\"https://github.com/Unpackerr/unpackerr/releases\">Unpackerr</a></b>: {{.Event.Desc -}}
    \n<b>Title</b>: {{rawencode (index .IDs "title") -}}
    \n<b>App</b>: {{.App -}}
    \n\n<b>Path</b>: <code>{{rawencode .Path}}</code>
  {{- if .Data }}\n
    {{- if .Data.Elapsed.Duration}}\n <b>Elapsed</b>: {{.Data.Elapsed}}{{end -}}
    {{ if .Data.Archives}}\n <b>Archives</b>: {{len .Data.Archives}}{{end -}}
    {{ if .Data.Files}}\n <b>Files</b>: {{len .Data.Files}}{{end -}}
    {{ if .Data.Bytes}}\n <b>Size</b>: {{humanbytes .Data.Bytes}}{{end -}}
    {{ if and (gt .Event 1) (lt .Event 5)}}\n <b>Queue</b>: {{.Data.Queue}}{{end -}}
    {{ if .Data.Error}}\n\n <b>ERROR</b>: <pre>{{rawencode .Data.Error}}</pre>\n{{end -}}
  {{end -}}"
}
`

// The extra spaces before the newlines here are required to make this look good on web and on android.
const WebhookTemplateGotify = `{
  "title": "{{if nickname}}{{nickname}}{{else}}Unpackerr{{end}}: {{.Event.Desc}}",
  "message": "**App**: {{.App}}  \n**Name**: {{rawencode (index .IDs "title")}}  \n**Path**: {{rawencode .Path -}}
    {{ if .Data.Elapsed.Duration }}  \n**Elapsed**: {{.Data.Elapsed}}{{end -}}
    {{ if .Data.Archives }}  \n**RARs**: {{len .Data.Archives}}{{end -}}
    {{ if .Data.Files }}  \n**Files**: {{len .Data.Files}}{{end -}}
    {{ if .Data.Bytes }}  \n**Bytes**: {{humanbytes .Data.Bytes}}{{end -}}
    {{ if and (gt .Event 1) (lt .Event 5) }}  \n**Queue**: {{.Data.Queue}}{{end -}}
    {{ if .Data.Error}}  \n**ERROR**:\n~~~\n{{rawencode .Data.Error}}\n~~~{{end}}",
  "extras": {
    "client::display": {
      "contentType": "text/markdown"
    },
    "client::notification": {
      "click": {
          "url": ""
      },
      "bigImageUrl": ""
    }
  }
}
`

// WebhookTemplateDiscord is used when sending a webhook to discord.com.
const WebhookTemplateDiscord = `{
  "username": "{{nickname}}",
  "avatar_url": "https://raw.githubusercontent.com/wiki/Unpackerr/unpackerr/images/logo.png",
  "embeds": [{
    "title": {{encode (index .IDs "title")}},
    "timestamp": "{{timestamp .Time}}",
    "author": {
     "name": "Unpackerr: {{.Event.Desc}}",
     "icon_url": "https://raw.githubusercontent.com/wiki/Unpackerr/unpackerr/images/logo.png",
     "url": "https://github.com/Unpackerr/unpackerr/releases"
    },
    "color": {{ if (eq 1 .Event)}}1752220
            {{- else if (eq 2 .Event)}}16384255
            {{- else if(eq 3 .Event)}}10038562
            {{- else if(eq 4 .Event)}}786176
            {{- else if(eq 5 .Event)}}12745742
            {{- else}}16711695{{end}},
    "fields": [
     {"name": "Path", "value": {{encode .Path}}, "inline": false},
     {"name": "App", "value": "{{.App}}", "inline": true}{{ if .Data }}
     {{ if .Data.Archives}},{"name": "Archives", "value": "{{len .Data.Archives}}", "inline": true}
     {{end -}}
     {{ if .Data.Elapsed.Duration}},{"name": "Elapsed", "value": "{{.Data.Elapsed}}", "inline": true}
     {{end -}}
     {{ if .Data.Files}},{"name": "Files", "value": "{{len .Data.Files}}", "inline": true}
     {{end -}}
     {{ if .Data.Bytes}},{"name": "Size", "value": "{{humanbytes .Data.Bytes}}", "inline": true}
     {{end -}}
     {{ if and (gt .Event 1) (lt .Event 5)}},{"name": "Queue", "value": "{{.Data.Queue}}", "inline": true}
     {{end -}}
     {{ if .Data.Error }},{"name": "Error", "value": {{encode .Data.Error}}, "inline": false}
     {{end}}{{end -}}
    ],
    "footer": {
     "text": "v{{.Version}}-{{.Revision}} ({{.OS}}/{{.Arch}})",
     "icon_url": "https://docs.golift.io/integrations/golift.png"
    }
  }]
}
`

const WebhookTemplatePushover = `token={{token}}&user={{channel}}&html=1&title={{formencode .Event.Desc}}&` +
	`{{if nickname}}device={{nickname}}&{{end}}message=<pre><b>App</b>: {{.App}}
<b>Name</b>: {{formencode (index .IDs "title")}}
<b>Path</b>: {{formencode .Path}}
{{ if .Data -}}
{{ if .Data.Elapsed.Duration}}<b>Time</b>: {{.Data.Elapsed}}
{{end}}{{ if .Data.Archives}}<b>RARs</b>: {{len .Data.Archives}}
{{end}}{{ if .Data.Files}}<b>Files</b>: {{len .Data.Files}}
{{end}}{{ if .Data.Bytes}}<b>Bytes</b>: {{humanbytes .Data.Bytes}}
{{end}}{{ if and (gt .Event 1) (lt .Event 5)}}<b>Queue</b>: {{.Data.Queue}}
{{end}}{{ if .Data.Error}}
<font color="#FF0000"><b>ERROR</b>: {{formencode .Data.Error}}</font>
{{end}}{{end -}}</pre>`

// WebhookTemplateSlack is a built-in template for sending a message to Slack.
const WebhookTemplateSlack = `
{
  "username": "{{nickname}}",
  {{if channel}}"channel": "{{channel}}",{{end}}
  "icon_url": "https://raw.githubusercontent.com/wiki/Unpackerr/unpackerr/images/logo.png",
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
        "text": {{encode (print ":star: *" (index .IDs "title") "*")}}
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": {{encode (print "*Path*: " .Path)}}
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
        }{{ if .Data }}
        {{ if .Data.Bytes }},{
          "type": "mrkdwn",
          "text": "*Size*\n{{humanbytes .Data.Bytes}}"
        }{{end -}}
        {{ if .Data.Archives }},{
          "type": "mrkdwn",
          "text": "*Archives*\n{{len .Data.Archives}}"
        }{{end -}}
        {{ if .Data.Files }},{
          "type": "mrkdwn",
          "text": "*Files*\n{{len .Data.Files}}"
        }{{end -}}
        {{ if and (gt .Event 1) (lt .Event 5)}},{
          "type": "mrkdwn",
          "text": "*Queue*\n{{.Data.Queue}}"
        }{{end -}}
        {{ if .Data.Elapsed.Duration }},{
          "type": "mrkdwn",
          "text": "*Elapsed*\n{{.Data.Elapsed}}"
        }{{end}}{{end -}}
      ]
    }{{if and .Data .Data.Error}},{
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": {{encode (print "*Error*: " .Data.Error)}}
      }
    }{{end}}
  ]
}
`

// Template returns a template specific to this webhook.
//
//nolint:wrapcheck
func (w *WebhookConfig) Template() (*template.Template, error) {
	template := template.New("webhook").Funcs(template.FuncMap{
		"encode":     func(v any) string { b, _ := json.Marshal(v); return string(b) },
		"rawencode":  func(v any) string { b, _ := json.Marshal(v); return strings.Trim(string(b), `"`) }, // yuck
		"formencode": url.QueryEscape,
		"separator":  separator,
		"humanbytes": humanbytes,
		"nickname":   func() string { return w.Nickname },
		"channel":    func() string { return w.Channel },
		"token":      func() string { return w.Token },
		"timestamp":  func(t time.Time) string { return t.Format(time.RFC3339) },
		"name":       func() string { return w.Name },
	})

	// Providing a template name that exists overrides template_path.
	// Do not add a 'default' case here.
	switch strings.ToLower(w.TempName) {
	case "notifiarr", "default":
		return template.Parse(WebhookTemplateNotifiarr)
	case "discord":
		return template.Parse(WebhookTemplateDiscord)
	case "telegram":
		return template.Parse(WebhookTemplateTelegram)
	case "slack":
		return template.Parse(WebhookTemplateSlack)
	case "pushover":
		return template.Parse(WebhookTemplatePushover)
	case "gotify":
		return template.Parse(WebhookTemplateGotify)
	}

	// Figure out which template to use based on URL or template_path.
	switch url := strings.ToLower(w.URL); {
	default:
		fallthrough
	case strings.Contains(url, "discordnotifier.com"), strings.Contains(url, "notifiarr.com"):
		return template.Parse(WebhookTemplateNotifiarr)
	case w.TmplPath != "":
		s, err := os.ReadFile(w.TmplPath)
		if err != nil {
			return nil, fmt.Errorf("template file: %w", err)
		}

		return template.Parse(string(s))
	case strings.Contains(url, "discord.com"), strings.Contains(url, "discordapp.com"):
		return template.Parse(WebhookTemplateDiscord)
	case strings.Contains(url, "api.telegram.org"):
		return template.Parse(WebhookTemplateTelegram)
	case strings.Contains(url, "hooks.slack.com"):
		return template.Parse(WebhookTemplateSlack)
	case strings.Contains(url, "pushover.net"):
		return template.Parse(WebhookTemplatePushover)
	case strings.Contains(url, "gotify"):
		return template.Parse(WebhookTemplateGotify)
	}
}

func separator(s string) func() string {
	var i bool

	return func() string {
		if !i {
			i = true
			return ""
		}

		return s
	}
}

func humanbytes(size int64) string {
	const byteUnit = 1024

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

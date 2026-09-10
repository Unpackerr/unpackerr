package configdef

import "strings"

// FieldHelp is the English short/desc text from definitions.yml, keyed for the UI.
type FieldHelp struct {
	Short string `json:"short,omitempty"`
	Desc  string `json:"desc,omitempty"`
	Env   string `json:"env,omitempty"`
}

func uiSectionName(name section) string {
	switch name {
	case "global":
		return "general"
	case "webserver":
		return "webserver"
	case "starr":
		return "starr"
	case "folders", "folder":
		return "folders"
	case "webhook", "cmdhook":
		return "hooks"
	default:
		return ""
	}
}

// UIHelp returns field help keyed as config.<section>.<jsonName> and
// config.<section>.<yamlName> so the SPA can look up either form.
func (c *Config) UIHelp() map[string]FieldHelp {
	out := make(map[string]FieldHelp)

	for name, header := range c.Sections {
		section := uiSectionName(name)
		if header == nil || section == "" {
			continue
		}

		for _, param := range header.Params {
			if param == nil || param.Name == "" {
				continue
			}

			help := FieldHelp{
				Short: foldHelp(param.Short),
				Desc:  foldHelp(param.Desc),
				Env:   c.Prefix + header.Prefix + param.EnvVar,
			}
			if help.Short == "" && help.Desc == "" {
				continue
			}

			addHelp(out, section, param.Name, help)

			if param.Name == "paths" {
				addHelp(out, section, "path", help)
			}
		}
	}

	return out
}

func addHelp(out map[string]FieldHelp, section, name string, help FieldHelp) {
	out["config."+section+"."+name] = help
	if camel := snakeToCamel(name); camel != name {
		out["config."+section+"."+camel] = help
	}
}

func snakeToCamel(name string) string {
	parts := strings.Split(name, "_")
	if len(parts) == 1 {
		return name
	}

	var buf strings.Builder
	buf.WriteString(parts[0])

	for _, part := range parts[1:] {
		if part == "" {
			continue
		}

		buf.WriteString(strings.ToUpper(part[:1]))
		buf.WriteString(part[1:])
	}

	return buf.String()
}

// foldHelp turns YAML `|` hard-wraps into wrapping paragraphs. Single newlines
// become spaces; blank lines stay as paragraph breaks.
func foldHelp(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}

	var paras []string

	var buf []string

	flush := func() {
		if len(buf) == 0 {
			return
		}

		paras = append(paras, strings.Join(buf, " "))
		buf = nil
	}

	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}

		buf = append(buf, line)
	}

	flush()

	return strings.Join(paras, "\n\n")
}

package configdef

import "strings"

// FieldHelp is the English short/desc text from definitions.yml, keyed for the UI.
type FieldHelp struct {
	Short   string `json:"short,omitempty"`
	UIShort string `json:"uishort,omitempty"`
	Desc    string `json:"desc,omitempty"`
	Env     string `json:"env,omitempty"`
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
	case "webhook":
		return "webhook"
	case "cmdhook":
		return "cmdhook"
	default:
		return ""
	}
}

func uiSectionAlias(name section) string {
	switch name {
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

	for _, name := range c.Order {
		header := c.Sections[name]
		section := uiSectionName(name)

		if header == nil || section == "" {
			continue
		}

		envHeader := c.helpHeader(name, header)
		alias := uiSectionAlias(name)

		for _, param := range header.Params {
			if param == nil || param.Name == "" {
				continue
			}

			help := paramHelp(c.Prefix, envHeader, param)
			if help.Short == "" && help.Desc == "" {
				continue
			}

			addHelp(out, section, param.Name, help, false)

			if alias != "" {
				addHelp(out, alias, param.Name, help, true)
			}

			if param.Name == "paths" {
				addHelp(out, section, "path", help, false)

				if alias != "" {
					addHelp(out, alias, "path", help, true)
				}
			}
		}
	}

	return out
}

func paramHelp(prefix string, header *Header, param *Param) FieldHelp {
	short := foldHelp(param.Short)
	uishort := foldHelp(param.UIShort)

	if uishort != "" {
		short = uishort
	}

	return FieldHelp{
		Short:   short,
		UIShort: uishort,
		Desc:    foldHelp(param.Desc),
		Env:     header.exampleEnv(prefix, param),
	}
}

func (c *Config) helpHeader(name section, header *Header) *Header {
	order := c.DefOrder[name]
	if len(order) == 0 || c.Defs[name] == nil {
		return header
	}

	def := c.Defs[name][order[0]]
	if def == nil {
		return header
	}

	clone := *header
	clone.Prefix = def.Prefix

	return &clone
}

// exampleEnv is the docs/UI example variable: list sections insert 0_, list
// params append 0, tables append 0_*.
func (h *Header) exampleEnv(prefix string, param *Param) string {
	if h == nil || param == nil || param.EnvVar == "" {
		return ""
	}

	hSuffix := ""
	if h.Kind == list {
		hSuffix = "0_"
	}

	envVar := prefix + h.Prefix + hSuffix + param.EnvVar

	switch param.Kind {
	case list:
		envVar += "0"
	case tables:
		envVar += "0_*"
	}

	return envVar
}

func addHelp(out map[string]FieldHelp, section, name string, help FieldHelp, keepFirst bool) {
	put := func(key string) {
		if keepFirst {
			if _, exists := out[key]; exists {
				return
			}
		}

		out[key] = help
	}

	put("config." + section + "." + name)

	if camel := snakeToCamel(name); camel != name {
		put("config." + section + "." + camel)
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

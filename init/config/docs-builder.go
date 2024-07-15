package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	outputDir = "generated/"
	dirMode   = 0o755
	fileMode  = 0o644
)

func printDocusaurus(config *Config) {
	// Loop the 'Order' list.
	if err := makeGenerated(config); err != nil {
		panic(err)
	}

	for _, section := range config.Order {
		// If Order contains a missing section, panic.
		if config.Sections[section] == nil {
			panic(section + ": in order, but missing from sections. This is a bug in conf-builder.yml.")
		}

		if len(config.Sections[section].Params) < 1 {
			continue
		}

		if config.Defs[section] != nil {
			data := config.Sections[section].makeDefinedDocs(config.Prefix, config.Defs[section], config.DefOrder[section])
			if err := output(string(section), data); err != nil {
				panic(err)
			}
		} else {
			data := config.Sections[section].makeDocs(config.Prefix, section)
			if err := output(string(section), data); err != nil {
				panic(err)
			}
		}
	}
}

func output(file, content string) error {
	_ = os.Mkdir(outputDir, dirMode)
	date := "---\n# Generated: " + time.Now().Round(time.Second).String() + "\n---\n\n"
	//nolint:wrapcheck
	return os.WriteFile(filepath.Join(outputDir, file+".md"), []byte(date+content), fileMode)
}

// makeGenerated writes a special file that the website can import.
// Adds all param sections except global into a docusaurus import format.
func makeGenerated(config *Config) error {
	var (
		first  bytes.Buffer
		second bytes.Buffer
	)

	for _, section := range config.Order {
		if len(config.Sections[section].Params) > 0 && section != "global" {
			title := "G" + string(section)
			first.WriteString("import " + title + " from './" + string(section) + ".md';\n")
			second.WriteString("<" + title + "/>\n")
		}
	}

	return output("index", first.String()+"\n"+second.String())
}

func (h *Header) makeDocs(prefix string, section section) string {
	buf := bytes.Buffer{}
	buf.WriteString("## " + h.Title + "\n\n<details>\n")

	conf := h.makeSection(section, true, true)
	env := h.makeCompose(h.Title, prefix, true)
	header := "[" + string(section) + "]"

	if h.Kind == list {
		header = "[[" + string(section) + "]]"
	}

	if h.NoHeader {
		buf.WriteString("  <summary>Examples. Prefix: <b>" + prefix + "</b></summary>\n\n")
	} else {
		buf.WriteString("  <summary>Examples. Prefix: <b>" + prefix + h.Prefix + "</b>, Header: <b>" + header + "</b></summary>\n\n")
	}

	buf.WriteString("- Using the config file:\n\n```yaml\n")
	buf.WriteString(strings.TrimSpace(conf) + "\n```\n\n")
	buf.WriteString("- Using environment variables:\n\n```js\n")
	buf.WriteString(env + "```\n\n</details>\n\n")
	buf.WriteString(h.Docs + "\n") // Docs comes before the table.
	buf.WriteString(h.makeDocsTable(prefix) + "\n")
	buf.WriteString(h.Tail) // Tail goes after the docs and table.

	if h.Notes != "" { // Notes become a sub header.
		buf.WriteString("### " + h.Title + " Notes\n\n" + h.Notes)
	}

	return buf.String()
}

func (h *Header) makeDocsTable(prefix string) string {
	const (
		tableHeader = "|Config Name|Variable Name|Default / Note|\n|---|---|---|\n"
		tableFormat = "|%s|`%s`|%v / %s|\n"
	)

	buf := bytes.Buffer{}
	buf.WriteString(tableHeader)

	for _, param := range h.Params {
		envVar := prefix + h.Prefix + param.EnvVar
		if param.Kind == list {
			envVar += "0"
		}

		def := "No Default"

		if rv := reflect.ValueOf(param.Default); rv.Kind() == reflect.Bool || !rv.IsZero() {
			if t, _ := toml.Marshal(param.Default); len(t) > 0 {
				def = "`" + string(t) + "`"
			}
		}

		buf.WriteString(fmt.Sprintf(tableFormat, param.Name, envVar, def, param.Short))
	}

	return buf.String()
}

func (h *Header) makeDefinedDocs(prefix string, defs Defs, order []section) string {
	var buf bytes.Buffer

	for _, section := range order {
		buf.WriteString(createDefinedSection(defs[section], h).makeDocs(prefix, section))
	}

	return buf.String()
}

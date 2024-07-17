package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

/* This file creates a folder full of docusaurus markdown files for https://unpackerr.zip */

func createDocusaurus(config *Config, output string) {
	// Generate index file first.
	if err := makeGenerated(config, output); err != nil {
		log.Fatalln(err)
	}
	// Loop the 'Order' list.
	for _, section := range config.Order {
		// If Order contains a missing section, bail.
		if config.Sections[section] == nil {
			log.Fatalln(section + ": in order, but missing from sections. This is a bug in definitions.yml.")
		}
		// We only care about sections with parameters defined.
		if len(config.Sections[section].Params) < 1 {
			continue
		}

		if config.Defs[section] != nil {
			// Repeat this section based on defined definitions.
			data := config.Sections[section].makeDefinedDocs(config.Prefix, config.Defs[section], config.DefOrder[section])
			if err := writeDocusaurus(output, string(section), data); err != nil {
				log.Fatalln(err)
			}
		} else {
			data := config.Sections[section].makeDocs(config.Prefix, section)
			if err := writeDocusaurus(output, string(section), data); err != nil {
				log.Fatalln(err)
			}
		}
	}
}

func writeDocusaurus(dir, file, content string) error {
	_ = os.Mkdir(dir, dirMode)
	date := "---\n## => Content Auto Generated, " +
		strings.ToUpper(time.Now().UTC().Round(time.Second).Format("02 Jan 2006 15:04 UTC")) + "\n---\n\n"
	filePath := filepath.Join(dir, file+".md")
	log.Printf("Writing: %s, size: %d", filePath, len(content))
	//nolint:wrapcheck
	return os.WriteFile(filePath, []byte(date+content), fileMode)
}

// makeGenerated writes a special file that the website can import.
// Adds all param sections except global into a docusaurus import format.
func makeGenerated(config *Config, output string) error {
	var first, second bytes.Buffer

	for _, section := range config.Order {
		if len(config.Sections[section].Params) > 0 && section != "global" {
			first.WriteString("import G" + string(section) + " from './" + string(section) + ".md';\n")
			second.WriteString("<G" + string(section) + "/>\n")
		}
	}

	err := writeDocusaurus(output, "index", first.String()+"\n"+second.String())
	if err != nil {
		return err
	}

	date := strings.ToUpper(time.Now().UTC().Round(time.Second).Format("02 Jan 2006 15:04 UTC"))
	// Create a footer file that can be imported.
	return writeDocusaurus(output, "footer", `<font color="gray" style={{'float': 'right', 'font-style': 'italic'}}>`+
		"This page was [generated automatically](https://github.com/Unpackerr/unpackerr/tree/main/init/config), "+date+"</font>\n")
}

func (h *Header) makeDocs(prefix string, section section) string {
	conf := h.makeSection(section, true, true) // Generate this portion of the config file.
	env := h.makeCompose(prefix, true)         // Generate this portion of the docker-compose example.

	buf := bytes.Buffer{}
	buf.WriteString("## " + h.Title + "\n\n<details>\n  <summary>Examples. Prefix: <b>" + prefix)

	if !h.NoHeader {
		brace1, brace2 := "[", "]"
		if h.Kind == list {
			brace1, brace2 = "[[", "]]"
		}

		buf.WriteString(h.Prefix + "</b>, Header: <b> ")   // Add to the line above.
		buf.WriteString(brace1 + string(section) + brace2) // Add to the line above.
	}

	buf.WriteString("</b></summary>\n\n") // Add to the line above.
	buf.WriteString("- Using the config file:\n\n```yaml\n")
	buf.WriteString(strings.TrimSpace(conf) + "\n```\n\n")
	buf.WriteString("- Using environment variables:\n\n```js\n")
	buf.WriteString(env + "```\n\n</details>\n\n")
	buf.WriteString(h.Docs + "\n" + h.makeDocsTable(prefix) + "\n" + h.Tail)

	if h.Notes != "" { // Notes become a sub header.
		buf.WriteString("### Notes for " + h.Title + "\n\n" + h.Notes)
	}

	return buf.String()
}

const (
	tableHeader = "|Config Name|Variable Name|Default / Note|\n|---|---|---|\n"
	tableFormat = "|%s|`%s`|%v / %s|\n"
)

func (h *Header) makeDocsTable(prefix string) string {
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

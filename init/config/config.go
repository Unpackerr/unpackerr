package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

/* This file creates an example config file: unpackerr.conf.example */

func createConfFile(config *Config, output, dir string) {
	buf := bytes.Buffer{}

	// Loop the 'Order' list.
	for _, section := range config.Order {
		// If Order contains a missing section, bail.
		if config.Sections[section] == nil {
			log.Fatalln(section + ": in order, but missing from sections. This is a bug in definitions.yml.")
		}

		if config.Defs[section] != nil {
			buf.WriteString(config.Sections[section].makeDefinedSection(config.Defs[section], config.DefOrder[section], false))
		} else {
			buf.WriteString(config.Sections[section].makeSection(section, false, false))
		}
	}

	_ = os.Mkdir(dir, dirMode)
	filePath := filepath.Join(dir, output)
	log.Printf("Writing: %s, size: %d", filePath, buf.Len())

	if err := os.WriteFile(filePath, buf.Bytes(), fileMode); err != nil {
		log.Fatalln(err)
	}
}

// Not all sections have defs, and it may be nil. Defs only work on 'list' sections.
func (h *Header) makeSection(name section, showHeader, showValue bool) string {
	var buf bytes.Buffer

	// Print section header text.
	if h.Text != "" {
		buf.WriteString(h.Text)
	}

	space, comment := "", "#"
	if showHeader {
		// this only happens when a defined section has a comment override on the repeating headers.
		comment = ""
	}

	if !h.NoHeader { // Print the [section] or [[section]] header.
		space = " "

		if h.Kind == list { // list sections are commented by default.
			buf.WriteString(comment + "[[" + string(name) + "]]" + "\n") // list sections use double-brackets.
		} else {
			buf.WriteString("[" + string(name) + "]" + "\n") // non-list sections use single brackets.
		}
	}

	for _, param := range h.Params {
		// Print an empty newline for each param if the section has no header and the param has a description.
		if h.NoHeader && param.Desc != "" {
			buf.WriteString("\n")
		}

		// Add ## to the beginning of each line in the description.
		// Uses the newline \n character to figure out where each line begins.
		if param.Desc != "" {
			buf.WriteString("## " + strings.ReplaceAll(strings.TrimSpace(param.Desc), "\n", "\n## ") + "\n")
		}

		switch {
		default:
			fallthrough
		case showValue:
			buf.WriteString(fmt.Sprintf("%s%s = %s\n", space, param.Name, param.Value()))
		case param.Example != nil:
			// If example is not empty, use that commented out, otherwise use the default.
			fallthrough
		case h.Kind == list:
			// If the 'kind' is a 'list', we comment all the parameters.
			buf.WriteString(fmt.Sprintf("#%s%s = %s\n", space, param.Name, param.Value()))
		}
	}

	// Each section needs a newline at the end.
	buf.WriteString("\n")

	return buf.String()
}

func (p *Param) Value() string {
	// If example is not empty, use that commented out, otherwise use the default.
	out, _ := toml.Marshal(p.Default)
	if p.Example != nil {
		out, _ = toml.Marshal(p.Example)
	}

	// The toml marshaller uses only regular quotes " which kinda suck, so replace them with single quotes ' on file paths.
	if strings.Contains(p.Name, "path") || strings.HasSuffix(p.Name, "file") || p.Name == "command" {
		return string(bytes.ReplaceAll(out, []byte{'"'}, []byte("'")))
	}

	return string(out)
}

// makeDefinedSection duplicates sections from overrides, and prints it once for each override.
func (h *Header) makeDefinedSection(defs Defs, order []section, showValue bool) string {
	var buf bytes.Buffer

	for _, section := range order {
		newHeader := createDefinedSection(defs[section], h)
		// Make a brand new section and pass it back in.
		// Only defined sections can comment the header.
		buf.WriteString(newHeader.makeSection(section, !defs[section].Comment, showValue))
	}

	return buf.String()
}

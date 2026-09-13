package configdef

import (
	"bytes"
	"fmt"
	"log"
	"strings"
)

/* This file creates an example config file: unpackerr.conf.example */

func createConfFile(config *Config, output, dir string) {
	buf := bytes.NewBufferString(config.ExampleTOML())
	writeFile(dir, output, buf)
}

// ExampleTOML renders the commented example configuration (go generate / first-run).
func (c *Config) ExampleTOML() string {
	var buf bytes.Buffer

	for _, name := range c.Order {
		header := c.Sections[name]
		if header == nil {
			log.Fatalln(name + ": in order, but missing from sections. This is a bug in definitions.yml.")
		}

		if c.Defs[name] != nil {
			buf.WriteString(header.makeDefinedSection(c.Defs[name], c.DefOrder[name], false))
		} else {
			buf.WriteString(header.makeSection(name, false, false))
		}
	}

	return buf.String()
}

// ParamNames is the set of every schema parameter name across sections.
func (c *Config) ParamNames() map[string]struct{} {
	out := make(map[string]struct{})

	for name, header := range c.Sections {
		if name != "" {
			out[string(name)] = struct{}{}
		}

		if header == nil {
			continue
		}

		for _, param := range header.Params {
			if param != nil && param.Name != "" {
				out[param.Name] = struct{}{}
			}
		}
	}

	for _, defs := range c.Defs {
		for name := range defs {
			if name != "" {
				out[string(name)] = struct{}{}
			}
		}
	}

	return out
}

// Not all sections have defs, and it may be nil. Defs only work on 'list' sections.
func (h *Header) makeSection(name section, showHeader, showValue bool) string {
	var buf bytes.Buffer

	// Print section header text.
	if h.Text != "" {
		buf.WriteString(h.Text)
	}

	space := ""

	if !h.NoHeader { // Print the [section], [section.0], or [[section]] header.
		space = " "
		comment := ""
		// Repeatable templates start commented. Singleton tables ([webserver],
		// [folders]) stay live so their keys do not fall into the root table.
		if h.repeatable() && !showHeader {
			comment = "#"
		}

		h.writeTOMLHeader(&buf, name, "0", comment)
	}

	for _, param := range h.Params {
		if param == nil {
			continue
		}

		// Print an empty newline for each param if the section has no header and the param has a description.
		if h.NoHeader && param.Desc != "" {
			buf.WriteString("\n")
		}

		// Add ## to the beginning of each line in the description.
		// Uses the newline \n character to figure out where each line begins.
		if param.Desc != "" {
			buf.WriteString("## ")
			buf.WriteString(strings.ReplaceAll(strings.TrimSpace(param.Desc), "\n", "\n## "))
			buf.WriteByte('\n')
		}

		if param.isNested() {
			continue
		}

		switch {
		default:
			fallthrough
		case showValue:
			fmt.Fprintf(&buf, "%s%s = %s\n", space, param.Name, param.Value())
		case param.Example != nil:
			// If example is not empty, use that commented out, otherwise use the default.
			fallthrough
		case h.repeatable():
			// Repeatable sections comment every parameter in the example template.
			fmt.Fprintf(&buf, "#%s%s = %s\n", space, param.Name, param.Value())
		}
	}

	// Each section needs a newline at the end.
	buf.WriteString("\n")

	return buf.String()
}

func (p *Param) Value() string {
	if p.Example != nil {
		return formatTOML(p.Name, p.Example)
	}

	return formatTOML(p.Name, p.Default)
}

func (p *Param) isNested() bool {
	return p != nil && (p.Kind == "map" || p.Kind == tables)
}

func (h *Header) repeatable() bool {
	return h != nil && (h.Kind == list || h.Kind == named)
}

func (h *Header) writeTOMLHeader(buf *bytes.Buffer, name section, key, comment string) {
	left, inner, right := "[", string(name), "]"

	switch h.Kind {
	case list:
		left, right = "[[", "]]"
	case named:
		if key == "" {
			key = "0"
		}

		inner = string(name) + "." + key
	}

	buf.WriteString(comment)
	buf.WriteString(left)
	buf.WriteString(inner)
	buf.WriteString(right)
	buf.WriteByte('\n')
}

// makeDefinedSection duplicates sections from overrides, and prints it once for each override.
func (h *Header) makeDefinedSection(defs Defs, order []section, showValue bool) string {
	var buf bytes.Buffer

	for _, section := range order {
		newHeader := createDefinedSection(defs[section], h, section)
		// Make a brand new section and pass it back in.
		// Only defined sections can comment the header.
		buf.WriteString(newHeader.makeSection(section, !defs[section].Comment, showValue))
	}

	return buf.String()
}

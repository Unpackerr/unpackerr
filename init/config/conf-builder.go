package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

func printConfFile(config *Config) {
	// Loop the 'Order' list.
	for _, section := range config.Order {
		// If Order contains a missing section, panic.
		if config.Sections[section] == nil {
			panic(section + ": in order, but missing from sections. This is a bug in conf-builder.yml.")
		}

		if config.Defs[section] != nil {
			printDefinedSection(config.Sections[section], config.Defs[section])
		} else {
			printSection(section, config.Sections[section], false)
		}
	}
}

// Not all sections have defs, and it may be nil. Defs only work on 'list' sections.
func printSection(name string, section *Header, noComment bool) {
	// Print section header text.
	if section.Text != "" {
		fmt.Printf("%s", section.Text)
	}

	comment := "#"
	if noComment {
		// this only happens when a defined section has a comment override on the repeating headers.
		comment = ""
	}

	if !section.NoHeader { // Print the [section] or [[section]] header.
		if section.Kind == list { // list sections are commented by default.
			fmt.Println(comment + "[[" + name + "]]") // list sections use double-brackets.
		} else {
			fmt.Println("[" + name + "]") // non-list sections use single brackets.
		}
	}

	for _, param := range section.Params {
		// Print an empty newline for each param if the section has no header and the param has a description.
		if section.NoHeader && param.Desc != "" {
			fmt.Println()
		}

		// Add ## to the beginning of each line in the description.
		// Uses the newline \n character to figure out where each line begins.
		if param.Desc != "" {
			fmt.Println("##", strings.ReplaceAll(strings.TrimSpace(param.Desc), "\n", "\n## "))
		}

		// If example is not empty, use that commented out, otherwise use the default.
		if comment = ""; param.Example != nil {
			comment = "#"
		}

		if section.Kind == list {
			// If the 'kind' is a 'list', we comment all the parameters.
			fmt.Printf("# %s = %s\n", param.Name, param.Value())
		} else {
			fmt.Printf("%s%s = %s\n", comment, param.Name, param.Value())
		}
	}

	// Each section needs a newline at the end.
	fmt.Println()
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

// printDefinedSection duplicates sections from overrides, and prints it once for each override.
func printDefinedSection(section *Header, defs Defs) {
	for name, def := range defs {
		// Loop each defined section Defaults, and see if one of the param names match.
		for overrideName, override := range def.Defaults {
			for _, defined := range section.Params {
				// If the name of the default (override) matches this param name, overwrite the value.
				if defined.Name == overrideName {
					defined.Default = override
				}
			}
		}

		// Make a brand new section and pass it back in.
		printSection(name, &Header{
			Text:     def.Text,
			Prefix:   section.Prefix,
			Params:   section.Params,
			Kind:     section.Kind,
			NoHeader: false,
		}, !def.Comment) // Only defined sections can comment the header.
	}
}

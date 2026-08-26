package main

import (
	"errors"
	"regexp"
	"strings"
)

var (
	admonitionSpace = regexp.MustCompile(
		`(?m)^(:::(?:note|tip|info|caution|danger|warning|important|success|secondary)) +.+$`,
	)
	importIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (c *Config) validate() error {
	var errs []string

	if c.Prefix == "" {
		errs = append(errs, "envvar_prefix is required")
	}

	if len(c.Order) == 0 {
		errs = append(errs, "order is empty")
	}

	errs = append(errs, c.validateOrder()...)
	errs = append(errs, c.validateDefs()...)

	for _, name := range c.generatedImportNames() {
		if !validImportIdent(name) {
			errs = append(errs, name+": not a valid JS import identifier (website will not compile)")
		}
	}

	return fmtErrs(errs)
}

func (c *Config) validateOrder() []string {
	var errs []string

	seen := make(map[section]struct{}, len(c.Order))

	for _, name := range c.Order {
		if _, dup := seen[name]; dup {
			errs = append(errs, string(name)+": duplicated in order (generated MDX imports would collide)")
			continue
		}

		seen[name] = struct{}{}

		header := c.Sections[name]
		if header == nil {
			errs = append(errs, string(name)+": in order, but missing from sections")
			continue
		}

		errs = append(errs, header.validate(name)...)
	}

	return errs
}

func (c *Config) validateDefs() []string {
	var errs []string

	for defName, defs := range c.Defs {
		if len(c.DefOrder[defName]) == 0 {
			errs = append(errs, string(defName)+": in defs, but def_order is missing or empty")
		}

		for item, def := range defs {
			if def == nil {
				continue
			}

			errs = append(errs, mdxProblems(def.Title, string(defName)+"."+string(item)+" title")...)
		}
	}

	for defName, order := range c.DefOrder {
		if c.Sections[defName] == nil {
			errs = append(errs, string(defName)+": in def_order, but missing from sections")
		}

		if c.Defs[defName] == nil {
			errs = append(errs, string(defName)+": in def_order, but missing from defs")
			continue
		}

		for _, item := range order {
			if c.Defs[defName][item] == nil {
				errs = append(errs, string(defName)+": def_order lists "+string(item)+" missing from defs")
			}
		}
	}

	return errs
}

func (h *Header) validate(name section) []string {
	var errs []string

	errs = append(errs, mdxProblems(h.Title, string(name)+" title")...)
	errs = append(errs, mdxProblems(h.Docs, string(name)+" docs")...)
	errs = append(errs, mdxProblems(h.Notes, string(name)+" notes")...)
	errs = append(errs, mdxProblems(h.Tail, string(name)+" tail")...)

	for _, param := range h.Params {
		if param.Name == "" {
			errs = append(errs, string(name)+": param missing name")
		}

		if param.EnvVar == "" {
			errs = append(errs, string(name)+"."+param.Name+": missing envvar")
		}

		if param.Default == nil {
			errs = append(errs, string(name)+"."+param.Name+": missing default")
		}

		switch param.Kind {
		case "", list, "conlist":
		default:
			errs = append(errs, string(name)+"."+param.Name+": unknown kind "+param.Kind)
		}

		where := string(name) + "." + param.Name + " short"
		errs = append(errs, mdxProblems(param.Short, where)...)

		if strings.Contains(param.Short, "|") {
			errs = append(errs, where+": contains | which breaks the Docusaurus table")
		}
	}

	return errs
}

// mdxProblems finds Docusaurus MDX v2 compile breakers in generated website markdown.
func mdxProblems(content, where string) []string {
	var errs []string

	if admonitionSpace.MatchString(content) {
		errs = append(errs, where+": use :::note[Title] (MDX v2); :::note Title fails Docusaurus compile")
	}

	stripped := stripFencedCode(content)
	stripped = stripInlineCode(stripped)

	for line := range strings.SplitSeq(stripped, "\n") {
		if strings.Contains(stripAllowedBraces(line), "{") {
			errs = append(errs, where+": unescaped { breaks MDX (wrap in backticks or use {{ )")
			break
		}
	}

	return errs
}

// stripAllowedBraces removes complete {{...}} JSX and {/*...*/} comment spans.
// Leftover { characters are MDX compile breakers.
func stripAllowedBraces(line string) string {
	var out strings.Builder

	for idx := 0; idx < len(line); {
		rest := line[idx:]

		switch {
		case strings.HasPrefix(rest, "{/*"):
			end := strings.Index(rest, "*/}")
			if end < 0 {
				out.WriteString(rest)
				return out.String()
			}

			idx += end + len("*/}")
		case strings.HasPrefix(rest, "{{"):
			end := strings.Index(rest[len("{{"):], "}}")
			if end < 0 {
				out.WriteString(rest)
				return out.String()
			}

			idx += len("{{") + end + len("}}")
		default:
			out.WriteByte(line[idx])
			idx++
		}
	}

	return out.String()
}

func stripFencedCode(content string) string {
	var out strings.Builder

	inFence := false

	for line := range strings.SplitSeq(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}

		if !inFence {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	return out.String()
}

func stripInlineCode(content string) string {
	var out strings.Builder

	inCode := false

	for _, char := range content {
		if char == '`' {
			inCode = !inCode
			continue
		}

		if !inCode {
			out.WriteRune(char)
		}
	}

	return out.String()
}

func validImportIdent(name string) bool {
	return importIdent.MatchString(name)
}

func (c *Config) generatedImportNames() []string {
	var names []string

	for _, section := range c.Order {
		header := c.Sections[section]
		if header == nil || len(header.Params) < 1 || section == "global" {
			continue
		}

		names = append(names, string(section))
	}

	return names
}

func checkGeneratedMDX(filename, content string) []string {
	var errs []string

	errs = append(errs, mdxProblems(content, filename)...)

	for line := range strings.SplitSeq(content, "\n") {
		if !strings.HasPrefix(line, "import G") {
			continue
		}

		ident := strings.TrimPrefix(strings.Fields(line)[1], "G")
		if !validImportIdent(ident) {
			errs = append(errs, filename+": import G"+ident+" is not a valid JS identifier")
		}
	}

	return errs
}

func fmtErrs(errs []string) error {
	if len(errs) == 0 {
		return nil
	}

	return errors.New(strings.Join(errs, "\n")) //nolint:err113
}

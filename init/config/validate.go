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

	for name := range c.Sections {
		if _, ok := seen[name]; !ok {
			errs = append(errs, string(name)+": in sections, but missing from order")
		}
	}

	return errs
}

func (c *Config) validateDefs() []string {
	var errs []string

	for defName, defs := range c.Defs {
		order := c.DefOrder[defName]
		if len(order) == 0 {
			errs = append(errs, string(defName)+": in defs, but def_order is missing or empty")
		}

		ordered := make(map[section]struct{}, len(order))
		for _, item := range order {
			ordered[item] = struct{}{}
		}

		for item, def := range defs {
			if _, ok := ordered[item]; !ok {
				errs = append(errs, string(defName)+"."+string(item)+": in defs, but missing from def_order")
			}

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

		seen := make(map[section]struct{}, len(order))

		for _, item := range order {
			if _, dup := seen[item]; dup {
				errs = append(errs, string(defName)+": def_order duplicates "+string(item))
				continue
			}

			seen[item] = struct{}{}

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

	stripped := stripFencedCode(content)

	if admonitionSpace.MatchString(stripped) {
		errs = append(errs, where+": use :::note[Title] (MDX v2); :::note Title fails Docusaurus compile")
	}

	stripped = stripInlineCode(stripped)

	for line := range strings.SplitSeq(stripped, "\n") {
		if strings.ContainsAny(stripAllowedBraces(line), "{}") {
			errs = append(errs, where+": unescaped { or } breaks MDX (wrap in backticks or use {{ )")
			break
		}
	}

	return errs
}

// stripAllowedBraces removes complete {{...}} JSX and {/*...*/} comment spans.
// Leftover { or } characters are MDX compile breakers.
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

// stripInlineCode removes markdown code spans. A span opens with a run of
// backticks and closes only with a run of the same length.
func stripInlineCode(content string) string {
	var out strings.Builder

	for idx := 0; idx < len(content); {
		if content[idx] != '`' {
			out.WriteByte(content[idx])
			idx++

			continue
		}

		run := 0
		for idx+run < len(content) && content[idx+run] == '`' {
			run++
		}

		closer := strings.Repeat("`", run)

		end := strings.Index(content[idx+run:], closer)
		if end < 0 {
			// No matching closer; not a code span. Keep the backticks as text.
			out.WriteString(closer)

			idx += run

			continue
		}

		idx += run + end + run
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

		names = append(names, "G"+string(section))
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

		ident := strings.Fields(line)[1]
		if !validImportIdent(ident) {
			errs = append(errs, filename+": import "+ident+" is not a valid JS identifier")
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

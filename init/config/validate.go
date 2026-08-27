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

	errs = append(errs, mdxProblems(c.Prefix, "envvar_prefix")...)

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

	errs = append(errs, c.validateGeneratedDocs()...)

	return fmtErrs(errs)
}

// validateGeneratedDocs renders every docusaurus document and checks it for MDX
// compile breakers before any file is written. This catches problems in values
// that are interpolated but not individually validated (prefixes, table cells).
func (c *Config) validateGeneratedDocs() []string {
	var errs []string

	errs = append(errs, checkGeneratedMDX("index.md", c.makeIndexDocs())...)

	for _, name := range c.Order {
		header := c.Sections[name]
		if header == nil || len(header.Params) < 1 {
			continue
		}

		var data string
		if c.Defs[name] != nil {
			data = header.makeDefinedDocs(c.Prefix, c.Defs[name], c.DefOrder[name])
		} else {
			data = header.makeDocs(c.Prefix, name)
		}

		errs = append(errs, checkGeneratedMDX(string(name)+".md", data)...)
	}

	return errs
}

// makeIndexDocs renders the generated index.md content (without the front matter).
func (c *Config) makeIndexDocs() string {
	var first, second strings.Builder

	for _, name := range c.Order {
		header := c.Sections[name]
		if header != nil && len(header.Params) > 0 && name != "global" {
			first.WriteString("import G" + string(name) + " from './" + string(name) + ".md';\n")
			second.WriteString("<G" + string(name) + "/>\n")
		}
	}

	return first.String() + "\n" + second.String()
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
				errs = append(errs, string(defName)+"."+string(item)+": definition is empty (null)")
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
	errs = append(errs, mdxProblems(h.Prefix, string(name)+" envvar_prefix")...)

	for _, param := range h.Params {
		if param == nil {
			errs = append(errs, string(name)+": param is empty (null)")
			continue
		}

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
		if hasUnescapedBrace(line) {
			errs = append(errs, where+": unescaped { or } breaks MDX (wrap in backticks or use {{ )")
			break
		}
	}

	return errs
}

// hasUnescapedBrace reports whether a line contains a { or } that MDX will
// parse as JSX. Complete {{...}} and {/*...*/} spans are removed first, then
// backslash-escaped braces (\{, \}, and \\{) are ignored.
func hasUnescapedBrace(line string) bool {
	stripped := stripAllowedBraces(line)

	for idx := range len(stripped) {
		if stripped[idx] != '{' && stripped[idx] != '}' {
			continue
		}

		// Count the backslashes immediately before this brace.
		slashes := 0
		for j := idx - 1; j >= 0 && stripped[j] == '\\'; j-- {
			slashes++
		}

		// An odd number of backslashes means the brace is escaped.
		if slashes%2 == 0 {
			return true
		}
	}

	return false
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

// stripFencedCode removes fenced code blocks. A fence opens with a run of 3+
// backticks or tildes and closes only with the same marker character and a run
// at least as long (CommonMark). Info strings are ignored.
func stripFencedCode(content string) string {
	var out strings.Builder

	inFence := false
	fenceChar := byte(0)
	fenceLen := 0

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if !inFence {
			if char, length, ok := fenceMarker(trimmed); ok {
				inFence, fenceChar, fenceLen = true, char, length
				continue
			}

			out.WriteString(line)
			out.WriteByte('\n')

			continue
		}

		// Inside a fence: close only on a compatible marker (same char, run >= opener).
		if char, length, ok := fenceMarker(trimmed); ok && char == fenceChar && length >= fenceLen {
			inFence = false
		}
	}

	return out.String()
}

// minFenceLen is the minimum run length that opens a CommonMark code fence.
const minFenceLen = 3

// fenceMarker reports whether a trimmed line is a code fence marker (``` or ~~~,
// 3 or more of the same character), returning the marker char and run length.
func fenceMarker(line string) (byte, int, bool) {
	if len(line) < minFenceLen {
		return 0, 0, false
	}

	char := line[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}

	length := 0
	for length < len(line) && line[length] == char {
		length++
	}

	if length < minFenceLen {
		return 0, 0, false
	}

	return char, length, true
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

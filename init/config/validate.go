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

	// Rendering panics on structurally invalid params (e.g. a nil Default).
	// Return those errors instead of crashing.
	if len(errs) == 0 {
		errs = append(errs, c.validateGeneratedDocs()...)
	}

	return fmtErrs(errs)
}

// definedListValid reports whether a defined section can be rendered: it must
// have a base section, a defs map, and a def_order whose entries all exist.
func (c *Config) definedListValid(name section) bool {
	if c.Sections[name] == nil || c.Defs[name] == nil {
		return false
	}

	for _, item := range c.DefOrder[name] {
		if c.Defs[name][item] == nil {
			return false
		}
	}

	return true
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
			// Rendering panics on a nil def; the structural validator already
			// recorded that error, so skip rendering this broken list.
			if !c.definedListValid(name) {
				continue
			}

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
			errs = append(errs, c.validateDefListOverrides(defName, item, def)...)
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

		errs = append(errs, param.validateListValues(string(name)+"."+param.Name)...)

		where := string(name) + "." + param.Name + " short"
		errs = append(errs, mdxProblems(param.Short, where)...)

		if strings.Contains(param.Short, "|") {
			errs = append(errs, where+": contains | which breaks the Docusaurus table")
		}
	}

	return errs
}

func (p *Param) validateListValues(where string) []string {
	if p.Kind != list && p.Kind != "conlist" {
		return nil
	}

	var errs []string

	for _, item := range []struct {
		label string
		val   any
	}{
		{"default", p.Default},
		{"example", p.Example},
		{"docker", p.Docker},
	} {
		if item.val != nil && !isSeq(item.val) {
			errs = append(errs, where+": "+p.Kind+" "+item.label+" must be a list")
		}
	}

	return errs
}

func (c *Config) validateDefListOverrides(defName, item section, def *Def) []string {
	header := c.Sections[defName]
	if header == nil {
		return nil
	}

	var errs []string

	for _, param := range header.Params {
		if param == nil || (param.Kind != list && param.Kind != "conlist") {
			continue
		}

		where := string(defName) + "." + string(item) + "." + param.Name
		if val, ok := def.Defaults[param.Name]; ok && !isSeq(val) {
			errs = append(errs, where+": "+param.Kind+" default override must be a list")
		}

		if val, ok := def.Examples[param.Name]; ok && !isSeq(val) {
			errs = append(errs, where+": "+param.Kind+" example override must be a list")
		}

		if val, ok := def.DockerExample[param.Name]; ok && !isSeq(val) {
			errs = append(errs, where+": "+param.Kind+" docker override must be a list")
		}
	}

	return errs
}

func isSeq(val any) bool {
	_, ok := val.([]any)
	return ok
}

// mdxProblems finds Docusaurus MDX v2 compile breakers in generated website markdown.
func mdxProblems(content, where string) []string {
	var errs []string

	stripped := stripInlineCode(stripIndentedCode(stripFencedCode(content)))

	if admonitionSpace.MatchString(stripped) {
		errs = append(errs, where+": use :::note[Title] (MDX v2); :::note Title fails Docusaurus compile")
	}

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
// braces preceded by an odd number of backslashes are ignored.
func hasUnescapedBrace(line string) bool {
	stripped := stripAllowedBraces(line)

	for idx := range len(stripped) {
		if stripped[idx] != '{' && stripped[idx] != '}' {
			continue
		}

		if !oddEscapes(stripped, idx) {
			return true
		}
	}

	return false
}

// oddEscapes reports whether idx is preceded by an odd number of backslashes,
// meaning the character is escaped in CommonMark.
func oddEscapes(content string, idx int) bool {
	slashes := 0
	for j := idx - 1; j >= 0 && content[j] == '\\'; j-- {
		slashes++
	}

	return slashes%2 == 1
}

// stripAllowedBraces removes complete {{...}} JSX and {/*...*/} comment spans.
// JSX spans are depth-balanced so nested objects like {{a: {b: 1}}} match.
// Leftover { or } characters are MDX compile breakers.
func stripAllowedBraces(line string) string {
	var out strings.Builder

	for idx := 0; idx < len(line); {
		rest := line[idx:]

		switch {
		case strings.HasPrefix(rest, "{/*") && !oddEscapes(line, idx):
			end := strings.Index(rest, "*/}")
			if end < 0 {
				out.WriteString(rest)
				return out.String()
			}

			idx += end + len("*/}")
		case strings.HasPrefix(rest, "{{") && !oddEscapes(line, idx):
			end := balancedBraceEnd(line, idx)
			if end < 0 {
				out.WriteByte(line[idx])
				idx++

				continue
			}

			idx = end
		default:
			out.WriteByte(line[idx])
			idx++
		}
	}

	return out.String()
}

// balancedBraceEnd returns the index after a complete brace group starting at
// idx, counting nested unescaped { and }. idx must point at an unescaped '{'.
func balancedBraceEnd(content string, idx int) int {
	depth := 0

	for pos := idx; pos < len(content); pos++ {
		if content[pos] != '{' && content[pos] != '}' {
			continue
		}

		if oddEscapes(content, pos) {
			continue
		}

		if content[pos] == '{' {
			depth++
			continue
		}

		depth--
		if depth == 0 {
			return pos + 1
		}
	}

	return -1
}

// stripFencedCode removes fenced code blocks. A fence opens with a run of 3+
// backticks or tildes (indented at most three spaces) and closes only with the
// same marker character, a run at least as long, and whitespace-only trailing
// text (CommonMark). Info strings on the opening fence are ignored.
func stripFencedCode(content string) string {
	var out strings.Builder

	inFence := false
	fenceChar := byte(0)
	fenceLen := 0

	for line := range strings.SplitSeq(content, "\n") {
		// CommonMark allows up to 3 leading spaces; more makes it indented code.
		indent := leadingSpaces(line)
		trimmed := strings.TrimSpace(line)

		if !inFence {
			if indent <= maxFenceIndent {
				if char, length, isOpen := fenceOpener(trimmed); isOpen {
					inFence, fenceChar, fenceLen = true, char, length
					continue
				}
			}

			out.WriteString(line)
			out.WriteByte('\n')

			continue
		}

		// Inside a fence: close on a compatible marker with at most three leading
		// spaces and only trailing whitespace after the run.
		if indent <= maxFenceIndent {
			if char, length, isClose := fenceCloser(trimmed); isClose && char == fenceChar && length >= fenceLen {
				inFence = false
			}
		}
	}

	return out.String()
}

// stripIndentedCode removes CommonMark indented code lines (4+ leading spaces).
func stripIndentedCode(content string) string {
	var out strings.Builder

	for line := range strings.SplitSeq(content, "\n") {
		if leadingSpaces(line) > maxFenceIndent {
			continue
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	return out.String()
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// minFenceLen is the minimum run length that opens a CommonMark code fence.
// maxFenceIndent is the maximum leading spaces before a fence becomes indented code.
const (
	minFenceLen    = 3
	maxFenceIndent = 3
)

// fenceOpener reports whether a trimmed line opens a code fence: 3+ of the same
// backtick or tilde. An optional info string may follow the run.
func fenceOpener(line string) (byte, int, bool) {
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

	// CommonMark: a backtick fence's info string cannot contain backticks.
	if char == '`' && strings.ContainsRune(line[length:], '`') {
		return 0, 0, false
	}

	return char, length, true
}

// fenceCloser reports whether a trimmed line closes a code fence: 3+ of the same
// backtick or tilde with only whitespace after the run (no info string).
func fenceCloser(line string) (byte, int, bool) {
	char, length, ok := fenceOpener(line)
	if !ok {
		return 0, 0, false
	}

	if strings.Trim(line[length:], " \t") != "" {
		return 0, 0, false
	}

	return char, length, true
}

// stripInlineCode removes markdown code spans. A span opens with a run of
// backticks and closes only with a run of exactly the same length (CommonMark).
func stripInlineCode(content string) string {
	var out strings.Builder

	for idx := 0; idx < len(content); {
		if content[idx] != '`' || oddEscapes(content, idx) {
			out.WriteByte(content[idx])
			idx++

			continue
		}

		run := 0
		for idx+run < len(content) && content[idx+run] == '`' {
			run++
		}

		closer := findClosingBacktickRun(content, idx+run, run)
		if closer < 0 {
			// No matching closer; not a code span. Keep the backticks as text.
			out.WriteString(strings.Repeat("`", run))

			idx += run

			continue
		}

		idx = closer + run
	}

	return out.String()
}

// findClosingBacktickRun returns the index of the first backtick run whose
// length is exactly `run` at or after `from`, or -1 if none exists. A longer
// run does not close the span, so it is skipped over rather than matched.
func findClosingBacktickRun(content string, from, run int) int {
	for idx := from; idx < len(content); {
		if content[idx] != '`' || oddEscapes(content, idx) {
			idx++
			continue
		}

		length := 0
		for idx+length < len(content) && content[idx+length] == '`' {
			length++
		}

		if length == run {
			return idx
		}

		// Skip past this mismatched run so a longer run is not partially matched.
		idx += length
	}

	return -1
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

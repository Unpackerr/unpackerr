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

	stripped := stripInlineCode(stripCodeBlocks(content))

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
// idx, counting nested unescaped { and }. Braces inside JavaScript strings,
// comments, and regex literals are ignored. idx must point at an unescaped '{'.
func balancedBraceEnd(content string, idx int) int {
	depth := 0

	var (
		punct    jsPunct
		adjacent bool
	)

	for pos := idx; pos < len(content); {
		if !isJSIdentChar(content[pos], punct.ident != "") {
			punct.finishIdent()
		}

		if skip, next := skipJSLexical(content, pos, punct.canStartRegex()); skip {
			if next < 0 {
				return -1
			}

			punct.afterLexical(content, pos, next)

			adjacent = false
			pos = next

			continue
		}

		if isJSSpace(content[pos]) {
			punct.finishIdent()

			adjacent = false
			pos++

			continue
		}

		if oddEscapes(content, pos) {
			pos++
			continue
		}

		var done bool

		done, depth, adjacent = punct.brace(content[pos], depth, adjacent)
		if done {
			return pos + 1
		}

		pos++
	}

	return -1
}

type jsPunct struct {
	prev, before byte
	ident        string
	parenDepth   int
	keyword      bool
	member       bool
	control      bool
	afterCtrl    bool
	forCtrl      bool
	forParen     bool
}

func (punct *jsPunct) saw(char byte, adjacent bool) {
	if isJSIdentChar(char, punct.ident != "") {
		if punct.ident == "" {
			punct.member = punct.prev == '.'
			punct.afterCtrl = false
		}

		punct.ident += string(char)
		punct.sawPunct(char, adjacent)

		return
	}

	punct.finishIdent()
	punct.keyword = false
	punct.trackControl(char)
	punct.sawPunct(char, adjacent)
}

func (punct *jsPunct) trackControl(char byte) {
	switch char {
	case '(':
		if punct.control && punct.parenDepth == 0 {
			punct.parenDepth = 1
			punct.forParen = punct.forCtrl
		} else if punct.parenDepth > 0 {
			punct.parenDepth++
		}

		punct.control = false
		punct.forCtrl = false
		punct.afterCtrl = false
	case ')':
		if punct.parenDepth == 0 {
			punct.afterCtrl = false

			return
		}

		punct.parenDepth--
		if punct.parenDepth == 0 {
			punct.afterCtrl = true
			punct.forParen = false
		}
	default:
		punct.control = false
		punct.forCtrl = false
		punct.afterCtrl = false
	}
}

func (punct *jsPunct) sawPunct(char byte, adjacent bool) {
	if adjacent {
		punct.before = punct.prev
	} else {
		punct.before = 0
	}

	punct.prev = char
}

func (punct *jsPunct) finishIdent() {
	if punct.ident == "" {
		return
	}

	isFor := punct.ident == "for"
	ofInFor := punct.ident == "of" && punct.forParen && punct.parenDepth > 0
	punct.keyword = !punct.member && (jsRegexKeyword(punct.ident) || ofInFor)
	punct.control = !punct.member && jsControlKeyword(punct.ident)
	punct.forCtrl = !punct.member && isFor
	punct.ident = ""
	punct.member = false
}

func (punct *jsPunct) brace(char byte, depth int, adjacent bool) (bool, int, bool) {
	switch char {
	case '{':
		punct.saw('{', adjacent)
		return false, depth + 1, true
	case '}':
		depth--
		if depth == 0 {
			return true, 0, adjacent
		}

		punct.saw('}', adjacent)

		return false, depth, true
	default:
		punct.saw(char, adjacent)

		return false, depth, true
	}
}

func (punct *jsPunct) canStartRegex() bool {
	if punct.keyword || punct.afterCtrl {
		return true
	}

	if punct.prev == '+' && punct.before == '+' {
		return false
	}

	if punct.prev == '-' && punct.before == '-' {
		return false
	}

	return canStartJSRegex(punct.prev)
}

func (punct *jsPunct) afterLexical(content string, start, end int) {
	if start >= len(content) {
		return
	}

	if content[start] == '/' && start+1 < len(content) && (content[start+1] == '/' || content[start+1] == '*') {
		return
	}

	punct.ident = ""
	punct.keyword = false
	punct.member = false
	punct.control = false
	punct.afterCtrl = false

	if end > start {
		punct.prev = content[end-1]
		punct.before = 0
	}
}

func isJSIdentChar(char byte, cont bool) bool {
	if char == '_' || char == '$' ||
		(char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') {
		return true
	}

	return cont && char >= '0' && char <= '9'
}

func jsRegexKeyword(ident string) bool {
	switch ident {
	case "return", "throw", "case", "else", "do",
		"typeof", "delete", "void", "new", "yield", "await",
		"in", "instanceof":
		return true
	default:
		return false
	}
}

func jsControlKeyword(ident string) bool {
	switch ident {
	case "if", "while", "for", "with":
		return true
	default:
		return false
	}
}

func isJSSpace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\r'
}

// skipJSLexical reports whether pos starts a JS string, comment, or regex
// literal and, if so, the index after that construct (or -1 if unterminated).
func skipJSLexical(content string, pos int, startRegex bool) (bool, int) {
	if pos >= len(content) {
		return false, pos
	}

	switch content[pos] {
	case '"', '\'', '`':
		return true, skipJSString(content, pos)
	case '/':
		if pos+1 >= len(content) {
			return false, pos
		}

		switch content[pos+1] {
		case '/':
			return true, skipJSLineComment(content, pos)
		case '*':
			return true, skipJSBlockComment(content, pos)
		}

		if startRegex {
			return true, skipJSRegex(content, pos)
		}
	}

	return false, pos
}

func canStartJSRegex(prev byte) bool {
	switch prev {
	case 0, '(', '[', '{', ',', '=', ':', ';', '!', '&', '|', '?', '+', '-', '*', '%', '^', '~', '<', '>':
		return true
	default:
		return false
	}
}

func skipJSRegex(content string, pos int) int {
	inClass := false

	for idx := pos + 1; idx < len(content); idx++ {
		if content[idx] == '\n' {
			return -1
		}

		if oddEscapes(content, idx) {
			continue
		}

		switch {
		case inClass:
			if content[idx] == ']' {
				inClass = false
			}
		case content[idx] == '[':
			inClass = true
		case content[idx] == '/':
			return skipJSRegexpFlags(content, idx+1)
		}
	}

	return -1
}

func skipJSRegexpFlags(content string, pos int) int {
	for pos < len(content) && isJSRegexpFlag(content[pos]) {
		pos++
	}

	return pos
}

func isJSRegexpFlag(char byte) bool {
	switch char {
	case 'd', 'g', 'i', 'm', 's', 'u', 'v', 'y':
		return true
	default:
		return false
	}
}

func skipJSString(content string, pos int) int {
	quote := content[pos]

	for idx := pos + 1; idx < len(content); idx++ {
		if content[idx] == quote && !oddEscapes(content, idx) {
			return idx + 1
		}
	}

	return -1
}

func skipJSLineComment(content string, pos int) int {
	if nl := strings.IndexByte(content[pos:], '\n'); nl >= 0 {
		return pos + nl
	}

	return len(content)
}

func skipJSBlockComment(content string, pos int) int {
	end := strings.Index(content[pos+2:], "*/")
	if end < 0 {
		return len(content)
	}

	return pos + 2 + end + len("*/")
}

// minFenceLen is the minimum run length that opens a CommonMark code fence.
// maxFenceIndent is the maximum leading columns before a fence becomes indented code.
// tabWidth is the CommonMark tab-stop width used when measuring indentation.
const (
	minFenceLen    = 3
	maxFenceIndent = 3
	tabWidth       = 4
)

// stripCodeBlocks removes fenced and indented code. A fence opens with a run of
// 3+ backticks or tildes (indented at most three columns) and closes only with
// the same marker character, a run at least as long, and whitespace-only
// trailing text (CommonMark). Info strings on the opening fence are ignored.
// Indented code (4+ columns, including tabs) cannot interrupt a paragraph.
func stripCodeBlocks(content string) string {
	scan := mdScan{canStartIndented: true}

	for line := range strings.SplitSeq(content, "\n") {
		scan.line(line)
	}

	return scan.out.String()
}

type mdScan struct {
	out              strings.Builder
	listIndents      []int
	inFence          bool
	fenceChar        byte
	fenceLen         int
	fenceBase        int
	inIndented       bool
	canStartIndented bool
	lastWasParagraph bool
	contentIndent    int
}

func (scan *mdScan) line(line string) {
	indent := leadingIndent(line)
	trimmed := strings.TrimSpace(line)

	if scan.inFence && scan.continueFence(indent, trimmed) {
		return
	}

	scan.syncList(indent, trimmed)

	rel := indent - scan.contentIndent
	if rel < 0 {
		rel = indent
	}

	if scan.openFence(indent, rel, trimmed) {
		return
	}

	if trimmed == "" {
		scan.out.WriteString(line)
		scan.out.WriteByte('\n')

		scan.inIndented, scan.canStartIndented, scan.lastWasParagraph = false, true, false

		return
	}

	if rel > maxFenceIndent && (scan.canStartIndented || scan.inIndented) {
		scan.inIndented = true
		scan.lastWasParagraph = false

		return
	}

	scan.writeContent(line, indent, trimmed, rel)
}

func (scan *mdScan) syncList(indent int, trimmed string) {
	if trimmed == "" {
		return
	}

	scan.popLists(indent)
}

func (scan *mdScan) pushList(col int) {
	scan.listIndents = append(scan.listIndents, col)
	scan.contentIndent = col
}

func (scan *mdScan) popLists(indent int) {
	for len(scan.listIndents) > 0 && indent < scan.listIndents[len(scan.listIndents)-1] {
		scan.listIndents = scan.listIndents[:len(scan.listIndents)-1]
	}

	if len(scan.listIndents) == 0 {
		scan.contentIndent = 0
		return
	}

	scan.contentIndent = scan.listIndents[len(scan.listIndents)-1]
}

func (scan *mdScan) writeContent(line string, indent int, trimmed string, rel int) {
	scan.out.WriteString(line)
	scan.out.WriteByte('\n')

	scan.inIndented = false
	if !thematicBreak(trimmed) && (!scan.lastWasParagraph || !setextUnderline(trimmed)) {
		if col, _, ok := listItemSplit(indent, trimmed); ok && rel <= maxFenceIndent && scan.allowListItem(trimmed) {
			scan.pushList(col)
		}
	}

	scan.canStartIndented = rel <= maxFenceIndent && leafBlockLine(trimmed, scan.lastWasParagraph)
	scan.lastWasParagraph = !scan.canStartIndented
}

func (scan *mdScan) continueFence(indent int, trimmed string) bool {
	if fenceClosed(indent-scan.fenceBase, trimmed, scan.fenceChar, scan.fenceLen) {
		scan.inFence = false
		scan.canStartIndented, scan.lastWasParagraph = true, false

		return true
	}

	if trimmed == "" || indent >= scan.fenceBase {
		return true
	}

	scan.inFence = false
	scan.popLists(indent)
	scan.canStartIndented, scan.lastWasParagraph = true, false

	return false
}

func (scan *mdScan) openFence(indent, rel int, trimmed string) bool {
	if rel > maxFenceIndent {
		return false
	}

	if char, length, isOpen := fenceOpener(trimmed); isOpen {
		scan.startFence(char, length, scan.contentIndent)

		return true
	}

	if scan.lastWasParagraph && setextUnderline(trimmed) {
		return false
	}

	col, rest, ok := listItemSplit(indent, trimmed)
	if !ok || !scan.allowListItem(trimmed) {
		return false
	}

	if char, length, isOpen := fenceOpener(strings.TrimSpace(rest)); isOpen {
		scan.pushList(col)
		scan.startFence(char, length, col)

		return true
	}

	return false
}

func (scan *mdScan) startFence(char byte, length, base int) {
	scan.inFence, scan.fenceChar, scan.fenceLen, scan.fenceBase = true, char, length, base
	scan.inIndented, scan.canStartIndented, scan.lastWasParagraph = false, true, false
}

func fenceClosed(rel int, trimmed string, fenceChar byte, fenceLen int) bool {
	if rel < 0 || rel > maxFenceIndent {
		return false
	}

	char, length, isClose := fenceCloser(trimmed)

	return isClose && char == fenceChar && length >= fenceLen
}

// listItemSplit reports the content column of a CommonMark list item and the
// remainder of the line after the marker and padding. Tabs in the padding
// advance to the next tab stop from the marker's ending column.
func listItemSplit(indent int, trimmed string) (int, string, bool) {
	end, _, ok := consumeListMarker(trimmed)
	if !ok {
		return 0, "", false
	}

	col := indent + end
	if end == len(trimmed) {
		return col + 1, "", true
	}

	if trimmed[end] != ' ' && trimmed[end] != '\t' {
		return 0, "", false
	}

	start := col
	idx := end

	for idx < len(trimmed) {
		var advance int

		switch trimmed[idx] {
		case ' ':
			advance = 1
		case '\t':
			advance = tabWidth - col%tabWidth
		default:
			if col == start {
				return 0, "", false
			}

			return col, trimmed[idx:], true
		}

		if col-start+advance > tabWidth {
			return start + 1, trimmed[end+1:], true
		}

		col += advance
		idx++

		if col-start >= tabWidth {
			return col, trimmed[idx:], true
		}
	}

	if col == start {
		return 0, "", false
	}

	return col, "", true
}

// bulletStart is consumeListMarker's start value for -, *, and +. Ordered
// markers return the parsed number, which is never negative.
const (
	bulletStart = -1
	decimalBase = 10
)

func consumeListMarker(trimmed string) (int, int, bool) {
	if trimmed == "" {
		return 0, 0, false
	}

	switch trimmed[0] {
	case '-', '*', '+':
		return 1, bulletStart, true
	}

	digits := 0
	start := 0

	for digits < len(trimmed) && digits < 9 && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		start = start*decimalBase + int(trimmed[digits]-'0')
		digits++
	}

	if digits == 0 || digits >= len(trimmed) || (trimmed[digits] != '.' && trimmed[digits] != ')') {
		return 0, 0, false
	}

	return digits + 1, start, true
}

func (scan *mdScan) allowListItem(trimmed string) bool {
	if !scan.lastWasParagraph {
		return true
	}

	_, start, ok := consumeListMarker(trimmed)
	if !ok || !listItemHasContent(trimmed) {
		return false
	}

	return start == bulletStart || start == 1
}

func listItemHasContent(trimmed string) bool {
	end, _, ok := consumeListMarker(trimmed)
	if !ok || end >= len(trimmed) {
		return false
	}

	return strings.TrimSpace(trimmed[end:]) != ""
}

// leadingIndent returns the visual column of the first non-whitespace rune.
// CommonMark tabs advance to the next tab stop.
func leadingIndent(line string) int {
	col := 0

	for _, r := range line {
		switch r {
		case ' ':
			col++
		case '\t':
			col += tabWidth - col%tabWidth
		default:
			return col
		}
	}

	return col
}

// leafBlockLine reports whether trimmed is a CommonMark leaf that ends a
// paragraph so indented code may start on the next line. ATX headings always
// qualify. Setext underlines only qualify after a paragraph. Thematic breaks
// qualify when they are not a setext underline (after a paragraph, dashes are
// setext; stars and underscores are still thematic).
func leafBlockLine(trimmed string, lastWasParagraph bool) bool {
	if atxHeading(trimmed) {
		return true
	}

	if lastWasParagraph {
		return setextUnderline(trimmed) || thematicBreak(trimmed)
	}

	return thematicBreak(trimmed)
}

func atxHeading(trimmed string) bool {
	hashes := 0
	for hashes < len(trimmed) && trimmed[hashes] == '#' {
		hashes++
	}

	if hashes < 1 || hashes > 6 {
		return false
	}

	if hashes == len(trimmed) {
		return true
	}

	return trimmed[hashes] == ' ' || trimmed[hashes] == '\t'
}

func thematicBreak(trimmed string) bool {
	var marker byte

	count := 0

	for idx := range len(trimmed) {
		char := trimmed[idx]
		if char == ' ' || char == '\t' {
			continue
		}

		if char != '-' && char != '*' && char != '_' {
			return false
		}

		if marker == 0 {
			marker = char
		} else if char != marker {
			return false
		}

		count++
	}

	return count >= minFenceLen
}

func setextUnderline(trimmed string) bool {
	if trimmed == "" {
		return false
	}

	marker := trimmed[0]
	if marker != '=' && marker != '-' {
		return false
	}

	for idx := 1; idx < len(trimmed); idx++ {
		if trimmed[idx] != marker {
			return false
		}
	}

	return true
}

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

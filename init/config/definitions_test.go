package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadTestConfig(t *testing.T) *Config {
	t.Helper()

	config := &Config{}
	if err := yaml.Unmarshal(confBuilder, config); err != nil {
		t.Fatalf("definitions.yml: %v", err)
	}

	return config
}

func TestDefinitionsFile(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	createDocusaurus(config, dir)
	createConfFile(config, "unpackerr.conf.example", dir)
	createCompose(config, "docker-compose.yml", dir)

	for _, name := range []string{
		"index.md", "footer.md", "global.md", "webserver.md", "folders.md",
		"starr.md", "folder.md", "webhook.md", "cmdhook.md",
		"unpackerr.conf.example", "docker-compose.yml",
	} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing generated file %s: %v", name, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "starr_header.md")); err == nil {
		t.Error("starr_header has no params and must not become a Docusaurus import")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var mdxErrs []string

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}

		mdxErrs = append(mdxErrs, checkGeneratedMDX(entry.Name(), string(body))...)
	}

	if err := fmtErrs(mdxErrs); err != nil {
		t.Fatal(err)
	}

	for _, name := range config.generatedImportNames() {
		if !validImportIdent(name) {
			t.Errorf("section %q is not a valid JS import identifier (website will not compile)", name)
		}
	}
}

func TestMDXAdmonitionInInlineCode(t *testing.T) {
	t.Parallel()

	accepted := mdxProblems("`code\n:::note Title\nmore`", "fixture")
	if len(accepted) != 0 {
		t.Fatalf(":::note Title inside a multiline code span is code: %v", accepted)
	}
}

func TestMDXBacktickFenceInfoString(t *testing.T) {
	t.Parallel()

	problems := mdxProblems("```go`\n{broken}\n```\n", "fixture")
	if len(problems) == 0 {
		t.Fatal("a backtick in a backtick-fence info string is not a fence; { must be flagged")
	}
}

func TestMDXEscapedBackticks(t *testing.T) {
	t.Parallel()

	problems := mdxProblems("\\`{\\`", "fixture")
	if len(problems) == 0 {
		t.Fatal("escaped backticks are not a code span; { must be flagged")
	}
}

func TestMDXAdmonitionTitleNeedsBrackets(t *testing.T) {
	t.Parallel()

	problems := mdxProblems(":::note Metrics\nhello\n:::\n", "fixture")
	if len(problems) == 0 {
		t.Fatal(":::note Title must be flagged; that form fails Docusaurus MDX v2 compile")
	}

	accepted := mdxProblems(":::note[Metrics]\nhello\n:::\n", "fixture")
	if len(accepted) != 0 {
		t.Fatalf(":::note[Title] should be accepted: %v", accepted)
	}
}

func TestMDXUnescapedBrace(t *testing.T) {
	t.Parallel()

	problems := mdxProblems("template value {name} here", "fixture")
	if len(problems) == 0 {
		t.Fatal("bare {name} must be flagged; MDX treats { as JSX")
	}

	accepted := mdxProblems("template value `{{name}}` here", "fixture")
	if len(accepted) != 0 {
		t.Fatalf("braces inside backticks should be accepted: %v", accepted)
	}

	accepted = mdxProblems(`<font style={{'float': 'right', 'font-style': 'italic'}}>`, "fixture")
	if len(accepted) != 0 {
		t.Fatalf("complete {{ JSX }} should be accepted: %v", accepted)
	}

	nested := mdxProblems("{{a: {b: 1}}}", "fixture")
	if len(nested) != 0 {
		t.Fatalf("nested {{ JSX }} objects should be accepted: %v", nested)
	}

	quoted := mdxProblems(`<X value={{text: "}"}} />`, "fixture")
	if len(quoted) != 0 {
		t.Fatalf("braces inside JS strings should be accepted: %v", quoted)
	}

	quoted = mdxProblems(`<X value={{text: "{"}} />`, "fixture")
	if len(quoted) != 0 {
		t.Fatalf("an opening brace inside a JS string should be accepted: %v", quoted)
	}

	commented := mdxProblems("{{a: 1 /* } */}}", "fixture")
	if len(commented) != 0 {
		t.Fatalf("braces inside JS comments should be accepted: %v", commented)
	}

	regex := mdxProblems(`<X value={{re: /[}]/}} />`, "fixture")
	if len(regex) != 0 {
		t.Fatalf("braces inside JS regex character classes should be accepted: %v", regex)
	}

	escapedOpen := mdxProblems(`\{{x}}`, "fixture")
	if len(escapedOpen) == 0 {
		t.Fatal(`\{{x}} is an escaped { plus leftover braces and must be flagged`)
	}

	mixed := mdxProblems("{{name}} and {broken", "fixture")
	if len(mixed) == 0 {
		t.Fatal("a bare { after a valid {{ }} span must still be flagged")
	}

	mixed = mdxProblems("{/* comment */} and {broken", "fixture")
	if len(mixed) == 0 {
		t.Fatal("a bare { after a comment span must still be flagged")
	}
}

func TestMDXAdmonitionInFenceIsCode(t *testing.T) {
	t.Parallel()

	fenced := mdxProblems("```md\n:::note Metrics\n```\n", "fixture")
	if len(fenced) != 0 {
		t.Fatalf(":::note Title inside a code fence is code and should be accepted: %v", fenced)
	}
}

func TestMDXClosingBrace(t *testing.T) {
	t.Parallel()

	problems := mdxProblems("text } more", "fixture")
	if len(problems) == 0 {
		t.Fatal("a lone } must be flagged; MDX treats it as JSX")
	}
}

func TestMDXDoubleBacktickSpan(t *testing.T) {
	t.Parallel()

	accepted := mdxProblems("value ``{name}`` here", "fixture")
	if len(accepted) != 0 {
		t.Fatalf("braces inside a double-backtick span should be accepted: %v", accepted)
	}

	problems := mdxProblems("unmatched ` backtick then {broken", "fixture")
	if len(problems) == 0 {
		t.Fatal("an unmatched backtick must not hide a later bare brace")
	}
}

func TestValidateSectionMissingFromOrder(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Sections["extra"] = config.Sections["global"]

	if err := config.validate(); err == nil {
		t.Fatal("a section missing from order must fail validation")
	}
}

func TestValidateDuplicateOrder(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Order = append(config.Order, config.Order[0])

	if err := config.validate(); err == nil {
		t.Fatal("duplicate order entries must fail validation")
	}
}

func TestValidateDefsNeedOrder(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.DefOrder["starr"] = nil

	if err := config.validate(); err == nil {
		t.Fatal("defs without a non-empty def_order must fail validation")
	}
}

func TestValidateDuplicateDefOrder(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.DefOrder["starr"] = append(config.DefOrder["starr"], config.DefOrder["starr"][0])

	if err := config.validate(); err == nil {
		t.Fatal("duplicate def_order entries must fail validation")
	}
}

func TestValidateDefMissingFromOrder(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	order := config.DefOrder["starr"]
	config.DefOrder["starr"] = order[:len(order)-1]

	if err := config.validate(); err == nil {
		t.Fatal("a defs entry missing from def_order must fail validation")
	}
}

func TestValidateImportIdent(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	header := config.Sections["webserver"]
	delete(config.Sections, "webserver")
	config.Sections["web-server"] = header

	for idx, name := range config.Order {
		if name == "webserver" {
			config.Order[idx] = "web-server"
		}
	}

	if err := config.validate(); err == nil {
		t.Fatal("section web-server must fail; generated import would not compile")
	}
}

func TestValidateNumericImportIdent(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	header := config.Sections["webserver"]
	delete(config.Sections, "webserver")
	config.Sections["1starr"] = header

	for idx, name := range config.Order {
		if name == "webserver" {
			config.Order[idx] = "1starr"
		}
	}

	if err := config.validate(); err != nil {
		t.Fatalf("section 1starr yields valid identifier G1starr and must pass: %v", err)
	}
}

func TestValidateTitleMDX(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Sections["global"].Title = "Hello {world}"

	if err := config.validate(); err == nil {
		t.Fatal("section title with a bare brace must fail validation")
	}

	config = loadTestConfig(t)
	config.Defs["starr"]["radarr"].Title = "Radarr {app}"

	if err := config.validate(); err == nil {
		t.Fatal("defined title with a bare brace must fail validation")
	}
}

func TestValidatePrefixMDX(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Prefix = "BAD{"

	if err := config.validate(); err == nil {
		t.Fatal("envvar_prefix with a bare brace must fail validation")
	}
}

func TestValidateNilParam(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Sections["global"].Params = append(config.Sections["global"].Params, nil)

	if err := config.validate(); err == nil {
		t.Fatal("a null param must fail validation, not panic")
	}
}

func TestValidateMissingDefaultDoesNotPanic(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Sections["global"].Params[0].Default = nil

	if err := config.validate(); err == nil {
		t.Fatal("a missing default must fail validation, not panic")
	}
}

func TestMDXEscapedBrace(t *testing.T) {
	t.Parallel()

	accepted := mdxProblems(`literal \{name\} here`, "fixture")
	if len(accepted) != 0 {
		t.Fatalf("backslash-escaped braces should be accepted: %v", accepted)
	}

	// Even backslashes escape the backslash, leaving the brace bare.
	problems := mdxProblems(`literal \\{name`, "fixture")
	if len(problems) == 0 {
		t.Fatal("an even backslash run leaves { unescaped and must be flagged")
	}
}

func TestMDXTildeAndLongFence(t *testing.T) {
	t.Parallel()

	// A four-backtick fence containing a triple-backtick line stays code.
	fenced := mdxProblems("````\n```\n{notFlagged}\n````\n", "fixture")
	if len(fenced) != 0 {
		t.Fatalf("braces inside a longer fence should be accepted: %v", fenced)
	}

	// A tilde fence is also valid code fencing.
	tilde := mdxProblems("~~~md\n:::note Title\n~~~\n", "fixture")
	if len(tilde) != 0 {
		t.Fatalf("content inside a ~~~ fence is code and should be accepted: %v", tilde)
	}

	// A marker indented four spaces is indented code, not a fence opener, so
	// braces on that line are also code and must not be flagged.
	indented := mdxProblems("    ```{broken}\n", "fixture")
	if len(indented) != 0 {
		t.Fatalf("a four-space-indented line is indented code: %v", indented)
	}

	// A closing fence indented four spaces does not close; later braces stay code.
	indentedClose := mdxProblems("```\n    ```\n{stillCode}\n```\n", "fixture")
	if len(indentedClose) != 0 {
		t.Fatalf("a four-space-indented closer must not end the fence: %v", indentedClose)
	}

	// A closing fence with trailing text does not close the block.
	notClose := mdxProblems("```\n```not-a-close\n{inside}\n```\n", "fixture")
	if len(notClose) != 0 {
		t.Fatalf("a fence line with trailing text must not close the block: %v", notClose)
	}
}

func TestMDXIndentedCodeContext(t *testing.T) {
	t.Parallel()

	// CommonMark: indented code cannot interrupt a paragraph.
	continuation := mdxProblems("text\n    {broken", "fixture")
	if len(continuation) == 0 {
		t.Fatal("an indented line continuing a paragraph is MDX; { must be flagged")
	}

	afterBlank := mdxProblems("text\n\n    {ok}\n", "fixture")
	if len(afterBlank) != 0 {
		t.Fatalf("indented code after a blank line is code: %v", afterBlank)
	}

	afterFence := mdxProblems("```\ncode\n```\n    {ok}\n", "fixture")
	if len(afterFence) != 0 {
		t.Fatalf("indented code after a fence is code: %v", afterFence)
	}

	tabbed := mdxProblems("\t{ok}\n", "fixture")
	if len(tabbed) != 0 {
		t.Fatalf("a tab-indented line is indented code: %v", tabbed)
	}

	tabCloser := mdxProblems("```\n\t```\n{stillCode}\n```\n", "fixture")
	if len(tabCloser) != 0 {
		t.Fatalf("a tab-indented closer must not end the fence: %v", tabCloser)
	}

	tabOpener := mdxProblems("\t```\n{broken}\n```\n", "fixture")
	if len(tabOpener) == 0 {
		t.Fatal("a tab-indented ``` is indented code, not a fence; later { must be flagged")
	}

	afterHeading := mdxProblems("# Heading\n    {ok}\n", "fixture")
	if len(afterHeading) != 0 {
		t.Fatalf("indented code after an ATX heading is code: %v", afterHeading)
	}

	afterBreak := mdxProblems("---\n    {ok}\n", "fixture")
	if len(afterBreak) != 0 {
		t.Fatalf("indented code after a thematic break is code: %v", afterBreak)
	}

	afterList := mdxProblems("- item\n    {broken", "fixture")
	if len(afterList) == 0 {
		t.Fatal("an indented line continuing a list paragraph is MDX; { must be flagged")
	}

	listBlank := mdxProblems("- item\n\n    {broken", "fixture")
	if len(listBlank) == 0 {
		t.Fatal("four spaces in a list item is a paragraph, not indented code; { must be flagged")
	}

	listCode := mdxProblems("- item\n\n      {ok}\n", "fixture")
	if len(listCode) != 0 {
		t.Fatalf("six spaces in a list item is indented code: %v", listCode)
	}
}

func TestMDXJSXKeywordRegex(t *testing.T) {
	t.Parallel()

	ret := mdxProblems("{{fn: function () { return /}/ }}}", "fixture")
	if len(ret) != 0 {
		t.Fatalf("a regex after return should be accepted: %v", ret)
	}

	thr := mdxProblems("{{fn: function () { throw /}/ }}}", "fixture")
	if len(thr) != 0 {
		t.Fatalf("a regex after throw should be accepted: %v", thr)
	}

	prop := mdxProblems("{{value: obj.return / 2}}", "fixture")
	if len(prop) != 0 {
		t.Fatalf("division after a .return property is not a regex: %v", prop)
	}

	div := mdxProblems("{{fn: () => { return 1 / 2 }}}", "fixture")
	if len(div) != 0 {
		t.Fatalf("division after return 1 is not a regex: %v", div)
	}

	ctrl := mdxProblems("{{fn: x => { if (x) /}/.test(x) }}}", "fixture")
	if len(ctrl) != 0 {
		t.Fatalf("a regex after if (x) should be accepted: %v", ctrl)
	}

	inOp := mdxProblems(`{{ok: "x" in /}/}}`, "fixture")
	if len(inOp) != 0 {
		t.Fatalf("a regex after in should be accepted: %v", inOp)
	}

	inst := mdxProblems("{{ok: x instanceof /}/}}", "fixture")
	if len(inst) != 0 {
		t.Fatalf("a regex after instanceof should be accepted: %v", inst)
	}

	ofFor := mdxProblems("{{fn: () => { for (const x of /}/g) {} }}}", "fixture")
	if len(ofFor) != 0 {
		t.Fatalf("a regex after for-of should be accepted: %v", ofFor)
	}

	ofVar := mdxProblems("{{n: of /}/}}", "fixture")
	if len(ofVar) == 0 {
		t.Fatal("division after an of variable is not a regex; } must be flagged")
	}
}

func TestMDXJSXPostfixDivision(t *testing.T) {
	t.Parallel()

	plus := mdxProblems("{{n: i++ / 2}}", "fixture")
	if len(plus) != 0 {
		t.Fatalf("division after postfix ++ is not a regex: %v", plus)
	}

	minus := mdxProblems("{{n: i-- / 2}}", "fixture")
	if len(minus) != 0 {
		t.Fatalf("division after postfix -- is not a regex: %v", minus)
	}
}

func TestMDXSetextAndIndentedHeading(t *testing.T) {
	t.Parallel()

	indentedHeading := mdxProblems("text\n    # still paragraph\n    {broken", "fixture")
	if len(indentedHeading) == 0 {
		t.Fatal("an indented # line continues the paragraph; later { must be flagged")
	}

	setext := mdxProblems("Title\n-\n    {ok}\n", "fixture")
	if len(setext) != 0 {
		t.Fatalf("indented code after a one-dash setext heading is code: %v", setext)
	}

	notSetext := mdxProblems("--\n    {broken", "fixture")
	if len(notSetext) == 0 {
		t.Fatal("a standalone -- is a paragraph, not a setext underline; { must be flagged")
	}

	spaced := mdxProblems("text\n- -\n    {broken", "fixture")
	if len(spaced) == 0 {
		t.Fatal("spaces in - - are not a setext underline; { must be flagged")
	}
}

func TestMDXListFenceAndTabPad(t *testing.T) {
	t.Parallel()

	tabPad := mdxProblems("-\titem\n\n      {broken", "fixture")
	if len(tabPad) == 0 {
		t.Fatal("a tab after - pads to column 4; six spaces is still a list paragraph")
	}

	listFence := mdxProblems("- ```\n  code\n  ```\n{broken", "fixture")
	if len(listFence) == 0 {
		t.Fatal("a fence starting after a list marker must close; later { must be flagged")
	}

	dedent := mdxProblems("- ```\n  code\noutside {broken", "fixture")
	if len(dedent) == 0 {
		t.Fatal("a list fence ends when the line leaves the list; later { must be flagged")
	}

	blank := mdxProblems("- ```\n  code\n\n  {ok}\n  ```\n", "fixture")
	if len(blank) != 0 {
		t.Fatalf("a blank line does not end a list fence: %v", blank)
	}

	indentedList := mdxProblems("    - ```\n{broken", "fixture")
	if len(indentedList) == 0 {
		t.Fatal("an indented-code list marker is not a fence opener; later { must be flagged")
	}

	rootFence := mdxProblems("- ```\n  inner\n```\n{ok}\n", "fixture")
	if len(rootFence) != 0 {
		t.Fatalf("a dedented fence after a list item opens a new fence: %v", rootFence)
	}

	nested := mdxProblems("- outer\n  - inner\n  outer\n\n    {broken", "fixture")
	if len(nested) == 0 {
		t.Fatal("dedenting to the outer list keeps its indent; { must be flagged")
	}

	ordered := mdxProblems("text\n2. ~~~\n   {broken}\n   ~~~", "fixture")
	if len(ordered) == 0 {
		t.Fatal("a 2. marker cannot interrupt a paragraph; { must be flagged")
	}

	one := mdxProblems("text\n1. ~~~\n   {ok}\n   ~~~\n", "fixture")
	if len(one) != 0 {
		t.Fatalf("a 1. list can interrupt a paragraph with a fence: %v", one)
	}
}

func TestMDXThematicAndEmptyList(t *testing.T) {
	t.Parallel()

	breakThenCode := mdxProblems("* * *\n    {ok}\n", "fixture")
	if len(breakThenCode) != 0 {
		t.Fatalf("indented code after a thematic break is code: %v", breakThenCode)
	}

	emptyStar := mdxProblems("text\n*\n\n    {ok}\n", "fixture")
	if len(emptyStar) != 0 {
		t.Fatalf("an empty * cannot interrupt a paragraph; later indent is code: %v", emptyStar)
	}
}

func TestMDXExactBacktickRunCloses(t *testing.T) {
	t.Parallel()

	// `` opens a span; a later ``` run must NOT close it, so {broken} is MDX.
	mismatched := mdxProblems("`` {broken} ```", "fixture")
	if len(mismatched) == 0 {
		t.Fatal("a longer backtick run must not close the span; { must be flagged")
	}

	// But an exact matching run does close it.
	matched := mdxProblems("`` {ok} ``", "fixture")
	if len(matched) != 0 {
		t.Fatalf("an exact backtick run closes the span: %v", matched)
	}
}

func TestValidateNilDefRendering(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	config.Defs["starr"]["radarr"] = nil

	if err := config.validate(); err == nil {
		t.Fatal("a null def entry must fail validation, not panic")
	}
}

func TestValidateListKindScalar(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)
	found := false

	for _, param := range config.Sections["global"].Params {
		if param != nil && param.Kind == list {
			param.Default = "not-a-list"
			found = true

			break
		}
	}

	if !found {
		t.Fatal("fixture missing a list param")
	}

	if err := config.validate(); err == nil {
		t.Fatal("a scalar default on a list param must fail validation, not panic")
	}
}

func TestValidateListKindOverride(t *testing.T) {
	t.Parallel()

	config := loadTestConfig(t)

	def := config.Defs["starr"]["radarr"]
	if def.Defaults == nil {
		def.Defaults = map[string]any{}
	}

	def.Defaults["paths"] = "/downloads"

	if err := config.validate(); err == nil {
		t.Fatal("a scalar list override must fail validation, not panic")
	}
}

func TestComposeListScalarDoesNotPanic(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{list, "conlist"} {
		param := &Param{Kind: kind, EnvVar: "X", Default: "not-a-list"}
		if got := param.Compose("UN_"); got != "" {
			t.Fatalf("%s scalar compose should emit nothing, got %q", kind, got)
		}
	}
}

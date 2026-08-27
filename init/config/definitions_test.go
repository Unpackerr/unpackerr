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

	param := &Param{Kind: list, EnvVar: "X", Default: "not-a-list"}
	if got := param.Compose("UN_"); got != "" {
		t.Fatalf("scalar list compose should emit nothing, got %q", got)
	}
}

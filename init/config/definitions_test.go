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

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

	ok := mdxProblems(":::note[Metrics]\nhello\n:::\n", "fixture")
	if len(ok) != 0 {
		t.Fatalf(":::note[Title] should be accepted: %v", ok)
	}
}

func TestMDXUnescapedBrace(t *testing.T) {
	t.Parallel()

	problems := mdxProblems("template value {name} here", "fixture")
	if len(problems) == 0 {
		t.Fatal("bare {name} must be flagged; MDX treats { as JSX")
	}

	ok := mdxProblems("template value `{{name}}` here", "fixture")
	if len(ok) != 0 {
		t.Fatalf("braces inside backticks should be accepted: %v", ok)
	}
}

//go:generate go run . --type config,compose --file definitions.yml --output ../../examples

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

const (
	list           = "list"
	dirMode        = 0o755
	fileMode       = 0o644
	outputDir      = "generated/"
	exampleConfig  = "unpackerr.conf.example"
	exampleCompose = "docker-compose.yml"
	opTimeout      = 6 * time.Second
)

//go:embed definitions.yml
var confBuilder []byte

type section string

type Option struct {
	Name  string `yaml:"name"`
	Value any    `yaml:"value"`
}

type Config struct {
	DefOrder map[section][]section `yaml:"def_order"`
	Defs     map[section]Defs      `yaml:"defs"`
	Prefix   string                `yaml:"envvar_prefix"`
	Order    []section             `yaml:"order"`
	Sections map[section]*Header   `yaml:"sections"`
}

type Header struct {
	Tail     string   `yaml:"tail"`
	Title    string   `yaml:"title"`
	Text     string   `yaml:"text"`
	Docs     string   `yaml:"docs"`
	Notes    string   `yaml:"notes"`
	Prefix   string   `yaml:"envvar_prefix"`
	Params   []*Param `yaml:"params"`
	Kind     string   `yaml:"kind"`      // "", list
	NoHeader bool     `yaml:"no_header"` // Do not print [section] header.
}

type Param struct {
	Name      string   `yaml:"name"`
	EnvVar    string   `yaml:"envvar"`
	Default   any      `yaml:"default"`
	Docker    any      `yaml:"docker"`
	Example   any      `yaml:"example"`
	Short     string   `yaml:"short"`
	Desc      string   `yaml:"desc"`
	Kind      string   `yaml:"kind"` // "", list, conlist
	Recommend []Option `yaml:"recommend"`
	Apps      []string `yaml:"apps"` // If set, param only appears for these starr app names (e.g. lidarr).
}

type Def struct {
	Comment       bool           `yaml:"comment"` // just the header.
	Title         string         `yaml:"title"`
	Prefix        string         `yaml:"prefix"`
	Text          string         `yaml:"text"`
	Defaults      map[string]any `yaml:"defaults"`
	Examples      map[string]any `yaml:"examples"`
	DockerExample map[string]any `yaml:"docker_example"`
}

type Defs map[section]*Def

func main() {
	flags := parseFlags()

	file, err := openFile(flags.File)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	config := &Config{}
	// Decode definitions file into Go data structure.
	if err = yaml.NewDecoder(file).Decode(config); err != nil {
		log.Fatalln(err) //nolint:gocritic
	}

	for _, builder := range flags.Type {
		switch builder {
		case "doc", "docs", "documentation", "docusaurus":
			log.Println("Building Docusaurus")
			createDocusaurus(config, flags.Output)
		case "conf", "config", "example":
			log.Println("Building Config File")
			createConfFile(config, flags.Config, flags.Output)
		case "docker", "compose", "yml":
			log.Println("Building Docker Compose")
			createCompose(config, flags.Compose, flags.Output)
		default:
			log.Println("Unknown type: " + builder)
		}
	}
}

type flags struct {
	Type    []string
	Config  string
	Compose string
	Output  string
	File    string
}

func parseFlags() *flags {
	flags := flags{}
	flag.StringSliceVarP(&flags.Type, "type", "t", []string{"compose", "docs", "config"},
		"Choose 1 or more outputs, or don't and get them all.")
	flag.StringVar(&flags.Config, "config", exampleConfig,
		"Choose filename for generated config file.")
	flag.StringVar(&flags.Compose, "compose", exampleCompose,
		"Choose a filename for the generated docker compose service.")
	flag.StringVar(&flags.Output, "output", outputDir,
		"Choose folder for generated files.")
	flag.StringVarP(&flags.File, "file", "f", "internal",
		"URL or filepath for definitions.yml, 'internal' uses the compiled-in file.")
	flag.Parse()

	return &flags
}

// openFile opens a file or url for the parser, or returns the internal file.
func openFile(fileName string) (io.ReadCloser, error) {
	if fileName == "internal" {
		buf := bytes.NewBuffer(confBuilder)
		return io.NopCloser(buf), nil
	}

	if !strings.HasPrefix(fileName, "http") {
		file, err := os.Open(fileName)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fileName, err)
		}

		return file, nil
	}

	http.DefaultClient.Timeout = opTimeout
	//nolint:noctx,gosec // because we set a timeout.
	resp, err := http.Get(fileName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileName, err)
	}

	return resp.Body, nil
}

func createDefinedSection(def *Def, section *Header, sectionName section) *Header {
	params := make([]*Param, 0, len(section.Params))
	// Filter params to only those that apply to this app (empty Apps = all apps).
	for _, p := range section.Params {
		if len(p.Apps) == 0 || slices.Contains(p.Apps, string(sectionName)) {
			params = append(params, p)
		}
	}

	newSection := &Header{
		Text:   def.Text,
		Prefix: def.Prefix,
		Title:  def.Title,
		Params: params,
		Kind:   section.Kind,
	}

	// Loop each defined section Defaults, and see if one of the param names match.
	for overrideName, override := range def.Defaults {
		for _, defined := range newSection.Params {
			// If the name of the default (override) matches this param name, overwrite the value.
			if defined.Name == overrideName {
				defined.Default = override
			}
		}
	}

	// Do it again, but with examples.
	for overrideName, override := range def.Examples {
		for _, defined := range newSection.Params {
			if defined.Name == overrideName {
				defined.Example = override
			}
		}
	}

	// Do it again, but with the docker defaults.
	for overrideName, override := range def.DockerExample {
		for _, defined := range newSection.Params {
			if defined.Name == overrideName {
				defined.Docker = override
			}
		}
	}

	return newSection
}

// This is used only by compose and config. docs has it's own.
func writeFile(dir, output string, buf *bytes.Buffer) {
	_ = os.Mkdir(dir, dirMode)
	filePath := filepath.Join(dir, output)
	log.Printf("Writing: %s, size: %d", filePath, buf.Len())
	buf.WriteString("## => Content Auto Generated, " +
		strings.ToUpper(time.Now().UTC().Round(time.Second).Format("02 Jan 2006 15:04 UTC")+"\n"))

	if err := os.WriteFile(filePath, buf.Bytes(), fileMode); err != nil {
		log.Fatalln(err)
	}
}

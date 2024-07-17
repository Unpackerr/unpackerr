//go:generate go run . --config ../../examples/unpackerr.conf.example --compose ../../examples/docker-compose.yml --type config,compose --file definitions.yml

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	Example   any      `yaml:"example"`
	Short     string   `yaml:"short"`
	Desc      string   `yaml:"desc"`
	Kind      string   `yaml:"kind"` // "", list, conlist
	Recommend []Option `yaml:"recommend"`
}

type Def struct {
	Comment  bool           `yaml:"comment"` // just the header.
	Title    string         `yaml:"title"`
	Prefix   string         `yaml:"prefix"`
	Text     string         `yaml:"text"`
	Defaults map[string]any `yaml:"defaults"`
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
			printDocusaurus(config, flags.Docs)
		case "conf", "config", "example":
			log.Println("Building Config File")
			printConfFile(config, flags.Config)
		case "docker", "compose", "yml":
			log.Println("Building Docker Compose")
			createCompose(config, flags.Compose)
		default:
			log.Println("Unknown type: " + builder)
		}
	}
}

type flags struct {
	Type    []string
	Config  string
	Compose string
	Docs    string
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
	flag.StringVar(&flags.Docs, "docs", outputDir,
		"Choose folder for generated documentation.")
	flag.StringVarP(&flags.File, "file", "f", "internal",
		"URL or filepath for definitions.yml, 'internal' uses the compiled-in file.")
	flag.Parse()

	return &flags
}

// openFile opens a file or url for the parser, or returns the internal file.
func openFile(fileName string) (io.ReadCloser, error) {
	if fileName == "internal" {
		buf := bytes.Buffer{}
		buf.Write(confBuilder)

		return io.NopCloser(&buf), nil
	}

	if strings.HasPrefix(fileName, "http") {
		http.DefaultClient.Timeout = opTimeout

		resp, err := http.Get(fileName) //nolint:noctx,gosec // because we set a timeout.
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fileName, err)
		}

		return resp.Body, nil
	}

	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileName, err)
	}

	return file, nil
}

func createDefinedSection(def *Def, section *Header) *Header {
	newSection := &Header{
		Text:   def.Text,
		Prefix: def.Prefix,
		Title:  def.Title,
		Params: section.Params,
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

	return newSection
}

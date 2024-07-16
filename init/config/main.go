package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	list      = "list"
	inputFile = "https://raw.githubusercontent.com/Unpackerr/unpackerr/main/init/config/conf-builder.yml"
	opTimeout = 6 * time.Second
)

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
	file, err := openFile()
	if err != nil {
		panic(err)
	}
	defer file.Close()

	config := &Config{}
	// Decode conf-builder file into Go data structure.
	if err = yaml.NewDecoder(file).Decode(config); err != nil {
		panic(err)
	}

	switch {
	default:
		fallthrough
	case len(os.Args) <= 1:
		fallthrough
	case os.Args[1] == "conf":
		printConfFile(config)
	case os.Args[1] == "compose", os.Args[1] == "docker":
		printCompose(config)
	case os.Args[1] == "docs":
		printDocusaurus(config)
	}
}

// openFile opens a file or url for the parser.
func openFile() (io.ReadCloser, error) {
	fileName := inputFile
	if len(os.Args) > 2 { //nolint:mnd
		fileName = os.Args[len(os.Args)-1] // take last arg as file.
	}

	if strings.HasPrefix(fileName, "http") {
		http.DefaultClient.Timeout = opTimeout

		resp, err := http.Get(fileName) //nolint:noctx // because we set a timeout.
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

package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

const (
	list = "list"
)

type Config struct {
	Defs     map[string]Defs    `yaml:"defs"`
	Prefix   string             `yaml:"envvar_prefix"`
	Order    []string           `yaml:"order"`
	Sections map[string]*Header `yaml:"sections"`
}

type Header struct {
	Text     string   `yaml:"text"`
	Prefix   string   `yaml:"envvar_prefix"`
	Params   []*Param `yaml:"params"`
	Kind     string   `yaml:"kind"`      // "", list
	NoHeader bool     `yaml:"no_header"` // Do not print [section] header.
}

type Param struct {
	Name    string `yaml:"name"`
	EnvVar  string `yaml:"envvar"`
	Default any    `yaml:"default"`
	Example any    `yaml:"example"`
	Short   string `yaml:"short"`
	Desc    string `yaml:"desc"`
	Kind    string `yaml:"kind"` // "", list, conlist
}

type Def struct {
	Comment  bool           `yaml:"comment"` // just the header.
	Prefix   string         `yaml:"prefix"`
	Text     string         `yaml:"text"`
	Defaults map[string]any `yaml:"defaults"`
}

type Defs map[string]*Def

func main() {
	file, err := os.Open("./conf-builder.yml")
	if err != nil {
		panic(err)
	}

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

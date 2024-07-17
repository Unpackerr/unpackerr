package examples

// This file allows unpackerr to import the example config file into the binary.

import _ "embed"

var (
	//go:embed unpackerr.conf.example
	ConfigFile []byte
	//go:embed docker-compose.yml
	DockerCompose []byte
	//go:embed MANUAL.md
	ManualMakrdown []byte
)

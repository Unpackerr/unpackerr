package main

import (
	"fmt"
	"strings"
)

const (
	space         = "   "
	composeHeader = `### Unpackerr docker-compose.yml Example
### Please read this page for help using this example:
### https://unpackerr.zip/docs/install/compose
### Generator: https://notifiarr.com/unpackerr
##################################################################
services:

  unpackerr:
    image: golift/unpackerr
    container_name: unpackerr
    volumes:
      # You need at least this one volume mapped so Unpackerr can find your files to extract.
      # Make sure this matches your Starr apps; the folder mount (/downloads or /data) should be identical.
      - /mnt/HostDownloads:/downloads
    restart: always
    # Get the user:group correct so unpackerr can read and write to your files.
    user: ${PUID}:${PGID}
    #user: 1000:100
    # What you see below are defaults for this compose. You only need to modify things specific to your environment.
    # Remove apps and feature configs you do not use or need. 
    # ie. Remove all lines that begin with UN_CMDHOOK, UN_WEBHOOK, UN_FOLDER, UN_WEBSERVER, and other apps you do not use.
    environment:
    - TZ=${TZ}`
)

func printCompose(config *Config) {
	fmt.Println(composeHeader)

	// Loop the 'Order' list.
	for _, section := range config.Order {
		// If Order contains a missing section, panic.
		if config.Sections[section] == nil {
			panic(section + ": in order, but missing from sections. This is a bug in conf-builder.yml.")
		}

		if config.Defs[section] == nil {
			config.Sections[section].printCompose(strings.Title(section), config.Prefix) //nolint:staticcheck
		} else {
			config.Sections[section].printComposeDefined(config.Prefix, config.Defs[section])
		}
	}
}

func (h *Header) printCompose(title, prefix string) {
	if len(h.Params) > 0 {
		fmt.Println(space, "##", title)
	}

	for _, param := range h.Params {
		if h.Kind == list {
			fmt.Print(param.Compose(prefix + h.Prefix + "0_"))
		} else {
			fmt.Print(param.Compose(prefix + h.Prefix))
		}
	}
}

func (h *Header) printComposeDefined(prefix string, defs Defs) {
	for section, def := range defs {
		// Loop each defined section Defaults, and see if one of the param names match.
		for overrideName, override := range def.Defaults {
			for _, defined := range h.Params {
				// If the name of the default (override) matches this param name, overwrite the value.
				if defined.Name == overrideName {
					defined.Default = override
				}
			}
		}

		// Make a brand new section and print it.
		(&Header{
			Text:   def.Text,
			Prefix: def.Prefix,
			Params: h.Params,
			Kind:   h.Kind,
		}).printCompose(strings.Title(section), prefix) //nolint:staticcheck
	}
}

func (p *Param) Compose(prefix string) string {
	val := p.Default
	if p.Example != nil {
		val = p.Example
	}

	switch p.Kind {
	default:
		return fmt.Sprint(space, " - ", prefix, p.EnvVar, "=", val, "\n")
	case list:
		var out string

		for idx, sv := range val.([]any) { //nolint:forcetypeassert
			out += fmt.Sprint(space, " - ", prefix, p.EnvVar, idx, "=", sv, "\n")
		}

		return out
	case "conlist":
		out := []string{}

		for _, sv := range val.([]any) { //nolint:forcetypeassert
			out = append(out, fmt.Sprint(sv))
		}

		return fmt.Sprint(space, " - ", prefix, p.EnvVar, "=", strings.Join(out, ","), "\n")
	}
}

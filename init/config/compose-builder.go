package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
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
  ## Copy the service below to your file if you have other services.
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

func createCompose(config *Config, output string) {
	buf := bytes.Buffer{}
	buf.WriteString(composeHeader + "\n")

	// Loop the 'Order' list.
	for _, section := range config.Order {
		// If Order contains a missing section, bail.
		if config.Sections[section] == nil {
			log.Fatalln(section + ": in order, but missing from sections. This is a bug in conf-builder.yml.")
		}

		if config.Defs[section] == nil {
			buf.WriteString(config.Sections[section].makeCompose(config.Prefix, false))
		} else {
			buf.WriteString(config.Sections[section].makeComposeDefined(config.Prefix, config.Defs[section], config.DefOrder[section], false))
		}
	}

	log.Println("Writing", output, "size:", buf.Len())

	if err := os.WriteFile(output, buf.Bytes(), fileMode); err != nil {
		log.Fatalln(err)
	}
}

func (h *Header) makeCompose(prefix string, bare bool) string {
	var buf bytes.Buffer

	if len(h.Params) > 0 && bare {
		buf.WriteString("## " + h.Title + "\n")
	} else if len(h.Params) > 0 {
		buf.WriteString(space + " ## " + h.Title + "\n")
	}

	pfx := space + " - "
	if bare {
		pfx = ""
	}

	for _, param := range h.Params {
		if h.Kind == list {
			buf.WriteString(param.Compose(pfx + prefix + h.Prefix + "0_"))
		} else {
			buf.WriteString(param.Compose(pfx + prefix + h.Prefix))
		}
	}

	return buf.String()
}

func (h *Header) makeComposeDefined(prefix string, defs Defs, order []section, bare bool) string {
	var buf bytes.Buffer

	for _, section := range order {
		newHeader := createDefinedSection(defs[section], h)
		// Make a brand new section and print it.
		buf.WriteString(newHeader.makeCompose(prefix, bare))
	}

	return buf.String()
}

func (p *Param) Compose(prefix string) string {
	val := p.Default
	if p.Example != nil {
		val = p.Example
	}

	switch p.Kind {
	default:
		return fmt.Sprint(prefix, p.EnvVar, "=", val, "\n")
	case list:
		var out string

		for idx, sv := range val.([]any) { //nolint:forcetypeassert
			out += fmt.Sprint(prefix, p.EnvVar, idx, "=", sv, "\n")
		}

		return out
	case "conlist":
		out := []string{}

		for _, sv := range val.([]any) { //nolint:forcetypeassert
			out = append(out, fmt.Sprint(sv))
		}

		return fmt.Sprint(prefix, p.EnvVar, "=", strings.Join(out, ","), "\n")
	}
}

package main

import (
	"log"
	"os"
	"time"

	"github.com/davidnewhall/unpackerr/pkg/ui"
	"github.com/davidnewhall/unpackerr/pkg/unpackerr"
)

// Keep it simple.
func main() {
	// Set time zone based on TZ env variable.
	setTimeZone(os.Getenv("TZ"))
	ui.HideConsoleWindow()

	defer func() {
		if r := recover(); r != nil {
			ui.ShowConsoleWindow()
			log.Printf("[PANIC] %v", r)
		}
	}()

	if err := unpackerr.Start(); err != nil {
		//nolint:gocritic // defer will not run, that's ok!
		log.Fatalln("[ERROR]", err)
	}
}

func setTimeZone(tz string) {
	if tz == "" {
		return
	}

	var err error

	if time.Local, err = time.LoadLocation(tz); err != nil {
		log.Printf("[ERROR] Loading TZ Location '%s': %v", tz, err)
	}
}

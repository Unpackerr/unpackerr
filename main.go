package main

import (
	"log"

	"github.com/davidnewhall/unpackerr/pkg/ui"
	"github.com/davidnewhall/unpackerr/pkg/unpackerr"
)

// Keep it simple.
func main() {
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

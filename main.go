package main

import (
	"log"
	"runtime/debug"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/Unpackerr/unpackerr/pkg/unpackerr"
)

// Keep it simple.
func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] %v\n%s", r, string(debug.Stack()))
		}
	}()

	if err := unpackerr.Start(); err != nil {
		_, _ = ui.Error("Unpackerr Error", err.Error())
		log.Fatalln("[ERROR]", err) //nolint:gocritic
	}
}

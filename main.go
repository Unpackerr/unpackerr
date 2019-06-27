package main

import (
	"log"

	"github.com/davidnewhall/unpacker-poller/unpackerpoller"
)

// Keep it simple.
func main() {
	if err := unpackerpoller.Start(); err != nil {
		log.Fatalln("[ERROR]", err)
	}
}

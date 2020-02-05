package main

import (
	"log"

	"github.com/davidnewhall/deluge-unpacker/delugeunpacker"
)

// Keep it simple.
func main() {
	if err := delugeunpacker.Start(); err != nil {
		log.Fatalln("[ERROR]", err)
	}
}

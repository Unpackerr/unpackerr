package main

import (
	"log"

	"github.com/davidnewhall/unpackerr/unpacker"
)

// Keep it simple.
func main() {
	if err := unpacker.Start(); err != nil {
		log.Fatalln("[ERROR]", err)
	}
}

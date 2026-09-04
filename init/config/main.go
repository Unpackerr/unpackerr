//go:generate go run . --type config,compose --output ../../examples

package main

import "github.com/Unpackerr/unpackerr/pkg/configdef"

func main() {
	configdef.Run()
}

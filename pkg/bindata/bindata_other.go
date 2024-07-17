//go:build !windows

package bindata

import _ "embed"

//go:embed macos.png
var SystrayIcon []byte

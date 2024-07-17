//go:build windows

package bindata

import _ "embed"

//go:embed windows.ico
var SystrayIcon []byte

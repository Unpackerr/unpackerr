//go:build windows

package unpackerr

const defaultSavePath = `C:\downloads`

func getUmask() int {
	return -1
}

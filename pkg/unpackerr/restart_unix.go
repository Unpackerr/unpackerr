//go:build !windows

package unpackerr

import (
	"fmt"
	"os"
	"syscall"
)

// restartProcess replaces this process image in place. The PID is kept, so a
// Docker PID 1 or systemd unit sees a reload, not an exit.
func restartProcess(exe string) error {
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil { //nolint:gosec // re-exec of our own binary and args.
		return fmt.Errorf("exec %s: %w", exe, err)
	}

	return nil
}

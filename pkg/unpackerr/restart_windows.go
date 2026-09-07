package unpackerr

import (
	"fmt"
	"os"
	"os/exec"
)

// restartProcess starts a fresh copy; the caller exits. Windows has no exec(2).
func restartProcess(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...) //nolint:gosec,noctx // relaunching ourselves.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", exe, err)
	}

	return nil
}

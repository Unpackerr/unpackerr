package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golift.io/cnfg"
)

func runCmdWithLog(log Logger, hook *Config, payload *Payload, qlen, qcap int) {
	out, err := runCmd(hook, payload)

	hook.Lock() // we only lock for the integer increments.
	defer hook.Unlock()
	hook.posts++ //nolint:wsl_v5

	switch {
	case err != nil:
		log.Errorf("Command Hook (%s) %s: %v: %s", payload.Event, hook.Name, err, out.String())
		hook.fails++
	case hook.Silent || out == nil:
		log.Printf("[Cmdhook] Queue: %d/%d. Ran command %s", qlen, qcap, hook.Name)
	default:
		log.Printf("[Cmdhook] Queue: %d/%d. Ran command %s: %s",
			qlen, qcap, hook.Name, strings.TrimSpace(out.String()))
	}
}

func runCmd(hook *Config, payload *Payload) (*bytes.Buffer, error) {
	if hook.Command == "" {
		return nil, ErrCmdhookNoCmd
	}

	payload.Config = hook

	env, err := cnfg.MarshalENV(payload, "UN")
	if err != nil {
		return nil, fmt.Errorf("creating environment: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hook.Timeout.Duration)
	defer cancel()

	var cmd *exec.Cmd

	args := strings.Fields(hook.Command)
	if len(args) == 0 {
		return nil, ErrCmdhookNoCmd
	}

	if args[0], err = filepath.Abs(args[0]); err != nil {
		return nil, fmt.Errorf("finding command hook command: %w", err)
	}

	if hook.Shell {
		if runtime.GOOS == "windows" {
			args = append([]string{"cmd", "/C"}, args...)
		} else {
			args = append([]string{"/bin/sh", "-c"}, args...)
		}
	}

	switch len(args) {
	case 0:
		return nil, ErrCmdhookNoCmd
	case 1:
		cmd = exec.CommandContext(ctx, args[0]) //nolint:gosec
	default:
		cmd = exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	}

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = env.Env()
	cmd.Env = append(cmd.Env, "PATH="+os.Getenv("PATH"))

	if err := cmd.Run(); err != nil {
		return &out, fmt.Errorf("running cmd %q: %w", strings.Join(cmd.Args, " "), err)
	}

	return &out, nil
}

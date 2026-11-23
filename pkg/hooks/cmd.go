package hooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golift.io/cnfg"
)

// gateNotReadyExit is the command exit code that means "not yet".
// Any other non-zero exit, or a timeout, is not an opinion.
const gateNotReadyExit = 75

// outputWait is how long a command's output is still read after the command
// has ended or was stopped at its timeout. A process the command started can
// keep that output open for as long as it lives, and nothing waits for it.
const outputWait = time.Second

func runCmdWithLog(log Logger, hook *Config, payload *Payload, qlen, qcap int) error {
	out, err := runCmd(context.Background(), hook, payload)

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

	return err
}

func runCmd(ctx context.Context, hook *Config, payload *Payload) (*bytes.Buffer, error) {
	if hook.Command == "" {
		return nil, ErrCmdhookNoCmd
	}

	payload.Config = hook

	env, err := cnfg.MarshalENV(payload, "UN")
	if err != nil {
		return nil, fmt.Errorf("creating environment: %w", err)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, hook.Timeout.Duration)
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
		cmd = exec.CommandContext(cmdCtx, args[0]) //nolint:gosec
	default:
		cmd = exec.CommandContext(cmdCtx, args[0], args[1:]...) //nolint:gosec
	}

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = env.Env()
	cmd.Env = append(cmd.Env, "PATH="+os.Getenv("PATH"))
	cmd.WaitDelay = outputWait

	// ErrWaitDelay is a command that ended well and left its output open.
	// The command's own result stands.
	if err := cmd.Run(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return &out, fmt.Errorf("running cmd %q: %w", strings.Join(cmd.Args, " "), err)
	}

	return &out, nil
}

// RunSync runs a command hook in the caller's goroutine. The command is
// stopped at the hook timeout, and its output is read for outputWait more at
// most. The first line the command printed is returned with every result.
// Exit 0 returns a nil error. Exit 75 returns ErrGateNotReady. Any other
// exit, a timeout, or a spawn error is returned as-is.
func RunSync(hook *Config, payload *Payload) (string, error) {
	if hook == nil {
		return "", ErrNilConfig
	}

	out, err := runCmd(context.Background(), hook, payload)
	line := firstLine(out)

	// Exit 75 ran and said "not yet". It counts as a run, and it is not a failure.
	var exitErr *exec.ExitError

	held := errors.As(err, &exitErr) && exitErr.ExitCode() == gateNotReadyExit
	if held {
		err = ErrGateNotReady
	}

	hook.Lock()
	hook.posts++

	if err != nil && !held {
		hook.fails++
	}

	hook.Unlock()

	return line, err
}

// firstLine is the first line of a command's output that is not blank.
func firstLine(out *bytes.Buffer) string {
	if out == nil {
		return ""
	}

	line, _, _ := strings.Cut(strings.TrimSpace(out.String()), "\n")

	return strings.TrimSpace(line)
}

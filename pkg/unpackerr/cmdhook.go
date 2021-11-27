package unpackerr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golift.io/cnfg"
)

// Errors produced by this file.
var (
	ErrCmdhookNoCmd = fmt.Errorf("cmdhook without a command configured; fix it")
)

func (u *Unpackerr) validateCmdhook() error {
	for i := range u.Cmdhook {
		u.Cmdhook[i].URL = ""

		if u.Cmdhook[i].Command == "" {
			return ErrCmdhookNoCmd
		}

		if u.Cmdhook[i].Name == "" {
			u.Cmdhook[i].Name = strings.Fields(u.Cmdhook[i].Command)[0]
		}

		if u.Cmdhook[i].Timeout.Duration == 0 {
			u.Cmdhook[i].Timeout.Duration = u.Timeout.Duration
		}

		if len(u.Cmdhook[i].Events) == 0 {
			u.Cmdhook[i].Events = []ExtractStatus{WAITING}
		}
	}

	return nil
}

func (u *Unpackerr) runCmdhookWithLog(hook *WebhookConfig, payload *WebhookPayload) {
	out, err := u.runCmdhook(hook, payload)

	hook.Lock() // we only lock for the integer increments.
	defer hook.Unlock()

	hook.posts++

	switch {
	case err != nil:
		u.Printf("[ERROR] Command Hook %s: %v", hook.Name, err)
		hook.fails++
	case hook.Silent || out == nil:
		u.Printf("[Cmdhook] Queue: %d/%d. Ran command %s", len(u.hookChan), cap(u.hookChan), hook.Name)
	default:
		u.Printf("[Cmdhook] Queue: %d/%d. Ran command %s: %s",
			len(u.hookChan), cap(u.hookChan), hook.Name, strings.TrimSpace(out.String()))
	}
}

func (u *Unpackerr) runCmdhook(hook *WebhookConfig, payload *WebhookPayload) (*bytes.Buffer, error) {
	payload.Config = hook

	env, err := cnfg.MarshalENV(payload, "UN")
	if err != nil {
		return nil, fmt.Errorf("creating environment: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hook.Timeout.Duration)
	defer cancel()

	var cmd *exec.Cmd

	if hook.Shell {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", hook.Command) //nolint:gosec
	} else {
		switch args := strings.Fields(hook.Command); len(args) {
		case 0:
			return nil, ErrCmdhookNoCmd
		case 1:
			cmd = exec.CommandContext(ctx, args[0]) //nolint:gosec
		default:
			cmd = exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
		}
	}

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = env.Env()
	cmd.Env = append(cmd.Env, os.Getenv("PATH"))

	if err := cmd.Run(); err != nil {
		return &out, fmt.Errorf("running cmd: %w", err)
	}

	return &out, nil
}

func (u *Unpackerr) logCmdhook() {
	var pfx string

	if len(u.Cmdhook) == 1 {
		pfx = " => Command Hook Config: 1 cmd"
	} else {
		u.Print(" => Command Hook Configs:", len(u.Cmdhook), "cmds")
		pfx = " =>    Command"
	}

	for _, f := range u.Cmdhook {
		u.Printf("%s: %s, timeout: %v, silent: %v, events: %v, shell: %v, cmd: %s",
			pfx, f.Name, f.Timeout, f.Silent, logEvents(f.Events), f.Shell, f.Command)
	}
}

// CmdhookCounts returns the total count of requests and errors for all webhooks.
func (u *Unpackerr) CmdhookCounts() (total uint, fails uint) {
	for _, hook := range u.Cmdhook {
		t, f := hook.Counts()
		total += t
		fails += f
	}

	return total, fails
}

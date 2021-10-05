package unpackerr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"golift.io/cnfg"
	"golift.io/version"
)

// CmdhookConfig is the configuration for a command hook.
type CmdhookConfig struct {
	Name       string          `json:"name" toml:"name" xml:"name" yaml:"name"`
	Command    string          `json:"command" toml:"command" xml:"command" yaml:"command"`
	Shell      bool            `json:"shell" toml:"shell" xml:"shell" yaml:"shell"`
	Silent     bool            `json:"silent" toml:"silent" xml:"silent" yaml:"silent"`
	Timeout    cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	Events     []ExtractStatus `json:"events" toml:"events" xml:"events" yaml:"events"`
	Exclude    []string        `json:"exclude" toml:"exclude" xml:"exclude" yaml:"exclude"`
	fails      uint
	execs      uint
	sync.Mutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// Errors produced by this file.
var (
	ErrCmdhookNoCmd = fmt.Errorf("cmdhook without a command configured; fix it")
)

func (u *Unpackerr) validateCmdhook() error {
	for i := range u.Cmdhook {
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

func (u *Unpackerr) runCmdHooks(i *Extract) {
	if i.Status == IMPORTED && i.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := &WebhookPayload{
		Path:  i.Path,
		App:   i.App,
		IDs:   i.IDs,
		Time:  i.Updated,
		Event: i.Status,
		// Application Metadata.
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
	}

	if i.Status <= EXTRACTED && i.Resp != nil {
		payload.Data = &XtractPayload{
			Archives: append(i.Resp.Extras, i.Resp.Archives...),
			Files:    i.Resp.NewFiles,
			Start:    i.Resp.Started,
			Output:   i.Resp.Output,
			Bytes:    i.Resp.Size,
			Queue:    i.Resp.Queued,
			Elapsed:  cnfg.Duration{Duration: i.Resp.Elapsed},
		}

		if i.Resp.Error != nil {
			payload.Data.Error = i.Resp.Error.Error()
		}
	}

	go u.runCmdhooksWithLog(i, payload)
}

func (u *Unpackerr) runCmdhooksWithLog(i *Extract, payload *WebhookPayload) {
	for _, hook := range u.Cmdhook {
		if !hook.HasEvent(i.Status) || hook.Excluded(i.App) {
			continue
		}

		switch out, err := u.runCmdhook(hook, payload); {
		case err != nil:
			u.Printf("[ERROR] Command Hook %s: %v", hook.Name, err)
		case hook.Silent || out == nil:
			u.Printf("[Cmdhook] Ran command %s", hook.Name)
		default:
			u.Printf("[Cmdhook] Ran command %s: %s", hook.Name, strings.TrimSpace(out.String()))
		}
	}
}

func (u *Unpackerr) runCmdhook(hook *CmdhookConfig, payload *WebhookPayload) (*bytes.Buffer, error) {
	hook.Lock()
	defer hook.Unlock()

	hook.execs++

	env, err := cnfg.MarshalENV(payload, "UN")
	if err != nil {
		hook.fails++
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
		hook.fails++
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

// Excluded returns true if an app is in the Exclude slice.
func (w *CmdhookConfig) Excluded(app string) bool {
	for _, a := range w.Exclude {
		if strings.EqualFold(a, app) {
			return true
		}
	}

	return false
}

// HasEvent returns true if a status event is in the Events slice.
// Also returns true if the Events slice has only one value of WAITING.
func (w *CmdhookConfig) HasEvent(e ExtractStatus) bool {
	for _, h := range w.Events {
		if (h == WAITING && len(w.Events) == 1) || h == e {
			return true
		}
	}

	return false
}

// Counts returns the total count of requests and failures for a webhook.
func (w *CmdhookConfig) Counts() (uint, uint) {
	w.Lock()
	defer w.Unlock()

	return w.execs, w.fails
}

package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
)

// A gate run is a command-hook run. Exit 75 counts as a run and not as a
// failure. Every other error counts as both.
func TestRunSyncCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		code  int
		posts uint
		fails uint
		held  bool
	}{
		{name: "ready", code: 0, posts: 1},
		{name: "hold", code: 75, posts: 2, held: true},
		{name: "fail", code: 2, posts: 1, fails: 1},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			hook := &Config{
				Command: exitCommand(t, one.code),
				Timeout: cnfg.Duration{Duration: 5 * time.Second},
			}

			runs := 1
			if one.held {
				runs = 2
			}

			for range runs {
				_, err := RunSync(hook, &Payload{})
				if one.held && !errors.Is(err, ErrGateNotReady) {
					t.Fatalf("error %v, want not ready", err)
				}

				if !one.held && one.fails > 0 && err == nil {
					t.Fatal("the failing command returned no error")
				}
			}

			if posts, fails := hook.Counts(); posts != one.posts || fails != one.fails {
				t.Fatalf("counts %d runs %d failures, want %d and %d", posts, fails, one.posts, one.fails)
			}
		})
	}

	t.Run("no command", func(t *testing.T) {
		t.Parallel()

		hook := &Config{Timeout: cnfg.Duration{Duration: time.Second}}
		if _, err := RunSync(hook, &Payload{}); err == nil {
			t.Fatal("a hook with no command returned no error")
		}

		if posts, fails := hook.Counts(); posts != 1 || fails != 1 {
			t.Fatalf("counts %d runs %d failures, want 1 and 1", posts, fails)
		}
	})
}

// exitCommand is a command that prints nothing and exits with code.
// runCmd splits the command on spaces, so a path with one is skipped.
func exitCommand(t *testing.T, code int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "exit.sh")
	body := fmt.Sprintf("#!/bin/sh\nexit %d\n", code)

	if runtime.GOOS == "windows" {
		path = filepath.Join(t.TempDir(), "exit.cmd")
		body = fmt.Sprintf("@exit /b %d\r\n", code)
	}

	if strings.ContainsAny(path, " \t") {
		t.Skip("command hook tests need a temp path without spaces")
	}

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		return path
	}

	return "/bin/sh " + path
}

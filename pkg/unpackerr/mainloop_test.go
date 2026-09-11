package unpackerr

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestOnMainLoopAlreadyCanceledDoesNotRun(t *testing.T) {
	t.Parallel()

	unpack := New()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var ran atomic.Bool

	err := unpack.onMainLoop(ctx, func() error {
		ran.Store(true)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}

	if ran.Load() {
		t.Fatal("canceled context must not queue work")
	}
}

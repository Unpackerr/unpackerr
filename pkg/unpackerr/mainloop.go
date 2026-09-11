package unpackerr

import (
	"context"
	"fmt"
)

// mainTask is work an HTTP handler hands to the main goroutine in Run().
// Live config, the queue map, and the folder tracker are owned by that
// goroutine; HTTP validates and then asks the loop to apply.
type mainTask struct {
	fn     func() error
	result chan error
}

// onMainLoop runs fn on the main goroutine and waits for it.
func (u *Unpackerr) onMainLoop(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("main loop: %w", err)
	}

	task := &mainTask{fn: fn, result: make(chan error, 1)}

	select {
	case u.taskChan <- task:
	case <-ctx.Done():
		return fmt.Errorf("main loop: %w", ctx.Err())
	}

	select {
	case err := <-task.result:
		return err
	case <-ctx.Done():
		return fmt.Errorf("main loop: %w", ctx.Err())
	}
}

// runMainTasks stands in for Run() in tests that exercise HTTP handlers.
func (u *Unpackerr) runMainTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-u.taskChan:
			task.result <- task.fn()
		}
	}
}

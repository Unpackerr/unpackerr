//go:build windows || darwin

package ui

import (
	"github.com/gen2brain/dlgs"
)

// Warning wraps dlgs.Warning.
func Warning(title, msg string) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dlgs.Warning(title, msg) //nolint:wrapcheck
}

// Error wraps dlgs.Error.
func Error(title, msg string) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dlgs.Error(title, msg) //nolint:wrapcheck
}

// Info wraps dlgs.Info.
func Info(title, msg string) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dlgs.Info(title, msg) //nolint:wrapcheck
}

// Entry wraps dlgs.Entry.
func Entry(title, msg, val string) (string, bool, error) {
	if !HasGUI() {
		return val, true, nil
	}

	return dlgs.Entry(title, msg, val) //nolint:wrapcheck
}

// Question wraps dlgs.Question.
func Question(title, text string, defaultCancel bool) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dlgs.Question(title, text, defaultCancel) //nolint:wrapcheck
}

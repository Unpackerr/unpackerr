//go:build windows || darwin

package ui

import (
	"errors"
	"fmt"

	"github.com/ncruces/zenity"
)

func dialogResult(err error) (bool, error) {
	if errors.Is(err, zenity.ErrCanceled) {
		return false, nil
	}

	return err == nil, err
}

// Warning wraps a warning dialog.
func Warning(title, msg string) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dialogResult(zenity.Warning(msg, zenity.Title(title)))
}

// Error wraps an error dialog.
func Error(title, msg string, v ...any) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dialogResult(zenity.Error(fmt.Sprintf(msg, v...), zenity.Title(title)))
}

// Info wraps an info dialog.
func Info(title, msg string, v ...any) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	return dialogResult(zenity.Info(fmt.Sprintf(msg, v...), zenity.Title(title)))
}

// Entry wraps a text-entry dialog.
func Entry(title, msg, val string) (string, bool, error) {
	if !HasGUI() {
		return val, true, nil
	}

	entry, err := zenity.Entry(msg, zenity.Title(title), zenity.EntryText(val))
	if errors.Is(err, zenity.ErrCanceled) {
		return val, false, nil
	}

	return entry, err == nil, err
}

// Question wraps a question dialog.
func Question(title string, defaultCancel bool, text string, v ...any) (bool, error) {
	if !HasGUI() {
		return true, nil
	}

	opts := []zenity.Option{zenity.Title(title)}
	if defaultCancel {
		opts = append(opts, zenity.DefaultCancel())
	}

	return dialogResult(zenity.Question(fmt.Sprintf(text, v...), opts...))
}

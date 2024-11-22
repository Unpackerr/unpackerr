//go:build !windows && !darwin

package ui

// Warning wraps dlgs.Warning.
func Warning(_, _ string) (bool, error) {
	return true, nil
}

// Error wraps dlgs.Error.
func Error(_, _ string, _ any) (bool, error) {
	return true, nil
}

// Info wraps dlgs.Info.
func Info(_, _ string, _ any) (bool, error) {
	return true, nil
}

// Entry wraps dlgs.Entry.
func Entry(_, _, val string) (string, bool, error) {
	return val, false, nil
}

// Question wraps dlgs.Question.
func Question(_ string, _ bool, _ string, _ any) (bool, error) {
	return true, nil
}

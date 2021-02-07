// +build !windows,!darwin

package ui

// Warning wraps dlgs.Warning.
func Warning(title, msg string) (bool, error) {
	return true, nil
}

// Error wraps dlgs.Error.
func Error(title, msg string) (bool, error) {
	return true, nil
}

// Info wraps dlgs.Info.
func Info(title, msg string) (bool, error) {
	return true, nil
}

// Entry wraps dlgs.Entry.
func Entry(title, msg, val string) (string, bool, error) {
	return val, false, nil
}

// Question wraps dlgs.Question.
func Question(title, text string, defaultCancel bool) (bool, error) {
	return true, nil
}

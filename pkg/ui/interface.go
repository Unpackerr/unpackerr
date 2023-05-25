package ui

// MenuItem is an interface to allow exposing menu items to operating systems
// that do not have a menu or a GUI.
type MenuItem interface { //nolint:interfacebloat
	Check()
	Checked() bool
	Disable()
	Disabled() bool
	Enable()
	Hide()
	SetIcon(iconBytes []byte)
	SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte)
	SetTitle(title string)
	SetTooltip(tooltip string)
	Show()
	String() string
	Uncheck()
	Clicked() chan struct{}
}

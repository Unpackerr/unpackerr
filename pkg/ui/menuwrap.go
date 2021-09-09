//go:build windows || darwin

package ui

import "github.com/getlantern/systray"

type menuItem struct {
	*systray.MenuItem
}

// WrapMenu is just gross.
func WrapMenu(m *systray.MenuItem) MenuItem {
	return MenuItem(&menuItem{MenuItem: m})
}

// Clicked returns the ClickedCh.
func (m *menuItem) Clicked() chan struct{} {
	return m.ClickedCh
}

var _ = MenuItem(&menuItem{MenuItem: nil})

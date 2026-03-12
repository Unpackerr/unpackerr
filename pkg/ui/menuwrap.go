//go:build windows || darwin

package ui

import "github.com/energye/systray"

type menuItem struct {
	*systray.MenuItem
	clickedCh chan struct{}
}

// WrapMenu is just gross.
func WrapMenu(m *systray.MenuItem) MenuItem { //nolint: ireturn
	item := &menuItem{MenuItem: m, clickedCh: make(chan struct{}, 1)}
	m.Click(func() {
		select {
		case item.clickedCh <- struct{}{}:
		default:
		}
	})

	return MenuItem(item)
}

// Clicked returns the click notification channel.
func (m *menuItem) Clicked() chan struct{} {
	return m.clickedCh
}

var _ = MenuItem(&menuItem{MenuItem: nil})

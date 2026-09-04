package unpackerr

import (
	"strconv"
	"sync"

	"github.com/Unpackerr/unpackerr/pkg/ui"
)

// Safety constants.
const (
	hist     = "hist_"
	histNone = "hist_none"
)

// History holds the history of extracted items.
// mu guards Map, Finished, Retries, forgotten, and per-item Status/Updated so
// HTTP stats and Prometheus Collect can snapshot without racing the main loop.
// It is not reentrant; do not lock inside a caller that already holds it.
type History struct {
	mu        sync.RWMutex
	Items     []string
	Finished  uint
	Retries   uint
	Map       map[string]*Extract
	forgotten map[string]struct{}
}

func (h *History) lockHistory() {
	h.mu.Lock()
}

func (h *History) unlockHistory() {
	h.mu.Unlock()
}

func (h *History) rLockHistory() {
	h.mu.RLock()
}

func (h *History) rUnlockHistory() {
	h.mu.RUnlock()
}

// This is called every time an item is queued.
func (u *Unpackerr) updateHistory(item string) {
	if u.KeepHistory == 0 {
		return
	}

	if ui.HasGUI() && item != "" {
		u.menu[histNone].Hide()
	}

	u.Items[0] = item

	// Do not process 0; this isn't an `intrange`.
	for idx := len(u.Items) - 1; idx > 0; idx-- {
		// u.History.Items is a slice with a set (identical) length and capacity.
		switch u.Items[idx] = u.Items[idx-1]; {
		case !ui.HasGUI():
			continue
		case u.Items[idx] != "":
			u.menu[hist+strconv.Itoa(idx)].SetTitle(u.Items[idx])
			u.menu[hist+strconv.Itoa(idx)].Show()
		default:
			u.menu[hist+strconv.Itoa(idx)].Hide()
		}
	}
}

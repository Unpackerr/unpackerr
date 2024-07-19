package unpackerr

import (
	"strconv"

	"github.com/Unpackerr/unpackerr/pkg/ui"
)

// Safety constants.
const (
	hist     = "hist_"
	histNone = "hist_none"
)

// History holds the history of extracted items.
type History struct {
	Items    []string
	Finished uint
	Retries  uint
	Map      map[string]*Extract
}

// This is called every time an item is queued.
func (u *Unpackerr) updateHistory(item string) {
	if u.KeepHistory == 0 {
		return
	}

	if ui.HasGUI() && item != "" {
		u.menu[histNone].Hide()
	}

	u.History.Items[0] = item

	// Do not process 0; this isn't an `intrange`.
	for idx := len(u.History.Items) - 1; idx > 0; idx-- {
		// u.History.Items is a slice with a set (identical) length and capacity.
		switch u.History.Items[idx] = u.History.Items[idx-1]; {
		case !ui.HasGUI():
			continue
		case u.History.Items[idx] != "":
			u.menu[hist+strconv.Itoa(idx)].SetTitle(u.History.Items[idx])
			u.menu[hist+strconv.Itoa(idx)].Show()
		default:
			u.menu[hist+strconv.Itoa(idx)].Hide()
		}
	}
}

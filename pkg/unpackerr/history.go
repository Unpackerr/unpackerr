package unpackerr

import (
	"sync"
)

// History holds the history of extracted items.
// mu guards Map, Finished, Retries, forgotten, per-item Status/Updated, Note, XProg
// progress, and the Starr poll snapshot (Queue, lastQueued, lastRetrieved,
// lastPollErr) so HTTP stats, queue snapshots, and Prometheus Collect cannot race
// poll workers. It is not reentrant; do not lock inside a caller that already holds it.
type History struct {
	mu        sync.RWMutex
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

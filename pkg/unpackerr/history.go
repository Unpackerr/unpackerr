package unpackerr

import (
	"sync"
)

// History holds the history of extracted items.
// mu guards Map, Finished, Retries, forgotten, per-item Status/Updated, Note,
// HookFail, HookMessages, XProg progress, and the Starr poll snapshot (Queue,
// lastQueued, lastRetrieved, lastPolled, lastPollErr) so HTTP stats, queue
// snapshots, and Prometheus Collect cannot race poll workers. It is not
// reentrant; do not lock inside a caller that already holds it.
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

// unlockHistory only drops the mutex. Unpackerr.unlockHistory shadows this
// and flushes pendingHooks after the unlock.
func (h *History) unlockHistory() {
	h.mu.Unlock()
}

type pendingHook struct {
	itemID string
	item   Extract  // status snapshot; Map can move on after History.mu drops.
	live   *Extract // Map pointer at queue time; SaveID must not attach to a reused entry.
}

// queuePendingHook records a hook to fire after History.mu is dropped.
// Caller must hold History.mu.
func (u *Unpackerr) queuePendingHook(itemID string, item *Extract) {
	if item == nil {
		return
	}

	u.pendingHooks = append(u.pendingHooks, pendingHook{itemID: itemID, item: *item, live: item})
}

// unlockHistory drops History.mu then delivers any hooks queued while it was
// held. Enqueue can block on a full worker, so this cannot run under the lock.
func (u *Unpackerr) unlockHistory() {
	pending := u.pendingHooks
	u.pendingHooks = nil
	u.History.unlockHistory()

	for i := range pending {
		u.runAllHooks(pending[i].itemID, &pending[i].item, pending[i].live)
	}
}

func (h *History) rLockHistory() {
	h.mu.RLock()
}

func (h *History) rUnlockHistory() {
	h.mu.RUnlock()
}

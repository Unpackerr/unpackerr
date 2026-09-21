package unpackerr

import (
	"fmt"
	"maps"
	"runtime"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/version"
)

func (u *Unpackerr) runAllHooks(itemID string, item, live *Extract) {
	if item == nil || (item.Status == IMPORTED && item.App == FolderString) {
		return // This is an internal state change we don't need to fire on.
	}

	u.seedHookMessages(itemID, item.HookMessages, live)

	payload := u.hookPayload(item)

	for _, entry := range u.hookEntries() {
		hook := entry.Val
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(itemID, entry.Key, live, &hooks.Item{Config: hook, Payload: payload})
		}
	}

	for _, entry := range u.cmdhookEntries() {
		hook := entry.Val
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(itemID, entry.Key, live, &hooks.Item{Config: hook, Payload: payload})
		}
	}
}

func (u *Unpackerr) hookExtras() (map[string]string, HookTitles) {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return maps.Clone(u.Hooks.CustomIDs), u.Hooks.Titles
}

func (u *Unpackerr) hookPayload(item *Extract) *hooks.Payload {
	global, titles := u.hookExtras()

	payload := &hooks.Payload{
		Path:       item.Path,
		App:        starr.App(item.Label()),
		IDs:        cloneIDs(item.IDs),
		CustomIDs:  payloadCustomIDs(global),
		Time:       item.Updated,
		Data:       nil,
		Event:      item.Status,
		Retries:    item.Retries,
		EventTitle: titles.forStatus(item.Status),
		// Application Metadata.
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
	}

	if item.Resp != nil {
		payload.Data = &hooks.XtractPayload{
			Files:   hooks.StringSlice(item.Resp.NewFiles),
			File:    item.Resp.NewFiles,
			Start:   item.Resp.Started,
			Output:  item.Resp.Output,
			Bytes:   item.Resp.Size,
			Queue:   item.Resp.Queued,
			Elapsed: cnfg.Duration{Duration: item.Resp.Elapsed},
		}

		for _, v := range item.Resp.Archives {
			payload.Data.Archives = append(payload.Data.Archives, v...)
			payload.Data.Archive = append(payload.Data.Archive, v...)
		}

		for _, v := range item.Resp.Extras {
			payload.Data.Archives = append(payload.Data.Archives, v...)
			payload.Data.Archive = append(payload.Data.Archive, v...)
		}

		if item.Resp.Error != nil {
			payload.Data.Error = item.Resp.Error.Error()
		}
	}

	return payload
}

func (u *Unpackerr) decorateSamplePayload(payload *hooks.Payload, event extract.Status) {
	if payload == nil {
		return
	}

	u.configMu.RLock()
	global := maps.Clone(u.Hooks.CustomIDs)
	titles := u.Hooks.Titles
	u.configMu.RUnlock()

	payload.EventTitle = titles.forStatus(event)
	payload.CustomIDs = payloadCustomIDs(global)
}

func (u *Unpackerr) hookList() []*hooks.Config {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return instanceValues(u.Webhook)
}

func (u *Unpackerr) hookEntries() []instanceEntry[WebhookConfig] {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return instanceEntries(u.Webhook)
}

func (u *Unpackerr) cmdhookList() []*hooks.Config {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return instanceValues(u.Cmdhook)
}

func (u *Unpackerr) cmdhookEntries() []instanceEntry[WebhookConfig] {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return instanceEntries(u.Cmdhook)
}

func (u *Unpackerr) validateWebhook() error {
	return u.validateWebhookList(u.Webhook)
}

func (u *Unpackerr) validateWebhookList(list InstanceMap[WebhookConfig]) error {
	for key := range list {
		if err := validateInstanceSlug(key); err != nil {
			return err
		}
	}

	if err := hooks.ValidateWebhooks(instanceValues(list), u.Timeout.Duration); err != nil {
		return fmt.Errorf("validating webhooks: %w", err)
	}

	return nil
}

func (u *Unpackerr) validateCmdhook() error {
	return u.validateCmdhookList(u.Cmdhook)
}

func (u *Unpackerr) validateCmdhookList(list InstanceMap[WebhookConfig]) error {
	for key := range list {
		if err := validateInstanceSlug(key); err != nil {
			return err
		}
	}

	if err := hooks.ValidateCmdhooks(instanceValues(list), u.Timeout.Duration, expandHomedir); err != nil {
		return fmt.Errorf("validating cmdhooks: %w", err)
	}

	return nil
}

// WebhookCounts returns the total count of requests and errors for all webhooks.
func (u *Unpackerr) WebhookCounts() (uint, uint) {
	return hooks.CountAll(u.hookList())
}

// CmdhookCounts returns the total count of requests and errors for all command hooks.
func (u *Unpackerr) CmdhookCounts() (uint, uint) {
	return hooks.CountAll(u.cmdhookList())
}

// reportHookFail queues a failure for the main loop. Done runs on the hook
// worker and must not read live Config or mutate Map.
func (u *Unpackerr) reportHookFail(itemID string) {
	if itemID == "" {
		return
	}

	u.hookFailMu.Lock()
	u.hookFails = append(u.hookFails, itemID)
	u.hookFailMu.Unlock()

	select {
	case u.hookFailWake <- struct{}{}:
	default:
	}
}

func (u *Unpackerr) hasPendingHookFails() bool {
	u.hookFailMu.Lock()
	defer u.hookFailMu.Unlock()

	return len(u.hookFails) > 0
}

func (u *Unpackerr) drainHookFails() {
	u.hookFailMu.Lock()
	ids := u.hookFails
	u.hookFails = nil
	u.hookFailMu.Unlock()

	for _, itemID := range ids {
		u.recordHookFail(itemID)
	}
}

// recordHookFail increments the per-extract hook-failure counter.
// itemID is the Map key and history record ID (Starr title or folder path).
// Call from the main loop only.
func (u *Unpackerr) recordHookFail(itemID string) {
	if itemID == "" {
		return
	}

	u.lockHistory()

	item := u.Map[itemID]
	if item != nil {
		item.HookFail++
		u.maybeRecordHistory(itemID, item)

		if u.hub != nil {
			u.hub.notifyProgress(u.queueFromExtract(itemID, item))
		}

		persistable := isPersistedHistory(item)

		u.History.unlockHistory()

		if !persistable {
			u.bumpHistoryHookFail(itemID)
		}

		return
	}

	u.History.unlockHistory()
	u.bumpHistoryHookFail(itemID)
}

func (u *Unpackerr) bumpHistoryHookFail(itemID string) {
	if u.KeepHistory == 0 {
		return
	}

	u.histMu.Lock()
	defer u.histMu.Unlock()

	for _, row := range u.records {
		if row.ID != itemID {
			continue
		}

		rec := row
		rec.HookFail++
		u.upsertHistoryLocked(rec)

		return
	}
}

type hookMsgSave struct {
	itemID, key, msgID string
	live               *Extract
}

type hookMsgCache struct {
	live *Extract
	msgs map[string]string
}

func hookMsgsMap(msgs map[string]string) map[string]string {
	cloned := maps.Clone(msgs)
	if cloned == nil {
		return map[string]string{}
	}

	return cloned
}

func (c *hookMsgCache) ownedBy(live *Extract) bool {
	return c != nil && (live == nil || c.live == nil || c.live == live)
}

// seedHookMessages copies restored ids into the worker-visible cache for live.
// A different live extract replaces the cache so a reused title/path cannot
// edit the previous extract's Discord/Telegram message. Existing keys win so a
// FIFO SaveID is not overwritten by a stale snapshot of the same extract.
func (u *Unpackerr) seedHookMessages(itemID string, msgs map[string]string, live *Extract) {
	if itemID == "" {
		return
	}

	u.hookMsgMu.Lock()
	defer u.hookMsgMu.Unlock()

	if u.hookMsgs == nil {
		u.hookMsgs = map[string]*hookMsgCache{}
	}

	cache := u.hookMsgs[itemID]
	if !cache.ownedBy(live) {
		u.hookMsgs[itemID] = &hookMsgCache{live: live, msgs: hookMsgsMap(msgs)}

		return
	}

	if cache.live == nil {
		cache.live = live
	}

	if cache.msgs == nil {
		cache.msgs = map[string]string{}
	}

	for key, msgID := range msgs {
		if msgID == "" {
			continue
		}

		if _, ok := cache.msgs[key]; !ok {
			cache.msgs[key] = msgID
		}
	}
}

func (u *Unpackerr) lookupHookMessage(itemID, key string, live *Extract) string {
	if itemID == "" || key == "" {
		return ""
	}

	u.hookMsgMu.Lock()
	defer u.hookMsgMu.Unlock()

	cache := u.hookMsgs[itemID]
	if !cache.ownedBy(live) || cache.msgs == nil {
		return ""
	}

	return cache.msgs[key]
}

// storeHookMessage records an id for the next FIFO LookupID, then wakes the
// main loop to persist Map/history. SaveID runs on the hook worker.
func (u *Unpackerr) storeHookMessage(itemID, key, msgID string, live *Extract) {
	if itemID == "" || key == "" || msgID == "" {
		return
	}

	u.hookMsgMu.Lock()

	if u.hookMsgs == nil {
		u.hookMsgs = map[string]*hookMsgCache{}
	}

	cache := u.hookMsgs[itemID]
	if cache != nil && !cache.ownedBy(live) {
		u.hookMsgMu.Unlock()

		return
	}

	if cache == nil {
		cache = &hookMsgCache{live: live, msgs: map[string]string{}}
		u.hookMsgs[itemID] = cache
	} else if cache.live == nil {
		cache.live = live
	}

	if cache.msgs == nil {
		cache.msgs = map[string]string{}
	}

	cache.msgs[key] = msgID
	u.hookMsgSaves = append(u.hookMsgSaves, hookMsgSave{
		itemID: itemID, key: key, msgID: msgID, live: live,
	})
	u.hookMsgMu.Unlock()

	select {
	case u.hookMsgWake <- struct{}{}:
	default:
	}
}

func (u *Unpackerr) dropHookMessages(itemID string, live *Extract) {
	if itemID == "" {
		return
	}

	u.hookMsgMu.Lock()
	defer u.hookMsgMu.Unlock()

	if cache := u.hookMsgs[itemID]; cache.ownedBy(live) {
		delete(u.hookMsgs, itemID)
	}
}

func (u *Unpackerr) hasPendingHookMessages() bool {
	u.hookMsgMu.Lock()
	defer u.hookMsgMu.Unlock()

	return len(u.hookMsgSaves) > 0
}

func (u *Unpackerr) drainHookMessages() {
	u.hookMsgMu.Lock()
	saves := u.hookMsgSaves
	u.hookMsgSaves = nil
	u.hookMsgMu.Unlock()

	for _, save := range saves {
		u.saveHookMessage(save.itemID, save.key, save.msgID, save.live)
	}
}

func (u *Unpackerr) hookMessage(itemID, key string) string {
	if itemID == "" || key == "" {
		return ""
	}

	u.lockHistory()

	item := u.Map[itemID]
	if item != nil && item.HookMessages != nil {
		id := item.HookMessages[key]

		u.History.unlockHistory()

		return id
	}

	u.History.unlockHistory()

	return u.historyHookMessage(itemID, key)
}

func (u *Unpackerr) historyHookMessage(itemID, key string) string {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	for _, row := range u.records {
		if row.ID != itemID || row.HookMessages == nil {
			continue
		}

		return row.HookMessages[key]
	}

	return ""
}

func (u *Unpackerr) saveHookMessage(itemID, key, msgID string, live *Extract) {
	if itemID == "" || key == "" || msgID == "" {
		return
	}

	u.lockHistory()

	item := u.Map[itemID]
	if item != nil && (live == nil || item == live) {
		if item.HookMessages == nil {
			item.HookMessages = map[string]string{}
		}

		item.HookMessages[key] = msgID
		u.maybeRecordHistory(itemID, item)

		u.History.unlockHistory()

		return
	}

	u.History.unlockHistory()

	if live != nil && item != nil {
		return // itemID was reused by a newer extract.
	}

	u.bumpHistoryHookMessage(itemID, key, msgID)
}

func (u *Unpackerr) bumpHistoryHookMessage(itemID, key, msgID string) {
	if u.KeepHistory == 0 {
		return
	}

	u.histMu.Lock()
	defer u.histMu.Unlock()

	for _, row := range u.records {
		if row.ID != itemID {
			continue
		}

		rec := row
		if rec.HookMessages == nil {
			rec.HookMessages = map[string]string{}
		} else {
			rec.HookMessages = maps.Clone(rec.HookMessages)
		}

		rec.HookMessages[key] = msgID
		u.upsertHistoryLocked(rec)

		return
	}
}

func (u *Unpackerr) sampleWebhook(event extract.Status) error {
	u.Printf("Sending sample webhooks and exiting! (-w %d passed)", event)

	payload := hooks.SamplePayload()
	if err := hooks.PrepareSample(payload, event); err != nil {
		return fmt.Errorf("preparing sample webhook: %w", err)
	}

	u.decorateSamplePayload(payload, event)

	for _, hook := range instanceValues(u.Webhook) {
		_ = hooks.SendWithLog(u.Logger, hook, payload)
	}

	return nil
}

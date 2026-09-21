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

func (u *Unpackerr) runAllHooks(itemID string, item *Extract) {
	if item == nil || (item.Status == IMPORTED && item.App == FolderString) {
		return // This is an internal state change we don't need to fire on.
	}

	payload := u.hookPayload(item)

	for _, hook := range u.hookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(itemID, &hooks.Item{Config: hook, Payload: payload})
		}
	}

	for _, hook := range u.cmdhookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(itemID, &hooks.Item{Config: hook, Payload: payload})
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

func (u *Unpackerr) cmdhookList() []*hooks.Config {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return instanceValues(u.Cmdhook)
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

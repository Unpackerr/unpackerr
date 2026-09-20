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

func (u *Unpackerr) runAllHooks(item *Extract) {
	if item.Status == IMPORTED && item.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := u.hookPayload(item)

	for _, hook := range u.hookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(&hooks.Item{Config: hook, Payload: payload})
		}
	}

	for _, hook := range u.cmdhookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App, item.Name) {
			u.queueHook(&hooks.Item{Config: hook, Payload: payload})
		}
	}
}

func (u *Unpackerr) hookExtras() (map[string]string, map[string]string) {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return maps.Clone(u.Hooks.CustomIDs), maps.Clone(u.Hooks.Titles)
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
		EventTitle: eventTitle(item.Status, titles),
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
	titles := maps.Clone(u.Hooks.Titles)
	u.configMu.RUnlock()

	payload.EventTitle = eventTitle(event, titles)
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

func (u *Unpackerr) sampleWebhook(event extract.Status) error {
	u.Printf("Sending sample webhooks and exiting! (-w %d passed)", event)

	payload := hooks.SamplePayload()
	if err := hooks.PrepareSample(payload, event); err != nil {
		return fmt.Errorf("preparing sample webhook: %w", err)
	}

	u.decorateSamplePayload(payload, event)

	for _, hook := range instanceValues(u.Webhook) {
		hooks.SendWithLog(u.Logger, hook, payload)
	}

	return nil
}

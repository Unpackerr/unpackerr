package unpackerr

import (
	"fmt"
	"runtime"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/cnfg"
	"golift.io/version"
)

func (u *Unpackerr) runAllHooks(item *Extract) {
	if item.Status == IMPORTED && item.App == FolderString {
		return // This is an internal state change we don't need to fire on.
	}

	payload := hookPayload(item)

	for _, hook := range u.hookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App) {
			u.queueHook(&hooks.Item{Config: hook, Payload: payload})
		}
	}

	for _, hook := range u.cmdhookList() {
		if hook.HasEvent(item.Status) && !hook.Excluded(item.App) {
			u.queueHook(&hooks.Item{Config: hook, Payload: payload})
		}
	}
}

func hookPayload(item *Extract) *hooks.Payload {
	payload := &hooks.Payload{
		Path:  item.Path,
		App:   item.App,
		IDs:   item.IDs,
		Time:  item.Updated,
		Data:  nil,
		Event: item.Status,
		// Application Metadata.
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
	}

	if item.Status <= EXTRACTED && item.Resp != nil {
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

func (u *Unpackerr) hookList() []*hooks.Config {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return u.Webhook
}

func (u *Unpackerr) cmdhookList() []*hooks.Config {
	u.configMu.RLock()
	defer u.configMu.RUnlock()

	return u.Cmdhook
}

func (u *Unpackerr) validateWebhook() error {
	return u.validateWebhookList(u.Webhook)
}

func (u *Unpackerr) validateWebhookList(list []*WebhookConfig) error {
	if err := hooks.ValidateWebhooks(list, u.Timeout.Duration); err != nil {
		return fmt.Errorf("validating webhooks: %w", err)
	}

	return nil
}

func (u *Unpackerr) validateCmdhook() error {
	return u.validateCmdhookList(u.Cmdhook)
}

func (u *Unpackerr) validateCmdhookList(list []*WebhookConfig) error {
	if err := hooks.ValidateCmdhooks(list, u.Timeout.Duration, expandHomedir); err != nil {
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

func (u *Unpackerr) sampleWebhook(e extract.Status) error {
	u.Printf("Sending sample webhooks and exiting! (-w %d passed)", e)

	payload := hooks.SamplePayload()
	if err := hooks.PrepareSample(payload, e); err != nil {
		return fmt.Errorf("preparing sample webhook: %w", err)
	}

	for _, hook := range u.Webhook {
		hooks.SendWithLog(u.Logger, hook, payload)
	}

	return nil
}

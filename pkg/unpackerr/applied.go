package unpackerr

import "time"

// appliedGeneral is an immutable copy of live-applied general settings.
// PUT publishes a new pointer so extract/history/folder readers never take
// configMu (that lock already nests with histMu the other way).
type appliedGeneral struct {
	Activity      bool
	MaxRetries    uint
	KeepHistory   uint
	RemnantAction string
	Timeout       time.Duration
	DeleteDelay   time.Duration
	StartDelay    time.Duration
	RetryDelay    time.Duration
	Passwords     StringSlice
}

func (u *Unpackerr) publishAppliedGeneral() {
	if u == nil || u.Config == nil {
		return
	}

	u.rtGeneral.Store(&appliedGeneral{
		Activity:      u.Activity,
		MaxRetries:    u.MaxRetries,
		KeepHistory:   u.KeepHistory,
		RemnantAction: remnantAction(u.RemnantAction),
		Timeout:       u.Timeout.Duration,
		DeleteDelay:   u.DeleteDelay.Duration,
		StartDelay:    u.StartDelay.Duration,
		RetryDelay:    u.RetryDelay.Duration,
		Passwords:     append(StringSlice(nil), u.Passwords...),
	})
}

func (u *Unpackerr) applied() *appliedGeneral {
	if u == nil {
		return &appliedGeneral{}
	}

	if snap := u.rtGeneral.Load(); snap != nil {
		return snap
	}

	// Tests and startup before unmarshalConfig. Do not take configMu here:
	// extract/history already nest histMu the other way.

	if u.Config == nil {
		return &appliedGeneral{}
	}

	return &appliedGeneral{
		Activity:      u.Activity,
		MaxRetries:    u.MaxRetries,
		KeepHistory:   u.KeepHistory,
		RemnantAction: remnantAction(u.RemnantAction),
		Timeout:       u.Timeout.Duration,
		DeleteDelay:   u.DeleteDelay.Duration,
		StartDelay:    u.StartDelay.Duration,
		RetryDelay:    u.RetryDelay.Duration,
		Passwords:     u.Passwords,
	}
}

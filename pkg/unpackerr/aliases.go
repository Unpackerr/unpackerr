package unpackerr

import (
	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
)

// Type aliases keep the daemon and tests on the historical names while the
// types themselves live in pkg/extract and pkg/hooks.
type (
	ExtractStatus   = extract.Status
	Extract         = extract.Extract
	ExtractProgress = extract.Progress
	WebhookConfig   = hooks.Config
	WebhookPayload  = hooks.Payload
	ExtractStatuses = hooks.Statuses
	XtractPayload   = hooks.XtractPayload
)

// Extract Statuses.
const (
	WAITING          = extract.WAITING
	QUEUED           = extract.QUEUED
	EXTRACTING       = extract.EXTRACTING
	EXTRACTFAILED    = extract.EXTRACTFAILED
	EXTRACTED        = extract.EXTRACTED
	IMPORTED         = extract.IMPORTED
	DELETING         = extract.DELETING
	DELETEFAILED     = extract.DELETEFAILED
	DELETED          = extract.DELETED
	EXTRACTEDNOTHING = extract.EXTRACTEDNOTHING
)

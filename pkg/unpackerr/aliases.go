package unpackerr

import "github.com/Unpackerr/unpackerr/pkg/extract"

// Type aliases keep the daemon and tests on the historical names while the
// types themselves live in pkg/extract.
type (
	ExtractStatus   = extract.Status
	Extract         = extract.Extract
	ExtractProgress = extract.Progress
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

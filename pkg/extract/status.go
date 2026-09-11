// Package extract holds extract status and item types shared by the daemon, hooks, and folders.
package extract

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Status is our enum for an extract's status.
type Status uint8

// Extract Statuses.
const (
	WAITING = Status(iota)
	QUEUED
	EXTRACTING
	EXTRACTFAILED
	EXTRACTED
	IMPORTED
	DELETING
	DELETEFAILED // unused
	DELETED
	EXTRACTEDNOTHING
)

var errUnknownExtractStatus = errors.New("unknown extract status")

// Desc makes Status human readable.
func (status Status) Desc() string {
	if status > EXTRACTEDNOTHING {
		return "Unknown"
	}

	return []string{
		// The order must not be faulty.
		"Waiting, pre-Queue",
		"Queued",
		"Extracting",
		"Extraction Failed",
		"Extracted, Awaiting Import",
		"Imported",
		"Deleting",
		"Delete Failed",
		"Deleted",
		"Nothing Extracted",
	}[status]
}

// MarshalText turns a status into a word, for a json identifier.
func (status Status) MarshalText() ([]byte, error) {
	return []byte(status.String()), nil
}

// UnmarshalText turns a json identifier or TOML event ID back into a status.
func (status *Status) UnmarshalText(text []byte) error {
	name := strings.TrimSpace(string(text))
	if parsed, err := strconv.ParseUint(name, 10, 8); err == nil {
		got := Status(parsed)
		if got <= EXTRACTEDNOTHING {
			*status = got
			return nil
		}
	}

	for candidate := WAITING; candidate <= EXTRACTEDNOTHING; candidate++ {
		if candidate.String() == name {
			*status = candidate
			return nil
		}
	}

	return fmt.Errorf("%w: %s", errUnknownExtractStatus, name)
}

// String turns a status into a short string.
func (status Status) String() string {
	if status > EXTRACTEDNOTHING {
		return "unknown"
	}

	return []string{
		// The order must not be faulty.
		"waiting",
		"queued",
		"extracting",
		"extractfailed",
		"extracted",
		"imported",
		"deleting",
		"deletefailed",
		"deleted",
		"extractednothing",
	}[status]
}

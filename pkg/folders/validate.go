package folders

import (
	"errors"
	"fmt"

	"golift.io/cnfg"
)

// ErrNilConfig is returned when a folder list contains a nil entry.
var ErrNilConfig = errors.New("nil config entry")

// ValidateList applies folder defaults and parses max_bytes.
func ValidateList(list []*FolderConfig, parseMax func(string) (uint64, bool, error)) error {
	for idx := range list {
		if list[idx] == nil {
			return ErrNilConfig
		}

		if list[idx].DeleteAfter == nil {
			list[idx].DeleteAfter = &cnfg.Duration{Duration: DefaultDeleteAfter}
		}

		n, _, err := parseMax(list[idx].MaxBytes)
		if err != nil {
			return fmt.Errorf("folder %s: %w", list[idx].Path, err)
		}

		list[idx].ResolvedMaxBytes = n
	}

	return nil
}

// CloneList copies folder configs without sharing DeleteAfter or ExcludePaths.
func CloneList(src []*FolderConfig) []*FolderConfig {
	if src == nil {
		return nil
	}

	out := make([]*FolderConfig, len(src))
	for idx, folder := range src {
		cloned := *folder
		if folder.DeleteAfter != nil {
			dur := *folder.DeleteAfter
			cloned.DeleteAfter = &dur
		}

		cloned.ExcludePaths = append([]string(nil), folder.ExcludePaths...)
		out[idx] = &cloned
	}

	return out
}

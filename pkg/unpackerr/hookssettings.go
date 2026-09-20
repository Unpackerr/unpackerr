package unpackerr

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

var (
	errInvalidHookIDKey   = errors.New("hook id key must be letters, digits, underscore, or hyphen")
	errDuplicateHookIDKey = errors.New("duplicate hook id key")
)

func (u *Unpackerr) validateHooks() error {
	return validateHooksConfig(&u.Hooks)
}

func validateHooksConfig(cfg *HooksConfig) error {
	if cfg == nil {
		return nil
	}

	return validateHookIDs(cfg.CustomIDs)
}

func validateHookIDs(ids map[string]string) error {
	seen := make(map[string]struct{}, len(ids))

	for key := range ids {
		if !validRoleName(key) {
			return fmt.Errorf("%w: %q", errInvalidHookIDKey, key)
		}

		lower := strings.ToLower(key)
		if _, dup := seen[lower]; dup {
			return fmt.Errorf("%w: %q", errDuplicateHookIDKey, key)
		}

		seen[lower] = struct{}{}
	}

	return nil
}

func cloneHooks(src HooksConfig) HooksConfig {
	return HooksConfig{
		CustomIDs: maps.Clone(src.CustomIDs),
		Titles:    src.Titles,
	}
}

func (t HookTitles) forStatus(status ExtractStatus) string {
	var title string

	switch status {
	case WAITING:
		title = t.Waiting
	case QUEUED:
		title = t.Queued
	case EXTRACTING:
		title = t.Extracting
	case EXTRACTFAILED:
		title = t.ExtractFailed
	case EXTRACTED:
		title = t.Extracted
	case IMPORTED:
		title = t.Imported
	case DELETING:
		title = t.Deleting
	case DELETEFAILED:
		title = t.DeleteFailed
	case DELETED:
		title = t.Deleted
	case EXTRACTEDNOTHING:
		title = t.ExtractedNothing
	}

	if title = strings.TrimSpace(title); title != "" {
		return title
	}

	return status.Desc()
}

func (t HookTitles) nonEmpty() int {
	count := 0

	for _, title := range []string{
		t.Waiting, t.Queued, t.Extracting, t.ExtractFailed, t.Extracted,
		t.Imported, t.Deleting, t.DeleteFailed, t.Deleted, t.ExtractedNothing,
	} {
		if strings.TrimSpace(title) != "" {
			count++
		}
	}

	return count
}

func payloadCustomIDs(ids map[string]string) map[string]string {
	if len(ids) == 0 {
		return nil
	}

	out := make(map[string]string, len(ids))

	for key, val := range ids {
		if key == "" || val == "" {
			continue
		}

		out[key] = val
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

package unpackerr

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/extract"
)

var (
	errInvalidHookIDKey    = errors.New("hook id key must be letters, digits, underscore, or hyphen")
	errUnknownHookTitleKey = errors.New("unknown hook title event")
	errDuplicateHookIDKey  = errors.New("duplicate hook id key")
)

func (u *Unpackerr) validateHooks() error {
	return validateHooksConfig(&u.Hooks)
}

func validateHooksConfig(cfg *HooksConfig) error {
	if cfg == nil {
		return nil
	}

	if err := validateHookIDs(cfg.CustomIDs); err != nil {
		return fmt.Errorf("hooks custom ids: %w", err)
	}

	return validateHookTitles(cfg.Titles)
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

func validateHookTitles(titles map[string]string) error {
	for key := range titles {
		var status extract.Status
		if err := status.UnmarshalText([]byte(key)); err != nil || key != status.String() {
			return fmt.Errorf("%w: %q", errUnknownHookTitleKey, key)
		}
	}

	return nil
}

func cloneHooks(src HooksConfig) HooksConfig {
	return HooksConfig{
		CustomIDs: maps.Clone(src.CustomIDs),
		Titles:    maps.Clone(src.Titles),
	}
}

func eventTitle(status extract.Status, titles map[string]string) string {
	if title := strings.TrimSpace(titles[status.String()]); title != "" {
		return title
	}

	return status.Desc()
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

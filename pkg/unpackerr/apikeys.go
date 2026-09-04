package unpackerr

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	apiKeyMinLen = 60
	apiKeyMaxLen = 150
)

var (
	errAPIKeyLength    = errors.New("api key must be 60–150 ASCII characters")
	errAPIKeyASCII     = errors.New("api key must be ASCII")
	errAPIKeyName      = errors.New("api key name is required")
	errAPIKeyDup       = errors.New("duplicate api key")
	errAPIKeyNameDup   = errors.New("duplicate api key name")
	errUnknownRole     = errors.New("unknown role")
	errUnknownPerm     = errors.New("unknown permission")
	errEmptyRole       = errors.New("role has no permissions")
	errEmptyKeyRoles   = errors.New("api key needs at least one role")
	errInvalidRoleName = errors.New("role name must be letters, digits, underscore, or hyphen")
	errReservedPerm    = errors.New("permission * is reserved for built-in admin")
)

// APIKey is a named secret assigned one or more roles.
type APIKey struct {
	Name  string   `json:"name"  toml:"name"  xml:"name"  yaml:"name"`
	Key   string   `json:"key"   toml:"key"   xml:"key"   yaml:"key"`
	Roles []string `json:"roles" toml:"roles" xml:"roles" yaml:"roles"`
}

// Role is a named permission set. Built-in admin is not stored here.
type Role struct {
	Permissions []string `json:"permissions" toml:"permissions" xml:"permissions" yaml:"permissions"`
}

func (k APIKey) validate(roles map[string]Role) error {
	if strings.TrimSpace(k.Name) == "" {
		return errAPIKeyName
	}

	if !isASCII(k.Key) {
		return fmt.Errorf("%w: %s", errAPIKeyASCII, k.Name)
	}

	if length := len(k.Key); length < apiKeyMinLen || length > apiKeyMaxLen {
		return fmt.Errorf("%w: %s is %d", errAPIKeyLength, k.Name, length)
	}

	if len(k.Roles) == 0 {
		return fmt.Errorf("%w: %s", errEmptyKeyRoles, k.Name)
	}

	for _, role := range k.Roles {
		if role == RoleAdmin {
			continue
		}

		if _, exists := roles[role]; !exists {
			return fmt.Errorf("%w: %s on key %s", errUnknownRole, role, k.Name)
		}
	}

	return nil
}

func (r Role) validate(name string) error {
	if !validRoleName(name) {
		return fmt.Errorf("%w: %q", errInvalidRoleName, name)
	}

	if name == RoleAdmin {
		return fmt.Errorf("%w: %s is built in and cannot be redefined", errUnknownRole, name)
	}

	if len(r.Permissions) == 0 {
		return fmt.Errorf("%w: %s", errEmptyRole, name)
	}

	for _, perm := range r.Permissions {
		if perm == PermAll {
			return fmt.Errorf("%w: on role %s", errReservedPerm, name)
		}

		if !KnownPermission(perm) {
			return fmt.Errorf("%w: %s on role %s", errUnknownPerm, perm, name)
		}
	}

	return nil
}

func (w *WebServer) validateAuth() error {
	if w == nil {
		return nil
	}

	for name, role := range w.Roles {
		if err := role.validate(name); err != nil {
			return err
		}
	}

	seenKeys := make(map[string]struct{}, len(w.APIKeys))
	seenNames := make(map[string]struct{}, len(w.APIKeys))
	w.keyPerms = make(map[string][]string, len(w.APIKeys))

	for _, key := range w.APIKeys {
		if err := key.validate(w.Roles); err != nil {
			return err
		}

		if _, dup := seenNames[key.Name]; dup {
			return fmt.Errorf("%w: %s", errAPIKeyNameDup, key.Name)
		}

		if _, dup := seenKeys[key.Key]; dup {
			return fmt.Errorf("%w: %s", errAPIKeyDup, key.Name)
		}

		seenNames[key.Name] = struct{}{}
		seenKeys[key.Key] = struct{}{}
		w.keyPerms[key.Key] = w.permissionsForRoles(key.Roles)
	}

	return nil
}

// PermissionsForKey returns the union of role permissions for this key.
// Built-in admin is all permissions.
func (w *WebServer) PermissionsForKey(key string) []string {
	if w == nil {
		return nil
	}

	if w.keyPerms != nil {
		return append([]string(nil), w.keyPerms[key]...)
	}

	for idx := range w.APIKeys {
		if w.APIKeys[idx].Key == key {
			return w.permissionsForRoles(w.APIKeys[idx].Roles)
		}
	}

	return nil
}

func isASCII(value string) bool {
	for idx := range len(value) {
		if value[idx] > unicode.MaxASCII {
			return false
		}
	}

	return true
}

func validRoleName(name string) bool {
	if name == "" {
		return false
	}

	for _, char := range name {
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z',
			char >= '0' && char <= '9', char == '_', char == '-':
		default:
			return false
		}
	}

	return true
}

func (w *WebServer) permissionsForRoles(roles []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)

	add := func(perms []string) {
		for _, perm := range perms {
			if _, exists := seen[perm]; exists {
				continue
			}

			seen[perm] = struct{}{}
			out = append(out, perm)
		}
	}

	for _, role := range roles {
		if role == RoleAdmin {
			return append([]string{}, AllPermissions()...)
		}

		if custom, exists := w.Roles[role]; exists {
			add(custom.Permissions)
		}
	}

	return out
}

func (w *WebServer) HasPermission(key, perm string) bool {
	for _, have := range w.PermissionsForKey(key) {
		if have == PermAll || have == perm {
			return true
		}
	}

	return false
}

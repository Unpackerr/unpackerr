package unpackerr

import (
	"errors"
	"fmt"
	"strings"
)

const (
	apiKeyMinLen = 60
	apiKeyMaxLen = 150
)

var (
	errAPIKeyLength = errors.New("api key must be 60–150 characters")
	errAPIKeyName   = errors.New("api key name is required")
	errAPIKeyDup    = errors.New("duplicate api key")
	errUnknownRole  = errors.New("unknown role")
	errUnknownPerm  = errors.New("unknown permission")
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

	if length := len(k.Key); length < apiKeyMinLen || length > apiKeyMaxLen {
		return fmt.Errorf("%w: %s is %d", errAPIKeyLength, k.Name, length)
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
	for _, perm := range r.Permissions {
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

	seen := make(map[string]struct{}, len(w.APIKeys))

	for _, key := range w.APIKeys {
		if err := key.validate(w.Roles); err != nil {
			return err
		}

		if _, dup := seen[key.Key]; dup {
			return fmt.Errorf("%w: %s", errAPIKeyDup, key.Name)
		}

		seen[key.Key] = struct{}{}
	}

	return nil
}

// PermissionsForKey returns the union of role permissions for this key.
// Built-in admin is all permissions.
func (w *WebServer) PermissionsForKey(key string) []string {
	if w == nil {
		return nil
	}

	var found *APIKey

	for idx := range w.APIKeys {
		if w.APIKeys[idx].Key == key {
			found = &w.APIKeys[idx]
			break
		}
	}

	if found == nil {
		return nil
	}

	return w.permissionsForRoles(found.Roles)
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

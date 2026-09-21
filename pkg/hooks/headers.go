package hooks

import (
	"fmt"
	"strings"
)

func skipRequestHeader(name string) bool {
	switch strings.ToLower(name) {
	case "content-length", "content-type", "host", "transfer-encoding", "connection", "upgrade":
		return true
	default:
		return false
	}
}

// ValidateHeaders checks extra webhook header names and values.
func ValidateHeaders(headers map[string]string) error {
	seen := make(map[string]struct{}, len(headers))

	for name, value := range headers {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || !validHeaderName(trimmed) {
			return fmt.Errorf("%w: %q", ErrHeaderName, name)
		}

		if strings.ContainsAny(trimmed, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: %q", ErrHeaderValue, trimmed)
		}

		lower := strings.ToLower(trimmed)
		if _, dup := seen[lower]; dup {
			return fmt.Errorf("%w: %q", ErrHeaderDup, trimmed)
		}

		seen[lower] = struct{}{}
	}

	return nil
}

func validHeaderName(name string) bool {
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

// SecretHeaderName reports whether a webhook header looks like a secret
// (Authorization, tokens, API keys, cookies). Shared with env GET redaction.
func SecretHeaderName(name string) bool {
	lower := strings.ToLower(name)

	for _, part := range []string{
		"authorization", "token", "secret", "password", "passwd",
		"cookie", "credential", "api-key", "apikey",
	} {
		if strings.Contains(lower, part) {
			return true
		}
	}

	return strings.HasSuffix(lower, "-key") || strings.HasSuffix(lower, "_key")
}

// RedactHeaderSecrets blanks Authorization and other secret-looking header values.
func RedactHeaderSecrets(headers map[string]string) {
	for name := range headers {
		if SecretHeaderName(name) {
			headers[name] = ""
		}
	}
}

package hooks

import (
	"strings"
	"time"
)

// ValidateWebhooks applies webhook defaults and requires a URL.
func ValidateWebhooks(list []*Config, defaultTimeout time.Duration) error {
	for idx := range list {
		if list[idx] == nil {
			return ErrNilConfig
		}

		list[idx].Command = ""

		if list[idx].URL == "" {
			return ErrWebhookNoURL
		}

		if list[idx].Name == "" {
			list[idx].Name = list[idx].URL
		}

		if list[idx].Nickname == "" && list[idx].TmplPath == "" &&
			!strings.Contains(list[idx].URL, "pushover.net") {
			list[idx].Nickname = "Unpackerr"
		}

		if list[idx].CType == "" {
			list[idx].CType = "application/json"
			if strings.Contains(list[idx].URL, "pushover.net") {
				list[idx].CType = "application/x-www-form-urlencoded"
			}
		}

		applyDefaults(list[idx], defaultTimeout)
		list[idx].ensureClient()
	}

	return nil
}

// ValidateCmdhooks applies command-hook defaults and requires a command.
// expandHome is optional; when set it is applied to the command path.
func ValidateCmdhooks(list []*Config, defaultTimeout time.Duration, expandHome func(string) string) error {
	for idx := range list {
		if list[idx] == nil {
			return ErrNilConfig
		}

		list[idx].URL = ""

		cmd := strings.TrimSpace(list[idx].Command)
		if expandHome != nil {
			cmd = strings.TrimSpace(expandHome(cmd))
		}

		list[idx].Command = cmd
		if list[idx].Command == "" {
			return ErrCmdhookNoCmd
		}

		if list[idx].Name == "" {
			list[idx].Name = strings.Fields(list[idx].Command)[0]
		}

		applyDefaults(list[idx], defaultTimeout)
	}

	return nil
}

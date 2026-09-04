//go:build windows || darwin

package unpackerr

import (
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/ui"
)

func (u *Unpackerr) changePasswordDialog() {
	current := defaultUIUser
	if u.Webserver != nil {
		current = u.Webserver.UIPassword.Username()
	}

	value, accepted, err := ui.Password("Unpackerr",
		"New web UI password. Prefix with username: to also change the user (current: "+current+").")
	if err != nil {
		u.Errorf("Password dialog: %v", err)
		_, _ = ui.Error("Unpackerr", "Password dialog failed: %v", err)

		return
	}

	if !accepted || strings.TrimSpace(value) == "" {
		return
	}

	user, plain := splitUserPass(value, current)
	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		u.Errorf("Setting UI password: %v", err)
		_, _ = ui.Error("Unpackerr", "Could not set password: %v", err)

		return
	}

	u.syncFileUIPassword()

	if err := u.writeConfigFile(); err != nil {
		u.Errorf("Writing config after password change: %v", err)
		_, _ = ui.Error("Unpackerr", "Password set in memory, but saving the config failed: %v", err)

		return
	}

	u.Printf("Updated UI password for user %q", user)
	_, _ = ui.Info("Unpackerr", "Web UI password updated for user %s.", user)
}

func (u *Unpackerr) showGeneratedPassword() {
	if u.uiPasswordNotice == "" || !ui.HasGUI() {
		return
	}

	_, _ = ui.Info("Unpackerr",
		"Temporary web UI password for user %s:\n\n%s\n\nChange it from the tray menu. This is also in the log.",
		defaultUIUser, u.uiPasswordNotice)
}

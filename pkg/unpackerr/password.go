package unpackerr

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

// CryptPass stores a UI password the same way Notifiarr does: !!cryptd!!, user:pass, webauth:Header, noauth.
//
//nolint:recvcheck // Set needs a pointer; lookups use a value like the Notifiarr type.
type CryptPass string

// AuthType is the configured web UI authentication mode.
type AuthType int

const (
	AuthPassword AuthType = iota
	AuthHeader
	AuthNone
)

const (
	authPassword  = "!!cryptd!!"
	authHeader    = "webauth"
	authNone      = "noauth"
	defaultUIUser = "admin"
	kdfIters      = 210000
	kdfKeyLen     = 32
	kdfSaltPrefix = "unpackerr:"
	minUIPassword = 8
	genPasswordN  = 12
)

var (
	errShortUIPassword = errors.New("ui password must be at least 8 characters")
	errEmptyAuthHeader = errors.New("auth header may not be empty")
)

func (t AuthType) String() string {
	switch t {
	case AuthHeader:
		return "header"
	case AuthNone:
		return "noauth"
	default:
		return "password"
	}
}

// DeriveKDF is PBKDF2-HMAC-SHA-256(password, salt="unpackerr:"+username, 210000, 32 bytes) as hex.
func DeriveKDF(username, password string) string {
	salt := []byte(kdfSaltPrefix + username)
	key := pbkdf2.Key([]byte(password), salt, kdfIters, kdfKeyLen, sha256.New)

	return hex.EncodeToString(key)
}

// GeneratePassword returns a random URL-safe temporary password.
func GeneratePassword() string {
	raw := make([]byte, genPasswordN)
	_, _ = rand.Read(raw)

	return base64.RawURLEncoding.EncodeToString(raw)
}

func splitUserPass(input string) (string, string) {
	user, pass, found := strings.Cut(strings.TrimSpace(input), ":")
	if !found {
		return defaultUIUser, user
	}

	if user == "" {
		user = defaultUIUser
	}

	return user, pass
}

func (p CryptPass) Val() string { return string(p) }

func (p CryptPass) IsCrypted() bool {
	return strings.HasPrefix(p.Val(), authPassword)
}

func (p CryptPass) Noauth() bool {
	return p == authNone || strings.HasPrefix(p.Val(), authNone+":")
}

func (p CryptPass) Webauth() bool {
	return p == authHeader || strings.HasPrefix(p.Val(), authHeader+":") || p.Noauth()
}

func (p CryptPass) Type() AuthType {
	switch {
	case p.Noauth():
		return AuthNone
	case p.Webauth():
		return AuthHeader
	default:
		return AuthPassword
	}
}

func (p CryptPass) Header() string {
	if user, rest, found := strings.Cut(p.Val(), ":"); found && user == authHeader {
		return rest
	}

	return "X-Webauth-User"
}

func (p CryptPass) Username() string {
	if p.IsCrypted() {
		rest := strings.TrimPrefix(p.Val(), authPassword)

		user, _, found := strings.Cut(rest, ":")
		if found && user != "" && !strings.HasPrefix(user, "$") {
			return user
		}

		return defaultUIUser
	}

	if p.Webauth() || p.Val() == "" {
		return defaultUIUser
	}

	user, _ := splitUserPass(p.Val())

	return user
}

// Set stores header/noauth as-is, or bcrypts the KDF hex as !!cryptd!!user:$2a$...
func (p *CryptPass) Set(username, secret string) error {
	if username == authHeader {
		if secret == "" {
			return errEmptyAuthHeader
		}

		*p = CryptPass(authHeader + ":" + secret)

		return nil
	}

	if username == authNone {
		*p = authNone
		if secret != "" {
			*p = CryptPass(authNone + ":" + secret)
		}

		return nil
	}

	if username == "" {
		username = defaultUIUser
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("encrypting password: %w", err)
	}

	*p = CryptPass(authPassword + username + ":" + string(hash))

	return nil
}

// SetPlain PBKDF2-hashes a plaintext password then bcrypts the KDF hex.
func (p *CryptPass) SetPlain(username, password string) error {
	if username == authHeader || username == authNone {
		return p.Set(username, password)
	}

	if len(password) < minUIPassword {
		return errShortUIPassword
	}

	if username == "" {
		username = defaultUIUser
	}

	return p.Set(username, DeriveKDF(username, password))
}

func (p CryptPass) Valid(username, kdfHex string) bool {
	if p.Webauth() || !p.IsCrypted() {
		return false
	}

	if p.Username() != username {
		return false
	}

	rest := strings.TrimPrefix(p.Val(), authPassword)

	_, hash, found := strings.Cut(rest, ":")
	if !found {
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(kdfHex)) == nil
}

func (p CryptPass) ValidPlain(username, password string) bool {
	return p.Valid(username, DeriveKDF(username, password))
}

func (u *Unpackerr) setupUIPassword() error {
	if u.setUIPassword != "" || u.Webserver == nil || !u.Webserver.Enabled() {
		return nil
	}

	pass := u.Webserver.UIPassword
	if pass.Webauth() || pass.IsCrypted() {
		return nil
	}

	if pass.Val() == "" {
		plain := GeneratePassword()
		if err := u.Webserver.UIPassword.SetPlain(defaultUIUser, plain); err != nil {
			return err
		}

		u.uiPasswordNotice = plain

		return u.writeUIPasswordConfig()
	}

	user, plain := splitUserPass(pass.Val())
	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		return err
	}

	return u.writeUIPasswordConfig()
}

func (u *Unpackerr) writeUIPasswordConfig() error {
	if err := u.writeConfigFile(); err != nil && !errors.Is(err, errNoConfigFile) {
		return err
	}

	return nil
}

func (u *Unpackerr) applyCLIPassword(input string) error {
	user, plain := splitUserPass(input)
	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		return err
	}

	if err := u.writeConfigFile(); err != nil {
		return err
	}

	u.Printf("Updated UI password for user %q and wrote %s", user, u.ConfigFile)

	return nil
}

func (u *Unpackerr) handleStartupPassword() error {
	if u.setUIPassword != "" {
		return u.applyCLIPassword(u.setUIPassword)
	}

	if u.uiPasswordNotice != "" {
		u.Printf("Generated temporary UI password for user %s: %s", defaultUIUser, u.uiPasswordNotice)
		u.Printf("Change it with --set-ui-password or the tray menu. It will not be shown again.")
	}

	return nil
}

func (u *Unpackerr) changePasswordDialog() {
	current := defaultUIUser
	if u.Webserver != nil {
		current = u.Webserver.UIPassword.Username()
	}

	value, accepted, err := ui.Entry("Unpackerr",
		"New web UI password. Prefix with username: to also change the user (current: "+current+").", "")
	if err != nil {
		u.Errorf("Password dialog: %v", err)
		_, _ = ui.Error("Unpackerr", "Password dialog failed: %v", err)

		return
	}

	if !accepted || strings.TrimSpace(value) == "" {
		return
	}

	user, plain := splitUserPass(value)
	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		u.Errorf("Setting UI password: %v", err)
		_, _ = ui.Error("Unpackerr", "Could not set password: %v", err)

		return
	}

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

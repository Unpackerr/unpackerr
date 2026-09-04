package unpackerr

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

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
	errShortUIPassword     = errors.New("ui password must be at least 8 characters")
	errEmptyAuthHeader     = errors.New("auth header may not be empty")
	errMalformedUIPassword = errors.New("ui_password stored hash is invalid")
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
func GeneratePassword() (string, error) {
	raw := make([]byte, genPasswordN)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func splitUserPass(input, fallback string) (string, string) {
	if fallback == "" {
		fallback = defaultUIUser
	}

	user, pass, found := strings.Cut(strings.TrimSpace(input), ":")
	if !found {
		return fallback, user
	}

	if user == "" {
		user = fallback
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
		if found && user != "" && !isBcryptHash(user) {
			return user
		}

		return defaultUIUser
	}

	if p.Webauth() || p.Val() == "" {
		return defaultUIUser
	}

	user, _ := splitUserPass(p.Val(), defaultUIUser)

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

func isBcryptHash(value string) bool {
	return (strings.HasPrefix(value, "$2a$") ||
		strings.HasPrefix(value, "$2b$") ||
		strings.HasPrefix(value, "$2y$")) &&
		len(value) >= 59
}

func (p CryptPass) checkStored() error {
	if !p.IsCrypted() {
		return nil
	}

	rest := strings.TrimPrefix(p.Val(), authPassword)
	if isBcryptHash(rest) {
		return nil
	}

	_, hash, found := strings.Cut(rest, ":")
	if !found || !isBcryptHash(hash) {
		return errMalformedUIPassword
	}

	return nil
}

func (u *Unpackerr) setupUIPassword() error {
	if u.Webserver == nil || (!u.reset && !u.Webserver.Enabled()) {
		return nil
	}

	fromFile := strings.HasPrefix(u.Webserver.UIPassword.Val(), filePrefix)
	if err := u.expandUIPasswordFile(); err != nil {
		return err
	}

	if u.reset || !u.Webserver.Enabled() {
		return nil
	}

	pass := u.Webserver.UIPassword
	if err := pass.checkStored(); err != nil {
		return err
	}

	if pass.Webauth() {
		if pass.Type() == AuthHeader && pass.Header() == "" {
			return errEmptyAuthHeader
		}

		return nil
	}

	if pass.IsCrypted() {
		return nil
	}

	if pass.Val() == "" {
		plain, genErr := GeneratePassword()
		if genErr != nil {
			u.uiPasswordGenErr = genErr
			return nil //nolint:nilerr // logged after setupLogging; do not fail startup
		}

		if err := u.Webserver.UIPassword.SetPlain(defaultUIUser, plain); err != nil {
			return err
		}

		u.uiPasswordNotice = plain
		u.persistHashedUIPassword(fromFile)

		return nil
	}

	user, plain := splitUserPass(pass.Val(), defaultUIUser)
	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		return err
	}

	u.persistHashedUIPassword(fromFile)

	return nil
}

func expandCryptPassFile(pass *CryptPass) error {
	raw := pass.Val()
	if !strings.HasPrefix(raw, filePrefix) {
		return nil
	}

	data, err := os.ReadFile(strings.TrimPrefix(raw, filePrefix))
	if err != nil {
		return fmt.Errorf("reading ui_password file: %w", err)
	}

	*pass = CryptPass(strings.TrimSpace(string(data)))

	return nil
}

func (u *Unpackerr) expandUIPasswordFile() error {
	return expandCryptPassFile(&u.Webserver.UIPassword)
}

func (u *Unpackerr) persistHashedUIPassword(keepFilepath bool) {
	if keepFilepath || u.uiPasswordEnvSet() {
		return
	}

	u.syncFileUIPassword()
	u.persistConfigFile()
}

func (u *Unpackerr) resetUIPassword() error {
	user := defaultUIUser

	if u.Webserver != nil {
		if name := u.Webserver.UIPassword.Username(); name != "" {
			user = name
		}
	}

	plain, err := GeneratePassword()
	if err != nil {
		return err
	}

	if err := u.Webserver.UIPassword.SetPlain(user, plain); err != nil {
		return err
	}

	u.syncFileUIPassword()

	if err := u.writeConfigFile(); err != nil {
		return err
	}

	u.Printf("Reset UI password for user %q and wrote %s", user, u.ConfigFile)
	u.Printf("New %q user password: %s", user, plain)

	return nil
}

func (u *Unpackerr) handleStartupPassword() error {
	if u.reset {
		return u.resetUIPassword()
	}

	if u.uiPasswordNotice != "" {
		u.Printf("Generated temporary UI password for user %s: %s", defaultUIUser, u.uiPasswordNotice)
		u.Printf("Change it with --reset or the tray menu. It will not be shown again.")
	}

	if u.uiPasswordGenErr != nil {
		u.Errorf("Could not generate UI password: %v", u.uiPasswordGenErr)
	}

	if u.configWriteErr != nil {
		u.Errorf("Could not persist config to %s: %v", u.ConfigFile, u.configWriteErr)
	}

	if u.uiPasswordEnvSet() {
		u.Printf("%s is set; it overrides the config file on every start.", u.uiPasswordEnvName())
	}

	return nil
}

func (u *Unpackerr) uiPasswordEnvSet() bool {
	_, set := os.LookupEnv(u.uiPasswordEnvName())

	return set
}

func (u *Unpackerr) uiPasswordEnvName() string {
	prefix := u.EnvPrefix
	if prefix == "" {
		prefix = "UN"
	}

	return strings.ToUpper(prefix) + "_WEBSERVER_UI_PASSWORD"
}

package unpackerr

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDeriveKDFDeterministic(t *testing.T) {
	t.Parallel()

	const want = "c94fa10532fb8e444bb8fc1f0dbacb53f0505f3209ca0ff93aa6d5c7bc2b83bd"

	one := DeriveKDF("admin", "secret-password")
	if one != want {
		t.Fatalf("kdf %s want %s", one, want)
	}

	if DeriveKDF("admin", "secret-password") != one {
		t.Fatal("kdf must be deterministic")
	}

	if DeriveKDF("other", "secret-password") == one {
		t.Fatal("username must be part of the salt")
	}
}

func TestCryptPassPlainRoundTrip(t *testing.T) {
	t.Parallel()

	var pass CryptPass
	if err := pass.SetPlain("admin", "correct-horse"); err != nil {
		t.Fatal(err)
	}

	if !pass.IsCrypted() {
		t.Fatal("expected !!cryptd!! storage")
	}

	if pass.Username() != "admin" {
		t.Fatalf("username %q", pass.Username())
	}

	if !pass.ValidPlain("admin", "correct-horse") {
		t.Fatal("expected valid plaintext")
	}

	if pass.ValidPlain("admin", "wrong-password") {
		t.Fatal("wrong password must fail")
	}

	if pass.ValidPlain("other", "correct-horse") {
		t.Fatal("wrong user must fail")
	}
}

func TestCryptPassHeaderAndNone(t *testing.T) {
	t.Parallel()

	var pass CryptPass
	if err := pass.Set("webauth", "X-Remote-User"); err != nil {
		t.Fatal(err)
	}

	if pass.Type() != AuthHeader || pass.Header() != "X-Remote-User" {
		t.Fatalf("header %s %s", pass.Type(), pass.Header())
	}

	if err := pass.Set("noauth", ""); err != nil {
		t.Fatal(err)
	}

	if pass.Type() != AuthNone || !pass.Noauth() {
		t.Fatalf("noauth %s", pass.Type())
	}
}

func TestCryptPassShortRejected(t *testing.T) {
	t.Parallel()

	var pass CryptPass
	if err := pass.SetPlain("admin", "short"); err == nil {
		t.Fatal("expected short password error")
	}
}

func TestSetupUIPasswordSkipsDisabledServer(t *testing.T) {
	t.Parallel()

	unpack := New()

	unpack.Webserver.ListenAddr = ""
	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if unpack.Webserver.UIPassword.Val() != "" {
		t.Fatalf("got %q", unpack.Webserver.UIPassword)
	}
}

func TestSetupUIPasswordSkipsMissingFilepathWhenDisabled(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.ListenAddr = ""
	unpack.Webserver.UIPassword = CryptPass(filePrefix + "/no/such/ui.pass")

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}
}

func TestSetupUIPasswordHashesPlaintext(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.ConfigFile = t.TempDir() + "/unpackerr.conf"
	unpack.Webserver.UIPassword = "admin:correct-horse"

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if !unpack.Webserver.UIPassword.IsCrypted() {
		t.Fatal("expected hashed password")
	}

	if !unpack.Webserver.UIPassword.ValidPlain("admin", "correct-horse") {
		t.Fatal("hashed value must still validate")
	}

	if unpack.uiPasswordNotice != "" {
		t.Fatal("plaintext file should not invent a generated password")
	}
}

func TestGeneratePasswordLength(t *testing.T) {
	t.Parallel()

	pass, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}

	if len(pass) < minUIPassword {
		t.Fatalf("short generated password %q", pass)
	}

	if strings.Contains(pass, ":") {
		t.Fatalf("generated password should not include a colon: %q", pass)
	}
}

func TestCryptPassDollarUsername(t *testing.T) {
	t.Parallel()

	var pass CryptPass
	if err := pass.SetPlain("$bob", "correct-horse"); err != nil {
		t.Fatal(err)
	}

	if pass.Username() != "$bob" {
		t.Fatalf("username %q", pass.Username())
	}

	if !pass.ValidPlain("$bob", "correct-horse") {
		t.Fatal("expected valid $bob password")
	}
}

func TestSplitUserPassKeepsFallback(t *testing.T) {
	t.Parallel()

	user, plain := splitUserPass("brandnewpass1", "dave")
	if user != "dave" || plain != "brandnewpass1" {
		t.Fatalf("got %q %q", user, plain)
	}

	user, plain = splitUserPass("other:secret12", "dave")
	if user != "other" || plain != "secret12" {
		t.Fatalf("got %q %q", user, plain)
	}
}

func TestSetupUIPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.UIPassword = "!!cryptd!!$2a$10$notahash"

	if err := unpack.setupUIPassword(); err == nil {
		t.Fatal("expected malformed hash error")
	}
}

func TestSetupUIPasswordRejectsEmptyHeader(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.UIPassword = "webauth:"

	if err := unpack.setupUIPassword(); err == nil {
		t.Fatal("expected empty header error")
	}
}

func TestSetupUIPasswordExpandsFilepath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	passFile := dir + "/ui.pass"

	if err := os.WriteFile(passFile, []byte("correct-horse\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.ConfigFile = dir + "/unpackerr.conf"
	unpack.Webserver.UIPassword = CryptPass(filePrefix + passFile)

	unpack.snapshotFileConfig()

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if !unpack.Webserver.UIPassword.ValidPlain("admin", "correct-horse") {
		t.Fatal("filepath contents must become the password")
	}

	stored := unpack.fileConfig.Webserver.UIPassword.Val()
	if stored != filePrefix+passFile {
		t.Fatalf("file snapshot must keep filepath:, got %q", stored)
	}

	if _, err := os.Stat(unpack.ConfigFile); !os.IsNotExist(err) {
		t.Fatal("expanding filepath: must not rewrite the config file")
	}
}

func TestSetupUIPasswordEmptyFilepathKeepsPrefix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	passFile := dir + "/ui.pass"

	if err := os.WriteFile(passFile, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.ConfigFile = dir + "/unpackerr.conf"
	unpack.Webserver.UIPassword = CryptPass(filePrefix + passFile)
	unpack.snapshotFileConfig()

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if unpack.uiPasswordNotice == "" || !unpack.Webserver.UIPassword.IsCrypted() {
		t.Fatal("empty password file should generate a live password")
	}

	stored := unpack.fileConfig.Webserver.UIPassword.Val()
	if stored != filePrefix+passFile {
		t.Fatalf("empty filepath: must stay on disk, got %q", stored)
	}

	if _, err := os.Stat(unpack.ConfigFile); !os.IsNotExist(err) {
		t.Fatal("empty filepath: must not rewrite the config file")
	}
}

func TestResetUIPasswordWritesAndHashes(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.ConfigFile = t.TempDir() + "/unpackerr.conf"

	unpack.snapshotFileConfig()

	if err := unpack.resetUIPassword(); err != nil {
		t.Fatal(err)
	}

	if !unpack.Webserver.UIPassword.IsCrypted() {
		t.Fatal("expected hashed password")
	}

	if unpack.fileConfig == nil || !unpack.fileConfig.Webserver.UIPassword.IsCrypted() {
		t.Fatal("file snapshot must get the hashed password")
	}
}

func TestResetUIPasswordUsesFilepathUser(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	passFile := dir + "/ui.pass"

	if err := os.WriteFile(passFile, []byte("dave:topsecret99\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.reset = true
	unpack.ConfigFile = dir + "/unpackerr.conf"
	unpack.Webserver.UIPassword = CryptPass(filePrefix + passFile)
	unpack.snapshotFileConfig()

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if err := unpack.resetUIPassword(); err != nil {
		t.Fatal(err)
	}

	if unpack.Webserver.UIPassword.Username() != "dave" {
		t.Fatalf("reset user %q", unpack.Webserver.UIPassword.Username())
	}
}

func TestSetupUIPasswordUnwritableIsNotFatal(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("directory permissions are not unix-like")
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	conf := dir + "/unpackerr.conf"

	if err := os.WriteFile(conf, []byte("debug = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // need a read-only dir
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore after the read-only test

	unpack := New()
	unpack.ConfigFile = conf

	unpack.snapshotFileConfig()

	if err := unpack.setupUIPassword(); err != nil {
		t.Fatal(err)
	}

	if unpack.configWriteErr == nil {
		t.Fatal("expected persist error")
	}

	if unpack.uiPasswordNotice == "" {
		t.Fatal("expected generated password in memory")
	}
}

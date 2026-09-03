package unpackerr

import (
	"strings"
	"testing"
)

func TestDeriveKDFDeterministic(t *testing.T) {
	t.Parallel()

	one := DeriveKDF("admin", "secret-password")

	two := DeriveKDF("admin", "secret-password")
	if one != two || len(one) != 64 {
		t.Fatalf("kdf %s vs %s", one, two)
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

	pass := GeneratePassword()
	if len(pass) < minUIPassword {
		t.Fatalf("short generated password %q", pass)
	}

	if strings.Contains(pass, ":") {
		t.Fatalf("generated password should not include a colon: %q", pass)
	}
}

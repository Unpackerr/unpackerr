package hooks

import (
	"errors"
	"testing"
)

func TestValidateHeaders(t *testing.T) {
	t.Parallel()

	if err := ValidateHeaders(nil); err != nil {
		t.Fatalf("nil %v", err)
	}

	if err := ValidateHeaders(map[string]string{"X-Api-Key": "k", "Authorization": "Bearer x"}); err != nil {
		t.Fatalf("ok %v", err)
	}

	if err := ValidateHeaders(map[string]string{"Bad Name": "x"}); !errors.Is(err, ErrHeaderName) {
		t.Fatalf("space %v", err)
	}

	if err := ValidateHeaders(map[string]string{"X-Api-Key": "a\nb"}); !errors.Is(err, ErrHeaderValue) {
		t.Fatalf("newline %v", err)
	}

	if err := ValidateHeaders(map[string]string{"X-Api-Key": "a", "x-api-key": "b"}); !errors.Is(err, ErrHeaderDup) {
		t.Fatalf("dup %v", err)
	}
}

func TestRedactHeaderSecrets(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"Authorization":           "Bearer tok",
		"X-Api-Key":               "secret",
		"CF-Access-Client-Id":     "id",
		"CF-Access-Client-Secret": "shh",
		"Title":                   "Unpackerr",
	}
	RedactHeaderSecrets(headers)

	if headers["Authorization"] != "" || headers["X-Api-Key"] != "" || headers["CF-Access-Client-Secret"] != "" {
		t.Fatalf("secrets remain %+v", headers)
	}

	if headers["CF-Access-Client-Id"] != "id" || headers["Title"] != "Unpackerr" {
		t.Fatalf("non-secrets %+v", headers)
	}
}

func TestCloneListCopiesHeaders(t *testing.T) {
	t.Parallel()

	src := []*Config{{
		URL:     "https://example/hook",
		Headers: map[string]string{"X-Api-Key": "k"},
	}}
	got := CloneList(src)
	got[0].Headers["X-Api-Key"] = "changed"

	if src[0].Headers["X-Api-Key"] != "k" {
		t.Fatal("clone shared the headers map")
	}
}

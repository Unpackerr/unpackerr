package unpackerr

import "testing"

func TestPollQueueNilClientDoesNotPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v", rec)
		}
	}()

	app := &ReadarrConfig{}
	app.URL = "http://127.0.0.1:1"
	app.APIKey = "k"

	if _, _, _, err := app.pollQueue(); err == nil {
		t.Fatal("expected a queue request error")
	}
}

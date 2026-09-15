package hooks

import (
	"errors"
	"testing"

	"github.com/Unpackerr/unpackerr/pkg/extract"
)

func TestPrepareSampleEvents(t *testing.T) {
	t.Parallel()

	for _, event := range []extract.Status{
		extract.WAITING, extract.QUEUED, extract.EXTRACTING, extract.EXTRACTFAILED,
		extract.EXTRACTED, extract.IMPORTED, extract.DELETING, extract.DELETEFAILED,
		extract.DELETED, extract.EXTRACTEDNOTHING,
	} {
		t.Run(event.String(), func(t *testing.T) {
			t.Parallel()

			payload := SamplePayload()
			if err := PrepareSample(payload, event); err != nil {
				t.Fatal(err)
			}

			if payload.Event != event {
				t.Fatalf("event %s", payload.Event)
			}
		})
	}
}

func TestPrepareSampleUnknownEvent(t *testing.T) {
	t.Parallel()

	err := PrepareSample(SamplePayload(), extract.Status(255))
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("err %v", err)
	}
}

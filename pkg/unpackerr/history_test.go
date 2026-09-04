package unpackerr

import (
	"strconv"
	"testing"
)

func TestStatsConcurrentWithHistoryWrites(t *testing.T) {
	t.Parallel()

	unpack := New()
	done := make(chan struct{})

	go func() {
		defer close(done)

		for idx := range 2000 {
			name := strconv.Itoa(idx)

			unpack.lockHistory()
			unpack.Map[name] = &Extract{Status: WAITING}
			unpack.Retries++

			if idx > 0 {
				delete(unpack.Map, strconv.Itoa(idx-1))
			}

			unpack.Finished++
			unpack.unlockHistory()
		}
	}()

	for range 2000 {
		_ = unpack.stats()
	}

	<-done
}

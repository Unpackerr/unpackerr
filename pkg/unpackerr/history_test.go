package unpackerr

import (
	"strconv"
	"testing"

	"golift.io/xtractr"
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

func TestQueueSnapshotConcurrentWithProgress(t *testing.T) {
	t.Parallel()

	unpack := New()
	item := &Extract{Path: "/dl/a", Status: EXTRACTING}
	item.XProg = &ExtractProgress{Extract: item, Archives: 1}
	unpack.Map["a"] = item

	done := make(chan struct{})

	go func() {
		defer close(done)

		for idx := range 2000 {
			unpack.handleProgress(&ExtractProgress{
				Progress: &xtractr.Progress{
					XFile:      &xtractr.XFile{FilePath: "/dl/a/file.rar"},
					Compressed: 100,
					Read:       uint64(idx),
				},
				Extract: item,
			})
		}
	}()

	for range 2000 {
		_ = unpack.queueSnapshot()
	}

	<-done
}

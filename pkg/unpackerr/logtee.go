package unpackerr

import (
	"bytes"
	"io"
	"sync"
)

type lineRing struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newLineRing(maxLines int) *lineRing {
	return &lineRing{max: maxLines}
}

func (r *lineRing) add(line string) {
	if r == nil || line == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.lines = append(r.lines, line)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
}

func (r *lineRing) snapshot() []string {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.lines))
	copy(out, r.lines)

	return out
}

type logTee struct {
	hub  *liveHub
	id   string
	ring *lineRing
	mu   sync.Mutex
	buf  []byte
}

func newLogTee(hub *liveHub, id string) *logTee {
	return &logTee{
		hub:  hub,
		id:   id,
		ring: newLineRing(logRingMax),
	}
}

func (t *logTee) Write(p []byte) (int, error) { //nolint:varnamelen
	if t == nil {
		return len(p), nil
	}

	t.mu.Lock()
	t.buf = append(t.buf, p...)

	for {
		idx := bytes.IndexByte(t.buf, '\n')
		if idx < 0 {
			break
		}

		line := string(bytes.TrimRight(t.buf[:idx], "\r"))
		t.buf = t.buf[idx+1:]
		t.ring.add(line)
		t.hub.notifyLog(t.id, line)
	}

	t.mu.Unlock()

	return len(p), nil
}

var _ io.Writer = (*logTee)(nil)

func (u *Unpackerr) ensureAppLogTee() io.Writer {
	if u.appLogTee != nil {
		return u.appLogTee
	}

	id := encodeLogFileID(u.LogFile)
	u.appLogTee = newLogTee(u.hub, id)
	u.hub.setAppLogID(id)

	return u.appLogTee
}

func (u *Unpackerr) ensureHTTPLogTee() io.Writer {
	if u.httpLogTee != nil {
		return u.httpLogTee
	}

	id := encodeLogFileID(u.Webserver.LogFile)
	u.httpLogTee = newLogTee(u.hub, id)
	u.hub.setHTTPLogID(id)

	return u.httpLogTee
}

func (u *Unpackerr) logFollowSnapshot(fileID string, count int) []string {
	if fileID == "" {
		fileID = liveLogID
	}

	if fileID == errorLogID {
		return splitLogLines(formatErrorLines(u.hub.errorSnapshot(), count, 0))
	}

	info := u.findLogFile(fileID)
	if info != nil && info.Path != "" && !isSyntheticLog(info) {
		raw, err := getLinesFromFile(info.Path, count, 0)
		if err == nil && len(raw) > 0 {
			return splitLogLines(string(raw))
		}
	}

	switch fileID {
	case u.hub.httpLogID:
		if u.httpLogTee != nil {
			return lastN(u.httpLogTee.ring.snapshot(), count)
		}
	default:
		if u.appLogTee != nil {
			return lastN(u.appLogTee.ring.snapshot(), count)
		}
	}

	return nil
}

func lastN(lines []string, count int) []string {
	if count <= 0 || len(lines) <= count {
		return lines
	}

	return lines[len(lines)-count:]
}

func splitLogLines(text string) []string {
	text = string(bytes.TrimRight([]byte(text), "\n"))
	if text == "" {
		return nil
	}

	return bytesSplitLines(text)
}

func bytesSplitLines(text string) []string {
	raw := bytes.Split([]byte(text), []byte{'\n'})
	out := make([]string, 0, len(raw))

	for _, line := range raw {
		out = append(out, string(bytes.TrimRight(line, "\r")))
	}

	return out
}

package unpackerr

import (
	"encoding/json"
	"testing"
)

func TestStatsUnchangedWithStarrQueues(t *testing.T) {
	t.Parallel()

	prev := Stats{Waiting: 1, StarrQueues: []StarrQueueStat{{Name: "A", Queued: 2}}}
	same := prev
	same.StarrQueues = []StarrQueueStat{{Name: "A", Queued: 2}}

	if !statsUnchanged(prev, true, &same) {
		t.Fatal("equal stats should skip")
	}

	same.StarrQueues[0].Queued = 3
	if statsUnchanged(prev, true, &same) {
		t.Fatal("queued change should send")
	}
}

func TestLiveHubCoalescesQueue(t *testing.T) {
	t.Parallel()

	hub := newLiveHub()
	client := &liveClient{
		topics: map[string]struct{}{topicQueue: {}},
		send:   make(chan []byte, 8),
	}
	hub.clients[client] = struct{}{}

	hub.notify(topicQueue, &queueFrame{
		Stats: &Stats{HookFail: 1},
		Items: []QueueItem{{ID: "a"}},
	})
	hub.notify(topicQueue, &queueFrame{
		Stats: &Stats{HookFail: 2},
		Items: []QueueItem{{ID: "b"}},
	})
	hub.flushQueue()

	if len(client.send) != 1 {
		t.Fatalf("queue frames %d", len(client.send))
	}

	got := decodeQueue(t, <-client.send)
	if got.Stats == nil || got.Stats.HookFail != 2 {
		t.Fatalf("stats %+v", got.Stats)
	}

	if len(got.Items) != 1 || got.Items[0].ID != "b" {
		t.Fatalf("items %+v", got.Items)
	}

	hub.flushQueue()

	if len(client.send) != 0 {
		t.Fatal("second flush should be idle")
	}
}

func TestLiveHubFlushStatsOnChange(t *testing.T) {
	t.Parallel()

	fail := uint(4)
	hub := newLiveHub()
	hub.statsFn = func() *Stats { return &Stats{HookFail: fail} }
	client := &liveClient{
		topics: map[string]struct{}{topicQueue: {}},
		send:   make(chan []byte, 8),
	}
	hub.clients[client] = struct{}{}

	hub.flushStats()

	first := decodeQueue(t, <-client.send)
	if first.Stats == nil || first.Stats.HookFail != 4 || first.Items != nil {
		t.Fatalf("first stats %+v items %v", first.Stats, first.Items)
	}

	hub.flushStats()

	if len(client.send) != 0 {
		t.Fatal("unchanged stats should not send")
	}

	fail = 5

	hub.flushStats()

	second := decodeQueue(t, <-client.send)
	if second.Stats == nil || second.Stats.HookFail != 5 {
		t.Fatalf("updated %+v", second.Stats)
	}
}

func TestLiveHubFlushStatsNeedsQueueSub(t *testing.T) {
	t.Parallel()

	hub := newLiveHub()
	hub.statsFn = func() *Stats { return &Stats{HookFail: 1} }
	client := &liveClient{
		topics: map[string]struct{}{topicLogs: {}},
		send:   make(chan []byte, 8),
	}
	hub.clients[client] = struct{}{}
	hub.flushStats()

	if len(client.send) != 0 {
		t.Fatal("stats flush requires a queue subscription")
	}
}

func decodeQueue(t *testing.T, raw []byte) queueFrame {
	t.Helper()

	var frame wsFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}

	var got queueFrame
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}

	return got
}

func TestWSOriginPatternsEmptyIsSameOrigin(t *testing.T) {
	t.Parallel()

	unpack := New()
	if unpack.wsOriginPatterns() != nil {
		t.Fatal("empty origins must keep the library same-origin default")
	}

	unpack.Webserver.WSOrigins = StringSlice{"", " localhost:5173 "}

	got := unpack.wsOriginPatterns()
	if len(got) != 1 || got[0] != "localhost:5173" {
		t.Fatalf("trimmed origins %q", got)
	}
}

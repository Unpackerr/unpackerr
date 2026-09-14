package unpackerr

import (
	"encoding/json"
	"reflect"
	"sync"
	"time"
)

// Live UI pub/sub. Producers (Starr poll, xtractr callbacks, history JSONL, log
// tees) call notify* and never touch a websocket. livews.go upgrades GET /ws
// and registers a liveClient per browser; run() is the only goroutine that
// walks the client map.
//
// Queue and progress are coalesced (latest snapshot / latest item wins) so a
// slow UI cannot stall poll workers. History, errors, and log lines go through
// buffered channels and are dropped when full.

const (
	topicQueue    = "queue"    // dashboard table + Stats; full snapshot, then diffs via progress
	topicProgress = "progress" // one QueueItem; UI patches that row instead of replacing the table
	topicHistory  = "history"  // JSONL upsert/delete/clear
	topicLogs     = "logs"     // one rotatorr file at a time (client.file)
	topicErrors   = "errors"   // Errorf lines; no extra permission beyond being logged in

	liveLogID  = "live"
	errorLogID = "errors"

	hubEventBuf     = 64
	hubLogBuf       = 512
	clientSendBuf   = 32
	logRingMax      = 500
	errorRingMax    = 10
	progressFlush   = 100 * time.Millisecond
	wsPingInterval  = 30 * time.Second
	wsWriteWait     = 10 * time.Second
	wsMaxReadBytes  = 4096
	followLogLines  = 50
	defaultLogLines = 500
	maxLogLines     = 10000
	hubClientBuf    = 8
)

// wsFrame is one JSON message on the wire. Type is hello, denied, queue,
// progress, history, log, or error.
type wsFrame struct {
	Type    string   `json:"type"`
	Topics  []string `json:"topics,omitempty"`
	Payload any      `json:"payload,omitempty"`
	File    string   `json:"file,omitempty"`
	Line    string   `json:"line,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// clientOp is what the browser sends: {"op":"sub"|"unsub","topics":[...],"file":"..."}.
type clientOp struct {
	Op     string   `json:"op"`
	Topics []string `json:"topics"`
	File   string   `json:"file"`
}

// queueFrame is the dashboard payload. Items nil means "stats only — keep the table".
type queueFrame struct {
	Stats *Stats      `json:"stats"`
	Items []QueueItem `json:"items"`
}

type historyFrame struct {
	Op   string          `json:"op"` // snapshot, upsert, delete, clear
	Row  *HistoryRecord  `json:"row,omitempty"`
	Rows []HistoryRecord `json:"rows,omitempty"`
	ID   string          `json:"id,omitempty"`
}

type errorFrame struct {
	At  time.Time `json:"at"`
	Msg string    `json:"msg"`
}

type hubEvent struct {
	kind    string
	file    string
	payload any
}

// liveClient is one websocket. send is drained by liveWSWrite; enqueue drops
// when the buffer is full so a stuck browser cannot back up the hub.
type liveClient struct {
	mu     sync.Mutex
	topics map[string]struct{}
	file   string // rotatorr id when subscribed to logs
	send   chan []byte
	info   authInfo
}

// liveHub owns the client set. run() is the only writer of clients.
// pending / pendingQ are filled from any goroutine and flushed on the ticker.
type liveHub struct {
	register   chan *liveClient
	unregister chan *liveClient
	events     chan hubEvent
	logs       chan hubEvent // separate so a burst of log lines does not crowd out history
	clients    map[*liveClient]struct{}
	progMu     sync.Mutex
	pending    map[string]QueueItem // id → latest progress this tick
	queueMu    sync.Mutex
	pendingQ   *queueFrame // latest queue snapshot this tick; overwritten, never queued
	lastStats  Stats
	haveStats  bool
	statsFn    func() *Stats // Unpackerr.stats; set in New so tests can stub it
	appLogID   string
	httpLogID  string
	errMu      sync.Mutex
	errors     []errorFrame // ring of recent Errorf lines for GET /api/logs/errors
	stop       chan struct{}
	stopOnce   sync.Once
}

func newLiveHub() *liveHub {
	return &liveHub{
		register:   make(chan *liveClient, hubClientBuf),
		unregister: make(chan *liveClient, hubClientBuf),
		events:     make(chan hubEvent, hubEventBuf),
		logs:       make(chan hubEvent, hubLogBuf),
		clients:    make(map[*liveClient]struct{}),
		pending:    make(map[string]QueueItem),
		appLogID:   liveLogID,
		stop:       make(chan struct{}),
	}
}

// run is started from Start. It is the only goroutine that mutates clients.
func (h *liveHub) run() {
	ticker := time.NewTicker(progressFlush)
	defer ticker.Stop()

	for {
		select {
		case <-h.stop:
			return
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			delete(h.clients, client)
		case event := <-h.events:
			h.broadcast(event)
		case event := <-h.logs:
			h.broadcast(event)
		case <-ticker.C:
			h.flushProgress()
			h.flushQueue()
			h.flushStats()
		}
	}
}

func (h *liveHub) shutdown() {
	if h == nil {
		return
	}

	h.stopOnce.Do(func() { close(h.stop) })
}

// flushProgress sends at most one progress frame per item ID per tick.
func (h *liveHub) flushProgress() {
	h.progMu.Lock()
	if len(h.pending) == 0 {
		h.progMu.Unlock()
		return
	}

	items := h.pending
	h.pending = make(map[string]QueueItem)
	h.progMu.Unlock()

	for _, item := range items {
		h.broadcast(hubEvent{kind: topicProgress, payload: item})
	}
}

func (h *liveHub) flushQueue() {
	h.queueMu.Lock()
	frame := h.pendingQ
	h.pendingQ = nil
	h.queueMu.Unlock()

	if frame == nil {
		return
	}

	h.broadcast(hubEvent{kind: topicQueue, payload: frame})
	h.rememberStats(frame.Stats)
}

// flushStats pushes a stats-only queue frame when counters changed without a Map update
// (hook totals, event/hook/delete stacks). Items is left nil so the UI keeps its table.
func (h *liveHub) flushStats() {
	if h.statsFn == nil || !h.hasTopic(topicQueue) {
		return
	}

	stats := h.statsFn()
	if stats == nil || statsUnchanged(h.lastStats, h.haveStats, stats) {
		return
	}

	h.rememberStats(stats)
	h.broadcast(hubEvent{kind: topicQueue, payload: &queueFrame{Stats: stats}})
}

func (h *liveHub) rememberStats(stats *Stats) {
	if stats == nil {
		return
	}

	h.lastStats = *stats
	h.haveStats = true
}

func statsUnchanged(prev Stats, have bool, next *Stats) bool {
	return have && next != nil && reflect.DeepEqual(prev, *next)
}

func (h *liveHub) hasTopic(topic string) bool {
	for client := range h.clients {
		if client.wants(topic, "") {
			return true
		}
	}

	return false
}

func (h *liveHub) broadcast(event hubEvent) {
	body, err := json.Marshal(frameFor(event))
	if err != nil {
		return
	}

	for client := range h.clients {
		if client.wants(event.kind, event.file) {
			client.enqueue(body)
		}
	}
}

func frameFor(event hubEvent) wsFrame {
	switch event.kind {
	case topicLogs:
		line, _ := event.payload.(string)

		return wsFrame{Type: "log", File: event.file, Line: line}
	case topicErrors:
		return wsFrame{Type: "error", Payload: event.payload}
	case topicQueue:
		return wsFrame{Type: "queue", Payload: event.payload}
	case topicProgress:
		return wsFrame{Type: "progress", Payload: event.payload}
	case topicHistory:
		return wsFrame{Type: "history", Payload: event.payload}
	default:
		return wsFrame{Type: event.kind, Payload: event.payload}
	}
}

// notify is safe from any goroutine, including while History.mu is held.
func (h *liveHub) notify(kind string, payload any) {
	if h == nil {
		return
	}

	// Queue is coalesced onto pendingQ so a full events chan cannot drop the
	// latest snapshot; flushQueue sends it on the next tick.
	if kind == topicQueue {
		frame, _ := payload.(*queueFrame)

		h.queueMu.Lock()
		h.pendingQ = frame
		h.queueMu.Unlock()

		return
	}

	select {
	case h.events <- hubEvent{kind: kind, payload: payload}:
	default:
	}
}

// notifyLog is called from the log tees. Drops the line if logs is full.
func (h *liveHub) notifyLog(file, line string) {
	if h == nil || line == "" {
		return
	}

	select {
	case h.logs <- hubEvent{kind: topicLogs, file: file, payload: line}:
	default:
	}
}

// notifyProgress keeps one pending row per item ID; flushProgress sends them.
func (h *liveHub) notifyProgress(item QueueItem) {
	if h == nil {
		return
	}

	h.progMu.Lock()
	h.pending[item.ID] = item
	h.progMu.Unlock()
}

// notifyError stores a ring for GET /api/logs/errors and fans the same frame to subscribers.
func (h *liveHub) notifyError(msg string) {
	if h == nil || msg == "" {
		return
	}

	frame := errorFrame{At: time.Now(), Msg: msg}

	h.errMu.Lock()

	h.errors = append(h.errors, frame)
	if len(h.errors) > errorRingMax {
		h.errors = h.errors[len(h.errors)-errorRingMax:]
	}
	h.errMu.Unlock()

	h.notify(topicErrors, frame)
}

func (h *liveHub) errorSnapshot() []errorFrame {
	if h == nil {
		return nil
	}

	h.errMu.Lock()
	defer h.errMu.Unlock()

	out := make([]errorFrame, len(h.errors))
	copy(out, h.errors)

	return out
}

func (h *liveHub) setAppLogID(id string) {
	if h == nil || id == "" {
		return
	}

	h.appLogID = id
}

func (h *liveHub) setHTTPLogID(id string) {
	if h == nil {
		return
	}

	h.httpLogID = id
}

func (c *liveClient) enqueue(msg []byte) {
	select {
	case c.send <- msg:
	default:
	}
}

// wants is true if the client subscribed to topic. Logs also match client.file.
func (c *liveClient) wants(topic, file string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.topics[topic]; !ok {
		return false
	}

	if topic != topicLogs {
		return true
	}

	return file == "" || c.file == file
}

func (c *liveClient) setTopics(add []string, file string, drop []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.topics == nil {
		c.topics = make(map[string]struct{})
	}

	for _, topic := range drop {
		delete(c.topics, topic)

		if topic == topicLogs {
			c.file = ""
		}
	}

	for _, topic := range add {
		c.topics[topic] = struct{}{}
		if topic == topicLogs {
			c.file = file
		}
	}
}

// topicPerm is the API key / session permission for a subscribe. Empty means
// any authenticated client (errors). "unknown" is rejected in handleSub.
func topicPerm(topic string) string {
	switch topic {
	case topicQueue, topicProgress:
		return PermReadSystemQueue
	case topicHistory:
		return PermReadSystemHistory
	case topicLogs:
		return PermReadSystemLogs
	case topicErrors:
		return ""
	default:
		return "unknown"
	}
}

func allowedTopics(info authInfo) []string {
	topics := []string{topicQueue, topicProgress, topicHistory, topicLogs, topicErrors}
	out := make([]string, 0, len(topics))

	for _, topic := range topics {
		if perm := topicPerm(topic); perm == "" || info.allows(perm) {
			out = append(out, topic)
		}
	}

	return out
}

// notifyQueueLocked pushes a full queue+stats frame. Caller holds History.mu;
// fillQueueStats then takes configMu (that lock order is required).
func (u *Unpackerr) notifyQueueLocked() {
	if u.hub == nil {
		return
	}

	stats := &Stats{}
	u.fillQueueStats(stats)
	u.hub.notify(topicQueue, &queueFrame{Stats: stats, Items: u.queueSnapshotLocked()})
}

func (u *Unpackerr) queueSnapshotLocked() []QueueItem {
	out := make([]QueueItem, 0, len(u.Map))

	for name, item := range u.Map {
		out = append(out, queueFromExtract(name, item))
	}

	return out
}

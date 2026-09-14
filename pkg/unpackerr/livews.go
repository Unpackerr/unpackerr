package unpackerr

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Live UI websocket edge. liveHub is the fan-out; this file only upgrades
// GET {URLBase}/ws, pumps frames, and translates sub/unsub JSON into topics.
// The browser client is frontend/src/lib/socket.svelte.ts.

func (u *Unpackerr) registerLiveWS() {
	u.Webserver.handleGet(path.Join(u.Webserver.URLBase, "ws"), u.requireAuth(u.liveWSHandler))
}

func (u *Unpackerr) liveWSHandler(response http.ResponseWriter, request *http.Request) {
	info, _ := request.Context().Value(authCtxKey).(authInfo)

	conn, err := websocket.Accept(response, request, &websocket.AcceptOptions{
		OriginPatterns: u.wsOriginPatterns(),
	})
	if err != nil {
		return
	}

	conn.SetReadLimit(wsMaxReadBytes)

	ctx := request.Context()
	client := &liveClient{
		topics: make(map[string]struct{}),
		send:   make(chan []byte, clientSendBuf),
		info:   info,
	}

	u.hub.register <- client
	defer func() {
		u.hub.unregister <- client

		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	// hello lists topics this session/key may subscribe to.
	enqueueFrame(client, wsFrame{Type: "hello", Topics: allowedTopics(info)})

	go u.liveWSWrite(ctx, conn, client)

	u.liveWSRead(ctx, conn, client)
}

// liveWSWrite is the only writer to conn. It also pings so proxies keep the socket.
func (u *Unpackerr) liveWSWrite(ctx context.Context, conn *websocket.Conn, client *liveClient) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.send:
			wctx, cancel := context.WithTimeout(ctx, wsWriteWait)
			err := conn.Write(wctx, websocket.MessageText, msg)

			cancel()

			if err != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, wsWriteWait)
			err := conn.Ping(pctx)

			cancel()

			if err != nil {
				return
			}
		}
	}
}

// liveWSRead blocks until the socket dies. Sub and unsub are the only client ops.
func (u *Unpackerr) liveWSRead(ctx context.Context, conn *websocket.Conn, client *liveClient) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var msg clientOp
		if json.Unmarshal(data, &msg) != nil {
			continue
		}

		switch msg.Op {
		case "sub":
			u.handleSub(client, msg)
		case "unsub":
			client.setTopics(nil, "", msg.Topics)
		}
	}
}

// handleSub checks topicPerm, then sendSnapshot so the page is not empty.
func (u *Unpackerr) handleSub(client *liveClient, msg clientOp) {
	allowed := make([]string, 0, len(msg.Topics))
	denied := make([]string, 0)

	for _, topic := range msg.Topics {
		perm := topicPerm(topic)
		if perm == "unknown" || (perm != "" && !client.info.allows(perm)) {
			denied = append(denied, topic)
			continue
		}

		allowed = append(allowed, topic)
	}

	file := msg.File
	if file == "" {
		file = u.hub.appLogID
	}

	client.setTopics(allowed, file, nil)

	for _, topic := range denied {
		enqueueFrame(client, wsFrame{Type: "denied", Error: topic})
	}

	for _, topic := range allowed {
		u.sendSnapshot(client, topic, file)
	}
}

// sendSnapshot is the catch-up payload right after subscribe so the UI is not
// empty until the next notify. Progress has no snapshot; the queue table is enough.
func (u *Unpackerr) sendSnapshot(client *liveClient, topic, file string) {
	switch topic {
	case topicQueue:
		stats := u.stats()
		enqueueFrame(client, wsFrame{Type: "queue", Payload: &queueFrame{
			Stats: stats,
			Items: u.queueSnapshot(),
		}})
	case topicHistory:
		enqueueFrame(client, wsFrame{Type: "history", Payload: historyFrame{
			Op:   "snapshot",
			Rows: u.historySnapshot(),
		}})
	case topicLogs:
		u.sendLogSnapshot(client, file)
	}
}

// sendLogSnapshot replays the last followLogLines from the rotatorr file.
func (u *Unpackerr) sendLogSnapshot(client *liveClient, file string) {
	for _, line := range u.logFollowSnapshot(file, followLogLines) {
		enqueueFrame(client, wsFrame{Type: "log", File: file, Line: line})
	}
}

func enqueueFrame(client *liveClient, frame wsFrame) {
	body, err := json.Marshal(frame)
	if err != nil {
		return
	}

	client.enqueue(body)
}

// wsOriginPatterns is the Trust-page allowlist. Empty keeps the library
// same-origin default so the embedded UI works without extra config.
func (u *Unpackerr) wsOriginPatterns() []string {
	if u.Webserver == nil {
		return nil
	}

	u.uiPassMu.RLock()
	defer u.uiPassMu.RUnlock()

	out := make([]string, 0, len(u.Webserver.WSOrigins))

	for _, origin := range u.Webserver.WSOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			out = append(out, origin)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

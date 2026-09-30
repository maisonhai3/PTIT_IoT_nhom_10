// Package hub fans state updates out to WebSocket clients.
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Message is the envelope every WebSocket frame uses: {"type": "...", "data": ...}.
type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

const (
	sendBuffer   = 32
	writeTimeout = 5 * time.Second
	pingEvery    = 25 * time.Second
	maxClients   = 100
)

type client struct {
	send chan []byte
	done chan struct{} // closed to make the writer loop drop this client
	once sync.Once
}

func (c *client) kick() { c.once.Do(func() { close(c.done) }) }

type Hub struct {
	log *slog.Logger

	mu      sync.Mutex
	clients map[*client]struct{}
}

func New(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log, clients: map[*client]struct{}{}}
}

// Len is the number of connected clients.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Broadcast sends m to every client without ever blocking: a client whose buffer is full is too slow
// (or dead) and gets disconnected, so one bad browser tab cannot stall the MQTT handler.
func (h *Hub) Broadcast(m Message) {
	b, err := json.Marshal(m)
	if err != nil {
		h.log.Error("hub: cannot marshal message", "type", m.Type, "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- b:
		default:
			h.log.Warn("hub: dropping slow websocket client")
			c.kick()
		}
	}
}

func (h *Hub) add(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= maxClients {
		return false
	}
	h.clients[c] = struct{}{}
	return true
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Handler upgrades the request and serves one client until it disconnects. initial is called after the client
// is registered and its messages are sent first (a snapshot), so nothing is missed between snapshot and stream.
// originPatterns are host patterns allowed to connect from a browser (same-origin is always allowed).
func (h *Hub) Handler(originPatterns []string, initial func() []Message) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originPatterns})
		if err != nil {
			h.log.Debug("websocket accept failed", "err", err)
			return
		}
		defer conn.CloseNow()

		c := &client{send: make(chan []byte, sendBuffer), done: make(chan struct{})}
		if !h.add(c) {
			conn.Close(websocket.StatusTryAgainLater, "too many clients")
			return
		}
		defer h.remove(c)

		// The client never sends data; CloseRead handles pings/close and cancels ctx when the peer goes away.
		ctx := conn.CloseRead(r.Context())

		write := func(b []byte) error {
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			defer cancel()
			return conn.Write(wctx, websocket.MessageText, b)
		}
		if initial != nil {
			for _, m := range initial() {
				b, err := json.Marshal(m)
				if err != nil || write(b) != nil {
					return
				}
			}
		}

		ping := time.NewTicker(pingEvery)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				conn.Close(websocket.StatusPolicyViolation, "too slow")
				return
			case b := <-c.send:
				if write(b) != nil {
					return
				}
			case <-ping.C:
				pctx, cancel := context.WithTimeout(ctx, writeTimeout)
				err := conn.Ping(pctx)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}
}

package hub

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func dial(t *testing.T, srv *httptest.Server, origin string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	opts := &websocket.DialOptions{}
	if origin != "" {
		opts.HTTPHeader = map[string][]string{"Origin": {origin}}
	}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), opts)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func read(t *testing.T, c *websocket.Conn) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("bad frame %q: %v", b, err)
	}
	return Message{Type: raw.Type, Data: string(raw.Data)}
}

func TestInitialSnapshotThenBroadcast(t *testing.T) {
	h := New(nil)
	srv := httptest.NewServer(h.Handler(nil, func() []Message {
		return []Message{{"state", map[string]int{"n": 1}}, {"weather", map[string]int{"n": 2}}}
	}))
	defer srv.Close()

	c := dial(t, srv, "")
	if m := read(t, c); m.Type != "state" || m.Data != `{"n":1}` {
		t.Fatalf("first frame: %+v", m)
	}
	if m := read(t, c); m.Type != "weather" || m.Data != `{"n":2}` {
		t.Fatalf("second frame: %+v", m)
	}

	deadline := time.Now().Add(2 * time.Second)
	for h.Len() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	h.Broadcast(Message{"event", map[string]string{"k": "v"}})
	if m := read(t, c); m.Type != "event" || m.Data != `{"k":"v"}` {
		t.Fatalf("broadcast frame: %+v", m)
	}
}

func TestBroadcastReachesAllClientsAndCleansUp(t *testing.T) {
	h := New(nil)
	srv := httptest.NewServer(h.Handler(nil, nil))
	defer srv.Close()

	a, b := dial(t, srv, ""), dial(t, srv, "")
	for h.Len() != 2 {
		time.Sleep(5 * time.Millisecond)
	}
	h.Broadcast(Message{"state", 42})
	for _, c := range []*websocket.Conn{a, b} {
		if m := read(t, c); m.Type != "state" || m.Data != "42" {
			t.Fatalf("frame: %+v", m)
		}
	}
	a.Close(websocket.StatusNormalClosure, "bye")
	deadline := time.Now().Add(2 * time.Second)
	for h.Len() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.Len() != 1 {
		t.Errorf("client not removed after close: %d", h.Len())
	}
}

func TestSlowClientIsKickedWithoutBlockingBroadcast(t *testing.T) {
	h := New(nil)
	c := &client{send: make(chan []byte, 2), done: make(chan struct{})}
	h.clients[c] = struct{}{}

	start := time.Now()
	for i := 0; i < 5; i++ {
		h.Broadcast(Message{"state", i})
	}
	if time.Since(start) > time.Second {
		t.Fatal("Broadcast blocked on a full client buffer")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("slow client was not kicked")
	}
}

func TestOriginCheck(t *testing.T) {
	h := New(nil)
	srv := httptest.NewServer(h.Handler([]string{"allowed.test:5173"}, nil))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	try := func(origin string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: map[string][]string{"Origin": {origin}}})
		if err == nil {
			c.CloseNow()
		}
		return err
	}
	if err := try("http://allowed.test:5173"); err != nil {
		t.Errorf("allowed origin rejected: %v", err)
	}
	if err := try("http://evil.test"); err == nil {
		t.Error("foreign origin was accepted")
	}
}

func TestConcurrentBroadcastAndConnect(t *testing.T) {
	h := New(nil)
	srv := httptest.NewServer(h.Handler(nil, nil))
	defer srv.Close()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				h.Broadcast(Message{"state", i})
				time.Sleep(time.Millisecond)
			}
		}
	}()
	for i := 0; i < 5; i++ {
		c := dial(t, srv, "")
		_ = read(t, c)
		c.Close(websocket.StatusNormalClosure, "")
	}
	close(stop)
	wg.Wait()
}

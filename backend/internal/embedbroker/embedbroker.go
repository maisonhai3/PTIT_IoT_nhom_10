// Package embedbroker runs an in-process MQTT broker (mochi-mqtt). It exists so the whole system can be
// exercised without Docker, Mosquitto or hardware: in end-to-end tests and in the `demo` command.
// It accepts any client (no auth), so it is not a replacement for the Mosquitto deployment in deploy/.
package embedbroker

import (
	"fmt"
	"log/slog"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type Broker struct {
	srv  *mqtt.Server
	addr string
}

// Start listens on addr (use "127.0.0.1:0" to pick a free port).
func Start(addr string, log *slog.Logger) (*Broker, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	srv := mqtt.New(&mqtt.Options{Logger: log, InlineClient: false})
	if err := srv.AddHook(new(auth.AllowHook), nil); err != nil {
		return nil, err
	}
	ln := listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr})
	if err := srv.AddListener(ln); err != nil {
		return nil, fmt.Errorf("embedded broker cannot listen on %s: %w", addr, err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Error("embedded broker stopped", "err", err)
		}
	}()
	return &Broker{srv: srv, addr: ln.Address()}, nil
}

// Addr is the actual host:port the broker listens on.
func (b *Broker) Addr() string { return b.addr }

// URL is Addr in the form the MQTT clients expect.
func (b *Broker) URL() string { return "tcp://" + b.addr }

// Kick drops a client's TCP connection without a DISCONNECT packet, so the broker publishes its Last Will.
func (b *Broker) Kick(clientID string) bool {
	cl, ok := b.srv.Clients.Get(clientID)
	if !ok {
		return false
	}
	cl.Stop(fmt.Errorf("kicked by test"))
	return true
}

// HasClient reports whether a client with this id is currently connected.
func (b *Broker) HasClient(clientID string) bool {
	cl, ok := b.srv.Clients.Get(clientID)
	return ok && cl.StopCause() == nil && !strings.HasPrefix(clientID, "$")
}

func (b *Broker) Close() { _ = b.srv.Close() }

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

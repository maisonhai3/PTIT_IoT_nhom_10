package mqttx

import "sync"

// Published is one recorded Publish call.
type Published struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

// Fake is an in-memory Client for tests. It never touches the network.
type Fake struct {
	mu        sync.Mutex
	connected bool
	pubErr    error
	handlers  map[string][]Handler
	published []Published
}

func NewFake() *Fake {
	return &Fake{connected: true, handlers: map[string][]Handler{}}
}

func (f *Fake) SetConnected(v bool) { f.mu.Lock(); f.connected = v; f.mu.Unlock() }
func (f *Fake) SetPublishError(err error) {
	f.mu.Lock()
	f.pubErr = err
	f.mu.Unlock()
}

func (f *Fake) Connected() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.connected }
func (f *Fake) Close()          {}

func (f *Fake) Subscribe(topic string, _ byte, h Handler) error {
	f.mu.Lock()
	f.handlers[topic] = append(f.handlers[topic], h)
	f.mu.Unlock()
	return nil
}

func (f *Fake) Publish(topic string, qos byte, retained bool, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.connected {
		return ErrNotConnected
	}
	if f.pubErr != nil {
		return f.pubErr
	}
	f.published = append(f.published, Published{topic, qos, retained, append([]byte(nil), payload...)})
	return nil
}

// Deliver simulates an incoming message: it calls the handlers registered for the exact topic.
func (f *Fake) Deliver(topic string, payload []byte) {
	f.mu.Lock()
	hs := append([]Handler(nil), f.handlers[topic]...)
	f.mu.Unlock()
	for _, h := range hs {
		h(topic, append([]byte(nil), payload...))
	}
}

// Published returns a copy of everything published so far.
func (f *Fake) Published() []Published {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Published(nil), f.published...)
}

// PublishedOn returns the payloads published on one topic, oldest first.
func (f *Fake) PublishedOn(topic string) [][]byte {
	var out [][]byte
	for _, p := range f.Published() {
		if p.Topic == topic {
			out = append(out, p.Payload)
		}
	}
	return out
}

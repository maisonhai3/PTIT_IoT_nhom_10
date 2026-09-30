// Package mqttx is a thin wrapper over the Paho client with the behaviour this project needs:
// non-blocking start (the broker may come up later), automatic re-subscription after every reconnect,
// and an interface small enough to fake in tests.
package mqttx

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

var ErrNotConnected = errors.New("mqtt: not connected")

// Handler receives one message. Handlers run on the client's receive goroutine and MUST NOT block:
// do not call Publish and wait for it from inside a handler (start a goroutine instead).
type Handler func(topic string, payload []byte)

type Client interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
	// Subscribe registers a handler. It is applied immediately if connected and again after every reconnect.
	Subscribe(topic string, qos byte, h Handler) error
	Connected() bool
	Close()
}

type Will struct {
	Topic    string
	Payload  []byte
	QoS      byte
	Retained bool
}

type Options struct {
	URL       string // tcp://host:1883
	ClientID  string
	Username  string
	Password  string
	Will      *Will
	KeepAlive time.Duration
	Log       *slog.Logger
	// OnConnect runs (in its own goroutine) after each successful connection, once subscriptions are re-established.
	OnConnect func()
}

type subscription struct {
	topic string
	qos   byte
	h     Handler
}

type Paho struct {
	c   paho.Client
	log *slog.Logger
	on  func()

	mu   sync.Mutex
	subs []subscription
}

var setLoggerOnce sync.Once

// NewPaho builds the client without connecting: register subscriptions, then call Connect. Splitting the two steps
// means callbacks such as OnConnect can safely refer to the returned client.
func NewPaho(o Options) *Paho {
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	setLoggerOnce.Do(func() { paho.ERROR = pahoLogger{slog.Default()} })

	p := &Paho{log: log, on: o.OnConnect}
	keepAlive := o.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 30 * time.Second
	}
	po := paho.NewClientOptions().
		AddBroker(o.URL).
		SetClientID(o.ClientID).
		SetUsername(o.Username).
		SetPassword(o.Password).
		SetKeepAlive(keepAlive).
		SetPingTimeout(10 * time.Second).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(3 * time.Second).
		SetMaxReconnectInterval(15 * time.Second).
		SetConnectTimeout(5 * time.Second).
		SetWriteTimeout(5 * time.Second)
	if o.Will != nil {
		po.SetBinaryWill(o.Will.Topic, o.Will.Payload, o.Will.QoS, o.Will.Retained)
	}
	po.SetOnConnectHandler(func(paho.Client) {
		log.Info("mqtt connected", "broker", o.URL, "client_id", o.ClientID)
		go p.afterConnect()
	})
	po.SetConnectionLostHandler(func(_ paho.Client, err error) {
		log.Warn("mqtt connection lost", "err", err)
	})
	po.SetReconnectingHandler(func(paho.Client, *paho.ClientOptions) {
		log.Debug("mqtt reconnecting")
	})

	p.c = paho.NewClient(po)
	return p
}

// Connect starts connecting in the background and returns immediately: with ConnectRetry the client keeps trying
// until the broker is reachable, so the process can start before the broker does.
func (p *Paho) Connect() { p.c.Connect() }

func (p *Paho) afterConnect() {
	p.mu.Lock()
	subs := append([]subscription(nil), p.subs...)
	p.mu.Unlock()
	for _, s := range subs {
		p.subscribe(s)
	}
	if p.on != nil {
		p.on()
	}
}

func (p *Paho) subscribe(s subscription) {
	cb := func(_ paho.Client, m paho.Message) {
		payload := append([]byte(nil), m.Payload()...)
		s.h(m.Topic(), payload)
	}
	t := p.c.Subscribe(s.topic, s.qos, cb)
	if !t.WaitTimeout(5 * time.Second) {
		p.log.Error("mqtt subscribe timed out", "topic", s.topic)
		return
	}
	if err := t.Error(); err != nil {
		p.log.Error("mqtt subscribe failed", "topic", s.topic, "err", err)
		return
	}
	if st, ok := t.(*paho.SubscribeToken); ok {
		if q, found := st.Result()[s.topic]; found && q == 0x80 {
			p.log.Error("mqtt subscription refused by broker (check the ACL)", "topic", s.topic)
		}
	}
}

func (p *Paho) Subscribe(topic string, qos byte, h Handler) error {
	s := subscription{topic: topic, qos: qos, h: h}
	p.mu.Lock()
	p.subs = append(p.subs, s)
	p.mu.Unlock()
	if p.c.IsConnectionOpen() {
		go p.subscribe(s)
	}
	return nil
}

func (p *Paho) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if !p.c.IsConnectionOpen() {
		return ErrNotConnected
	}
	t := p.c.Publish(topic, qos, retained, payload)
	if !t.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt: publish to %s timed out", topic)
	}
	return t.Error()
}

func (p *Paho) Connected() bool { return p.c.IsConnectionOpen() }

func (p *Paho) Close() { p.c.Disconnect(250) }

type pahoLogger struct{ l *slog.Logger }

func (p pahoLogger) Println(v ...interface{}) { p.l.Warn(fmt.Sprint(v...), "component", "paho") }
func (p pahoLogger) Printf(format string, v ...interface{}) {
	p.l.Warn(fmt.Sprintf(format, v...), "component", "paho")
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
)

const prefix = "pkg/awning01/"

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type capture struct {
	mu   sync.Mutex
	msgs []hub.Message
}

func (c *capture) Broadcast(m hub.Message) { c.mu.Lock(); c.msgs = append(c.msgs, m); c.mu.Unlock() }
func (c *capture) all() []hub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]hub.Message(nil), c.msgs...)
}
func (c *capture) ofType(t string) []hub.Message {
	var out []hub.Message
	for _, m := range c.all() {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}
func (c *capture) events() []model.Event {
	var out []model.Event
	for _, m := range c.ofType("event") {
		out = append(out, m.Data.(model.Event))
	}
	return out
}

type env struct {
	svc *Service
	mq  *mqttx.Fake
	st  *store.SQLite
	bc  *capture
	clk *clock
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Memory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{mq: mqttx.NewFake(), st: st, bc: &capture{}, clk: &clock{t: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}}
	e.svc = New(Options{
		Prefix: prefix, DeviceTimeout: 20 * time.Second, WeatherPublishInterval: time.Minute,
		HistoryInterval: time.Minute, Now: e.clk.Now,
	}, e.mq, st, e.bc)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := e.svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return e
}

func tjson(state, mode, src string, light int) []byte {
	return []byte(fmt.Sprintf(`{"temp":29.5,"humidity":71,"light":%d,"state":%q,"mode":%q,"rain":false,"rain_source":%q,
		"weather_age_s":45,"fail_safe":false,"manual_left_s":0,"rssi":-58,"uptime_s":100}`, light, state, mode, src))
}

func (e *env) telemetry(state, mode, src string) {
	e.mq.Deliver(prefix+"telemetry", tjson(state, mode, src, 2300))
}

func TestTelemetryBecomesState(t *testing.T) {
	e := newEnv(t)
	if s := e.svc.State(); s.Online || s.Telemetry != nil || s.LastSeen != nil {
		t.Fatalf("fresh service must be offline with no telemetry: %+v", s)
	}
	e.telemetry("OPEN", "AUTO", "none")
	s := e.svc.State()
	if !s.Online || s.Telemetry == nil || s.LastSeen == nil {
		t.Fatalf("state after telemetry: %+v", s)
	}
	if !s.Telemetry.TS.Equal(e.clk.Now()) {
		t.Errorf("ts = %v, want the backend receive time %v", s.Telemetry.TS, e.clk.Now())
	}
	if s.Telemetry.State != model.StateOpen || s.Telemetry.Light != 2300 || *s.Telemetry.Temp != 29.5 {
		t.Errorf("fields lost: %+v", s.Telemetry)
	}
	msgs := e.bc.ofType("state")
	if len(msgs) != 1 || !msgs[0].Data.(model.State).Online {
		t.Fatalf("expected one state broadcast, got %+v", msgs)
	}
}

func TestInvalidTelemetryIsIgnored(t *testing.T) {
	e := newEnv(t)
	bad := map[string]string{
		"malformed json": `{"state":`,
		"bad state":      string(tjson("FLYING", "AUTO", "none", 100)),
		"bad mode":       string(tjson("OPEN", "AUTOMATIC", "none", 100)),
		"bad rain src":   string(tjson("OPEN", "AUTO", "cloud", 100)),
		"light too high": string(tjson("OPEN", "AUTO", "none", 5000)),
		"empty":          ``,
	}
	for name, payload := range bad {
		e.mq.Deliver(prefix+"telemetry", []byte(payload))
		if s := e.svc.State(); s.Online || s.Telemetry != nil {
			t.Fatalf("%s: garbage was accepted: %+v", name, s)
		}
	}
	if len(e.bc.all()) != 0 {
		t.Errorf("garbage produced broadcasts: %+v", e.bc.all())
	}
}

func TestDeviceGoesOfflineWhenTelemetryStops(t *testing.T) {
	e := newEnv(t)
	e.telemetry("OPEN", "AUTO", "none")
	e.clk.Advance(19 * time.Second)
	e.svc.tick()
	if !e.svc.State().Online {
		t.Fatal("went offline too early")
	}
	e.clk.Advance(2 * time.Second) // 21s > 20s timeout
	e.svc.tick()
	s := e.svc.State()
	if s.Online {
		t.Fatal("still online after the timeout")
	}
	if s.Telemetry == nil {
		t.Error("last known telemetry must stay visible while offline")
	}
	ev := e.bc.events()
	if len(ev) < 2 || ev[len(ev)-1].Kind != model.EventOnline || ev[len(ev)-1].Detail != "false" {
		t.Fatalf("expected an online=false event last, got %+v", ev)
	}
	// Ticking again must not repeat the event.
	n := len(e.bc.events())
	e.svc.tick()
	if len(e.bc.events()) != n {
		t.Error("offline event repeated")
	}
	// Coming back.
	e.telemetry("OPEN", "AUTO", "none")
	if !e.svc.State().Online {
		t.Fatal("did not come back online")
	}
	ev = e.bc.events()
	if ev[len(ev)-1].Kind != model.EventOnline || ev[len(ev)-1].Detail != "true" {
		t.Fatalf("expected online=true event, got %+v", ev[len(ev)-1])
	}
}

func TestLastWillMarksOfflineImmediately(t *testing.T) {
	e := newEnv(t)
	e.telemetry("OPEN", "AUTO", "none")
	e.mq.Deliver(prefix+"status", []byte("offline"))
	if e.svc.State().Online {
		t.Fatal("LWT offline ignored")
	}
	last := e.bc.ofType("state")
	if len(last) < 2 || last[len(last)-1].Data.(model.State).Online {
		t.Fatalf("clients not told about the LWT: %+v", last)
	}
	e.telemetry("OPEN", "AUTO", "none") // reconnected device
	if !e.svc.State().Online {
		t.Fatal("telemetry after LWT must bring the device back")
	}
}

func TestEventsOnTransitionsOnly(t *testing.T) {
	e := newEnv(t)
	e.telemetry("OPEN", "AUTO", "none")   // first telemetry: only "online"
	e.telemetry("OPEN", "AUTO", "none")   // no change
	e.telemetry("CLOSING", "AUTO", "api") // state + rain source
	e.telemetry("CLOSED", "MANUAL", "api")
	var got []string
	for _, ev := range e.bc.events() {
		got = append(got, string(ev.Kind)+":"+ev.Detail)
	}
	want := []string{"online:true", "state:CLOSING", "rain:api", "state:CLOSED", "mode:MANUAL"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	persisted, err := e.svc.Events(context.Background(), 20)
	if err != nil || len(persisted) != len(want) {
		t.Fatalf("persisted events: %d (err %v), want %d", len(persisted), err, len(want))
	}
}

func TestHistoryIsThrottledButKeepsTransitions(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 10; i++ { // 10 telemetry messages inside one minute
		e.telemetry("OPEN", "AUTO", "none")
		e.clk.Advance(5 * time.Second)
	}
	h, _ := e.svc.History(context.Background(), 1)
	if len(h) != 1 {
		t.Fatalf("stored %d rows within one interval, want 1", len(h))
	}
	e.telemetry("CLOSING", "AUTO", "api") // a transition is always stored
	h, _ = e.svc.History(context.Background(), 1)
	if len(h) != 2 || h[1].State != model.StateClosing {
		t.Fatalf("transition not stored: %+v", h)
	}
	e.clk.Advance(61 * time.Second)
	e.telemetry("CLOSING", "AUTO", "api")
	if h, _ = e.svc.History(context.Background(), 1); len(h) != 3 {
		t.Fatalf("periodic row missing: %d rows", len(h))
	}
}

func TestSendCommand(t *testing.T) {
	e := newEnv(t)

	if err := e.svc.SendCommand("fly"); !errors.Is(err, ErrInvalidAction) {
		t.Errorf("invalid action: %v", err)
	}
	if err := e.svc.SendCommand(model.ActionClose); !errors.Is(err, ErrDeviceOffline) {
		t.Errorf("offline device: %v", err)
	}
	if n := len(e.mq.Published()); n != 0 {
		t.Fatalf("%d messages published for refused commands", n)
	}

	e.telemetry("OPEN", "AUTO", "none")
	e.mq.SetConnected(false)
	if err := e.svc.SendCommand(model.ActionClose); !errors.Is(err, ErrMQTT) {
		t.Errorf("broker down: %v", err)
	}
	e.mq.SetConnected(true)
	e.mq.SetPublishError(errors.New("boom"))
	if err := e.svc.SendCommand(model.ActionClose); !errors.Is(err, ErrMQTT) {
		t.Errorf("publish failure: %v", err)
	}
	e.mq.SetPublishError(nil)

	if err := e.svc.SendCommand(model.ActionClose); err != nil {
		t.Fatal(err)
	}
	pub := e.mq.Published()
	if len(pub) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub))
	}
	p := pub[0]
	if p.Topic != prefix+"cmd" || p.QoS != 1 || p.Retained || string(p.Payload) != `{"action":"close"}` {
		t.Errorf("wrong publish: %+v payload=%s", p, p.Payload)
	}
	ev := e.bc.events()
	if last := ev[len(ev)-1]; last.Kind != model.EventCommand || last.Detail != "close" {
		t.Errorf("command event = %+v", last)
	}
	for _, a := range []model.Action{model.ActionOpen, model.ActionAuto, model.ActionSimulateRain, model.ActionClearRain} {
		if err := e.svc.SendCommand(a); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
}

func sampleWeather(now time.Time, raining, expected bool) model.Weather {
	return model.Weather{FetchedAt: now, Location: "Hà Nội", TemperatureC: 30, Humidity: 70,
		IsRaining: raining, RainExpected15m: expected, Description: "x", Forecast: []model.ForecastPoint{}}
}

func TestWeatherIsPushedToDeviceWithAge(t *testing.T) {
	e := newEnv(t)
	e.svc.publishWeather()
	if n := len(e.mq.PublishedOn(prefix + "weather")); n != 0 {
		t.Fatalf("published %d weather messages with an empty cache", n)
	}

	e.svc.SetWeather(sampleWeather(e.clk.Now(), false, true))
	msgs := e.mq.PublishedOn(prefix + "weather")
	if len(msgs) != 1 {
		t.Fatalf("SetWeather published %d messages, want 1", len(msgs))
	}
	var raw map[string]any
	json.Unmarshal(msgs[0], &raw)
	if len(raw) != 3 || raw["age_s"] != float64(0) || raw["is_raining"] != false || raw["rain_expected_15m"] != true {
		t.Fatalf("payload = %s", msgs[0])
	}
	pub := e.mq.Published()[0]
	if pub.QoS != 1 || pub.Retained {
		t.Errorf("weather must be QoS1 and NOT retained: %+v", pub)
	}
	if got := e.bc.ofType("weather"); len(got) != 1 {
		t.Errorf("weather not broadcast to websocket clients: %d", len(got))
	}

	// Heartbeat with the same cache: age keeps growing (Open-Meteo unreachable case).
	e.clk.Advance(90 * time.Second)
	e.svc.publishWeather()
	msgs = e.mq.PublishedOn(prefix + "weather")
	json.Unmarshal(msgs[len(msgs)-1], &raw)
	if raw["age_s"] != float64(90) {
		t.Errorf("age_s after 90s = %v, want 90", raw["age_s"])
	}

	// Device (re)connecting triggers an immediate push.
	before := len(e.mq.PublishedOn(prefix + "weather"))
	e.mq.Deliver(prefix+"status", []byte("online"))
	deadline := time.Now().Add(2 * time.Second)
	for len(e.mq.PublishedOn(prefix+"weather")) == before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(e.mq.PublishedOn(prefix+"weather")) == before {
		t.Error("status=online did not trigger a weather push")
	}
}

func TestWeatherAccessorAndErrorEvent(t *testing.T) {
	e := newEnv(t)
	if _, ok := e.svc.Weather(); ok {
		t.Fatal("Weather() reported data before any fetch")
	}
	e.svc.SetWeather(sampleWeather(e.clk.Now(), true, false))
	w, ok := e.svc.Weather()
	if !ok || !w.IsRaining {
		t.Fatalf("Weather() = %+v, %v", w, ok)
	}
	e.svc.RecordWeatherError(errors.New("dial tcp: lookup api.open-meteo.com: no such host"))
	ev := e.bc.events()
	if last := ev[len(ev)-1]; last.Kind != model.EventWeatherError || last.Detail == "" {
		t.Errorf("weather error event = %+v", last)
	}
}

func TestStartSubscribesToDeviceTopics(t *testing.T) {
	e := newEnv(t)
	// Deliver reaches handlers only for topics that were subscribed.
	e.mq.Deliver(prefix+"telemetry", tjson("OPEN", "AUTO", "none", 1))
	if !e.svc.State().Online {
		t.Error("telemetry topic not subscribed")
	}
	e.mq.Deliver(prefix+"status", []byte("offline"))
	if e.svc.State().Online {
		t.Error("status topic not subscribed")
	}
}

func TestConcurrentAccess(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				e.telemetry("OPEN", "AUTO", "none")
				e.svc.tick()
				_ = e.svc.State()
				e.svc.SetWeather(sampleWeather(e.clk.Now(), false, false))
				_ = e.svc.SendCommand(model.ActionAuto)
			}
		}()
	}
	wg.Wait()
}

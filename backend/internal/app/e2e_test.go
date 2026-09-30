package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/config"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/devicesim"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/embedbroker"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/weather"
)

const specPath = "../../../docs/openapi.yaml"

// The whole chain, for real except the hardware: embedded MQTT broker, Paho clients on both sides, the simulated
// ESP32, the backend service, HTTP and WebSocket. Responses are validated against docs/openapi.yaml.
type rig struct {
	t      *testing.T
	broker *embedbroker.Broker
	app    *App
	srv    *httptest.Server
	doc    *openapi3.T
	router routers.Router
}

func newRig(t *testing.T) *rig {
	t.Helper()
	if testing.Short() {
		t.Skip("end-to-end test skipped with -short")
	}
	broker, err := embedbroker.Start("127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)

	env := map[string]string{
		"MQTT_URL": broker.URL(), "DB_PATH": ":memory:", "WEATHER_PUBLISH_INTERVAL": "1s",
		"HISTORY_INTERVAL": "1s", "DEVICE_TIMEOUT": "5s", "WEATHER_POLL_INTERVAL": "1m",
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mq := mqttx.NewPaho(mqttx.Options{URL: broker.URL(), ClientID: "awning-backend"})
	t.Cleanup(mq.Close)
	// The poller fetches once at start (always clear); tests then push weather directly via Service.SetWeather,
	// the same path the poller uses.
	a := New(cfg, Deps{MQTT: mq, Weather: &weather.Scripted{Location: "Test"}, Store: st}, nil)
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	mq.Connect()

	sim := devicesim.New(devicesim.Options{
		BrokerURL: broker.URL(), Prefix: cfg.TopicPrefix, Every: 300 * time.Millisecond, Seed: 1,
		Brain: devicesim.Config{
			RainConfirm: 300 * time.Millisecond, DryConfirm: time.Second, ManualTimeout: 4 * time.Second,
			Travel: 400 * time.Millisecond, WeatherStale: 30 * time.Minute, FirstGrace: 2 * time.Minute,
			HumidityHigh: 85, DarkBelow: 800,
		},
	})
	go sim.Run(ctx)

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return &rig{t: t, broker: broker, app: a, srv: srv, doc: doc, router: router}
}

// call performs a real HTTP request and validates the response against the OpenAPI contract.
func (r *rig) call(method, path, body string) (int, []byte) {
	r.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	specReq, _ := http.NewRequest(method, "http://localhost:8080"+path, nil)
	route, params, err := r.router.FindRoute(specReq)
	if err != nil {
		r.t.Fatalf("%s %s not in openapi.yaml: %v", method, path, err)
	}
	if err := openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: specReq, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}},
		Status: resp.StatusCode, Header: resp.Header, Body: io.NopCloser(bytes.NewReader(b)),
	}); err != nil {
		r.t.Errorf("%s %s -> %d violates the contract: %v\n%s", method, path, resp.StatusCode, err, b)
	}
	return resp.StatusCode, b
}

func (r *rig) state() model.State {
	r.t.Helper()
	code, b := r.call("GET", "/api/state", "")
	if code != 200 {
		r.t.Fatalf("state: %d %s", code, b)
	}
	var s model.State
	if err := json.Unmarshal(b, &s); err != nil {
		r.t.Fatal(err)
	}
	return s
}

func (r *rig) command(action string) {
	r.t.Helper()
	code, b := r.call("POST", "/api/command", `{"action":"`+action+`"}`)
	if code != 202 {
		r.t.Fatalf("command %s: %d %s", action, code, b)
	}
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for: %s", timeout, what)
}

func (r *rig) waitTelemetry(what string, pred func(*model.Telemetry) bool) {
	r.t.Helper()
	waitFor(r.t, what, 10*time.Second, func() bool {
		s := r.state()
		return s.Online && s.Telemetry != nil && pred(s.Telemetry)
	})
}

func (r *rig) setWeather(raining, expected bool) {
	w := model.Weather{FetchedAt: time.Now().UTC().Truncate(time.Second), Location: "Test", TemperatureC: 28, Humidity: 80,
		IsRaining: raining, RainExpected15m: expected, WeatherCode: 3, Description: "Nhiều mây", Forecast: []model.ForecastPoint{}}
	r.app.Service.SetWeather(w)
}

type wsLog struct {
	mu   sync.Mutex
	msgs []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
}

func (l *wsLog) has(typ, substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.msgs {
		if m.Type == typ && strings.Contains(string(m.Data), substr) {
			return true
		}
	}
	return false
}

func TestEndToEnd(t *testing.T) {
	r := newRig(t)

	// 1. The simulated device shows up online with telemetry that satisfies the contract.
	r.waitTelemetry("device online", func(*model.Telemetry) bool { return true })
	s := r.state()
	if s.Telemetry.State != model.StateOpen || s.Telemetry.Mode != model.ModeAuto || s.LastSeen == nil {
		t.Fatalf("initial state: %+v", s.Telemetry)
	}

	// 2. Weather from the poller is served and was pushed to the device (age_s accounting).
	waitFor(t, "weather available", 5*time.Second, func() bool { code, _ := r.call("GET", "/api/weather", ""); return code == 200 })
	r.waitTelemetry("device received weather", func(tl *model.Telemetry) bool { return tl.WeatherAgeS >= 0 })

	// 3. WebSocket: snapshot first, then live updates.
	wctx, wcancel := context.WithCancel(context.Background())
	defer wcancel()
	conn, _, err := websocket.Dial(wctx, "ws"+strings.TrimPrefix(r.srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	log := &wsLog{}
	go func() {
		for {
			_, b, err := conn.Read(wctx)
			if err != nil {
				return
			}
			var m struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(b, &m) == nil {
				log.mu.Lock()
				log.msgs = append(log.msgs, m)
				log.mu.Unlock()
			}
		}
	}()
	waitFor(t, "ws snapshot (state + weather)", 3*time.Second, func() bool { return log.has("state", `"online":true`) && log.has("weather", "Test") })

	// 4. Manual close from the web reaches the device and comes back as state changes.
	r.command("close")
	r.waitTelemetry("closed manually", func(tl *model.Telemetry) bool {
		return tl.State == model.StateClosed && tl.Mode == model.ModeManual
	})
	waitFor(t, "ws saw CLOSING and CLOSED", 3*time.Second, func() bool { return log.has("state", `"state":"CLOSING"`) && log.has("state", `"state":"CLOSED"`) })
	waitFor(t, "ws saw the state event", 3*time.Second, func() bool { return log.has("event", `"detail":"CLOSED"`) })

	// 5. Back to AUTO, open, then rain from the weather source closes it by itself.
	r.command("open")
	r.waitTelemetry("open manually", func(tl *model.Telemetry) bool { return tl.State == model.StateOpen })
	r.command("auto")
	r.waitTelemetry("auto", func(tl *model.Telemetry) bool { return tl.Mode == model.ModeAuto })

	r.setWeather(true, false)
	r.waitTelemetry("auto-closed by rain", func(tl *model.Telemetry) bool {
		return tl.State == model.StateClosed && tl.Mode == model.ModeAuto && tl.Rain && tl.RainSource == model.RainAPI
	})

	// 6. Rain stops: after the dry confirmation the awning reopens by itself.
	r.setWeather(false, false)
	r.waitTelemetry("auto-reopened", func(tl *model.Telemetry) bool { return tl.State == model.StateOpen && !tl.Rain })

	// 7. Simulated rain via command.
	r.command("simulate_rain")
	r.waitTelemetry("closed by simulated rain", func(tl *model.Telemetry) bool {
		return tl.State == model.StateClosed && tl.RainSource == model.RainSim
	})
	r.command("clear_rain")
	r.waitTelemetry("sim rain cleared", func(tl *model.Telemetry) bool { return tl.RainSource == model.RainNone })

	// 8. Event log and history were recorded.
	code, b := r.call("GET", "/api/events?limit=100", "")
	if code != 200 {
		t.Fatalf("events: %d", code)
	}
	var events []model.Event
	json.Unmarshal(b, &events)
	kinds := map[model.EventKind]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
	}
	for _, k := range []model.EventKind{model.EventOnline, model.EventState, model.EventMode, model.EventCommand, model.EventRain} {
		if !kinds[k] {
			t.Errorf("no %q event recorded; got %+v", k, events)
		}
	}
	code, b = r.call("GET", "/api/history?hours=1", "")
	var hist []model.HistoryPoint
	json.Unmarshal(b, &hist)
	if code != 200 || len(hist) < 3 {
		t.Errorf("history: %d, %d points", code, len(hist))
	}
	sawClosed, sawOpen := false, false
	for _, p := range hist {
		sawClosed = sawClosed || p.State == model.StateClosed
		sawOpen = sawOpen || p.State == model.StateOpen
	}
	if !sawClosed || !sawOpen {
		t.Errorf("history misses state transitions (closed=%v open=%v)", sawClosed, sawOpen)
	}

	// 9. Bad requests keep their contract-conformant error shape.
	if code, _ := r.call("POST", "/api/command", `{"action":"explode"}`); code != 400 {
		t.Errorf("bad action: %d", code)
	}
}

// The broker publishes the device's Last Will when its connection drops without a DISCONNECT.
func TestDeviceDropIsDetectedAndRecovers(t *testing.T) {
	r := newRig(t)
	r.waitTelemetry("device online", func(*model.Telemetry) bool { return true })

	if !r.broker.Kick("esp32-awning01") {
		t.Fatal("device client not found on the broker")
	}
	// Whatever the timing of the reconnect, the outage must be visible in the event log and then heal.
	waitFor(t, "offline event", 8*time.Second, func() bool {
		_, b := r.call("GET", "/api/events?limit=100", "")
		return bytes.Contains(b, []byte(`"kind":"online","detail":"false"`))
	})
	r.waitTelemetry("device back online", func(*model.Telemetry) bool { return true })

	// Commands for an offline device are refused, not silently lost.
	r.broker.Kick("esp32-awning01")
	waitFor(t, "state offline or reconnected", 8*time.Second, func() bool { return !r.state().Online })
	if code, _ := r.call("POST", "/api/command", `{"action":"open"}`); code != http.StatusConflict {
		t.Errorf("command while offline: %d, want 409", code)
	}
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/service"
)

type fakeBackend struct {
	state    model.State
	weather  *model.Weather
	cmdErr   error
	lastCmd  model.Action
	cmdCalls int
	hist     []model.HistoryPoint
	events   []model.Event
	histHrs  int
	evLimit  int
	mqtt     bool
}

func (f *fakeBackend) State() model.State { return f.state }
func (f *fakeBackend) Weather() (model.Weather, bool) {
	if f.weather == nil {
		return model.Weather{}, false
	}
	return *f.weather, true
}
func (f *fakeBackend) SendCommand(a model.Action) error {
	f.cmdCalls++
	f.lastCmd = a
	return f.cmdErr
}
func (f *fakeBackend) History(_ context.Context, hours int) ([]model.HistoryPoint, error) {
	f.histHrs = hours
	return f.hist, nil
}
func (f *fakeBackend) Events(_ context.Context, limit int) ([]model.Event, error) {
	f.evLimit = limit
	return f.events, nil
}
func (f *fakeBackend) MQTTConnected() bool { return f.mqtt }
func (f *fakeBackend) DeviceOnline() bool  { return f.state.Online }

var t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func f64(v float64) *float64 { return &v }
func i32(v int) *int         { return &v }
func yes(v bool) *bool       { return &v }

func onlineState() model.State {
	ts := t0
	return model.State{Online: true, LastSeen: &ts, Telemetry: &model.Telemetry{TS: ts, DeviceTelemetry: model.DeviceTelemetry{
		Temp: f64(29.5), Humidity: f64(71), Light: 2300, RainLevel: i32(30), RainWet: yes(false),
		State: model.StateOpen, Mode: model.ModeAuto, RainSource: model.RainNone, WeatherAgeS: 45, ManualLeftS: 0,
	}}}
}

func newServer(t *testing.T, fb *fakeBackend, o Options) (*httptest.Server, *hub.Hub) {
	t.Helper()
	h := hub.New(nil)
	srv := httptest.NewServer(New(fb, h, o).Handler())
	t.Cleanup(srv.Close)
	return srv, h
}

func do(t *testing.T, method, url, body string, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

var jsonHdr = map[string]string{"Content-Type": "application/json"}

func TestGetStateWithAndWithoutTelemetry(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{})
	resp, body := do(t, "GET", srv.URL+"/api/state", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status/headers: %d %v", resp.StatusCode, resp.Header)
	}
	if strings.TrimSpace(string(body)) != `{"online":false,"last_seen":null,"telemetry":null}` {
		t.Errorf("empty state = %s", body)
	}

	fb.state = onlineState()
	_, body = do(t, "GET", srv.URL+"/api/state", "", nil)
	var got map[string]any
	json.Unmarshal(body, &got)
	tel := got["telemetry"].(map[string]any)
	if got["online"] != true || tel["state"] != "OPEN" || tel["light"] != float64(2300) || tel["ts"] != "2026-09-30T12:00:00Z" {
		t.Errorf("state = %s", body)
	}
}

func TestCommandHappyPathAndErrors(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{})
	url := srv.URL + "/api/command"

	resp, body := do(t, "POST", url, `{"action":"close"}`, jsonHdr)
	if resp.StatusCode != 202 || strings.TrimSpace(string(body)) != `{"status":"sent"}` || fb.lastCmd != model.ActionClose {
		t.Fatalf("happy path: %d %s cmd=%q", resp.StatusCode, body, fb.lastCmd)
	}

	// Mapping of backend errors to status codes.
	for name, c := range map[string]struct {
		err  error
		want int
	}{
		"invalid":  {fmt.Errorf("%w: x", service.ErrInvalidAction), 400},
		"offline":  {service.ErrDeviceOffline, 409},
		"mqtt":     {fmt.Errorf("%w: boom", service.ErrMQTT), 503},
		"internal": {errors.New("weird"), 500},
	} {
		fb.cmdErr = c.err
		resp, body := do(t, "POST", url, `{"action":"open"}`, jsonHdr)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d (%s)", name, resp.StatusCode, c.want, body)
		}
		var e map[string]string
		if json.Unmarshal(body, &e) != nil || e["error"] == "" {
			t.Errorf("%s: error body is not {\"error\": ...}: %s", name, body)
		}
	}
	fb.cmdErr = nil

	before := fb.cmdCalls
	for name, body := range map[string]string{
		"unknown action": `{"action":"fly"}`,
		"not json":       `close`,
		"unknown field":  `{"action":"open","x":1}`,
		"trailing data":  `{"action":"open"} {"action":"close"}`,
		"empty":          ``,
		"array":          `["open"]`,
	} {
		resp, _ := do(t, "POST", url, body, jsonHdr)
		if resp.StatusCode != 400 {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
	if fb.cmdCalls != before {
		t.Error("a malformed body reached the backend")
	}
	resp, _ = do(t, "POST", url, `{"action":"`+strings.Repeat("a", 5000)+`"}`, jsonHdr)
	if resp.StatusCode != 400 {
		t.Errorf("oversized body: status %d, want 400", resp.StatusCode)
	}
}

func TestCommandRejectsNonJSONContentType(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{})
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
		resp, _ := do(t, "POST", srv.URL+"/api/command", `{"action":"open"}`, map[string]string{"Content-Type": ct})
		if resp.StatusCode != 415 {
			t.Errorf("content-type %q: status %d, want 415", ct, resp.StatusCode)
		}
	}
	resp, _ := do(t, "POST", srv.URL+"/api/command", `{"action":"open"}`, map[string]string{"Content-Type": "application/json; charset=utf-8"})
	if resp.StatusCode != 202 {
		t.Errorf("charset parameter must be accepted: %d", resp.StatusCode)
	}
	if fb.cmdCalls != 1 {
		t.Errorf("backend calls = %d, want 1", fb.cmdCalls)
	}
}

func TestCommandOriginPolicy(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{CORSOrigins: []string{"http://localhost:5173"}})
	url := srv.URL + "/api/command"
	post := func(origin string) int {
		h := map[string]string{"Content-Type": "application/json"}
		if origin != "" {
			h["Origin"] = origin
		}
		resp, _ := do(t, "POST", url, `{"action":"auto"}`, h)
		return resp.StatusCode
	}
	if c := post(""); c != 202 {
		t.Errorf("no Origin (curl): %d", c)
	}
	if c := post(srv.URL); c != 202 {
		t.Errorf("same origin: %d", c)
	}
	if c := post("http://localhost:5173"); c != 202 {
		t.Errorf("allowed dev origin: %d", c)
	}
	if c := post("http://evil.example"); c != 403 {
		t.Errorf("foreign origin: %d, want 403", c)
	}
	if fb.cmdCalls != 3 {
		t.Errorf("backend calls = %d, want 3", fb.cmdCalls)
	}
}

func TestCommandToken(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{Token: "s3cret"})
	url := srv.URL + "/api/command"
	try := func(auth string) int {
		h := map[string]string{"Content-Type": "application/json"}
		if auth != "" {
			h["Authorization"] = auth
		}
		resp, _ := do(t, "POST", url, `{"action":"auto"}`, h)
		return resp.StatusCode
	}
	if c := try(""); c != 401 {
		t.Errorf("no token: %d", c)
	}
	if c := try("Bearer wrong"); c != 401 {
		t.Errorf("wrong token: %d", c)
	}
	if c := try("s3cret"); c != 401 {
		t.Errorf("missing Bearer prefix: %d", c)
	}
	if c := try("Bearer s3cret"); c != 202 {
		t.Errorf("right token: %d", c)
	}
	resp, _ := do(t, "POST", url, `{"action":"auto"}`, jsonHdr)
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("401 without WWW-Authenticate")
	}
	// Reads stay open even when a token is configured.
	if resp, _ := do(t, "GET", srv.URL+"/api/state", "", nil); resp.StatusCode != 200 {
		t.Errorf("GET /api/state needs no token: %d", resp.StatusCode)
	}
	if fb.cmdCalls != 1 {
		t.Errorf("backend calls = %d, want 1", fb.cmdCalls)
	}
}

func TestWeatherEndpoint(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{})
	if resp, _ := do(t, "GET", srv.URL+"/api/weather", "", nil); resp.StatusCode != 503 {
		t.Fatalf("no weather yet: %d, want 503", resp.StatusCode)
	}
	fb.weather = &model.Weather{FetchedAt: t0, Location: "Hà Nội", TemperatureC: 30, Description: "Ít mây", Forecast: []model.ForecastPoint{}}
	resp, body := do(t, "GET", srv.URL+"/api/weather", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"description":"Ít mây"`) || !strings.Contains(string(body), `"forecast":[]`) {
		t.Errorf("weather = %d %s", resp.StatusCode, body)
	}
}

func TestHistoryAndEventsParams(t *testing.T) {
	fb := &fakeBackend{}
	srv, _ := newServer(t, fb, Options{})

	resp, body := do(t, "GET", srv.URL+"/api/history", "", nil)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != "[]" || fb.histHrs != 24 {
		t.Errorf("default history: %d %s hours=%d (want [] not null)", resp.StatusCode, body, fb.histHrs)
	}
	do(t, "GET", srv.URL+"/api/history?hours=6", "", nil)
	if fb.histHrs != 6 {
		t.Errorf("hours = %d", fb.histHrs)
	}
	for _, bad := range []string{"0", "169", "abc", "-1", "1.5"} {
		if resp, _ := do(t, "GET", srv.URL+"/api/history?hours="+bad, "", nil); resp.StatusCode != 400 {
			t.Errorf("hours=%s: %d, want 400", bad, resp.StatusCode)
		}
	}

	resp, body = do(t, "GET", srv.URL+"/api/events", "", nil)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != "[]" || fb.evLimit != 50 {
		t.Errorf("default events: %d %s limit=%d", resp.StatusCode, body, fb.evLimit)
	}
	do(t, "GET", srv.URL+"/api/events?limit=5", "", nil)
	if fb.evLimit != 5 {
		t.Errorf("limit = %d", fb.evLimit)
	}
	for _, bad := range []string{"0", "201", "x"} {
		if resp, _ := do(t, "GET", srv.URL+"/api/events?limit="+bad, "", nil); resp.StatusCode != 400 {
			t.Errorf("limit=%s: %d, want 400", bad, resp.StatusCode)
		}
	}
}

func TestHealth(t *testing.T) {
	fb := &fakeBackend{mqtt: true, state: model.State{Online: true}}
	srv, _ := newServer(t, fb, Options{})
	resp, body := do(t, "GET", srv.URL+"/healthz", "", nil)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != `{"device_online":true,"mqtt_connected":true,"status":"ok"}` {
		t.Errorf("health = %d %s", resp.StatusCode, body)
	}
}

func TestUnknownAndWrongMethodAreJSON(t *testing.T) {
	srv, _ := newServer(t, &fakeBackend{}, Options{})
	resp, body := do(t, "GET", srv.URL+"/api/nope", "", nil)
	if resp.StatusCode != 404 || !strings.Contains(string(body), `"error"`) {
		t.Errorf("unknown api path: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, "POST", srv.URL+"/api/state", "", nil)
	if resp.StatusCode != 405 || resp.Header.Get("Allow") != "GET" || !strings.Contains(string(body), `"error"`) {
		t.Errorf("wrong method: %d allow=%q %s", resp.StatusCode, resp.Header.Get("Allow"), body)
	}
	resp, _ = do(t, "GET", srv.URL+"/api/command", "", nil)
	if resp.StatusCode != 405 || resp.Header.Get("Allow") != "POST" {
		t.Errorf("GET on command: %d allow=%q", resp.StatusCode, resp.Header.Get("Allow"))
	}
}

func TestCORS(t *testing.T) {
	srv, _ := newServer(t, &fakeBackend{}, Options{CORSOrigins: []string{"http://localhost:5173"}})
	resp, _ := do(t, "GET", srv.URL+"/api/state", "", map[string]string{"Origin": "http://localhost:5173"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" || resp.Header.Get("Vary") != "Origin" {
		t.Errorf("allowed origin headers: %v", resp.Header)
	}
	if resp.Header.Get("Access-Control-Expose-Headers") != "Date" {
		t.Errorf("Date header must be exposed to cross-origin pages: %v", resp.Header)
	}
	resp, _ = do(t, "GET", srv.URL+"/api/state", "", map[string]string{"Origin": "http://evil.example"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("foreign origin got CORS headers")
	}
	resp, _ = do(t, "OPTIONS", srv.URL+"/api/command", "", map[string]string{
		"Origin": "http://localhost:5173", "Access-Control-Request-Method": "POST",
		"Access-Control-Request-Headers": "content-type,authorization"})
	if resp.StatusCode != 204 || !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = do(t, "OPTIONS", srv.URL+"/api/command", "", map[string]string{
		"Origin": "http://evil.example", "Access-Control-Request-Method": "POST"})
	if resp.StatusCode == 204 || resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("foreign preflight must not be approved: %d %v", resp.StatusCode, resp.Header)
	}

	star, _ := newServer(t, &fakeBackend{}, Options{CORSOrigins: []string{"*"}})
	resp, _ = do(t, "GET", star.URL+"/api/state", "", map[string]string{"Origin": "http://anything.example"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("wildcard: %v", resp.Header)
	}
}

func TestStaticServing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>x</title>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "js"), 0o755)
	os.WriteFile(filepath.Join(dir, "js", "main.js"), []byte("export {}"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("outside"), 0o644)
	t.Cleanup(func() { os.Remove(filepath.Join(filepath.Dir(dir), "secret.txt")) })

	srv, _ := newServer(t, &fakeBackend{}, Options{StaticDir: dir})
	resp, body := do(t, "GET", srv.URL+"/", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "<title>x</title>") {
		t.Fatalf("index: %d %s", resp.StatusCode, body)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP = %q", csp)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("security/cache headers: %v", resp.Header)
	}
	if resp, _ := do(t, "GET", srv.URL+"/js/main.js", "", nil); resp.StatusCode != 200 {
		t.Errorf("asset: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", srv.URL+"/missing.css", "", nil); resp.StatusCode != 404 {
		t.Errorf("missing asset: %d", resp.StatusCode)
	}
	if resp, body := do(t, "GET", srv.URL+"/..%2fsecret.txt", "", nil); resp.StatusCode == 200 && strings.Contains(string(body), "outside") {
		t.Error("path traversal escaped the static dir")
	}
	if resp, _ := do(t, "POST", srv.URL+"/", "", nil); resp.StatusCode != 405 {
		t.Errorf("POST /: %d", resp.StatusCode)
	}
	// API routes keep working next to the static catch-all.
	if resp, _ := do(t, "GET", srv.URL+"/api/state", "", nil); resp.StatusCode != 200 {
		t.Errorf("api next to static: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", srv.URL+"/api/nope", "", nil); resp.StatusCode != 404 || resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("unknown api path next to static: %d %v", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestWebSocketSnapshotAndBroadcast(t *testing.T) {
	fb := &fakeBackend{state: onlineState(), weather: &model.Weather{FetchedAt: t0, Forecast: []model.ForecastPoint{}}}
	srv, h := newServer(t, fb, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	var types []string
	for i := 0; i < 2; i++ {
		_, b, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m struct{ Type string }
		json.Unmarshal(b, &m)
		types = append(types, m.Type)
	}
	if fmt.Sprint(types) != "[state weather]" {
		t.Fatalf("snapshot types = %v", types)
	}
	for h.Len() != 1 {
		time.Sleep(5 * time.Millisecond)
	}
	h.Broadcast(hub.Message{Type: "event", Data: model.Event{TS: t0, Kind: model.EventState, Detail: "CLOSING"}})
	_, b, err := c.Read(ctx)
	if err != nil || !strings.Contains(string(b), `"type":"event"`) || !strings.Contains(string(b), `"detail":"CLOSING"`) {
		t.Fatalf("broadcast frame %s err=%v", b, err)
	}
}

func TestWebSocketForeignOriginRejected(t *testing.T) {
	srv, _ := newServer(t, &fakeBackend{}, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://evil.example"}}})
	if err == nil {
		t.Fatal("cross-origin websocket accepted by default")
	}
}

type panicBackend struct{ fakeBackend }

func (p *panicBackend) State() model.State { panic("boom") }

func TestPanicBecomesJSON500(t *testing.T) {
	srv := httptest.NewServer(New(&panicBackend{}, hub.New(nil), Options{}).Handler())
	defer srv.Close()
	resp, body := do(t, "GET", srv.URL+"/api/state", "", nil)
	if resp.StatusCode != 500 || !strings.Contains(string(body), `"error"`) {
		t.Errorf("panic response: %d %s", resp.StatusCode, body)
	}
	// The server survives.
	if resp, _ := do(t, "GET", srv.URL+"/healthz", "", nil); resp.StatusCode != 200 {
		t.Errorf("server died after panic: %d", resp.StatusCode)
	}
}

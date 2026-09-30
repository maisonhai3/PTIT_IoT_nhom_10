package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/service"
)

// Contract tests: every response the server produces must validate against docs/openapi.yaml, the document the
// front-end is built from. If a handler or model drifts from the contract, these fail.

const specPath = "../../../docs/openapi.yaml"

func loadSpec(t *testing.T) (*openapi3.T, routers.Router) {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml is not a valid OpenAPI document: %v", err)
	}
	r, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return doc, r
}

// check sends the request to the handler and validates the response against the spec.
func check(t *testing.T, router routers.Router, h http.Handler, method, path, body string, hdr map[string]string, wantStatus int) {
	t.Helper()
	// The spec's server is http://localhost:8080; build the request against it so route matching works.
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://localhost:8080"+path, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: status %d, want %d (%s)", method, path, rec.Code, wantStatus, rec.Body.String())
	}

	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is not described in openapi.yaml: %v", method, path, err)
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: req, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		},
		Status: rec.Code,
		Header: rec.Header(),
		Body:   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		t.Errorf("%s %s -> %d violates the contract: %v\nbody: %s", method, path, rec.Code, err, rec.Body.String())
	}
}

func richBackend() *fakeBackend {
	p := 40
	return &fakeBackend{
		mqtt:  true,
		state: onlineState(),
		weather: &model.Weather{
			FetchedAt: t0, Location: "Hà Nội", TemperatureC: 29.4, Humidity: 78, PrecipitationMM: 0.3,
			IsRaining: true, WeatherCode: 61, Description: "Mưa nhẹ", RainExpected15m: true,
			Forecast: []model.ForecastPoint{{Time: t0, PrecipitationProbability: &p, PrecipitationMM: 0.3}, {Time: t0.Add(3600e9)}},
		},
		hist: []model.HistoryPoint{
			{TS: t0, Temp: f64(29), Humidity: f64(70), Light: 100, State: model.StateOpen, Mode: model.ModeAuto, RainSource: model.RainNone},
			{TS: t0.Add(60e9), Temp: nil, Humidity: nil, Light: 4095, State: model.StateClosing, Mode: model.ModeManual, Rain: true, RainSource: model.RainAPI},
		},
		events: []model.Event{
			{TS: t0, Kind: model.EventState, Detail: "CLOSING"}, {TS: t0, Kind: model.EventWeatherError, Detail: "boom"},
		},
	}
}

func TestContractResponses(t *testing.T) {
	_, router := loadSpec(t)
	fb := richBackend()
	h := New(fb, hub.New(nil), Options{Token: "tok"}).Handler()
	auth := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer tok"}

	check(t, router, h, "GET", "/api/state", "", nil, 200)
	check(t, router, h, "GET", "/api/weather", "", nil, 200)
	check(t, router, h, "GET", "/api/history?hours=6", "", nil, 200)
	check(t, router, h, "GET", "/api/events?limit=10", "", nil, 200)
	check(t, router, h, "GET", "/healthz", "", nil, 200)

	check(t, router, h, "POST", "/api/command", `{"action":"close"}`, auth, 202)
	check(t, router, h, "POST", "/api/command", `{"action":"bogus"}`, auth, 400)
	check(t, router, h, "POST", "/api/command", `{"action":"close"}`, map[string]string{"Content-Type": "application/json"}, 401)

	fb.cmdErr = service.ErrDeviceOffline
	check(t, router, h, "POST", "/api/command", `{"action":"open"}`, auth, 409)
	fb.cmdErr = service.ErrMQTT
	check(t, router, h, "POST", "/api/command", `{"action":"open"}`, auth, 503)
}

func TestContractEmptyAndDegradedStates(t *testing.T) {
	_, router := loadSpec(t)
	h := New(&fakeBackend{}, hub.New(nil), Options{}).Handler()
	check(t, router, h, "GET", "/api/state", "", nil, 200)   // online=false, telemetry=null, last_seen=null
	check(t, router, h, "GET", "/api/weather", "", nil, 503) // no data yet
	check(t, router, h, "GET", "/api/history", "", nil, 200) // []
	check(t, router, h, "GET", "/api/events", "", nil, 200)  // []
	check(t, router, h, "GET", "/api/history?hours=0", "", nil, 400)
}

// Every enum in the spec must be accepted by the corresponding Go validator, and vice versa, so the two cannot
// drift apart unnoticed.
func TestContractEnumsMatchGoModel(t *testing.T) {
	doc, _ := loadSpec(t)
	enum := func(name string) []string {
		var out []string
		for _, v := range doc.Components.Schemas[name].Value.Enum {
			out = append(out, v.(string))
		}
		return out
	}
	for _, s := range enum("AwningState") {
		if !model.AwningState(s).Valid() {
			t.Errorf("AwningState %q in spec but invalid in Go", s)
		}
	}
	for _, s := range enum("Mode") {
		if !model.Mode(s).Valid() {
			t.Errorf("Mode %q in spec but invalid in Go", s)
		}
	}
	for _, s := range enum("RainSource") {
		if !model.RainSource(s).Valid() {
			t.Errorf("RainSource %q in spec but invalid in Go", s)
		}
	}
	cmd := doc.Components.Schemas["Command"].Value.Properties["action"].Value.Enum
	for _, v := range cmd {
		if !model.Action(v.(string)).Valid() {
			t.Errorf("Action %q in spec but invalid in Go", v)
		}
	}
	if len(enum("AwningState")) != 5 || len(enum("Mode")) != 2 || len(enum("RainSource")) != 4 || len(cmd) != 5 {
		t.Error("enum sizes differ from the Go model: update model.go and this test together")
	}
	kinds := doc.Components.Schemas["Event"].Value.Properties["kind"].Value.Enum
	valid := map[string]bool{}
	for _, k := range []model.EventKind{model.EventState, model.EventMode, model.EventOnline, model.EventCommand, model.EventRain, model.EventWeatherError} {
		valid[string(k)] = true
	}
	if len(kinds) != len(valid) {
		t.Errorf("event kinds: spec has %d, Go has %d", len(kinds), len(valid))
	}
	for _, k := range kinds {
		if !valid[k.(string)] {
			t.Errorf("event kind %q in spec but unknown to Go", k)
		}
	}
}

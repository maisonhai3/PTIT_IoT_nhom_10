package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func valid() DeviceTelemetry {
	return DeviceTelemetry{Temp: f(30), Humidity: f(60), Light: 1000, State: StateOpen, Mode: ModeAuto,
		RainSource: RainNone, WeatherAgeS: -1}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid telemetry rejected: %v", err)
	}
	nullSensors := valid()
	nullSensors.Temp, nullSensors.Humidity = nil, nil // DHT11 read failure
	if err := nullSensors.Validate(); err != nil {
		t.Errorf("null sensor readings must be accepted: %v", err)
	}
	bad := map[string]func(*DeviceTelemetry){
		"state":        func(d *DeviceTelemetry) { d.State = "FLYING" },
		"mode":         func(d *DeviceTelemetry) { d.Mode = "" },
		"rain_source":  func(d *DeviceTelemetry) { d.RainSource = "cloud" },
		"light high":   func(d *DeviceTelemetry) { d.Light = 4096 },
		"light low":    func(d *DeviceTelemetry) { d.Light = -1 },
		"age":          func(d *DeviceTelemetry) { d.WeatherAgeS = -2 },
		"manual_left":  func(d *DeviceTelemetry) { d.ManualLeftS = -1 },
		"temp":         func(d *DeviceTelemetry) { d.Temp = f(500) },
		"humidity":     func(d *DeviceTelemetry) { d.Humidity = f(101) },
		"humidity low": func(d *DeviceTelemetry) { d.Humidity = f(-1) },
	}
	for name, mutate := range bad {
		d := valid()
		mutate(&d)
		if d.Validate() == nil {
			t.Errorf("%s: invalid telemetry accepted", name)
		}
	}
}

// The device payload and the API payload must share field names; Telemetry embeds DeviceTelemetry and adds ts.
func TestTelemetryJSONShape(t *testing.T) {
	tel := Telemetry{TS: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), DeviceTelemetry: valid()}
	b, err := json.Marshal(tel)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"ts", "temp", "humidity", "light", "state", "mode", "rain", "rain_source", "weather_age_s", "fail_safe", "manual_left_s"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	for _, k := range []string{"rssi", "uptime_s"} {
		if _, ok := m[k]; ok {
			t.Errorf("optional key %q must be omitted when unset: %s", k, b)
		}
	}
	if !strings.Contains(string(b), `"ts":"2026-09-30T12:00:00Z"`) {
		t.Errorf("ts format: %s", b)
	}
}

func TestActionValid(t *testing.T) {
	for _, a := range []Action{ActionOpen, ActionClose, ActionAuto, ActionSimulateRain, ActionClearRain} {
		if !a.Valid() {
			t.Errorf("%q should be valid", a)
		}
	}
	for _, a := range []Action{"", "OPEN", "stop", "open "} {
		if a.Valid() {
			t.Errorf("%q should be invalid", a)
		}
	}
}

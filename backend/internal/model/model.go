// Package model defines the data shared between the MQTT side, the REST/WebSocket API and storage.
// JSON field names match docs/openapi.yaml and docs/mqtt-topics.md exactly.
package model

import (
	"errors"
	"fmt"
	"time"
)

type AwningState string

const (
	StateOpen    AwningState = "OPEN"
	StateClosing AwningState = "CLOSING"
	StateClosed  AwningState = "CLOSED"
	StateOpening AwningState = "OPENING"
	StateError   AwningState = "ERROR"
)

func (s AwningState) Valid() bool {
	switch s {
	case StateOpen, StateClosing, StateClosed, StateOpening, StateError:
		return true
	}
	return false
}

type Mode string

const (
	ModeAuto   Mode = "AUTO"
	ModeManual Mode = "MANUAL"
)

func (m Mode) Valid() bool { return m == ModeAuto || m == ModeManual }

type RainSource string

const (
	RainNone   RainSource = "none"
	RainAPI    RainSource = "api"
	RainSim    RainSource = "sim"
	RainSensor RainSource = "sensor" // the rain plate on the device is wet
	RainLocal  RainSource = "local"
)

func (r RainSource) Valid() bool {
	switch r {
	case RainNone, RainAPI, RainSim, RainSensor, RainLocal:
		return true
	}
	return false
}

// Action is a command sent from the web to the device.
type Action string

const (
	ActionOpen         Action = "open"
	ActionClose        Action = "close"
	ActionAuto         Action = "auto"
	ActionSimulateRain Action = "simulate_rain"
	ActionClearRain    Action = "clear_rain"
)

func (a Action) Valid() bool {
	switch a {
	case ActionOpen, ActionClose, ActionAuto, ActionSimulateRain, ActionClearRain:
		return true
	}
	return false
}

// DeviceTelemetry is the JSON the ESP32 publishes. It carries no timestamp: the backend adds one.
type DeviceTelemetry struct {
	Temp        *float64    `json:"temp"`
	Humidity    *float64    `json:"humidity"`
	Light       int         `json:"light"`
	RainLevel   *int        `json:"rain_level"` // rain plate, 0..4095, high = wet; null without a sensor
	RainWet     *bool       `json:"rain_wet"`   // the device's wet/dry verdict for the plate; null without a sensor
	State       AwningState `json:"state"`
	Mode        Mode        `json:"mode"`
	Rain        bool        `json:"rain"`
	RainSource  RainSource  `json:"rain_source"`
	WeatherAgeS int         `json:"weather_age_s"`
	FailSafe    bool        `json:"fail_safe"`
	ManualLeftS int         `json:"manual_left_s"`
	RSSI        *int        `json:"rssi,omitempty"`
	UptimeS     *int64      `json:"uptime_s,omitempty"`
}

// Validate rejects payloads that would break the API contract (bad enums, absurd values).
func (t DeviceTelemetry) Validate() error {
	if !t.State.Valid() {
		return fmt.Errorf("invalid state %q", t.State)
	}
	if !t.Mode.Valid() {
		return fmt.Errorf("invalid mode %q", t.Mode)
	}
	if !t.RainSource.Valid() {
		return fmt.Errorf("invalid rain_source %q", t.RainSource)
	}
	if t.Light < 0 || t.Light > 4095 {
		return fmt.Errorf("light %d out of range 0..4095", t.Light)
	}
	if t.RainLevel != nil && (*t.RainLevel < 0 || *t.RainLevel > 4095) {
		return fmt.Errorf("rain_level %d out of range 0..4095", *t.RainLevel)
	}
	if t.WeatherAgeS < -1 {
		return fmt.Errorf("weather_age_s %d < -1", t.WeatherAgeS)
	}
	if t.ManualLeftS < 0 {
		return fmt.Errorf("manual_left_s %d < 0", t.ManualLeftS)
	}
	if t.Temp != nil && (*t.Temp < -60 || *t.Temp > 100) {
		return errors.New("temp out of range")
	}
	if t.Humidity != nil && (*t.Humidity < 0 || *t.Humidity > 100) {
		return errors.New("humidity out of range")
	}
	return nil
}

// Telemetry is DeviceTelemetry plus the time the backend received it.
type Telemetry struct {
	TS time.Time `json:"ts"`
	DeviceTelemetry
}

// HistoryPoint is the subset of telemetry that is stored and charted.
type HistoryPoint struct {
	TS         time.Time   `json:"ts"`
	Temp       *float64    `json:"temp"`
	Humidity   *float64    `json:"humidity"`
	Light      int         `json:"light"`
	State      AwningState `json:"state"`
	Mode       Mode        `json:"mode"`
	Rain       bool        `json:"rain"`
	RainSource RainSource  `json:"rain_source"`
}

func (t Telemetry) HistoryPoint() HistoryPoint {
	return HistoryPoint{
		TS: t.TS, Temp: t.Temp, Humidity: t.Humidity, Light: t.Light,
		State: t.State, Mode: t.Mode, Rain: t.Rain, RainSource: t.RainSource,
	}
}

type State struct {
	Online    bool       `json:"online"`
	LastSeen  *time.Time `json:"last_seen"`
	Telemetry *Telemetry `json:"telemetry"`
}

type ForecastPoint struct {
	Time                     time.Time `json:"time"`
	PrecipitationProbability *int      `json:"precipitation_probability"`
	PrecipitationMM          float64   `json:"precipitation_mm"`
}

type Weather struct {
	FetchedAt       time.Time       `json:"fetched_at"`
	Location        string          `json:"location"`
	TemperatureC    float64         `json:"temperature_c"`
	Humidity        float64         `json:"humidity"`
	PrecipitationMM float64         `json:"precipitation_mm"`
	IsRaining       bool            `json:"is_raining"`
	WeatherCode     int             `json:"weather_code"`
	Description     string          `json:"description"`
	RainExpected15m bool            `json:"rain_expected_15m"`
	Forecast        []ForecastPoint `json:"forecast"`
}

type EventKind string

const (
	EventState        EventKind = "state"
	EventMode         EventKind = "mode"
	EventOnline       EventKind = "online"
	EventCommand      EventKind = "command"
	EventRain         EventKind = "rain"
	EventWeatherError EventKind = "weather_error"
)

type Event struct {
	TS     time.Time `json:"ts"`
	Kind   EventKind `json:"kind"`
	Detail string    `json:"detail"`
}

// DeviceWeather is what the backend publishes to the device on the `weather` topic.
type DeviceWeather struct {
	AgeS            int  `json:"age_s"`
	IsRaining       bool `json:"is_raining"`
	RainExpected15m bool `json:"rain_expected_15m"`
}

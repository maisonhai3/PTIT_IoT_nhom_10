// Package service is the heart of the backend: it turns MQTT messages into device state, forwards commands and
// weather to the device, records history and events, and notifies WebSocket clients.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
)

var (
	ErrInvalidAction = errors.New("invalid action")
	ErrDeviceOffline = errors.New("device is offline")
	ErrMQTT          = errors.New("mqtt broker unavailable")
)

const (
	maxHistoryPoints = 1000
	eventRetention   = 30 * 24 * time.Hour
)

type Broadcaster interface{ Broadcast(hub.Message) }

type Options struct {
	Prefix                 string // topic prefix, ends with "/"
	DeviceTimeout          time.Duration
	WeatherPublishInterval time.Duration
	HistoryInterval        time.Duration
	HistoryRetention       time.Duration
	Now                    func() time.Time
	Log                    *slog.Logger
}

type Service struct {
	o  Options
	mq mqttx.Client
	st store.Store
	bc Broadcaster

	mu              sync.Mutex
	telemetry       *model.Telemetry // immutable once stored
	lastSeen        time.Time
	explicitOffline bool // the broker published the device's Last Will
	reportedOnline  bool // last online value announced to clients
	weather         *model.Weather
	lastStoredAt    time.Time
	badMessages     int
}

func New(o Options, mq mqttx.Client, st store.Store, bc Broadcaster) *Service {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.DeviceTimeout <= 0 {
		o.DeviceTimeout = 20 * time.Second
	}
	if o.WeatherPublishInterval <= 0 {
		o.WeatherPublishInterval = time.Minute
	}
	if o.HistoryInterval <= 0 {
		o.HistoryInterval = time.Minute
	}
	if o.HistoryRetention <= 0 {
		o.HistoryRetention = 7 * 24 * time.Hour
	}
	return &Service{o: o, mq: mq, st: st, bc: bc}
}

func (s *Service) now() time.Time           { return s.o.Now().UTC().Truncate(time.Millisecond) }
func (s *Service) topic(name string) string { return s.o.Prefix + name }

// Start subscribes to the device topics and runs the background loops until ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	if err := s.mq.Subscribe(s.topic("telemetry"), 0, s.handleTelemetry); err != nil {
		return err
	}
	if err := s.mq.Subscribe(s.topic("status"), 1, s.handleStatus); err != nil {
		return err
	}
	go s.loop(ctx)
	return nil
}

func (s *Service) loop(ctx context.Context) {
	watchdog := time.NewTicker(time.Second)
	heartbeat := time.NewTicker(s.o.WeatherPublishInterval)
	prune := time.NewTicker(time.Hour)
	defer watchdog.Stop()
	defer heartbeat.Stop()
	defer prune.Stop()
	s.pruneOld() // also clears anything left over from a previous run
	for {
		select {
		case <-ctx.Done():
			return
		case <-watchdog.C:
			s.tick()
		case <-heartbeat.C:
			s.publishWeather()
		case <-prune.C:
			s.pruneOld()
		}
	}
}

// ---- device -> backend ----

func (s *Service) handleTelemetry(_ string, payload []byte) {
	var dt model.DeviceTelemetry
	if err := json.Unmarshal(payload, &dt); err != nil {
		s.dropBad("telemetry", err)
		return
	}
	if err := dt.Validate(); err != nil {
		s.dropBad("telemetry", err)
		return
	}
	now := s.now()
	t := model.Telemetry{TS: now, DeviceTelemetry: dt}

	s.mu.Lock()
	prev := s.telemetry
	s.telemetry = &t
	s.lastSeen = now
	s.explicitOffline = false // telemetry after a Last Will means the device reconnected
	_, onlineChanged := s.evalOnlineLocked(now)
	persist := prev == nil || now.Sub(s.lastStoredAt) >= s.o.HistoryInterval ||
		prev.State != t.State || prev.Mode != t.Mode || prev.RainSource != t.RainSource
	if persist {
		s.lastStoredAt = now
	}
	state := s.stateLocked(now)
	s.mu.Unlock()

	if persist {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := s.st.InsertTelemetry(ctx, t); err != nil {
			s.o.Log.Error("store telemetry failed", "err", err)
		}
		cancel()
	}
	if onlineChanged {
		s.emit(model.EventOnline, "true")
	}
	if prev != nil {
		if prev.State != t.State {
			s.emit(model.EventState, string(t.State))
		}
		if prev.Mode != t.Mode {
			s.emit(model.EventMode, string(t.Mode))
		}
		if prev.RainSource != t.RainSource {
			s.emit(model.EventRain, string(t.RainSource))
		}
	}
	s.bc.Broadcast(hub.Message{Type: "state", Data: state})
}

func (s *Service) handleStatus(_ string, payload []byte) {
	switch strings.TrimSpace(string(payload)) {
	case "online":
		// The device just (re)connected: give it weather right away instead of making it wait for the heartbeat.
		go s.publishWeather()
	case "offline":
		now := s.now()
		s.mu.Lock()
		s.explicitOffline = true
		_, changed := s.evalOnlineLocked(now)
		state := s.stateLocked(now)
		s.mu.Unlock()
		if changed {
			s.emit(model.EventOnline, "false")
			s.bc.Broadcast(hub.Message{Type: "state", Data: state})
		}
	default:
		s.dropBad("status", fmt.Errorf("unexpected payload %q", payload))
	}
}

// dropBad logs a rejected message; only the first few are logged so a misbehaving device cannot flood the log.
func (s *Service) dropBad(topic string, err error) {
	s.mu.Lock()
	s.badMessages++
	n := s.badMessages
	s.mu.Unlock()
	if n <= 5 || n%100 == 0 {
		s.o.Log.Warn("ignoring invalid message", "topic", topic, "err", err, "count", n)
	}
}

// tick applies time-based transitions: a device that stops sending telemetry goes offline.
func (s *Service) tick() {
	now := s.now()
	s.mu.Lock()
	_, changed := s.evalOnlineLocked(now)
	state := s.stateLocked(now)
	s.mu.Unlock()
	if changed {
		s.emit(model.EventOnline, fmt.Sprint(state.Online))
		s.bc.Broadcast(hub.Message{Type: "state", Data: state})
	}
}

func (s *Service) onlineLocked(now time.Time) bool {
	return !s.explicitOffline && s.telemetry != nil && now.Sub(s.lastSeen) < s.o.DeviceTimeout
}

// evalOnlineLocked returns the current online flag and whether it differs from what clients were last told.
func (s *Service) evalOnlineLocked(now time.Time) (online, changed bool) {
	online = s.onlineLocked(now)
	if online != s.reportedOnline {
		s.reportedOnline = online
		return online, true
	}
	return online, false
}

func (s *Service) stateLocked(now time.Time) model.State {
	st := model.State{Online: s.onlineLocked(now)}
	if s.telemetry != nil {
		t := *s.telemetry
		ls := s.lastSeen
		st.Telemetry, st.LastSeen = &t, &ls
	}
	return st
}

// ---- reads for the API ----

func (s *Service) State() model.State {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(now)
}

func (s *Service) DeviceOnline() bool { return s.State().Online }

func (s *Service) MQTTConnected() bool { return s.mq.Connected() }

func (s *Service) Weather() (model.Weather, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.weather == nil {
		return model.Weather{}, false
	}
	return *s.weather, true
}

func (s *Service) History(ctx context.Context, hours int) ([]model.HistoryPoint, error) {
	return s.st.History(ctx, s.now().Add(-time.Duration(hours)*time.Hour), maxHistoryPoints)
}

func (s *Service) Events(ctx context.Context, limit int) ([]model.Event, error) {
	return s.st.Events(ctx, limit)
}

// ---- backend -> device ----

// SendCommand validates the action and publishes it. Commands are not retained, so an offline device is refused
// explicitly rather than silently losing the command.
func (s *Service) SendCommand(a model.Action) error {
	if !a.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidAction, a)
	}
	if !s.mq.Connected() {
		return ErrMQTT
	}
	now := s.now()
	s.mu.Lock()
	online := s.onlineLocked(now)
	s.mu.Unlock()
	if !online {
		return ErrDeviceOffline
	}
	b, _ := json.Marshal(struct {
		Action model.Action `json:"action"`
	}{a})
	if err := s.mq.Publish(s.topic("cmd"), 1, false, b); err != nil {
		return fmt.Errorf("%w: %v", ErrMQTT, err)
	}
	s.emit(model.EventCommand, string(a))
	return nil
}

// SetWeather stores fresh weather, notifies clients and pushes it to the device.
func (s *Service) SetWeather(w model.Weather) {
	s.mu.Lock()
	s.weather = &w
	s.mu.Unlock()
	s.bc.Broadcast(hub.Message{Type: "weather", Data: w})
	s.publishWeather()
}

func (s *Service) RecordWeatherError(err error) {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	s.emit(model.EventWeatherError, msg)
}

// publishWeather sends the cached weather to the device. age_s lets the device compute how old the data is
// without a wall clock; if Open-Meteo is unreachable the same cache is re-sent with a growing age.
func (s *Service) publishWeather() {
	s.mu.Lock()
	w := s.weather
	s.mu.Unlock()
	if w == nil {
		return
	}
	age := int(s.now().Sub(w.FetchedAt) / time.Second)
	if age < 0 {
		age = 0
	}
	b, _ := json.Marshal(model.DeviceWeather{AgeS: age, IsRaining: w.IsRaining, RainExpected15m: w.RainExpected15m})
	if err := s.mq.Publish(s.topic("weather"), 1, false, b); err != nil {
		lvl := slog.LevelWarn
		if errors.Is(err, mqttx.ErrNotConnected) {
			lvl = slog.LevelDebug // normal right after start; the heartbeat and the device's "online" resend it
		}
		s.o.Log.Log(context.Background(), lvl, "cannot publish weather to device", "err", err)
	}
}

func (s *Service) emit(kind model.EventKind, detail string) {
	e := model.Event{TS: s.now(), Kind: kind, Detail: detail}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.st.InsertEvent(ctx, e); err != nil {
		s.o.Log.Error("store event failed", "err", err)
	}
	s.bc.Broadcast(hub.Message{Type: "event", Data: e})
}

func (s *Service) pruneOld() {
	now := s.now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.st.Prune(ctx, now.Add(-s.o.HistoryRetention), now.Add(-eventRetention)); err != nil {
		s.o.Log.Error("prune failed", "err", err)
	}
}

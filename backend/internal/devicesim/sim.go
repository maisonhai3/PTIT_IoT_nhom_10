package devicesim

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
)

type Options struct {
	BrokerURL string
	Username  string
	Password  string
	Prefix    string // topic prefix, ends with "/"
	ClientID  string
	Brain     Config
	// Every is the telemetry period (the real device uses 5s).
	Every time.Duration
	Seed  int64
	Now   func() time.Time
	Log   *slog.Logger
}

// Sim connects a Brain to an MQTT broker exactly like the firmware does: Last Will, retained online status,
// periodic telemetry, immediate telemetry on change, `cmd` and `weather` subscriptions.
type Sim struct {
	o     Options
	log   *slog.Logger
	start time.Time

	mu    sync.Mutex
	brain *Brain
	env   *Environment
}

func New(o Options) *Sim {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.ClientID == "" {
		o.ClientID = "esp32-awning01"
	}
	if o.Prefix == "" {
		o.Prefix = "pkg/awning01/"
	}
	if o.Every <= 0 {
		o.Every = 5 * time.Second
	}
	if o.Brain.Travel == 0 {
		o.Brain = DefaultConfig()
	}
	now := o.Now()
	return &Sim{o: o, log: o.Log, start: now, brain: NewBrain(o.Brain, now), env: NewEnvironment(o.Seed)}
}

func (s *Sim) topic(name string) string { return s.o.Prefix + name }

// Run blocks until ctx is cancelled.
func (s *Sim) Run(ctx context.Context) error {
	var mq *mqttx.Paho
	mq = mqttx.NewPaho(mqttx.Options{
		URL: s.o.BrokerURL, ClientID: s.o.ClientID, Username: s.o.Username, Password: s.o.Password,
		Will: &mqttx.Will{Topic: s.topic("status"), Payload: []byte("offline"), QoS: 1, Retained: true},
		Log:  s.log,
		OnConnect: func() {
			if err := mq.Publish(s.topic("status"), 1, true, []byte("online")); err != nil {
				s.log.Warn("sim: cannot publish online status", "err", err)
			}
			s.publishTelemetry(mq)
		},
	})
	defer mq.Close()
	_ = mq.Subscribe(s.topic("cmd"), 1, s.onCommand)
	_ = mq.Subscribe(s.topic("weather"), 1, s.onWeather)
	mq.Connect()

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	nextPublish := s.o.Now().Add(s.o.Every)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		now := s.o.Now()
		s.mu.Lock()
		sensors := s.sensorsLocked(now)
		s.brain.Step(now, sensors)
		changed := s.brain.Changed(now, sensors)
		s.mu.Unlock()
		if changed || !now.Before(nextPublish) {
			s.publishTelemetry(mq)
			nextPublish = now.Add(s.o.Every)
		}
	}
}

func (s *Sim) sensorsLocked(now time.Time) Sensors {
	return s.env.Sample(now, Conditions{Rainy: s.brain.EnvRaining(), PlateWet: s.brain.WorldRaining()})
}

func (s *Sim) publishTelemetry(mq mqttx.Client) {
	now := s.o.Now()
	s.mu.Lock()
	sensors := s.sensorsLocked(now)
	t := s.brain.Telemetry(now, sensors, -55, now.Sub(s.start))
	s.mu.Unlock()
	b, err := json.Marshal(t)
	if err != nil {
		return
	}
	if err := mq.Publish(s.topic("telemetry"), 0, false, b); err != nil {
		s.log.Debug("sim: telemetry not published", "err", err)
	}
}

func (s *Sim) onCommand(_ string, payload []byte) {
	var c struct {
		Action model.Action `json:"action"`
	}
	if err := json.Unmarshal(payload, &c); err != nil || !c.Action.Valid() {
		s.log.Warn("sim: ignoring bad command", "payload", strings.TrimSpace(string(payload)))
		return
	}
	s.log.Info("sim: command", "action", c.Action)
	s.mu.Lock()
	s.brain.OnCommand(s.o.Now(), c.Action)
	s.mu.Unlock()
}

func (s *Sim) onWeather(_ string, payload []byte) {
	var w model.DeviceWeather
	if err := json.Unmarshal(payload, &w); err != nil || w.AgeS < 0 {
		s.log.Warn("sim: ignoring bad weather", "payload", strings.TrimSpace(string(payload)))
		return
	}
	s.mu.Lock()
	s.brain.OnWeather(s.o.Now(), w.IsRaining, w.RainExpected15m, time.Duration(w.AgeS)*time.Second)
	s.mu.Unlock()
}

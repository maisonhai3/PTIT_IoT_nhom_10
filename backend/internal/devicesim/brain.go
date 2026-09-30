// Package devicesim simulates the ESP32 firmware on the MQTT side: same topics, same payloads, same rules
// (rain confirmation, manual override, fail-safe). It lets the backend and the front-end be developed and tested
// without hardware. The real logic lives in firmware/lib/awning_core; this is a behavioural stand-in, not a port.
package devicesim

import (
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

// Config holds the timers. Defaults match docs/mqtt-topics.md; Fast shortens them for demos and tests.
type Config struct {
	RainConfirm   time.Duration
	DryConfirm    time.Duration
	ManualTimeout time.Duration
	Travel        time.Duration // simulated time to move the awning from one end to the other
	WeatherStale  time.Duration
	FirstGrace    time.Duration
	HumidityHigh  float64
	DarkBelow     int
}

func DefaultConfig() Config {
	return Config{
		RainConfirm: 30 * time.Second, DryConfirm: 15 * time.Minute, ManualTimeout: 10 * time.Minute,
		Travel: 8 * time.Second, WeatherStale: 30 * time.Minute, FirstGrace: 2 * time.Minute,
		HumidityHigh: 85, DarkBelow: 800,
	}
}

func FastConfig() Config {
	c := DefaultConfig()
	c.RainConfirm, c.DryConfirm, c.ManualTimeout, c.Travel = 3*time.Second, 20*time.Second, 60*time.Second, 4*time.Second
	return c
}

// Sensors are the local readings used by the fail-safe rule and reported in telemetry.
type Sensors struct {
	Temp, Humidity float64
	Light          int // 0..4095, high = bright
}

type wetness int

const (
	unknown wetness = iota
	dry
	wet
)

type snapshotKey struct {
	state    model.AwningState
	mode     model.Mode
	rain     bool
	src      model.RainSource
	failSafe bool
}

// Brain is the pure decision logic; the caller supplies the time, so it is deterministic to test.
type Brain struct {
	cfg  Config
	boot time.Time

	state       model.AwningState
	mode        model.Mode
	manualSince time.Time
	simRain     bool
	motionStart time.Time

	haveWeather bool
	raining     bool
	expected    bool
	ageAtRx     time.Duration
	rxAt        time.Time

	wetActive, dryActive bool
	wetSince, drySince   time.Time

	last snapshotKey
}

// NewBrain starts with the awning open (there are no limit switches to home against in the simulation).
func NewBrain(cfg Config, now time.Time) *Brain {
	b := &Brain{cfg: cfg, boot: now, state: model.StateOpen, mode: model.ModeAuto}
	b.last = snapshotKey{state: b.state, mode: b.mode, src: model.RainNone}
	return b
}

// OnWeather records a `weather` message; age is the data age reported by the backend at publish time.
func (b *Brain) OnWeather(now time.Time, isRaining, expected15m bool, age time.Duration) {
	b.haveWeather, b.raining, b.expected, b.ageAtRx, b.rxAt = true, isRaining, expected15m, age, now
}

func (b *Brain) OnCommand(now time.Time, a model.Action) {
	switch a {
	case model.ActionOpen:
		b.mode, b.manualSince = model.ModeManual, now
		b.startMotion(now, model.StateOpening)
	case model.ActionClose:
		b.mode, b.manualSince = model.ModeManual, now
		b.startMotion(now, model.StateClosing)
	case model.ActionAuto:
		b.mode = model.ModeAuto
	case model.ActionSimulateRain:
		b.simRain = true
	case model.ActionClearRain:
		b.simRain = false
	}
}

// startMotion begins moving toward the target unless already there (or already moving that way).
func (b *Brain) startMotion(now time.Time, moving model.AwningState) {
	atEnd := (moving == model.StateClosing && b.state == model.StateClosed) ||
		(moving == model.StateOpening && b.state == model.StateOpen)
	if atEnd || b.state == moving {
		return
	}
	b.state, b.motionStart = moving, now
}

func (b *Brain) weatherAge(now time.Time) (time.Duration, bool) {
	if !b.haveWeather {
		return 0, false
	}
	return b.ageAtRx + now.Sub(b.rxAt), true
}

// classify decides WET / DRY / UNKNOWN and where the verdict comes from (see docs/mqtt-topics.md).
func (b *Brain) classify(now time.Time, s Sensors) (w wetness, src model.RainSource, failSafe bool) {
	age, known := b.weatherAge(now)
	stale := (known && age > b.cfg.WeatherStale) || (!known && now.Sub(b.boot) > b.cfg.FirstGrace)
	switch {
	case b.simRain:
		return wet, model.RainSim, stale
	case known && !stale:
		if b.raining || b.expected {
			return wet, model.RainAPI, false
		}
		return dry, model.RainNone, false
	case stale:
		if s.Humidity >= b.cfg.HumidityHigh && s.Light < b.cfg.DarkBelow {
			return wet, model.RainLocal, true
		}
		return unknown, model.RainNone, true
	}
	return unknown, model.RainNone, false // no weather yet, still inside the grace period
}

// Step advances the simulation to now.
func (b *Brain) Step(now time.Time, s Sensors) {
	w, src, _ := b.classify(now, s)
	switch w {
	case wet:
		if !b.wetActive {
			b.wetActive, b.wetSince = true, now
		}
		b.dryActive = false
	case dry:
		if !b.dryActive {
			b.dryActive, b.drySince = true, now
		}
		b.wetActive = false
	default:
		b.wetActive, b.dryActive = false, false
	}
	confirmedWet := b.wetActive && (src == model.RainSim || now.Sub(b.wetSince) >= b.cfg.RainConfirm)
	confirmedDry := b.dryActive && now.Sub(b.drySince) >= b.cfg.DryConfirm

	if b.mode == model.ModeManual && now.Sub(b.manualSince) >= b.cfg.ManualTimeout {
		b.mode = model.ModeAuto
	}
	if b.mode == model.ModeAuto && b.state != model.StateError {
		if confirmedWet && (b.state == model.StateOpen || b.state == model.StateOpening) {
			b.startMotion(now, model.StateClosing)
		} else if confirmedDry && (b.state == model.StateClosed || b.state == model.StateClosing) {
			b.startMotion(now, model.StateOpening)
		}
	}
	if (b.state == model.StateClosing || b.state == model.StateOpening) && now.Sub(b.motionStart) >= b.cfg.Travel {
		if b.state == model.StateClosing {
			b.state = model.StateClosed
		} else {
			b.state = model.StateOpen
		}
	}
}

// EnvRaining tells the synthetic environment whether it is actually raining (not merely forecast).
func (b *Brain) EnvRaining() bool { return b.simRain || (b.haveWeather && b.raining) }

// Telemetry builds the payload the real device would publish.
func (b *Brain) Telemetry(now time.Time, s Sensors, rssi int, uptime time.Duration) model.DeviceTelemetry {
	w, src, failSafe := b.classify(now, s)
	age := -1
	if a, ok := b.weatherAge(now); ok {
		age = int(a / time.Second)
	}
	manualLeft := 0
	if b.mode == model.ModeManual {
		if left := b.cfg.ManualTimeout - now.Sub(b.manualSince); left > 0 {
			manualLeft = int((left + time.Second - 1) / time.Second)
		}
	}
	temp, hum := s.Temp, s.Humidity
	up := int64(uptime / time.Second)
	return model.DeviceTelemetry{
		Temp: &temp, Humidity: &hum, Light: s.Light,
		State: b.state, Mode: b.mode, Rain: w == wet, RainSource: src,
		WeatherAgeS: age, FailSafe: failSafe, ManualLeftS: manualLeft, RSSI: &rssi, UptimeS: &up,
	}
}

// Changed reports (once) whether state, mode, rain verdict, rain source or fail-safe changed since the last call,
// which is when the real device publishes immediately instead of waiting for the periodic tick.
func (b *Brain) Changed(now time.Time, s Sensors) bool {
	w, src, failSafe := b.classify(now, s)
	k := snapshotKey{state: b.state, mode: b.mode, rain: w == wet, src: src, failSafe: failSafe}
	if k == b.last {
		return false
	}
	b.last = k
	return true
}

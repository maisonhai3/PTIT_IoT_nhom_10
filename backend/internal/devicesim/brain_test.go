package devicesim

import (
	"testing"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

// rig drives a Brain with virtual time, calling Step every 100ms like the real 20ms loop would (coarser is fine).
type rig struct {
	t   *testing.T
	b   *Brain
	now time.Time
	s   Sensors
}

func newRig(t *testing.T, cfg Config) *rig {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return &rig{t: t, b: NewBrain(cfg, now), now: now, s: Sensors{Temp: 30, Humidity: 60, Light: 3000}}
}

func (r *rig) run(d time.Duration) {
	end := r.now.Add(d)
	for r.now.Before(end) {
		r.now = r.now.Add(100 * time.Millisecond)
		r.b.Step(r.now, r.s)
	}
}

func (r *rig) tel() model.DeviceTelemetry { return r.b.Telemetry(r.now, r.s, -50, time.Minute) }

func (r *rig) want(state model.AwningState) {
	r.t.Helper()
	if got := r.tel().State; got != state {
		r.t.Fatalf("state = %s, want %s (t=%v)", got, state, r.now.Sub(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)))
	}
}

func (r *rig) weather(raining, expected bool) { r.b.OnWeather(r.now, raining, expected, 0) }

// plate puts a reading on the rain plate; until it is called the rig simulates a device without one.
func (r *rig) plate(level int) { r.s.RainValid, r.s.RainLevel = true, level }

func TestBootsOpenAndAutoWithNoRainVerdict(t *testing.T) {
	r := newRig(t, FastConfig())
	tel := r.tel()
	if tel.State != model.StateOpen || tel.Mode != model.ModeAuto || tel.Rain || tel.RainSource != model.RainNone || tel.WeatherAgeS != -1 {
		t.Fatalf("boot telemetry: %+v", tel)
	}
	if tel.Temp == nil || tel.Humidity == nil || tel.RSSI == nil || tel.UptimeS == nil {
		t.Errorf("optional fields must be filled: %+v", tel)
	}
	if err := tel.Validate(); err != nil {
		t.Errorf("simulated telemetry violates the contract: %v", err)
	}
}

func TestRainClosesAfterConfirmThenDryReopens(t *testing.T) {
	cfg := FastConfig() // rain 3s, dry 20s, travel 4s
	r := newRig(t, cfg)
	r.weather(true, false)
	r.run(2 * time.Second)
	r.want(model.StateOpen) // not confirmed yet
	r.run(2 * time.Second)  // 4s > 3s
	r.want(model.StateClosing)
	if tel := r.tel(); !tel.Rain || tel.RainSource != model.RainAPI {
		t.Errorf("rain verdict = %v/%s", tel.Rain, tel.RainSource)
	}
	r.run(5 * time.Second)
	r.want(model.StateClosed)

	r.weather(false, false)
	r.run(19 * time.Second)
	r.want(model.StateClosed)
	r.run(2 * time.Second) // 21s > 20s dry confirm
	r.want(model.StateOpening)
	r.run(5 * time.Second)
	r.want(model.StateOpen)
}

func TestRainFlickerResetsConfirmation(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(true, false)
	r.run(2 * time.Second)
	r.weather(false, false) // stops before 3s
	r.run(1 * time.Second)
	r.weather(true, false)
	r.run(2 * time.Second) // only 2s of continuous rain again
	r.want(model.StateOpen)
	r.run(2 * time.Second)
	r.want(model.StateClosing)
}

func TestRainExpectedAlsoCloses(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(false, true)
	r.run(4 * time.Second)
	r.want(model.StateClosing)
}

func TestSimulatedRainActsImmediately(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(false, false)
	r.b.OnCommand(r.now, model.ActionSimulateRain)
	r.run(200 * time.Millisecond)
	r.want(model.StateClosing)
	if tel := r.tel(); tel.RainSource != model.RainSim || !tel.Rain {
		t.Errorf("sim verdict: %+v", tel)
	}
	r.b.OnCommand(r.now, model.ActionClearRain)
	if tel := r.tel(); tel.Rain || tel.RainSource != model.RainNone {
		t.Errorf("after clear_rain: %+v", tel)
	}
}

func TestManualCommandsAndTimeoutBackToAuto(t *testing.T) {
	cfg := FastConfig() // manual timeout 60s
	r := newRig(t, cfg)
	r.weather(false, false)

	r.b.OnCommand(r.now, model.ActionClose)
	r.run(500 * time.Millisecond)
	tel := r.tel()
	if tel.State != model.StateClosing || tel.Mode != model.ModeManual || tel.ManualLeftS < 59 || tel.ManualLeftS > 60 {
		t.Fatalf("after close: %+v", tel)
	}
	r.run(5 * time.Second)
	r.want(model.StateClosed)

	r.run(30 * time.Second)
	if left := r.tel().ManualLeftS; left < 24 || left > 26 {
		t.Errorf("manual_left_s after ~35s = %d, want ~25", left)
	}
	r.run(26 * time.Second) // ~61.5s in total: manual expired at 60s; with dry weather for >20s AUTO reopens
	if tel := r.tel(); tel.Mode != model.ModeAuto || tel.ManualLeftS != 0 {
		t.Fatalf("manual did not expire: %+v", tel)
	}
	r.want(model.StateOpening) // the 4s trip started at 60s and is still under way
	r.run(5 * time.Second)
	r.want(model.StateOpen)

	// `auto` command ends manual immediately.
	r.b.OnCommand(r.now, model.ActionOpen)
	if r.tel().Mode != model.ModeManual {
		t.Fatal("open must switch to MANUAL")
	}
	r.b.OnCommand(r.now, model.ActionAuto)
	if r.tel().Mode != model.ModeAuto {
		t.Fatal("auto command ignored")
	}
}

func TestManualOpenDuringRainIsRevertedByAutoAfterTimeout(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(true, false)
	r.run(10 * time.Second) // closes automatically
	r.want(model.StateClosed)
	r.b.OnCommand(r.now, model.ActionOpen)
	r.run(5 * time.Second)
	r.want(model.StateOpen) // manual wins while MANUAL
	r.run(50 * time.Second)
	r.want(model.StateOpen) // ~65s: manual (60s from t=10s) has not expired yet
	r.run(6 * time.Second)  // ~71s: expired at 70s, rain still confirmed => closing again
	r.want(model.StateClosing)
}

func TestCommandToCurrentEndIsIgnored(t *testing.T) {
	r := newRig(t, FastConfig())
	r.b.OnCommand(r.now, model.ActionOpen) // already open
	r.run(time.Second)
	r.want(model.StateOpen)
	r.b.OnCommand(r.now, model.ActionClose)
	r.run(500 * time.Millisecond)
	r.b.OnCommand(r.now, model.ActionClose) // already closing: must not restart the travel timer
	r.run(4 * time.Second)
	r.want(model.StateClosed)
}

func TestReverseMidTravel(t *testing.T) {
	r := newRig(t, FastConfig())
	r.b.OnCommand(r.now, model.ActionClose)
	r.run(time.Second)
	r.b.OnCommand(r.now, model.ActionOpen)
	r.run(500 * time.Millisecond)
	r.want(model.StateOpening)
	r.run(5 * time.Second)
	r.want(model.StateOpen)
}

func TestStaleWeatherEntersFailSafe(t *testing.T) {
	cfg := FastConfig()
	cfg.WeatherStale = 10 * time.Second
	cfg.FirstGrace = 5 * time.Second
	r := newRig(t, cfg)

	// Never received weather: inside the grace period nothing is claimed.
	r.run(3 * time.Second)
	if tel := r.tel(); tel.FailSafe || tel.Rain {
		t.Fatalf("inside grace: %+v", tel)
	}
	r.run(3 * time.Second) // 6s > 5s grace
	tel := r.tel()
	if !tel.FailSafe || tel.Rain || tel.RainSource != model.RainNone || tel.WeatherAgeS != -1 {
		t.Fatalf("after grace, dry/light room: %+v", tel)
	}
	r.want(model.StateOpen) // unknown => hold

	// Humid + dark => local verdict closes the awning (after the confirm time).
	r.s = Sensors{Temp: 25, Humidity: 92, Light: 300}
	r.run(2 * time.Second)
	if tel := r.tel(); !tel.Rain || tel.RainSource != model.RainLocal {
		t.Fatalf("local verdict: %+v", tel)
	}
	r.run(2 * time.Second)
	r.want(model.StateClosing)

	// Fresh weather ends the fail-safe.
	r.weather(false, false)
	if tel := r.tel(); tel.FailSafe || tel.WeatherAgeS != 0 {
		t.Fatalf("fresh weather: %+v", tel)
	}
}

func TestWeatherAgeAccountsForAgeAtReceiptAndElapsed(t *testing.T) {
	cfg := FastConfig()
	cfg.WeatherStale = 30 * time.Minute
	r := newRig(t, cfg)
	r.b.OnWeather(r.now, false, false, 25*time.Minute) // backend says the data is already 25 min old
	if a := r.tel().WeatherAgeS; a != 25*60 {
		t.Fatalf("age at receipt = %d", a)
	}
	r.run(4 * time.Minute)
	if tel := r.tel(); tel.FailSafe || tel.WeatherAgeS != 29*60 {
		t.Fatalf("29 min old: %+v", tel)
	}
	r.run(2 * time.Minute) // 31 min > 30 min
	if tel := r.tel(); !tel.FailSafe {
		t.Fatalf("31 min old must be stale: %+v", tel)
	}
}

func TestStaleWeatherIsNotTrustedEvenIfItSaidRain(t *testing.T) {
	cfg := FastConfig()
	r := newRig(t, cfg)
	r.b.OnWeather(r.now, true, false, 45*time.Minute) // stale on arrival
	r.run(10 * time.Second)
	r.want(model.StateOpen) // dry, bright room and stale data => hold, do not act on old rain
	if tel := r.tel(); tel.RainSource != model.RainNone || !tel.FailSafe {
		t.Fatalf("stale weather used: %+v", tel)
	}
}

func TestPlateWetClosesWithoutAnyWeather(t *testing.T) {
	r := newRig(t, FastConfig()) // plate confirm 2s; no weather message has ever arrived, still inside the grace
	r.plate(2600)
	r.run(time.Second)
	r.want(model.StateOpen) // a splash so far, not confirmed
	r.run(2 * time.Second)
	r.want(model.StateClosing)
	tel := r.tel()
	if !tel.Rain || tel.RainSource != model.RainSensor || tel.FailSafe {
		t.Fatalf("verdict: %+v", tel)
	}
	if tel.RainLevel == nil || *tel.RainLevel != 2600 || tel.RainWet == nil || !*tel.RainWet {
		t.Fatalf("plate fields: level=%v wet=%v", tel.RainLevel, tel.RainWet)
	}
	if err := tel.Validate(); err != nil {
		t.Errorf("telemetry violates the contract: %v", err)
	}
}

func TestPlateOutranksTheForecastAndUsesItsShorterConfirm(t *testing.T) {
	cfg := FastConfig()
	cfg.RainConfirm, cfg.SensorConfirm = 30*time.Second, 2*time.Second
	r := newRig(t, cfg)
	r.weather(false, false)
	r.plate(2600)
	r.run(time.Second)
	r.want(model.StateOpen)
	r.run(2 * time.Second)
	r.want(model.StateClosing) // the plate needs 2s, the forecast alone would have needed 30s

	r.weather(true, false) // the forecast agrees: the physical evidence is still the one reported
	if tel := r.tel(); tel.RainSource != model.RainSensor {
		t.Fatalf("source = %s, want sensor", tel.RainSource)
	}
}

func TestPlateHysteresis(t *testing.T) {
	r := newRig(t, FastConfig())
	for _, c := range []struct {
		level int
		wet   bool
	}{
		{RainWetAbove - 1, false}, // just under the wet threshold
		{RainWetAbove, true},      // the threshold itself counts
		{300, true},               // between the two: keeps wet
		{RainDryBelow + 1, true},
		{RainDryBelow, false}, // the dry threshold itself counts
		{RainWetAbove - 1, false},
		{RainWetAbove, true},
	} {
		r.plate(c.level)
		r.run(100 * time.Millisecond)
		if got := r.tel().RainWet; got == nil || *got != c.wet {
			t.Fatalf("level %d: rain_wet = %v, want %v", c.level, got, c.wet)
		}
	}
}

func TestPlateSplashShorterThanTheConfirmDoesNotClose(t *testing.T) {
	r := newRig(t, FastConfig()) // plate confirm 2s
	r.weather(false, false)
	r.plate(2600)
	r.run(1500 * time.Millisecond)
	r.plate(10)
	r.run(500 * time.Millisecond)
	r.plate(2600)
	r.run(1500 * time.Millisecond) // 1.5s + 1.5s of wet is not 2s in a row
	r.want(model.StateOpen)
	r.run(time.Second)
	r.want(model.StateClosing)
}

func TestPlateDryingReopensOnlyAfterTheDryConfirm(t *testing.T) {
	r := newRig(t, FastConfig()) // dry confirm 20s, trip 4s
	r.weather(false, false)
	r.plate(2600)
	r.run(8 * time.Second)
	r.want(model.StateClosed)

	r.plate(10)
	r.run(15 * time.Second)
	r.want(model.StateClosed) // the plate is dry, but not for long enough yet
	r.run(6 * time.Second)
	r.want(model.StateOpening)
	r.run(5 * time.Second)
	r.want(model.StateOpen)
}

func TestDryPlateIsNotDryEvidenceWhenTheWeatherIsLost(t *testing.T) {
	cfg := FastConfig()
	cfg.WeatherStale, cfg.FirstGrace, cfg.DryConfirm = 10*time.Second, 5*time.Second, 3*time.Second
	r := newRig(t, cfg)
	r.plate(2600)
	r.run(8 * time.Second)
	r.want(model.StateClosed)
	if tel := r.tel(); tel.RainSource != model.RainSensor || !tel.FailSafe {
		t.Fatalf("wet plate, no weather feed: %+v (fail_safe describes the feed, not the plate)", tel)
	}

	r.plate(10)
	r.run(10 * time.Second) // far longer than the dry confirm
	r.want(model.StateClosed)
	if tel := r.tel(); tel.Rain || tel.RainSource != model.RainNone || !tel.FailSafe {
		t.Fatalf("dry plate, no weather feed: %+v", tel)
	}

	r.weather(false, false) // a fresh forecast saying dry is real evidence
	r.run(3500 * time.Millisecond)
	r.want(model.StateOpening)
	r.run(5 * time.Second)
	r.want(model.StateOpen)
}

func TestSimulatedRainOutranksThePlate(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(false, false)
	r.plate(2600)
	r.b.OnCommand(r.now, model.ActionSimulateRain)
	r.run(200 * time.Millisecond)
	r.want(model.StateClosing) // simulated rain acts at once, long before the 2s plate confirm
	tel := r.tel()
	if tel.RainSource != model.RainSim || tel.RainWet == nil || !*tel.RainWet {
		t.Fatalf("sim must be the source while the plate keeps reporting wet: %+v", tel)
	}
	r.b.OnCommand(r.now, model.ActionClearRain)
	if tel := r.tel(); tel.RainSource != model.RainSensor {
		t.Fatalf("after clear_rain the wet plate is the reason again, got %s", tel.RainSource)
	}
}

func TestWithoutAPlateTheRainFieldsAreNull(t *testing.T) {
	r := newRig(t, FastConfig())
	tel := r.tel()
	if tel.RainLevel != nil || tel.RainWet != nil {
		t.Fatalf("no plate must mean null fields, got level=%v wet=%v", tel.RainLevel, tel.RainWet)
	}
	if err := tel.Validate(); err != nil {
		t.Errorf("telemetry without a plate violates the contract: %v", err)
	}

	// A plate that stops answering (cable pulled) must not keep claiming rain.
	r.weather(false, false)
	r.plate(2600)
	r.run(time.Second)
	if w := r.tel().RainWet; w == nil || !*w {
		t.Fatal("plate not wet")
	}
	r.s.RainValid = false
	r.run(100 * time.Millisecond)
	if tel := r.tel(); tel.RainLevel != nil || tel.RainWet != nil || tel.RainSource == model.RainSensor {
		t.Fatalf("a vanished plate still counts: %+v", tel)
	}
}

func TestTelemetryClampsThePlateLevelToTheContract(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-5, 0}, {0, 0}, {4095, 4095}, {5000, 4095}} {
		r := newRig(t, FastConfig())
		r.plate(c.in)
		tel := r.tel()
		if tel.RainLevel == nil || *tel.RainLevel != c.want {
			t.Errorf("level %d reported as %v, want %d", c.in, tel.RainLevel, c.want)
		}
		if err := tel.Validate(); err != nil {
			t.Errorf("level %d: %v", c.in, err)
		}
	}
}

func TestOnlyRealRainWetsThePlateNotTheForecastNorTheSimulatedRain(t *testing.T) {
	r := newRig(t, FastConfig())
	if r.b.WorldRaining() || r.b.EnvRaining() {
		t.Fatal("raining at boot")
	}
	r.weather(false, true) // forecast: rain within 15 minutes, nothing falls yet
	if r.b.WorldRaining() || r.b.EnvRaining() {
		t.Error("a forecast is not rain")
	}
	r.weather(false, false)
	r.b.OnCommand(r.now, model.ActionSimulateRain)
	if r.b.WorldRaining() {
		t.Error("simulate_rain is a firmware flag: it must not wet the plate")
	}
	if !r.b.EnvRaining() {
		t.Error("simulated rain should still make the air look rainy")
	}
	r.b.OnCommand(r.now, model.ActionClearRain)
	r.weather(true, false)
	if !r.b.WorldRaining() || !r.b.EnvRaining() {
		t.Error("rain from the weather source is real rain")
	}
}

func TestChangedFiresOncePerTransition(t *testing.T) {
	r := newRig(t, FastConfig())
	if r.b.Changed(r.now, r.s) {
		t.Fatal("changed at boot")
	}
	r.b.OnCommand(r.now, model.ActionClose)
	if !r.b.Changed(r.now, r.s) {
		t.Fatal("state/mode change not reported")
	}
	if r.b.Changed(r.now, r.s) {
		t.Fatal("reported twice")
	}
}

func TestChangedFiresWhenOnlyThePlateVerdictFlips(t *testing.T) {
	r := newRig(t, FastConfig())
	r.weather(false, false)
	r.b.OnCommand(r.now, model.ActionSimulateRain) // the source stays "sim" whatever the plate does
	r.run(6 * time.Second)
	r.want(model.StateClosed)
	r.b.Changed(r.now, r.s) // swallow everything up to here
	if r.b.Changed(r.now, r.s) {
		t.Fatal("reported twice")
	}
	r.plate(2600)
	r.run(100 * time.Millisecond)
	if !r.b.Changed(r.now, r.s) {
		t.Fatal("a plate that turned wet must trigger an immediate telemetry frame")
	}
	if r.b.Changed(r.now, r.s) {
		t.Fatal("reported twice")
	}
}

func TestEnvironmentIsPlausibleAndDeterministic(t *testing.T) {
	noon := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	night := time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)
	a, b := NewEnvironment(7), NewEnvironment(7)
	for i := 0; i < 20; i++ {
		if a.Sample(noon, Conditions{}) != b.Sample(noon, Conditions{}) {
			t.Fatal("not deterministic for the same seed")
		}
	}
	e := NewEnvironment(1)
	day, dark := e.Sample(noon, Conditions{}), e.Sample(night, Conditions{})
	if day.Light < 3000 || dark.Light > 800 {
		t.Errorf("light day=%d night=%d", day.Light, dark.Light)
	}
	rain := e.Sample(noon, Conditions{Rainy: true})
	if rain.Humidity <= day.Humidity+10 || rain.Light >= day.Light/2 || rain.Temp >= day.Temp {
		t.Errorf("rain must be wetter, darker, cooler: dry=%+v rain=%+v", day, rain)
	}
	for _, s := range []Sensors{day, dark, rain} {
		if s.Humidity < 30 || s.Humidity > 99 || s.Light < 0 || s.Light > 4095 {
			t.Errorf("out of range: %+v", s)
		}
	}
}

func TestEnvironmentRainPlateIsFarFromBothThresholds(t *testing.T) {
	noon := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	e := NewEnvironment(3)
	for i := 0; i < 1000; i++ {
		dryPlate, wetPlate := e.Sample(noon, Conditions{}), e.Sample(noon, Conditions{PlateWet: true})
		if !dryPlate.RainValid || !wetPlate.RainValid {
			t.Fatal("the simulated device has a plate")
		}
		if dryPlate.RainLevel < 0 || dryPlate.RainLevel > RainDryBelow/2 {
			t.Fatalf("dry plate reads %d: must sit well under %d so the noise never flips it", dryPlate.RainLevel, RainDryBelow)
		}
		if wetPlate.RainLevel < 2*RainWetAbove || wetPlate.RainLevel > 4095 {
			t.Fatalf("wet plate reads %d: must sit well over %d", wetPlate.RainLevel, RainWetAbove)
		}
	}
	// The air looking rainy says nothing about the plate: only real water wets it.
	if s := e.Sample(noon, Conditions{Rainy: true}); s.RainLevel > RainDryBelow {
		t.Errorf("rainy air alone wet the plate: %d", s.RainLevel)
	}
}

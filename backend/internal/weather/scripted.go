package weather

import (
	"context"
	"errors"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

// Scripted is a deterministic stand-in for Open-Meteo, used by the demo and tests so the whole system can run
// without Internet. Each Period it plays: clear (62.5%) -> "rain expected within 15 minutes" (12.5%) -> raining (25%).
// With Period == 0 the sky stays clear.
type Scripted struct {
	Location string
	Period   time.Duration
	Origin   time.Time // start of the first cycle
	Now      func() time.Time
}

type Phase int

const (
	PhaseClear Phase = iota
	PhaseExpected
	PhaseRaining
)

func (s *Scripted) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// PhaseAt reports which part of the cycle t falls in.
func (s *Scripted) PhaseAt(t time.Time) Phase {
	if s.Period <= 0 {
		return PhaseClear
	}
	pos := t.Sub(s.Origin) % s.Period
	if pos < 0 {
		pos += s.Period
	}
	switch frac := float64(pos) / float64(s.Period); {
	case frac < 0.625:
		return PhaseClear
	case frac < 0.75:
		return PhaseExpected
	default:
		return PhaseRaining
	}
}

func (s *Scripted) Fetch(context.Context) (model.Weather, error) {
	raw := s.now()
	now := raw.UTC().Truncate(time.Second)
	w := model.Weather{FetchedAt: now, Location: s.Location, Forecast: []model.ForecastPoint{}}
	var prob []int
	// Use the untruncated time for the phase: truncating to whole seconds can land just before Origin, which
	// wraps to the END of the cycle (raining) on the very first fetch.
	switch s.PhaseAt(raw) {
	case PhaseRaining:
		w.TemperatureC, w.Humidity, w.PrecipitationMM, w.WeatherCode, w.IsRaining = 26.5, 92, 1.4, 63, true
		prob = []int{90, 70, 40}
	case PhaseExpected:
		w.TemperatureC, w.Humidity, w.WeatherCode, w.RainExpected15m = 28.2, 84, 3, true
		prob = []int{60, 80, 50}
	default:
		w.TemperatureC, w.Humidity, w.WeatherCode = 30.1, 68, 1
		prob = []int{5, 10, 10}
	}
	w.Description = Describe(w.WeatherCode)
	hour := now.Truncate(time.Hour)
	for i, p := range prob {
		p := p
		mm := 0.0
		if p >= 60 {
			mm = float64(p) / 100
		}
		w.Forecast = append(w.Forecast, model.ForecastPoint{Time: hour.Add(time.Duration(i) * time.Hour), PrecipitationProbability: &p, PrecipitationMM: mm})
	}
	return w, nil
}

// Failing is a Client that always errors. The demo uses it (-no-weather) to show how the system behaves when the
// weather source is unavailable: no weather card, and the device eventually falls back to its local sensors.
type Failing struct{ Err error }

func (f Failing) Fetch(context.Context) (model.Weather, error) {
	if f.Err != nil {
		return model.Weather{}, f.Err
	}
	return model.Weather{}, errors.New("weather source unavailable (demo started with -no-weather)")
}

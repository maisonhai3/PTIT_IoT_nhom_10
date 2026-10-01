package devicesim

import (
	"context"
	"sort"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
)

// rainEpisode is a stretch of rain, as offsets before "now".
type rainEpisode struct{ startAgo, length time.Duration }

// Rain the seeded history contains, oldest first. Each closes the awning ~10s after it starts and reopens it
// 15 minutes after the rain ends, like the real rule does.
var seedEpisodes = []rainEpisode{
	{19 * time.Hour, 40 * time.Minute},
	{8*time.Hour + 30*time.Minute, 25 * time.Minute},
	{2*time.Hour + 40*time.Minute, 35 * time.Minute},
}

const (
	seedTrip = 10 * time.Second
	seedDry  = 15 * time.Minute
)

// SeedHistory fills the store with plausible telemetry for the last `hours` hours (a sample every `every`,
// plus exact state-transition rows), so a freshly started demo shows a meaningful chart instead of an empty one.
func SeedHistory(ctx context.Context, st store.Store, now time.Time, hours int, every time.Duration, seed int64) (int, error) {
	env := NewEnvironment(seed)
	start := now.Add(-time.Duration(hours) * time.Hour)

	type episode struct{ start, end time.Time }
	var eps []episode
	for _, e := range seedEpisodes {
		s := now.Add(-e.startAgo)
		if s.After(start) {
			eps = append(eps, episode{s, s.Add(e.length)})
		}
	}

	times := []time.Time{}
	for t := start; t.Before(now); t = t.Add(every) {
		times = append(times, t)
	}
	for _, e := range eps { // exact transition moments
		times = append(times, e.start, e.start.Add(seedTrip), e.end.Add(seedDry), e.end.Add(seedDry+seedTrip))
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	uniq := times[:0] // a transition can coincide with a regular sample: keep one row per instant
	for _, t := range times {
		if len(uniq) == 0 || !t.Equal(uniq[len(uniq)-1]) {
			uniq = append(uniq, t)
		}
	}
	times = uniq

	n := 0
	for _, t := range times {
		if !t.Before(now) {
			continue
		}
		state, raining := model.StateOpen, false
		for _, e := range eps {
			switch {
			case !t.Before(e.start) && t.Before(e.start.Add(seedTrip)):
				state = model.StateClosing
			case !t.Before(e.start.Add(seedTrip)) && t.Before(e.end.Add(seedDry)):
				state = model.StateClosed
			case !t.Before(e.end.Add(seedDry)) && t.Before(e.end.Add(seedDry+seedTrip)):
				state = model.StateOpening
			}
			if !t.Before(e.start) && t.Before(e.end) {
				raining = true
			}
		}
		s := env.Sample(t, Conditions{Rainy: raining}) // the history has no rain_level, so the plate is irrelevant
		temp, hum := s.Temp, s.Humidity
		src := model.RainNone
		if raining {
			src = model.RainAPI
		}
		row := model.Telemetry{TS: t.UTC().Truncate(time.Millisecond), DeviceTelemetry: model.DeviceTelemetry{
			Temp: &temp, Humidity: &hum, Light: s.Light, State: state, Mode: model.ModeAuto,
			Rain: raining, RainSource: src,
		}}
		if err := st.InsertTelemetry(ctx, row); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

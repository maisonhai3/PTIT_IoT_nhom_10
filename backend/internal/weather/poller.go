package weather

import (
	"context"
	"log/slog"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

// Poller fetches weather now and then every Interval. After a failure it retries with exponential backoff
// (RetryBase, doubling, capped at Interval) and keeps the previous data untouched, so consumers can see it age.
type Poller struct {
	Client    Client
	Interval  time.Duration
	RetryBase time.Duration // default 15s
	OnUpdate  func(model.Weather)
	// OnError is called once when the poller goes from healthy to failing (not on every retry).
	OnError func(err error)
	Log     *slog.Logger
}

func (p *Poller) Run(ctx context.Context) {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	retry := p.RetryBase
	if retry <= 0 {
		retry = 15 * time.Second
	}
	if p.Interval <= 0 {
		p.Interval = 5 * time.Minute // never spin: a zero interval would fetch in a tight loop
	}
	failures := 0
	delay := time.Duration(0) // fetch immediately on start
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		w, err := p.Client.Fetch(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failures++
			if failures == 1 && p.OnError != nil {
				p.OnError(err)
			}
			delay = retry << min(failures-1, 10)
			if delay > p.Interval {
				delay = p.Interval
			}
			log.Warn("weather fetch failed", "err", err, "retry_in", delay, "failures", failures)
			continue
		}
		if failures > 0 {
			log.Info("weather fetch recovered", "after_failures", failures)
		}
		failures = 0
		log.Debug("weather updated", "temp", w.TemperatureC, "raining", w.IsRaining, "rain_expected_15m", w.RainExpected15m)
		if p.OnUpdate != nil {
			p.OnUpdate(w)
		}
		delay = p.Interval
	}
}

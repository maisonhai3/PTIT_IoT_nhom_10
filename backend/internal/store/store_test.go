package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

func f(v float64) *float64 { return &v }

func tel(ts time.Time, temp *float64, state model.AwningState) model.Telemetry {
	return model.Telemetry{TS: ts, DeviceTelemetry: model.DeviceTelemetry{
		Temp: temp, Humidity: f(60), Light: 1000, State: state, Mode: model.ModeAuto,
		RainSource: model.RainNone, WeatherAgeS: 10,
	}}
}

func TestHistoryRoundTripAndOrder(t *testing.T) {
	s, err := Memory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	// Insert out of order to prove the query sorts.
	for _, i := range []int{2, 0, 1} {
		var temp *float64
		if i != 1 { // one row has a failed DHT reading
			temp = f(20 + float64(i))
		}
		if err := s.InsertTelemetry(ctx, tel(base.Add(time.Duration(i)*time.Minute), temp, model.StateOpen)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.History(ctx, base, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d points, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].TS.After(got[i-1].TS) {
			t.Fatalf("not ascending at %d: %v then %v", i, got[i-1].TS, got[i].TS)
		}
	}
	if got[1].Temp != nil {
		t.Errorf("null temperature did not round-trip: %v", *got[1].Temp)
	}
	if got[0].Temp == nil || *got[0].Temp != 20 {
		t.Errorf("first temp = %v, want 20", got[0].Temp)
	}
	if got[0].State != model.StateOpen || got[0].Mode != model.ModeAuto || got[0].RainSource != model.RainNone {
		t.Errorf("enums lost: %+v", got[0])
	}
}

func TestHistorySinceFilterAndDownsample(t *testing.T) {
	s, _ := Memory()
	defer s.Close()
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		if err := s.InsertTelemetry(ctx, tel(base.Add(time.Duration(i)*time.Minute), f(float64(i)), model.StateClosed)); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.History(ctx, base, 5000)
	if len(all) != 1000 {
		t.Fatalf("no-thinning case: %d rows", len(all))
	}
	thin, err := s.History(ctx, base, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(thin) == 0 || len(thin) > 100 {
		t.Fatalf("thinned to %d, want 1..100", len(thin))
	}
	if *thin[0].Temp != 0 {
		t.Errorf("thinning must keep the first row, got temp %v", *thin[0].Temp)
	}
	recent, _ := s.History(ctx, base.Add(990*time.Minute), 100)
	if len(recent) != 10 {
		t.Errorf("since filter: %d rows, want 10", len(recent))
	}
}

func TestEventsNewestFirstAndLimit(t *testing.T) {
	s, _ := Memory()
	defer s.Close()
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	for i, k := range []model.EventKind{model.EventState, model.EventMode, model.EventOnline} {
		if err := s.InsertEvent(ctx, model.Event{TS: base.Add(time.Duration(i) * time.Second), Kind: k, Detail: "d"}); err != nil {
			t.Fatal(err)
		}
	}
	// Same timestamp: insertion order (id) must break the tie, newest first.
	s.InsertEvent(ctx, model.Event{TS: base.Add(2 * time.Second), Kind: model.EventRain, Detail: "tie"})

	got, err := s.Events(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Kind != model.EventRain || got[1].Kind != model.EventOnline || got[2].Kind != model.EventMode {
		t.Fatalf("unexpected order: %+v", got)
	}
	empty, _ := (func() ([]model.Event, error) { s2, _ := Memory(); defer s2.Close(); return s2.Events(ctx, 5) })()
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty log must be an empty non-nil slice (JSON [] not null), got %#v", empty)
	}
}

func TestPrune(t *testing.T) {
	s, _ := Memory()
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s.InsertTelemetry(ctx, tel(now.Add(-48*time.Hour), f(1), model.StateOpen))
	s.InsertTelemetry(ctx, tel(now.Add(-1*time.Hour), f(2), model.StateOpen))
	s.InsertEvent(ctx, model.Event{TS: now.Add(-48 * time.Hour), Kind: model.EventState, Detail: "old"})
	s.InsertEvent(ctx, model.Event{TS: now.Add(-1 * time.Hour), Kind: model.EventState, Detail: "new"})

	if err := s.Prune(ctx, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	h, _ := s.History(ctx, now.Add(-100*time.Hour), 100)
	e, _ := s.Events(ctx, 10)
	if len(h) != 1 || *h[0].Temp != 2 {
		t.Errorf("telemetry after prune: %+v", h)
	}
	if len(e) != 1 || e[0].Detail != "new" {
		t.Errorf("events after prune: %+v", e)
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awning.db")
	ctx := context.Background()
	ts := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertTelemetry(ctx, tel(ts, f(30), model.StateClosing)); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.History(ctx, ts.Add(-time.Hour), 10)
	if err != nil || len(got) != 1 || got[0].State != model.StateClosing {
		t.Fatalf("after reopen: %+v err=%v", got, err)
	}
}

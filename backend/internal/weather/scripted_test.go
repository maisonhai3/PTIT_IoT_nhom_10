package weather

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScriptedPhases(t *testing.T) {
	origin := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s := &Scripted{Location: "Demo", Period: 4 * time.Minute, Origin: origin}
	cases := []struct {
		at   time.Duration
		want Phase
	}{
		{0, PhaseClear}, {149 * time.Second, PhaseClear},
		{150 * time.Second, PhaseExpected}, {179 * time.Second, PhaseExpected},
		{180 * time.Second, PhaseRaining}, {239 * time.Second, PhaseRaining},
		{240 * time.Second, PhaseClear}, // next cycle
		{-10 * time.Second, PhaseRaining},
	}
	for _, c := range cases {
		if got := s.PhaseAt(origin.Add(c.at)); got != c.want {
			t.Errorf("phase at %v = %v, want %v", c.at, got, c.want)
		}
	}
	if (&Scripted{}).PhaseAt(origin) != PhaseClear {
		t.Error("zero period must stay clear")
	}
}

func TestScriptedFetchMatchesPhase(t *testing.T) {
	origin := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	at := origin
	s := &Scripted{Location: "Demo", Period: 4 * time.Minute, Origin: origin, Now: func() time.Time { return at }}

	check := func(name string, raining, expected bool, code int) {
		t.Helper()
		w, err := s.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if w.IsRaining != raining || w.RainExpected15m != expected || w.WeatherCode != code {
			t.Errorf("%s: %+v", name, w)
		}
		if w.Description != Describe(code) || w.Location != "Demo" || len(w.Forecast) != 3 || !w.FetchedAt.Equal(at) {
			t.Errorf("%s: metadata wrong: %+v", name, w)
		}
	}
	check("clear", false, false, 1)
	at = origin.Add(160 * time.Second)
	check("expected", false, true, 3)
	at = origin.Add(200 * time.Second)
	check("raining", true, false, 63)
}

// Regression: the first fetch right after start must be clear. Truncating "now" to whole seconds used to put it
// before Origin, which wrapped to the raining phase at the end of the cycle.
func TestScriptedFirstFetchAfterStartIsClear(t *testing.T) {
	origin := time.Date(2026, 9, 30, 12, 0, 0, 700_000_000, time.UTC) // 700ms into the second
	for _, d := range []time.Duration{0, 10 * time.Millisecond, 250 * time.Millisecond, 300 * time.Millisecond} {
		at := origin.Add(d)
		s := &Scripted{Period: 4 * time.Minute, Origin: origin, Now: func() time.Time { return at }}
		w, err := s.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if w.IsRaining || w.RainExpected15m {
			t.Errorf("%v after start: %+v, want clear", d, w)
		}
	}
}

func TestFailingAlwaysErrors(t *testing.T) {
	if _, err := (Failing{}).Fetch(context.Background()); err == nil {
		t.Fatal("Failing returned no error")
	}
	want := errors.New("boom")
	if _, err := (Failing{Err: want}).Fetch(context.Background()); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

package devicesim

import (
	"context"
	"testing"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
)

func TestSeedHistoryShapesARealisticDay(t *testing.T) {
	st, err := store.Memory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	n, err := SeedHistory(context.Background(), st, now, 24, 2*time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	pts, err := st.History(context.Background(), now.Add(-25*time.Hour), 10000)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != n || n < 24*30 {
		t.Fatalf("inserted %d, read back %d (want >= 720: a row every 2 minutes for 24h)", n, len(pts))
	}
	closed, rainy := 0, 0
	for i, p := range pts {
		if !p.TS.Before(now) {
			t.Fatalf("row %d is in the future: %v", i, p.TS)
		}
		if i > 0 && !p.TS.After(pts[i-1].TS) {
			t.Fatalf("rows not strictly increasing at %d", i)
		}
		if p.State == model.StateClosed {
			closed++
		}
		if p.Rain {
			rainy++
			if p.RainSource != model.RainAPI {
				t.Fatalf("rain row with source %q", p.RainSource)
			}
		}
		if p.Temp == nil || p.Humidity == nil || *p.Humidity < 30 || *p.Humidity > 99 {
			t.Fatalf("implausible row: %+v", p)
		}
	}
	if closed == 0 || rainy == 0 || closed <= rainy {
		t.Errorf("closed=%d rainy=%d: the awning must be closed for longer than it rains (15 min dry delay)", closed, rainy)
	}

	// The awning goes OPEN -> CLOSING -> CLOSED -> OPENING -> OPEN, in that order, for every episode.
	var seq []model.AwningState
	for _, p := range pts {
		if len(seq) == 0 || seq[len(seq)-1] != p.State {
			seq = append(seq, p.State)
		}
	}
	want := []model.AwningState{model.StateOpen}
	for i := 0; i < 3; i++ {
		want = append(want, model.StateClosing, model.StateClosed, model.StateOpening, model.StateOpen)
	}
	if len(seq) != len(want) {
		t.Fatalf("state sequence %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("state sequence %v, want %v", seq, want)
		}
	}
}

func TestSeedHistoryShortWindowSkipsOlderEpisodes(t *testing.T) {
	st, _ := store.Memory()
	defer st.Close()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if _, err := SeedHistory(context.Background(), st, now, 1, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	pts, _ := st.History(context.Background(), now.Add(-2*time.Hour), 1000)
	for _, p := range pts {
		if p.State != model.StateOpen || p.Rain {
			t.Fatalf("a 1h window has no rain episode, got %+v", p)
		}
	}
}

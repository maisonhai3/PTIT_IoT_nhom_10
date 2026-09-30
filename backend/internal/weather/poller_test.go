package weather

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

type scriptedClient struct {
	mu      sync.Mutex
	results []error // one entry per call; nil = success; the last entry repeats
	calls   int
	times   []time.Time
}

func (c *scriptedClient) Fetch(ctx context.Context) (model.Weather, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.times = append(c.times, time.Now())
	i := c.calls
	if i >= len(c.results) {
		i = len(c.results) - 1
	}
	c.calls++
	if err := c.results[i]; err != nil {
		return model.Weather{}, err
	}
	return model.Weather{TemperatureC: float64(c.calls), Forecast: []model.ForecastPoint{}}, nil
}

func TestPollerFetchesImmediatelyThenOnInterval(t *testing.T) {
	c := &scriptedClient{results: []error{nil}}
	var updates atomic.Int32
	p := &Poller{Client: c, Interval: 30 * time.Millisecond, OnUpdate: func(model.Weather) { updates.Add(1) }}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	p.Run(ctx)
	if updates.Load() < 3 {
		t.Errorf("only %d updates in 200ms with a 30ms interval", updates.Load())
	}
}

func TestPollerBacksOffReportsOnceAndRecovers(t *testing.T) {
	boom := errors.New("network down")
	c := &scriptedClient{results: []error{boom, boom, boom, nil}}
	var errCalls, updates atomic.Int32
	p := &Poller{
		Client:    c,
		Interval:  500 * time.Millisecond,
		RetryBase: 10 * time.Millisecond,
		OnError:   func(error) { errCalls.Add(1) },
		OnUpdate:  func(model.Weather) { updates.Add(1) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for updates.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if updates.Load() == 0 {
		t.Fatal("poller never recovered")
	}
	if errCalls.Load() != 1 {
		t.Errorf("OnError called %d times for 3 consecutive failures, want 1", errCalls.Load())
	}
	// Retry delays are 10ms, 20ms (then success): the gaps between the first 3 calls must grow.
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.times) < 4 {
		t.Fatalf("only %d calls", len(c.times))
	}
	g1, g2 := c.times[1].Sub(c.times[0]), c.times[2].Sub(c.times[1])
	if g2 < g1 {
		t.Errorf("no backoff: gaps %v then %v", g1, g2)
	}
}

func TestPollerReportsAgainAfterRecovery(t *testing.T) {
	boom := errors.New("x")
	c := &scriptedClient{results: []error{boom, nil, boom, nil}}
	var errCalls atomic.Int32
	p := &Poller{Client: c, Interval: 15 * time.Millisecond, RetryBase: 5 * time.Millisecond,
		OnError: func(error) { errCalls.Add(1) }}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	p.Run(ctx)
	if errCalls.Load() != 2 {
		t.Errorf("OnError called %d times for two separate outages, want 2", errCalls.Load())
	}
}

func TestPollerStopsPromptlyOnCancel(t *testing.T) {
	c := &scriptedClient{results: []error{nil}}
	p := &Poller{Client: c, Interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

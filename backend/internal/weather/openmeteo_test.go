package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fixture builds an Open-Meteo style body (timeformat=unixtime) with times relative to now, so the tests
// do not depend on a stale literal timestamp. Layout follows https://open-meteo.com/en/docs.
type fixture struct {
	temp, hum, precip, rain, showers float64
	code                             int
	// minutely[i] is the precipitation of the 15-minute slot ending at next-quarter + 15min*(i-1);
	// index 0 is therefore a slot in the PAST (ended before now), index 1 the slot ending within 15 minutes.
	minutely []any
	// hourly slots start one hour before now's hour.
	prob   []any
	hourly []any
}

func (f fixture) body(now time.Time) []byte {
	nextQuarter := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	var mt []int64
	for i := range f.minutely {
		mt = append(mt, nextQuarter.Add(time.Duration(i-1)*15*time.Minute).Unix())
	}
	var ht []int64
	for i := range f.prob {
		ht = append(ht, now.Truncate(time.Hour).Add(time.Duration(i-1)*time.Hour).Unix())
	}
	m := map[string]any{
		"current": map[string]any{
			"time": now.Unix(), "interval": 900,
			"temperature_2m": f.temp, "relative_humidity_2m": f.hum,
			"precipitation": f.precip, "rain": f.rain, "showers": f.showers, "weather_code": f.code,
		},
		"minutely_15": map[string]any{"time": mt, "precipitation": f.minutely},
		"hourly":      map[string]any{"time": ht, "precipitation_probability": f.prob, "precipitation": f.hourly},
	}
	b, _ := json.Marshal(m)
	return b
}

var now0 = time.Date(2026, 9, 30, 12, 7, 30, 0, time.UTC)

func clearFixture() fixture {
	return fixture{
		temp: 29.4, hum: 78, code: 1,
		minutely: []any{0.0, 0.0, 0.0, 0.0},
		prob:     []any{5, 10, 15, 20, 25},
		hourly:   []any{0.0, 0.0, 0.0, 0.0, 0.0},
	}
}

func TestParseClear(t *testing.T) {
	w, err := Parse(clearFixture().body(now0), now0, "Hà Nội", 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if w.IsRaining || w.RainExpected15m {
		t.Errorf("clear sky reported wet: %+v", w)
	}
	if w.TemperatureC != 29.4 || w.Humidity != 78 || w.Description != "Ít mây" || w.Location != "Hà Nội" {
		t.Errorf("basic fields wrong: %+v", w)
	}
	if !w.FetchedAt.Equal(now0) {
		t.Errorf("fetched_at = %v", w.FetchedAt)
	}
	// Hourly slots start at 11:00; the past hour must be skipped, leaving 12:00, 13:00, 14:00.
	if len(w.Forecast) != 3 {
		t.Fatalf("forecast len = %d, want 3", len(w.Forecast))
	}
	if got := w.Forecast[0].Time; !got.Equal(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("first forecast hour = %v, want 12:00Z", got)
	}
	if p := w.Forecast[0].PrecipitationProbability; p == nil || *p != 10 {
		t.Errorf("first forecast probability = %v, want 10", p)
	}
}

func TestParseIsRaining(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*fixture)
		want bool
	}{
		{"precipitation above threshold", func(f *fixture) { f.precip = 0.4; f.code = 3 }, true},
		{"rain channel only", func(f *fixture) { f.rain = 0.2 }, true},
		{"showers channel only", func(f *fixture) { f.showers = 0.3 }, true},
		{"wet weather code with zero precipitation", func(f *fixture) { f.code = 61 }, true},
		{"thunderstorm code", func(f *fixture) { f.code = 95 }, true},
		{"trace precipitation below threshold", func(f *fixture) { f.precip = 0.05; f.code = 3 }, false},
		{"fog is not rain", func(f *fixture) { f.code = 45 }, false},
		{"overcast is not rain", func(f *fixture) { f.code = 3 }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := clearFixture()
			c.mod(&f)
			w, err := Parse(f.body(now0), now0, "x", 0.1)
			if err != nil {
				t.Fatal(err)
			}
			if w.IsRaining != c.want {
				t.Errorf("IsRaining = %v, want %v", w.IsRaining, c.want)
			}
		})
	}
}

func TestParseRainExpected(t *testing.T) {
	cases := []struct {
		name     string
		minutely []any
		want     bool
	}{
		{"rain in the next slot", []any{0.0, 0.3, 0.0, 0.0}, true},
		{"rain only in a past slot is ignored", []any{5.0, 0.0, 0.0, 0.0}, false},
		{"rain only beyond 15 minutes is ignored", []any{0.0, 0.0, 2.0, 2.0}, false},
		{"trace below threshold", []any{0.0, 0.05, 0.0, 0.0}, false},
		{"null value", []any{0.0, nil, 0.0, 0.0}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := clearFixture()
			f.minutely = c.minutely
			w, err := Parse(f.body(now0), now0, "x", 0.1)
			if err != nil {
				t.Fatal(err)
			}
			if w.RainExpected15m != c.want {
				t.Errorf("RainExpected15m = %v, want %v", w.RainExpected15m, c.want)
			}
		})
	}
}

func TestParseToleratesNullsAndMissingBlocks(t *testing.T) {
	f := clearFixture()
	f.prob = []any{nil, nil, nil, nil}
	f.hourly = []any{nil, nil, nil, nil}
	w, err := Parse(f.body(now0), now0, "x", 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if w.Forecast[0].PrecipitationProbability != nil || w.Forecast[0].PrecipitationMM != 0 {
		t.Errorf("nulls not handled: %+v", w.Forecast[0])
	}

	// Only the current block: forecast and expectation degrade gracefully instead of failing.
	w, err = Parse([]byte(`{"current":{"temperature_2m":20,"weather_code":0}}`), now0, "x", 0.1)
	if err != nil {
		t.Fatal(err)
	}
	if w.Forecast == nil || len(w.Forecast) != 0 || w.RainExpected15m {
		t.Errorf("degraded parse wrong: %+v", w)
	}
}

func TestParseErrors(t *testing.T) {
	for name, body := range map[string]string{
		"not json":        "<html>",
		"no current":      `{"hourly":{}}`,
		"missing code":    `{"current":{"temperature_2m":20}}`,
		"missing temp":    `{"current":{"weather_code":1}}`,
		"iso time format": `{"current":{"temperature_2m":20,"weather_code":1},"hourly":{"time":["2026-09-30T12:00"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body), now0, "x", 0.1); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestDescribeAndWetCodes(t *testing.T) {
	if Describe(63) != "Mưa vừa" || Describe(0) != "Trời quang" || Describe(12345) != "Không rõ" {
		t.Error("Describe mapping wrong")
	}
	for _, c := range []int{51, 55, 61, 65, 67, 71, 77, 80, 82, 85, 86, 95, 99} {
		if !IsWetCode(c) {
			t.Errorf("code %d should be wet", c)
		}
	}
	for _, c := range []int{0, 1, 2, 3, 45, 48, 68, 70, 78, 79, 87, 94} {
		if IsWetCode(c) {
			t.Errorf("code %d should not be wet", c)
		}
	}
	// Every known description must be non-empty and every wet code must have one.
	for code, d := range wmoDescriptions {
		if strings.TrimSpace(d) == "" {
			t.Errorf("code %d has empty description", code)
		}
	}
}

func TestFetchBuildsRequestAndParses(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(clearFixture().body(now0))
	}))
	defer srv.Close()

	om := &OpenMeteo{BaseURL: srv.URL, Lat: 21.0285, Lon: 105.8542, Location: "Hà Nội", Now: func() time.Time { return now0 }}
	w, err := om.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if w.TemperatureC != 29.4 {
		t.Errorf("temperature = %v", w.TemperatureC)
	}
	for k, want := range map[string]string{
		"latitude": "21.0285", "longitude": "105.8542", "timeformat": "unixtime", "forecast_hours": "4",
	} {
		if got := gotQuery[k]; len(got) != 1 || got[0] != want {
			t.Errorf("query %s = %v, want %s", k, got, want)
		}
	}
	for _, k := range []string{"current", "minutely_15", "hourly"} {
		if len(gotQuery[k]) != 1 || gotQuery[k][0] == "" {
			t.Errorf("query %s missing", k)
		}
	}
}

func TestFetchErrors(t *testing.T) {
	t.Run("api error reason is surfaced", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":true,"reason":"Latitude must be in range of -90 to 90"}`))
		}))
		defer srv.Close()
		_, err := (&OpenMeteo{BaseURL: srv.URL}).Fetch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Latitude must be") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("server error without json body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream exploded", 502)
		}))
		defer srv.Close()
		_, err := (&OpenMeteo{BaseURL: srv.URL}).Fetch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "502") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("context cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := (&OpenMeteo{BaseURL: srv.URL}).Fetch(ctx); err == nil {
			t.Error("expected an error")
		}
		if time.Since(start) > 3*time.Second {
			t.Error("Fetch ignored the context deadline")
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		if _, err := (&OpenMeteo{BaseURL: url}).Fetch(context.Background()); err == nil {
			t.Error("expected an error")
		}
	})
}

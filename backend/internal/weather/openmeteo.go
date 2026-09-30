// Package weather fetches current conditions and a short rain forecast from Open-Meteo
// (https://open-meteo.com, free, no API key) and derives the two booleans the device needs.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

// Client is what the poller needs; tests and the demo provide fakes.
type Client interface {
	Fetch(ctx context.Context) (model.Weather, error)
}

const defaultBaseURL = "https://api.open-meteo.com"

type OpenMeteo struct {
	BaseURL  string
	Lat, Lon float64
	Location string
	// RainThresholdMM: precipitation at or above this counts as rain. Default 0.1 mm.
	RainThresholdMM float64
	HTTP            *http.Client
	Now             func() time.Time
}

func (o *OpenMeteo) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *OpenMeteo) threshold() float64 {
	if o.RainThresholdMM > 0 {
		return o.RainThresholdMM
	}
	return 0.1
}

func (o *OpenMeteo) endpoint() string {
	base := o.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(o.Lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(o.Lon, 'f', 4, 64))
	q.Set("current", "temperature_2m,relative_humidity_2m,precipitation,rain,showers,weather_code")
	q.Set("minutely_15", "precipitation")
	q.Set("forecast_minutely_15", "8")
	q.Set("hourly", "precipitation_probability,precipitation")
	q.Set("forecast_hours", "4")
	q.Set("timeformat", "unixtime") // absolute epoch seconds: no time-zone ambiguity
	q.Set("timezone", "auto")
	return strings.TrimRight(base, "/") + "/v1/forecast?" + q.Encode()
}

func (o *OpenMeteo) Fetch(ctx context.Context) (model.Weather, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.endpoint(), nil)
	if err != nil {
		return model.Weather{}, err
	}
	req.Header.Set("User-Agent", "awning-backend/1.0")
	req.Header.Set("Accept", "application/json")
	hc := o.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return model.Weather{}, fmt.Errorf("open-meteo request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.Weather{}, fmt.Errorf("open-meteo read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Reason == "" {
			e.Reason = strings.TrimSpace(string(body[:min(len(body), 120)]))
		}
		return model.Weather{}, fmt.Errorf("open-meteo: HTTP %d: %s", resp.StatusCode, e.Reason)
	}
	return Parse(body, o.now(), o.Location, o.threshold())
}

// The subset of the Open-Meteo response we use. Time arrays are unix seconds because of timeformat=unixtime.
type response struct {
	Current *struct {
		Temperature   *float64 `json:"temperature_2m"`
		Humidity      *float64 `json:"relative_humidity_2m"`
		Precipitation *float64 `json:"precipitation"`
		Rain          *float64 `json:"rain"`
		Showers       *float64 `json:"showers"`
		WeatherCode   *int     `json:"weather_code"`
	} `json:"current"`
	Minutely15 *struct {
		Time          []int64    `json:"time"`
		Precipitation []*float64 `json:"precipitation"`
	} `json:"minutely_15"`
	Hourly *struct {
		Time                     []int64    `json:"time"`
		PrecipitationProbability []*int     `json:"precipitation_probability"`
		Precipitation            []*float64 `json:"precipitation"`
	} `json:"hourly"`
}

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Parse turns an Open-Meteo JSON body into a Weather value. now is the moment the data was fetched.
func Parse(body []byte, now time.Time, location string, thresholdMM float64) (model.Weather, error) {
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return model.Weather{}, fmt.Errorf("open-meteo: bad JSON: %w", err)
	}
	c := r.Current
	if c == nil || c.Temperature == nil || c.WeatherCode == nil {
		return model.Weather{}, errors.New("open-meteo: response has no usable current conditions")
	}
	now = now.UTC().Truncate(time.Second)

	w := model.Weather{
		FetchedAt:       now,
		Location:        location,
		TemperatureC:    *c.Temperature,
		Humidity:        val(c.Humidity),
		PrecipitationMM: val(c.Precipitation),
		WeatherCode:     *c.WeatherCode,
		Description:     Describe(*c.WeatherCode),
		Forecast:        []model.ForecastPoint{},
	}
	w.IsRaining = val(c.Precipitation) >= thresholdMM || val(c.Rain) >= thresholdMM ||
		val(c.Showers) >= thresholdMM || IsWetCode(*c.WeatherCode)

	// Rain within the next 15 minutes: the first 15-minute slot that ends after "now".
	// (Open-Meteo reports each slot as the precipitation of the preceding 15 minutes.)
	if m := r.Minutely15; m != nil {
		for i, t := range m.Time {
			if t > now.Unix() && i < len(m.Precipitation) {
				w.RainExpected15m = val(m.Precipitation[i]) >= thresholdMM
				break
			}
		}
	}

	// Hourly forecast: the current hour and the next ones, at most 3 entries.
	if h := r.Hourly; h != nil {
		hourStart := now.Truncate(time.Hour).Unix()
		for i, t := range h.Time {
			if t < hourStart {
				continue
			}
			p := model.ForecastPoint{Time: time.Unix(t, 0).UTC()}
			if i < len(h.PrecipitationProbability) {
				p.PrecipitationProbability = h.PrecipitationProbability[i]
			}
			if i < len(h.Precipitation) {
				p.PrecipitationMM = val(h.Precipitation[i])
			}
			w.Forecast = append(w.Forecast, p)
			if len(w.Forecast) == 3 {
				break
			}
		}
	}
	return w, nil
}

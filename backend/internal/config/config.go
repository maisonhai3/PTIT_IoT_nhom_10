// Package config loads the backend configuration from environment variables (optionally seeded from a .env file).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr  string
	StaticDir string // empty: do not serve the front-end
	APIToken  string // empty: POST /api/command is open (LAN only)
	// CORSOrigins lists browser origins (e.g. http://localhost:5173) allowed to call the API from another origin.
	// Empty means same-origin only, which is what you want when this server serves the front-end itself.
	// "*" allows any origin: convenient for development, but any web page a LAN user visits could then drive the
	// awning, so avoid it once real hardware is attached. It also applies to WebSocket origins.
	CORSOrigins []string

	MQTTURL      string
	MQTTUser     string
	MQTTPassword string
	MQTTClientID string
	TopicPrefix  string // always ends with "/"

	DBPath string

	WeatherLat             float64
	WeatherLon             float64
	WeatherLocation        string
	WeatherBaseURL         string // empty = https://api.open-meteo.com (set for a self-hosted instance or a test double)
	WeatherPollInterval    time.Duration
	WeatherPublishInterval time.Duration // heartbeat to the device

	DeviceTimeout    time.Duration // no telemetry for this long => offline
	HistoryRetention time.Duration
	HistoryInterval  time.Duration // minimum spacing of stored telemetry rows

	LogLevel string
}

// Load reads the configuration through getenv (usually os.Getenv) and validates it.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	dur := func(key string, def time.Duration, min time.Duration) time.Duration {
		raw := get(key, "")
		if raw == "" {
			return def
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d < min {
			errs = append(errs, fmt.Errorf("%s=%q: need a duration >= %s (e.g. 5m)", key, raw, min))
			return def
		}
		return d
	}
	float := func(key string, def, lo, hi float64) float64 {
		raw := get(key, "")
		if raw == "" {
			return def
		}
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || f < lo || f > hi {
			errs = append(errs, fmt.Errorf("%s=%q: need a number in [%g, %g]", key, raw, lo, hi))
			return def
		}
		return f
	}

	c := Config{
		HTTPAddr:     get("HTTP_ADDR", ":8080"),
		APIToken:     get("API_TOKEN", ""),
		MQTTURL:      get("MQTT_URL", "tcp://localhost:1883"),
		MQTTUser:     get("MQTT_USER", ""),
		MQTTPassword: getenv("MQTT_PASSWORD"),
		MQTTClientID: get("MQTT_CLIENT_ID", "awning-backend"),
		TopicPrefix:  get("MQTT_TOPIC_PREFIX", "pkg/awning01/"),
		DBPath:       get("DB_PATH", "./awning.db"),

		// Default location: Hanoi.
		WeatherLat:             float("WEATHER_LAT", 21.0285, -90, 90),
		WeatherLon:             float("WEATHER_LON", 105.8542, -180, 180),
		WeatherLocation:        get("WEATHER_LOCATION", "Hà Nội"),
		WeatherBaseURL:         strings.TrimRight(get("WEATHER_BASE_URL", ""), "/"),
		WeatherPollInterval:    dur("WEATHER_POLL_INTERVAL", 5*time.Minute, time.Minute),
		WeatherPublishInterval: dur("WEATHER_PUBLISH_INTERVAL", time.Minute, time.Second),

		DeviceTimeout:    dur("DEVICE_TIMEOUT", 20*time.Second, 5*time.Second),
		HistoryRetention: dur("HISTORY_RETENTION", 7*24*time.Hour, time.Hour),
		HistoryInterval:  dur("HISTORY_INTERVAL", time.Minute, time.Second),

		LogLevel: strings.ToLower(get("LOG_LEVEL", "info")),
	}

	if !strings.HasSuffix(c.TopicPrefix, "/") {
		c.TopicPrefix += "/"
	}
	if strings.ContainsAny(c.TopicPrefix, "#+") {
		errs = append(errs, errors.New("MQTT_TOPIC_PREFIX must not contain wildcards (# or +)"))
	}
	if c.WeatherBaseURL != "" && !strings.HasPrefix(c.WeatherBaseURL, "http://") && !strings.HasPrefix(c.WeatherBaseURL, "https://") {
		errs = append(errs, fmt.Errorf("WEATHER_BASE_URL=%q: need an http(s) URL", c.WeatherBaseURL))
	}
	if !strings.Contains(c.MQTTURL, "://") {
		errs = append(errs, fmt.Errorf("MQTT_URL=%q: need a URL like tcp://host:1883", c.MQTTURL))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL=%q: use debug, info, warn or error", c.LogLevel))
	}

	for _, o := range strings.Split(get("CORS_ORIGINS", ""), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}
	c.StaticDir = ResolveStaticDir(get("STATIC_DIR", ""), dirHasIndex)

	return c, errors.Join(errs...)
}

// ResolveStaticDir returns the explicit directory when given, otherwise the first well-known location
// (running from backend/, from the repo root, or inside the Docker image) that contains an index.html.
func ResolveStaticDir(explicit string, hasIndex func(dir string) bool) string {
	if explicit != "" {
		return explicit
	}
	for _, cand := range []string{"../frontend", "./frontend", "/app/frontend"} {
		if hasIndex(cand) {
			return cand
		}
	}
	return ""
}

func dirHasIndex(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !st.IsDir()
}

// ParseDotEnv reads KEY=VALUE lines. Blank lines and lines starting with # are ignored; surrounding quotes are removed.
func ParseDotEnv(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, sc.Err()
}

// LoadDotEnv sets variables from the file into the process environment without overriding ones that are already set.
// A missing file is not an error.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	vars, err := ParseDotEnv(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for k, v := range vars {
		if _, set := os.LookupEnv(k); !set {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

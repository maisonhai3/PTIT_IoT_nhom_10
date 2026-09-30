package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.TopicPrefix != "pkg/awning01/" || c.MQTTURL != "tcp://localhost:1883" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.WeatherPollInterval != 5*time.Minute || c.DeviceTimeout != 20*time.Second {
		t.Fatalf("unexpected durations: %+v", c)
	}
	if len(c.CORSOrigins) != 0 {
		t.Fatalf("CORS must be same-origin only by default, got %v", c.CORSOrigins)
	}
}

func TestLoadOverridesAndPrefixSlash(t *testing.T) {
	c, err := Load(env(map[string]string{
		"MQTT_TOPIC_PREFIX": "home/awning",
		"WEATHER_LAT":       "10.8",
		"WEATHER_LON":       "106.7",
		"CORS_ORIGINS":      "http://a.test, http://b.test",
		"API_TOKEN":         " secret ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.TopicPrefix != "home/awning/" {
		t.Errorf("prefix = %q", c.TopicPrefix)
	}
	if c.WeatherLat != 10.8 || c.WeatherLon != 106.7 {
		t.Errorf("coords = %v,%v", c.WeatherLat, c.WeatherLon)
	}
	if strings.Join(c.CORSOrigins, "|") != "http://a.test|http://b.test" {
		t.Errorf("origins = %v", c.CORSOrigins)
	}
	if c.APIToken != "secret" {
		t.Errorf("token = %q", c.APIToken)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"lat out of range":  {"WEATHER_LAT": "123"},
		"lat not a number":  {"WEATHER_LAT": "abc"},
		"poll too fast":     {"WEATHER_POLL_INTERVAL": "5s"},
		"bad duration":      {"DEVICE_TIMEOUT": "soon"},
		"wildcard prefix":   {"MQTT_TOPIC_PREFIX": "pkg/#"},
		"mqtt url no proto": {"MQTT_URL": "localhost:1883"},
		"bad log level":     {"LOG_LEVEL": "loud"},
		"weather url":       {"WEATHER_BASE_URL": "localhost:9999"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(vars)); err == nil {
				t.Fatalf("expected an error for %v", vars)
			}
		})
	}
}

func TestLoadWeatherBaseURL(t *testing.T) {
	c, err := Load(env(map[string]string{"WEATHER_BASE_URL": "http://127.0.0.1:9999/"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.WeatherBaseURL != "http://127.0.0.1:9999" {
		t.Errorf("trailing slash must be trimmed: %q", c.WeatherBaseURL)
	}
	if c, _ := Load(env(nil)); c.WeatherBaseURL != "" {
		t.Errorf("default must be empty (real Open-Meteo), got %q", c.WeatherBaseURL)
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"WEATHER_LAT": "x", "LOG_LEVEL": "loud"}))
	if err == nil || !strings.Contains(err.Error(), "WEATHER_LAT") || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("want both problems reported, got %v", err)
	}
}

func TestResolveStaticDir(t *testing.T) {
	has := func(want string) func(string) bool { return func(d string) bool { return d == want } }
	if got := ResolveStaticDir("/x", has("")); got != "/x" {
		t.Errorf("explicit ignored: %q", got)
	}
	if got := ResolveStaticDir("", has("./frontend")); got != "./frontend" {
		t.Errorf("got %q", got)
	}
	if got := ResolveStaticDir("", has("nowhere")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseDotEnv(t *testing.T) {
	in := "# comment\n\nA=1\nexport B = two words \nC=\"quoted value\"\nD='single'\nE=a=b\n"
	got, err := ParseDotEnv(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two words", "C": "quoted value", "D": "single", "E": "a=b"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, err := ParseDotEnv(strings.NewReader("oops\n")); err == nil {
		t.Error("expected error for a line without '='")
	}
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("AWN_TEST_KEEP=file\nAWN_TEST_NEW=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWN_TEST_KEEP", "real")
	t.Cleanup(func() { os.Unsetenv("AWN_TEST_NEW") })
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("AWN_TEST_KEEP") != "real" {
		t.Error("existing variable was overridden")
	}
	if os.Getenv("AWN_TEST_NEW") != "file" {
		t.Error("new variable not loaded")
	}
	if err := LoadDotEnv(filepath.Join(dir, "missing.env")); err != nil {
		t.Errorf("missing file must not be an error: %v", err)
	}
}

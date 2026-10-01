package model

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

// The JSON examples in docs/mqtt-topics.md are what the firmware author reads. They must decode into the Go model
// with no unknown fields and pass validation, so the document and the code cannot drift apart unnoticed.
func TestMQTTDocExamplesMatchTheModel(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/mqtt-topics.md")
	if err != nil {
		t.Fatal(err)
	}
	block := func(heading string) []byte {
		re := regexp.MustCompile("(?s)### `" + heading + "`[^\n]*\n```json\n(.*?)\n```")
		m := re.FindSubmatch(doc)
		if m == nil {
			t.Fatalf("no ```json example under the heading for %q in docs/mqtt-topics.md", heading)
		}
		return m[1]
	}
	strict := func(raw []byte, into any) error {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		return dec.Decode(into)
	}

	var tel DeviceTelemetry
	if err := strict(block("telemetry"), &tel); err != nil {
		t.Fatalf("telemetry example does not match DeviceTelemetry: %v", err)
	}
	if err := tel.Validate(); err != nil {
		t.Fatalf("telemetry example is not valid: %v", err)
	}
	if tel.RSSI == nil || tel.UptimeS == nil {
		t.Error("the telemetry example should show the optional rssi/uptime_s fields")
	}
	if tel.RainLevel == nil || tel.RainWet == nil {
		t.Error("the telemetry example should show the rain sensor fields rain_level/rain_wet")
	}

	var w DeviceWeather
	if err := strict(block("weather"), &w); err != nil {
		t.Fatalf("weather example does not match DeviceWeather: %v", err)
	}
	if w.AgeS == 0 && !w.IsRaining && !w.RainExpected15m {
		t.Error("weather example decoded to all zero values: field names probably differ")
	}
}

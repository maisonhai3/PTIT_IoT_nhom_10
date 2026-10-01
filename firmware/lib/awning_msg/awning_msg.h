// JSON codec for the MQTT payloads in docs/mqtt-topics.md (ArduinoJson v7, header-only,
// so it also builds on the host for unit tests).
#pragma once
#include <stddef.h>
#include <stdint.h>

#include "awning_core.h"

namespace awning {

struct WeatherMsg {
  uint32_t ageS = 0;
  bool isRaining = false;
  bool rainExpected15m = false;
};

struct TelemetryData {
  bool tempValid = false;
  float temp = 0.0f;
  bool humidityValid = false;
  float humidity = 0.0f;
  int light = 0;
  // false = no plate fitted, disabled at build time, or no reading yet: rain_level and rain_wet are null.
  bool rainSensorValid = false;
  int rainLevel = 0;  // 0..4095, high = wet
  bool rainSensorWet = false;
  State state = State::Open;
  Mode mode = Mode::Auto;
  bool rain = false;
  RainSource rainSource = RainSource::None;
  int32_t weatherAgeS = -1;
  bool failSafe = false;
  uint32_t manualLeftS = 0;
  int rssi = 0;
  uint32_t uptimeS = 0;
};

// `cmd` payload: {"action":"open|close|auto|simulate_rain|clear_rain"}.
// Unknown/malformed payloads return false and leave *out untouched.
bool parseCommand(const char* json, size_t len, Action* out);

// `weather` payload: all of age_s (integer >= 0), is_raining, rain_expected_15m (booleans)
// must be present with the right type, otherwise false.
bool parseWeather(const char* json, size_t len, WeatherMsg* out);

// Writes the NUL-terminated telemetry JSON into buf. Returns its length, or 0 if it
// does not fit in cap (buf is then an empty string).
size_t formatTelemetry(char* buf, size_t cap, const TelemetryData& d);

}  // namespace awning

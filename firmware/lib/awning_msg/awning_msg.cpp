#include "awning_msg.h"

#include <ArduinoJson.h>
#include <math.h>

namespace awning {

namespace {

// null when invalid; otherwise a number rounded to 0.1, printed without ".0" when whole
// so the value is also valid for a consumer that decodes it as an integer.
void putNumber(JsonDocument& doc, const char* key, bool valid, float x) {
  if (!valid || (x - x) != 0.0f) {  // (x - x) != 0 is true for NaN and +-inf
    doc[key] = nullptr;
    return;
  }
  const float r = roundf(x * 10.0f) / 10.0f;
  if (r == floorf(r)) {
    doc[key] = static_cast<long>(r);
  } else {
    doc[key] = r;
  }
}

bool parseDocument(const char* json, size_t len, JsonDocument& doc) {
  if (json == nullptr || len == 0) return false;
  return deserializeJson(doc, json, len) == DeserializationError::Ok;
}

}  // namespace

bool parseCommand(const char* json, size_t len, Action* out) {
  if (out == nullptr) return false;
  JsonDocument doc;
  if (!parseDocument(json, len, doc)) return false;
  JsonVariantConst action = doc["action"];
  if (!action.is<const char*>()) return false;
  return parseAction(action.as<const char*>(), out);
}

bool parseWeather(const char* json, size_t len, WeatherMsg* out) {
  if (out == nullptr) return false;
  JsonDocument doc;
  if (!parseDocument(json, len, doc)) return false;
  JsonVariantConst age = doc["age_s"];
  JsonVariantConst raining = doc["is_raining"];
  JsonVariantConst expected = doc["rain_expected_15m"];
  // is<double>() is true for any JSON number (not for strings/bools/null).
  if (!age.is<double>() || !raining.is<bool>() || !expected.is<bool>()) return false;
  // Be liberal with the age: a backend that sends float seconds (42.7) must not silence
  // the weather feed, and an absurdly large age just means "very stale". Negative is nonsense.
  const double a = age.as<double>();
  if (!(a >= 0.0)) return false;  // also rejects NaN
  out->ageS = a >= 4294967295.0 ? 0xFFFFFFFFu : static_cast<uint32_t>(a);
  out->isRaining = raining.as<bool>();
  out->rainExpected15m = expected.as<bool>();
  return true;
}

size_t formatTelemetry(char* buf, size_t cap, const TelemetryData& d) {
  if (buf == nullptr || cap == 0) return 0;
  buf[0] = '\0';

  // Key order follows docs/mqtt-topics.md.
  JsonDocument doc;
  putNumber(doc, "temp", d.tempValid, d.temp);
  putNumber(doc, "humidity", d.humidityValid, d.humidity);
  doc["light"] = d.light;
  doc["state"] = toString(d.state);
  doc["mode"] = toString(d.mode);
  doc["rain"] = d.rain;
  doc["rain_source"] = toString(d.rainSource);
  doc["weather_age_s"] = d.weatherAgeS;
  doc["fail_safe"] = d.failSafe;
  doc["manual_left_s"] = d.manualLeftS;
  doc["rssi"] = d.rssi;
  doc["uptime_s"] = d.uptimeS;

  if (measureJson(doc) >= cap) return 0;  // never emit truncated JSON
  return serializeJson(doc, buf, cap);
}

}  // namespace awning

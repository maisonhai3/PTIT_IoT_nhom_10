// Host tests for lib/awning_msg (run: pio test -e native).
#include <ArduinoJson.h>
#include <math.h>
#include <stdint.h>
#include <string.h>
#include <unity.h>

#include <string>
#include <vector>

#include "awning_msg.h"

using namespace awning;

void setUp(void) {}
void tearDown(void) {}

namespace {

bool cmd(const char* json, Action* out) { return parseCommand(json, strlen(json), out); }
bool wx(const char* json, WeatherMsg* out) { return parseWeather(json, strlen(json), out); }

TelemetryData sample() {
  // The example from docs/mqtt-topics.md.
  TelemetryData d;
  d.tempValid = true;
  d.temp = 29.5f;
  d.humidityValid = true;
  d.humidity = 71.0f;
  d.light = 2300;
  d.rainSensorValid = true;
  d.rainLevel = 30;
  d.rainSensorWet = false;
  d.state = State::Open;
  d.mode = Mode::Auto;
  d.rain = false;
  d.rainSource = RainSource::None;
  d.weatherAgeS = 45;
  d.failSafe = false;
  d.manualLeftS = 0;
  d.rssi = -58;
  d.uptimeS = 1234;
  return d;
}

std::string format(const TelemetryData& d) {
  char buf[384];
  const size_t n = formatTelemetry(buf, sizeof(buf), d);
  TEST_ASSERT_TRUE(n > 0);
  TEST_ASSERT_EQUAL_UINT32(strlen(buf), n);
  return std::string(buf);
}

}  // namespace

// ---- cmd ------------------------------------------------------------------------

void test_parse_command_accepts_all_actions() {
  const struct {
    const char* json;
    Action expected;
  } cases[] = {
      {"{\"action\":\"open\"}", Action::Open},
      {"{\"action\":\"close\"}", Action::Close},
      {"{\"action\":\"auto\"}", Action::Auto},
      {"{\"action\":\"simulate_rain\"}", Action::SimulateRain},
      {"{\"action\":\"clear_rain\"}", Action::ClearRain},
  };
  for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); ++i) {
    Action a = (cases[i].expected == Action::Open) ? Action::Close : Action::Open;
    TEST_ASSERT_TRUE(cmd(cases[i].json, &a));
    TEST_ASSERT_EQUAL_INT(static_cast<int>(cases[i].expected), static_cast<int>(a));
  }
}

void test_parse_command_tolerates_whitespace_and_extra_keys() {
  Action a = Action::Auto;
  TEST_ASSERT_TRUE(cmd("  {\n \"action\" : \"open\" \n}  ", &a));
  TEST_ASSERT_EQUAL_INT(static_cast<int>(Action::Open), static_cast<int>(a));
  TEST_ASSERT_TRUE(cmd("{\"id\":7,\"action\":\"close\",\"by\":\"web\"}", &a));
  TEST_ASSERT_EQUAL_INT(static_cast<int>(Action::Close), static_cast<int>(a));
}

void test_parse_command_rejects_bad_payloads_and_leaves_out_untouched() {
  const char* bad[] = {
      "",
      "not json",
      "{",
      "{\"action\":",
      "{\"action\":\"open\"",       // truncated
      "{\"action\":\"OPEN\"}",      // case-sensitive
      "{\"action\":\"toggle\"}",    // unknown action
      "{\"action\":\"\"}",
      "{\"action\":\"open \"}",
      "{\"action\":1}",
      "{\"action\":null}",
      "{\"action\":true}",
      "{\"action\":[\"open\"]}",
      "{\"action\":{\"a\":\"open\"}}",
      "{\"Action\":\"open\"}",
      "{\"cmd\":\"open\"}",
      "{}",
      "[]",
      "[\"open\"]",
      "\"open\"",
      "open",
      "42",
      "null",
  };
  for (size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); ++i) {
    Action a = Action::Close;
    TEST_ASSERT_FALSE_MESSAGE(cmd(bad[i], &a), bad[i]);
    TEST_ASSERT_EQUAL_INT_MESSAGE(static_cast<int>(Action::Close), static_cast<int>(a), bad[i]);
  }
  Action a = Action::Close;
  TEST_ASSERT_FALSE(parseCommand(nullptr, 5, &a));
  TEST_ASSERT_FALSE(parseCommand("{\"action\":\"open\"}", 0, &a));
  TEST_ASSERT_FALSE(parseCommand("{\"action\":\"open\"}", 17, nullptr));
}

void test_parse_command_honours_the_length_argument() {
  // MQTT payloads are not NUL-terminated: only the first `len` bytes count.
  char buf[64];
  memset(buf, 'x', sizeof(buf));
  const char* json = "{\"action\":\"open\"}";
  memcpy(buf, json, strlen(json));  // followed by junk, no terminator
  Action a = Action::Close;
  TEST_ASSERT_TRUE(parseCommand(buf, strlen(json), &a));
  TEST_ASSERT_EQUAL_INT(static_cast<int>(Action::Open), static_cast<int>(a));
  a = Action::Close;
  TEST_ASSERT_FALSE(parseCommand(buf, strlen(json) - 3, &a));  // cut in the middle
  TEST_ASSERT_EQUAL_INT(static_cast<int>(Action::Close), static_cast<int>(a));
}

// ---- weather --------------------------------------------------------------------

void test_parse_weather_valid() {
  WeatherMsg w;
  TEST_ASSERT_TRUE(wx("{\"age_s\":42,\"is_raining\":false,\"rain_expected_15m\":true}", &w));
  TEST_ASSERT_EQUAL_UINT32(42, w.ageS);
  TEST_ASSERT_FALSE(w.isRaining);
  TEST_ASSERT_TRUE(w.rainExpected15m);

  TEST_ASSERT_TRUE(wx("{\"rain_expected_15m\":false,\"is_raining\":true,\"age_s\":0}", &w));
  TEST_ASSERT_EQUAL_UINT32(0, w.ageS);
  TEST_ASSERT_TRUE(w.isRaining);
  TEST_ASSERT_FALSE(w.rainExpected15m);

  TEST_ASSERT_TRUE(wx("{\"age_s\":4294967295,\"is_raining\":true,\"rain_expected_15m\":true,\"extra\":[1]}", &w));
  TEST_ASSERT_EQUAL_UINT32(4294967295u, w.ageS);
}

void test_parse_weather_rejects_missing_wrong_type_and_malformed() {
  const char* bad[] = {
      // missing fields
      "{}",
      "{\"is_raining\":true,\"rain_expected_15m\":true}",
      "{\"age_s\":1,\"rain_expected_15m\":true}",
      "{\"age_s\":1,\"is_raining\":true}",
      // wrong types
      "{\"age_s\":\"42\",\"is_raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":null,\"is_raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":true,\"is_raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":-1,\"is_raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"is_raining\":\"false\",\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"is_raining\":0,\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"is_raining\":null,\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"is_raining\":false,\"rain_expected_15m\":1}",
      "{\"age_s\":42,\"is_raining\":false,\"rain_expected_15m\":\"true\"}",
      "{\"age_s\":[42],\"is_raining\":false,\"rain_expected_15m\":false}",
      // wrong names
      "{\"age\":42,\"is_raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"raining\":false,\"rain_expected_15m\":false}",
      "{\"age_s\":42,\"is_raining\":false,\"rain_expected\":false}",
      // not an object / malformed
      "",
      "[]",
      "42",
      "null",
      "\"x\"",
      "{",
      "not json",
      "{\"age_s\":42,\"is_raining\":false,\"rain_expected_15m\":false",  // truncated
  };
  for (size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); ++i) {
    WeatherMsg w;
    w.ageS = 777;
    w.isRaining = true;
    w.rainExpected15m = true;
    TEST_ASSERT_FALSE_MESSAGE(wx(bad[i], &w), bad[i]);
    TEST_ASSERT_EQUAL_UINT32_MESSAGE(777, w.ageS, bad[i]);  // untouched on failure
    TEST_ASSERT_TRUE(w.isRaining);
    TEST_ASSERT_TRUE(w.rainExpected15m);
  }
  WeatherMsg w;
  TEST_ASSERT_FALSE(parseWeather(nullptr, 3, &w));
  TEST_ASSERT_FALSE(parseWeather("{}", 0, &w));
  TEST_ASSERT_FALSE(parseWeather("{\"age_s\":1,\"is_raining\":true,\"rain_expected_15m\":true}", 50, nullptr));
}

void test_parse_weather_fractional_and_huge_ages() {
  WeatherMsg w;
  // A backend that computes the age in float seconds must not silence the weather feed.
  TEST_ASSERT_TRUE(wx("{\"age_s\":42.9,\"is_raining\":false,\"rain_expected_15m\":false}", &w));
  TEST_ASSERT_EQUAL_UINT32(42, w.ageS);
  TEST_ASSERT_TRUE(wx("{\"age_s\":42.0,\"is_raining\":false,\"rain_expected_15m\":false}", &w));
  TEST_ASSERT_EQUAL_UINT32(42, w.ageS);
  // Astronomically old data is "very stale", not an error: clamp.
  TEST_ASSERT_TRUE(wx("{\"age_s\":1e20,\"is_raining\":false,\"rain_expected_15m\":false}", &w));
  TEST_ASSERT_EQUAL_UINT32(4294967295u, w.ageS);
  TEST_ASSERT_TRUE(wx("{\"age_s\":99999999999,\"is_raining\":false,\"rain_expected_15m\":false}", &w));
  TEST_ASSERT_EQUAL_UINT32(4294967295u, w.ageS);
}

// ---- telemetry ------------------------------------------------------------------

void test_telemetry_matches_the_documented_example_byte_for_byte() {
  TEST_ASSERT_EQUAL_STRING(
      "{\"temp\":29.5,\"humidity\":71,\"light\":2300,\"rain_level\":30,\"rain_wet\":false,"
      "\"state\":\"OPEN\",\"mode\":\"AUTO\","
      "\"rain\":false,\"rain_source\":\"none\",\"weather_age_s\":45,\"fail_safe\":false,"
      "\"manual_left_s\":0,\"rssi\":-58,\"uptime_s\":1234}",
      format(sample()).c_str());
}

void test_telemetry_has_exactly_the_documented_keys_in_order() {
  JsonDocument doc;
  const std::string out = format(sample());
  TEST_ASSERT_TRUE(deserializeJson(doc, out) == DeserializationError::Ok);
  const char* expected[] = {"temp", "humidity", "light", "rain_level", "rain_wet", "state", "mode", "rain",
                            "rain_source", "weather_age_s", "fail_safe", "manual_left_s",
                            "rssi", "uptime_s"};
  const size_t n = sizeof(expected) / sizeof(expected[0]);
  TEST_ASSERT_EQUAL_UINT32(n, doc.as<JsonObject>().size());
  size_t i = 0;
  for (JsonPair kv : doc.as<JsonObject>()) {
    TEST_ASSERT_TRUE(i < n);
    TEST_ASSERT_EQUAL_STRING(expected[i], kv.key().c_str());
    ++i;
  }
  TEST_ASSERT_EQUAL_UINT32(n, i);
}

void test_telemetry_invalid_sensors_are_json_null() {
  TelemetryData d = sample();
  d.tempValid = false;
  d.humidityValid = false;
  d.temp = 123;      // values are ignored when invalid
  d.humidity = 456;
  JsonDocument doc;
  TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
  TEST_ASSERT_TRUE(doc["temp"].isNull());
  TEST_ASSERT_TRUE(doc["humidity"].isNull());
  TEST_ASSERT_TRUE(doc["temp"].is<JsonVariant>());  // key present (value null)...
  TEST_ASSERT_TRUE(doc["humidity"].is<JsonVariant>());
  TEST_ASSERT_FALSE(doc["no_such_key"].is<JsonVariant>());  // ...unlike a missing one
  TEST_ASSERT_TRUE(format(d).find("\"temp\":null,\"humidity\":null,") != std::string::npos);

  d = sample();
  d.temp = NAN;
  d.humidity = INFINITY;
  TEST_ASSERT_TRUE(format(d).find("\"temp\":null,\"humidity\":null,") != std::string::npos);
}

void test_telemetry_never_seen_weather_is_minus_one() {
  TelemetryData d = sample();
  d.weatherAgeS = -1;
  const std::string s = format(d);
  TEST_ASSERT_TRUE(s.find("\"weather_age_s\":-1,") != std::string::npos);
}

void test_telemetry_enum_spellings() {
  const State states[] = {State::Open, State::Closing, State::Closed, State::Opening, State::Error};
  const char* stateNames[] = {"OPEN", "CLOSING", "CLOSED", "OPENING", "ERROR"};
  for (int i = 0; i < 5; ++i) {
    TelemetryData d = sample();
    d.state = states[i];
    JsonDocument doc;
    TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
    TEST_ASSERT_EQUAL_STRING(stateNames[i], doc["state"].as<const char*>());
  }
  const RainSource srcs[] = {RainSource::None, RainSource::Api, RainSource::Sim, RainSource::Local, RainSource::Sensor};
  const char* srcNames[] = {"none", "api", "sim", "local", "sensor"};
  for (int i = 0; i < 5; ++i) {
    TelemetryData d = sample();
    d.rainSource = srcs[i];
    JsonDocument doc;
    TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
    TEST_ASSERT_EQUAL_STRING(srcNames[i], doc["rain_source"].as<const char*>());
  }
  TelemetryData d = sample();
  d.mode = Mode::Manual;
  d.manualLeftS = 543;
  JsonDocument doc;
  TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
  TEST_ASSERT_EQUAL_STRING("MANUAL", doc["mode"].as<const char*>());
  TEST_ASSERT_EQUAL_UINT32(543, doc["manual_left_s"].as<uint32_t>());
}

void test_telemetry_value_types() {
  TelemetryData d = sample();
  d.rain = true;
  d.failSafe = true;
  d.rainSource = RainSource::Local;
  JsonDocument doc;
  TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
  TEST_ASSERT_TRUE(doc["temp"].is<float>());
  TEST_ASSERT_TRUE(doc["humidity"].is<int>());
  TEST_ASSERT_TRUE(doc["light"].is<int>());
  TEST_ASSERT_TRUE(doc["rain_level"].is<int>());
  TEST_ASSERT_TRUE(doc["rain_wet"].is<bool>());
  TEST_ASSERT_TRUE(doc["state"].is<const char*>());
  TEST_ASSERT_TRUE(doc["mode"].is<const char*>());
  TEST_ASSERT_TRUE(doc["rain"].is<bool>());
  TEST_ASSERT_TRUE(doc["rain"].as<bool>());
  TEST_ASSERT_TRUE(doc["rain_source"].is<const char*>());
  TEST_ASSERT_TRUE(doc["weather_age_s"].is<int>());
  TEST_ASSERT_TRUE(doc["fail_safe"].is<bool>());
  TEST_ASSERT_TRUE(doc["fail_safe"].as<bool>());
  TEST_ASSERT_TRUE(doc["manual_left_s"].is<int>());
  TEST_ASSERT_TRUE(doc["rssi"].is<int>());
  TEST_ASSERT_TRUE(doc["uptime_s"].is<int>());
}

void test_telemetry_rain_plate_fields() {
  TelemetryData d = sample();
  d.rainLevel = 2750;
  d.rainSensorWet = true;
  JsonDocument doc;
  TEST_ASSERT_TRUE(deserializeJson(doc, format(d)) == DeserializationError::Ok);
  TEST_ASSERT_EQUAL_INT(2750, doc["rain_level"].as<int>());
  TEST_ASSERT_TRUE(doc["rain_wet"].as<bool>());

  // The backend rejects a whole telemetry message whose rain_level is outside 0..4095: clamp instead.
  d.rainLevel = 5000;
  TEST_ASSERT_TRUE(format(d).find("\"rain_level\":4095,") != std::string::npos);
  d.rainLevel = -7;
  TEST_ASSERT_TRUE(format(d).find("\"rain_level\":0,") != std::string::npos);
  d.rainLevel = 0;
  d.rainSensorWet = false;
  TEST_ASSERT_TRUE(format(d).find("\"rain_level\":0,\"rain_wet\":false,") != std::string::npos);
}

void test_telemetry_without_a_plate_has_null_rain_fields() {
  TelemetryData d = sample();
  d.rainSensorValid = false;  // not fitted, disabled at build time, or no reading yet
  d.rainLevel = 999;          // ignored
  d.rainSensorWet = true;     // ignored
  JsonDocument doc;
  const std::string out = format(d);
  TEST_ASSERT_TRUE(deserializeJson(doc, out) == DeserializationError::Ok);
  TEST_ASSERT_TRUE(doc["rain_level"].isNull());
  TEST_ASSERT_TRUE(doc["rain_wet"].isNull());
  TEST_ASSERT_TRUE(doc["rain_level"].is<JsonVariant>());  // the keys are still there, with null values
  TEST_ASSERT_TRUE(doc["rain_wet"].is<JsonVariant>());
  TEST_ASSERT_TRUE(out.find("\"light\":2300,\"rain_level\":null,\"rain_wet\":null,\"state\"") != std::string::npos);
}

void test_telemetry_number_formatting() {
  TelemetryData d = sample();
  d.temp = 28.34f;
  d.humidity = 71.46f;
  std::string s = format(d);
  TEST_ASSERT_TRUE(s.find("\"temp\":28.3,\"humidity\":71.5,") != std::string::npos);

  d.temp = -3.0f;  // whole numbers have no ".0"
  d.humidity = 100.0f;
  s = format(d);
  TEST_ASSERT_TRUE(s.find("\"temp\":-3,\"humidity\":100,") != std::string::npos);

  d.temp = -0.04f;  // no "-0"
  d.humidity = 0.0f;
  s = format(d);
  TEST_ASSERT_TRUE(s.find("\"temp\":0,\"humidity\":0,") != std::string::npos);

  d.temp = 33.0f;
  d.light = 0;
  s = format(d);
  TEST_ASSERT_TRUE(s.find("\"temp\":33,") != std::string::npos);
  TEST_ASSERT_TRUE(s.find("\"light\":0,") != std::string::npos);
  d.light = 4095;
  TEST_ASSERT_TRUE(format(d).find("\"light\":4095,") != std::string::npos);
}

void test_telemetry_fits_in_384_bytes_in_the_worst_case() {
  TelemetryData d;
  d.tempValid = true;
  d.temp = -40.5f;
  d.humidityValid = true;
  d.humidity = 100.0f;
  d.light = 4095;
  d.state = State::Opening;  // longest state name
  d.mode = Mode::Manual;
  d.rain = true;
  d.rainSource = RainSource::Local;
  d.weatherAgeS = 4294967;
  d.failSafe = true;
  d.manualLeftS = 600;
  d.rssi = -100;
  d.uptimeS = 4294967295u;
  char buf[384];
  const size_t n = formatTelemetry(buf, sizeof(buf), d);
  TEST_ASSERT_TRUE(n > 0);
  TEST_ASSERT_TRUE(n < 300);  // comfortably below both 384 and PubSubClient's 512 buffer
}

void test_telemetry_never_returns_truncated_json() {
  const TelemetryData d = sample();
  char big[384];
  const size_t n = formatTelemetry(big, sizeof(big), d);
  TEST_ASSERT_TRUE(n > 0);

  char* exact = new char[n + 1];
  TEST_ASSERT_EQUAL_UINT32(n, formatTelemetry(exact, n + 1, d));  // room for the NUL: fits
  TEST_ASSERT_EQUAL_STRING(big, exact);
  delete[] exact;

  char* tight = new char[n];
  memset(tight, 'z', n);
  TEST_ASSERT_EQUAL_UINT32(0, formatTelemetry(tight, n, d));  // no room for the NUL: refuse
  TEST_ASSERT_EQUAL_STRING("", tight);
  delete[] tight;

  char tiny[8];
  TEST_ASSERT_EQUAL_UINT32(0, formatTelemetry(tiny, sizeof(tiny), d));
  TEST_ASSERT_EQUAL_STRING("", tiny);
  TEST_ASSERT_EQUAL_UINT32(0, formatTelemetry(tiny, 0, d));
  TEST_ASSERT_EQUAL_UINT32(0, formatTelemetry(nullptr, 10, d));
}

int main(int, char**) {
  UNITY_BEGIN();
  RUN_TEST(test_parse_command_accepts_all_actions);
  RUN_TEST(test_parse_command_tolerates_whitespace_and_extra_keys);
  RUN_TEST(test_parse_command_rejects_bad_payloads_and_leaves_out_untouched);
  RUN_TEST(test_parse_command_honours_the_length_argument);
  RUN_TEST(test_parse_weather_valid);
  RUN_TEST(test_parse_weather_rejects_missing_wrong_type_and_malformed);
  RUN_TEST(test_parse_weather_fractional_and_huge_ages);
  RUN_TEST(test_telemetry_matches_the_documented_example_byte_for_byte);
  RUN_TEST(test_telemetry_has_exactly_the_documented_keys_in_order);
  RUN_TEST(test_telemetry_invalid_sensors_are_json_null);
  RUN_TEST(test_telemetry_never_seen_weather_is_minus_one);
  RUN_TEST(test_telemetry_enum_spellings);
  RUN_TEST(test_telemetry_value_types);
  RUN_TEST(test_telemetry_rain_plate_fields);
  RUN_TEST(test_telemetry_without_a_plate_has_null_rain_fields);
  RUN_TEST(test_telemetry_number_formatting);
  RUN_TEST(test_telemetry_fits_in_384_bytes_in_the_worst_case);
  RUN_TEST(test_telemetry_never_returns_truncated_json);
  return UNITY_END();
}

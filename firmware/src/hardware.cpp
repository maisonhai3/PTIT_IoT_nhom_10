#include "hardware.h"

#include <Adafruit_GFX.h>
#include <Adafruit_SSD1306.h>
#include <DHT.h>
#include <Wire.h>

#include "awning_core.h"
#include "config.h"

namespace hw {

namespace {

// ---- relays -------------------------------------------------------------------

void relayWrite(uint8_t pin, bool on) {
  // Active-low board: LOW energises the relay.
  digitalWrite(pin, (on != (RELAY_ACTIVE_LOW != 0)) ? HIGH : LOW);
}

// ---- sensors ------------------------------------------------------------------

DHT g_dht(PIN_DHT, DHT11);
SensorData g_sensors;
uint8_t g_dhtFailures = 0;
awning::Every g_dhtTimer(DHT_PERIOD_MS);
awning::Every g_ldrTimer(LDR_PERIOD_MS);
bool g_lightInit = false;
int g_lightAcc = 0;  // smoothed light * 16: the extra bits let the filter converge exactly

void readDht() {
  const float t = g_dht.readTemperature();
  const float h = g_dht.readHumidity();
  if (isnan(t) || isnan(h)) {
    if (g_dhtFailures < 255) ++g_dhtFailures;
    if (g_dhtFailures >= DHT_MAX_FAILURES) {  // keep the last good value until then
      g_sensors.tempValid = false;
      g_sensors.humidityValid = false;
    }
    return;
  }
  g_dhtFailures = 0;
  g_sensors.temp = t;
  g_sensors.humidity = h;
  g_sensors.tempValid = true;
  g_sensors.humidityValid = true;
}

void readLdr() {
  uint32_t sum = 0;
  for (int i = 0; i < LDR_SAMPLES; ++i) sum += analogRead(PIN_LDR);
  const int raw = static_cast<int>(sum / LDR_SAMPLES);
  const int bright = LDR_INVERT ? 4095 - raw : raw;  // contract: high = bright
  g_sensors.lightRaw = raw;
  // Light smoothing so noise around LOCAL_DARK_BELOW does not flap the rain flag.
  const int target = bright * 16;
  g_lightAcc = g_lightInit ? g_lightAcc + (target - g_lightAcc) / 4 : target;
  g_lightInit = true;
  g_sensors.light = g_lightAcc / 16;
}

#if RAIN_SENSOR_ENABLED
// ---- rain plate (YL-83) -------------------------------------------------------

awning::Every g_rainTimer(RAIN_PERIOD_MS);
#if RAIN_PWR_PIN >= 0
bool g_rainPowered = false;  // the module is on and settling; the reading is taken once RAIN_SETTLE_MS passed
uint32_t g_rainPoweredAtMs = 0;
#endif

void sampleRain() {
  uint32_t sum = 0;
  for (int i = 0; i < RAIN_SAMPLES; ++i) sum += analogRead(PIN_RAIN_AO);
  const int raw = static_cast<int>(sum / RAIN_SAMPLES);
  const int level = RAIN_INVERT ? raw : 4095 - raw;  // contract: high = wet
  g_sensors.rainRaw = raw;
  g_sensors.rainLevel = level < 0 ? 0 : level > 4095 ? 4095 : level;
  g_sensors.rainValid = true;
}

// Never blocks. With RAIN_PWR_PIN the module only has power for RAIN_SETTLE_MS around a reading
// (the plate corrodes while it is wet and powered); the hysteresis and the confirm time live in the controller.
void updateRain(uint32_t nowMs) {
#if RAIN_PWR_PIN >= 0
  if (!g_rainPowered) {
    if (!g_rainTimer.due(nowMs)) return;
    digitalWrite(RAIN_PWR_PIN, HIGH);
    g_rainPowered = true;
    g_rainPoweredAtMs = nowMs;
    return;
  }
  if (awning::elapsedMs(nowMs, g_rainPoweredAtMs) < RAIN_SETTLE_MS) return;
  sampleRain();
  digitalWrite(RAIN_PWR_PIN, LOW);
  g_rainPowered = false;
#else
  if (g_rainTimer.due(nowMs)) sampleRain();
#endif
}
#endif  // RAIN_SENSOR_ENABLED

// ---- buzzer -------------------------------------------------------------------

enum class BeepPattern : uint8_t { None, Single, Triple };
BeepPattern g_pattern = BeepPattern::None;
uint32_t g_patternStartMs = 0;
bool g_alarm = false;
uint32_t g_alarmStartMs = 0;
bool g_buzzerOn = false;

// ---- OLED ---------------------------------------------------------------------

Adafruit_SSD1306 g_oled(OLED_WIDTH, OLED_HEIGHT, &Wire, -1);
bool g_oledOk = false;

const char* stateText(awning::State s) {
  switch (s) {
    case awning::State::Open: return "DA MO";
    case awning::State::Closing: return "DANG THU";
    case awning::State::Closed: return "DA THU";
    case awning::State::Opening: return "DANG MO";
    case awning::State::Error: break;
  }
  return "LOI";
}

}  // namespace

void relaysOffEarly() {
  // Latch the OFF level first, then switch to output: no glitch on an active-low board.
  relayWrite(PIN_RELAY_CLOSE, false);
  relayWrite(PIN_RELAY_OPEN, false);
  pinMode(PIN_RELAY_CLOSE, OUTPUT);
  pinMode(PIN_RELAY_OPEN, OUTPUT);
  relayWrite(PIN_RELAY_CLOSE, false);
  relayWrite(PIN_RELAY_OPEN, false);
}

void begin() {
  pinMode(PIN_LIMIT_CLOSED, INPUT_PULLUP);
  pinMode(PIN_LIMIT_OPEN, INPUT_PULLUP);
  pinMode(PIN_BTN_MANUAL, INPUT_PULLUP);
  digitalWrite(PIN_BUZZER, LOW);
  pinMode(PIN_BUZZER, OUTPUT);
  digitalWrite(PIN_BUZZER, LOW);

  analogReadResolution(12);
  analogSetPinAttenuation(PIN_LDR, ADC_11db);  // ~0..3.1 V: covers a 3.3 V divider
#if RAIN_SENSOR_ENABLED
  analogSetPinAttenuation(PIN_RAIN_AO, ADC_11db);  // AO swings between 0 V and VCC (3.3 V)
#if RAIN_PWR_PIN >= 0
  digitalWrite(RAIN_PWR_PIN, LOW);  // module off until the first reading
  pinMode(RAIN_PWR_PIN, OUTPUT);
  digitalWrite(RAIN_PWR_PIN, LOW);
#endif
#endif

  g_dht.begin();
  g_dhtTimer.startIn(millis(), 1500);  // DHT11 needs ~1 s after power-up

  Wire.begin(PIN_I2C_SDA, PIN_I2C_SCL);
  Wire.beginTransmission(OLED_I2C_ADDR);
  const bool present = Wire.endTransmission() == 0;
  if (present && g_oled.begin(SSD1306_SWITCHCAPVCC, OLED_I2C_ADDR, true, false /*Wire already started*/)) {
    g_oledOk = true;
    g_oled.clearDisplay();
    g_oled.display();
  } else {
    Serial.printf("[hw] OLED not found at 0x%02X (check wiring/address); continuing without it\n",
                  OLED_I2C_ADDR);
  }
}

void setRelays(bool close, bool open) {
  if (close && open) close = open = false;  // interlock, the controller never asks for this
  if (!close) relayWrite(PIN_RELAY_CLOSE, false);
  if (!open) relayWrite(PIN_RELAY_OPEN, false);
  if (close) relayWrite(PIN_RELAY_CLOSE, true);
  if (open) relayWrite(PIN_RELAY_OPEN, true);
}

bool rawLimitClosed() { return digitalRead(PIN_LIMIT_CLOSED) == LOW; }
bool rawLimitOpen() { return digitalRead(PIN_LIMIT_OPEN) == LOW; }
bool rawManualButton() { return digitalRead(PIN_BTN_MANUAL) == LOW; }

void sensorsUpdate(uint32_t nowMs) {
  if (g_ldrTimer.due(nowMs)) readLdr();
#if RAIN_SENSOR_ENABLED
  updateRain(nowMs);
#endif
  if (g_dhtTimer.due(nowMs)) readDht();
}

SensorData sensors() { return g_sensors; }

void buzzerBeep(uint32_t nowMs) {
  g_pattern = BeepPattern::Triple;
  g_patternStartMs = nowMs;
}

void buzzerClick(uint32_t nowMs) {
  g_pattern = BeepPattern::Single;
  g_patternStartMs = nowMs;
}

void buzzerAlarm(uint32_t nowMs, bool on) {
  if (on && !g_alarm) g_alarmStartMs = nowMs;
  g_alarm = on;
}

void buzzerUpdate(uint32_t nowMs) {
  bool on = false;
  if (g_alarm) {
    on = awning::elapsedMs(nowMs, g_alarmStartMs) % (ALARM_ON_MS + ALARM_OFF_MS) < ALARM_ON_MS;
  } else if (g_pattern != BeepPattern::None) {
    const uint32_t t = awning::elapsedMs(nowMs, g_patternStartMs);
    const uint32_t period = BEEP_ON_MS + BEEP_OFF_MS;
    const uint32_t beeps = g_pattern == BeepPattern::Triple ? 3 : 1;
    if (t >= beeps * period) {
      g_pattern = BeepPattern::None;
    } else {
      on = (t % period) < BEEP_ON_MS;
    }
  }
  if (on != g_buzzerOn) {
    g_buzzerOn = on;
    digitalWrite(PIN_BUZZER, on ? HIGH : LOW);
  }
}

void oledDraw(const awning::Snapshot& s, const SensorData& d, const NetStatus& n) {
  if (!g_oledOk) return;
  char line[32];

  g_oled.clearDisplay();
  g_oled.setTextColor(SSD1306_WHITE);

  g_oled.setTextSize(2);
  g_oled.setCursor(0, 0);
  g_oled.print(stateText(s.state));

  g_oled.setTextSize(1);

  char temp[8] = "--";
  char hum[8] = "--";
  if (d.tempValid) snprintf(temp, sizeof(temp), "%.0f", d.temp);
  if (d.humidityValid) snprintf(hum, sizeof(hum), "%.0f", d.humidity);
  snprintf(line, sizeof(line), "T:%sC H:%s%% L:%d", temp, hum, d.light);
  g_oled.setCursor(0, 20);
  g_oled.print(line);

  if (s.mode == awning::Mode::Manual) {
    snprintf(line, sizeof(line), "Che do: TAY (%us)", static_cast<unsigned>(s.manualLeftS));
  } else {
    snprintf(line, sizeof(line), "Che do: TU DONG");
  }
  g_oled.setCursor(0, 32);
  g_oled.print(line);

  const char* rain = s.rain ? awning::toString(s.rainSource) : "khong";
  if (s.weatherAgeS >= 0) {
    snprintf(line, sizeof(line), "Mua:%s %s%ds", rain, s.failSafe ? "CU " : "", static_cast<int>(s.weatherAgeS));
  } else {
    snprintf(line, sizeof(line), "Mua:%s chua co TT", rain);
  }
  g_oled.setCursor(0, 44);
  g_oled.print(line);

  snprintf(line, sizeof(line), "WiFi:%s MQTT:%s", n.wifi ? "OK" : "--", n.mqtt ? "OK" : "--");
  g_oled.setCursor(0, 56);
  g_oled.print(line);

  g_oled.display();
}

}  // namespace hw

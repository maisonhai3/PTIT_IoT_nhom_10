// ESP32 firmware for the smart clothes-drying awning (simulated with 2 relays + LEDs).
//
// Architecture: loop() is the control task on core 1, 20 ms tick, and NEVER waits for the
// network. The network task (net.cpp) runs on core 0; the two only talk through a
// non-blocking queue and a mutex-guarded struct (shared.h). A stalled WiFi/MQTT connect
// therefore cannot delay stopping a relay when a limit switch is hit.
#include <Arduino.h>
#include <esp_system.h>
#include <esp_task_wdt.h>
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>

#include "app_config.h"
#include "awning_core.h"
#include "config.h"
#include "hardware.h"
#include "net.h"
#include "shared.h"

using awning::Action;
using awning::Snapshot;
using awning::State;

namespace {

awning::Controller g_controller(appConfig());
awning::Button g_btnManual(BUTTON_DEBOUNCE_MS, BUTTON_LONG_PRESS_MS);
awning::Button g_btnClosed(BUTTON_DEBOUNCE_MS, BUTTON_LONG_PRESS_MS);  // limit switches: debounced level only
awning::Button g_btnOpen(BUTTON_DEBOUNCE_MS, BUTTON_LONG_PRESS_MS);

awning::Every g_oledTimer(OLED_PERIOD_MS);
awning::Every g_logTimer(SERIAL_LOG_PERIOD_MS);
TickType_t g_lastWake = 0;

const char* resetReasonName() {
  switch (esp_reset_reason()) {
    case ESP_RST_POWERON: return "poweron";
    case ESP_RST_SW: return "software";
    case ESP_RST_PANIC: return "panic";
    case ESP_RST_INT_WDT: return "int_wdt";
    case ESP_RST_TASK_WDT: return "task_wdt";
    case ESP_RST_WDT: return "wdt";
    case ESP_RST_BROWNOUT: return "brownout";
    default: return "other";
  }
}

void readSwitches(uint32_t now) {
  g_btnClosed.update(now, hw::rawLimitClosed());
  g_btnOpen.update(now, hw::rawLimitOpen());
}

void logStatus(const Snapshot& s, const SensorData& d, const NetStatus& n) {
  char temp[8] = "--";
  char hum[8] = "--";
  if (d.tempValid) snprintf(temp, sizeof(temp), "%.1f", d.temp);
  if (d.humidityValid) snprintf(hum, sizeof(hum), "%.0f", d.humidity);
  // light_raw is what you need to calibrate LDR_INVERT / LOCAL_DARK_BELOW (see README);
  // rain_level and wet are what you need to calibrate RAIN_WET_ABOVE / RAIN_DRY_BELOW ("-" = no plate).
  char plate[24] = "rain_level=- wet=-";
  if (d.rainValid) snprintf(plate, sizeof(plate), "rain_level=%d wet=%d", d.rainLevel, s.rainSensorWet ? 1 : 0);
  Serial.printf("[ctl] light_raw=%d light=%d %s T=%s H=%s state=%s mode=%s rain=%s src=%s age=%ds fs=%d wifi=%d mqtt=%d\n",
                d.lightRaw, d.light, plate, temp, hum, awning::toString(s.state), awning::toString(s.mode),
                s.rain ? "yes" : "no", awning::toString(s.rainSource), static_cast<int>(s.weatherAgeS),
                s.failSafe ? 1 : 0, n.wifi ? 1 : 0, n.mqtt ? 1 : 0);
}

void handleManualButton(uint32_t now) {
  switch (g_btnManual.update(now, hw::rawManualButton())) {
    case awning::Button::ShortPress:
      Serial.println("[ctl] manual button: toggle");
      g_controller.onManualButton(now);
      break;
    case awning::Button::LongPress: {
      const bool sim = g_controller.simulatingRain();
      Serial.printf("[ctl] manual button held: simulated rain %s\n", sim ? "OFF" : "ON");
      g_controller.onCommand(now, sim ? Action::ClearRain : Action::SimulateRain);
      hw::buzzerClick(now);
      break;
    }
    case awning::Button::None:
      break;
  }
}

void drainNetEvents(uint32_t now) {
  NetEvent ev;
  while (shared::popEvent(&ev)) {
    if (ev.type == NetEvent::Command) {
      g_controller.onCommand(now, ev.action);
    } else {
      g_controller.onWeather(now, ev.weather.isRaining, ev.weather.rainExpected15m, ev.weather.ageS);
    }
  }
}

void controlTick(uint32_t now) {
  // 1. Inputs (all local, all non-blocking).
  readSwitches(now);
  handleManualButton(now);
  drainNetEvents(now);
  hw::sensorsUpdate(now);
  const SensorData sensors = hw::sensors();

  // 2. Decide.
  awning::Inputs in;
  in.nowMs = now;
  in.limitClosed = g_btnClosed.pressed();
  in.limitOpen = g_btnOpen.pressed();
  in.humidityValid = sensors.humidityValid;
  in.humidity = sensors.humidity;
  in.light = sensors.light;
  in.rainSensorValid = sensors.rainValid;
  in.rainLevel = sensors.rainLevel;
  g_controller.update(in);
  const Snapshot snap = g_controller.snapshot(now);

  // 3. Act: relays first, everything else after.
  hw::setRelays(snap.driveClose, snap.driveOpen);

  const uint8_t events = g_controller.consumeEvents();
  if (events & awning::EvBeep) hw::buzzerBeep(now);
  if (events & awning::EvAlarm) Serial.println("[ctl] ALARM: limit switch not reached in time, relays off");
  hw::buzzerAlarm(now, snap.state == State::Error);  // keeps sounding until ERROR is left
  hw::buzzerUpdate(now);

  shared::publishState(snap, sensors);
  if (events & awning::EvChanged) {
    logStatus(snap, sensors, shared::netStatus());
    net::notifyChanged();
  }

  // 4. Slow stuff.
  if (g_oledTimer.due(now)) hw::oledDraw(snap, sensors, shared::netStatus());
  if (g_logTimer.due(now)) logStatus(snap, sensors, shared::netStatus());
}

}  // namespace

void setup() {
  hw::relaysOffEarly();  // very first thing: never let a relay glitch on at boot

  Serial.begin(115200);
  Serial.printf("\n[boot] awning firmware, reset reason: %s\n", resetReasonName());
#ifdef DEMO_FAST_TIMERS
  Serial.println("[boot] DEMO_FAST_TIMERS: confirm times shortened, not for real operation");
#endif

  hw::begin();
  shared::init();

  // Let the debounced limit switches settle (> debounce time) before choosing the boot state.
  for (int i = 0; i < 20; ++i) {
    readSwitches(millis());
    delay(5);
  }
  g_controller.begin(millis(), g_btnClosed.pressed(), g_btnOpen.pressed());
  Serial.printf("[boot] limit closed=%d open=%d, sim travel=%lu ms\n", g_btnClosed.pressed() ? 1 : 0,
                g_btnOpen.pressed() ? 1 : 0, static_cast<unsigned long>(SIM_TRAVEL_MS));

  net::start();

  // Watchdog on this task: if the control loop ever hangs, reboot into the relays-off state.
  // (IDF 4.4 signature, i.e. Arduino core 2.x; the platform version is pinned in platformio.ini.)
  esp_task_wdt_init(WDT_TIMEOUT_S, true);
  esp_task_wdt_add(nullptr);

  g_lastWake = xTaskGetTickCount();
}

void loop() {
  controlTick(millis());
  esp_task_wdt_reset();
  vTaskDelayUntil(&g_lastWake, pdMS_TO_TICKS(CONTROL_TICK_MS));
}

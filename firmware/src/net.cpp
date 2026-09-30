#include "net.h"

#include <Arduino.h>
#include <PubSubClient.h>
#include <WiFi.h>
#include <esp_timer.h>
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>

#include "awning_core.h"
#include "awning_msg.h"
#include "config.h"
#include "shared.h"

#if defined(__has_include)
#if !__has_include("secrets.h")
#error "Missing include/secrets.h: copy include/secrets.h.example to include/secrets.h and fill in WiFi/MQTT (thieu file secrets.h)"
#endif
#endif
#include "secrets.h"

namespace {

const char* const kTopicTelemetry = MQTT_TOPIC_PREFIX "telemetry";
const char* const kTopicStatus = MQTT_TOPIC_PREFIX "status";
const char* const kTopicCmd = MQTT_TOPIC_PREFIX "cmd";
const char* const kTopicWeather = MQTT_TOPIC_PREFIX "weather";

uint32_t minU32(uint32_t a, uint32_t b) { return a < b ? a : b; }

WiFiClient g_wifiClient;
PubSubClient g_mqtt(g_wifiClient);
TaskHandle_t g_task = nullptr;

// ---- WiFi: non-blocking, reconnect with growing pause ----------------------------

bool g_wifiWasUp = false;
bool g_wifiStarted = false;
uint32_t g_wifiLastBeginMs = 0;
uint32_t g_wifiRetryMs = WIFI_RETRY_MIN_MS;

void wifiStep(uint32_t now) {
  if (WiFi.status() == WL_CONNECTED) {
    if (!g_wifiWasUp) {
      Serial.printf("[net] WiFi up, ip=%s rssi=%d\n", WiFi.localIP().toString().c_str(), WiFi.RSSI());
      g_wifiRetryMs = WIFI_RETRY_MIN_MS;
    }
    g_wifiWasUp = true;
    return;
  }
  if (g_wifiWasUp) {
    Serial.println("[net] WiFi lost");
    g_wifiWasUp = false;
    g_wifiLastBeginMs = now;  // the core's auto-reconnect gets the first chance
  }
  if (!g_wifiStarted || awning::elapsedMs(now, g_wifiLastBeginMs) >= g_wifiRetryMs) {
    if (g_wifiStarted) WiFi.disconnect(false, false);  // retry: drop a stuck attempt first
    g_wifiStarted = true;
    g_wifiLastBeginMs = now;
    Serial.println("[net] WiFi connecting");
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);  // returns immediately; status is polled above
    g_wifiRetryMs = minU32(g_wifiRetryMs * 2, WIFI_RETRY_MAX_MS);
  }
}

// ---- MQTT ---------------------------------------------------------------------

bool g_mqttTried = false;
uint32_t g_mqttLastAttemptMs = 0;
uint32_t g_mqttRetryMs = MQTT_RETRY_MIN_MS;

void onMessage(char* topic, uint8_t* payload, unsigned int length) {
  NetEvent ev;
  if (strcmp(topic, kTopicCmd) == 0) {
    awning::Action action;
    if (!awning::parseCommand(reinterpret_cast<const char*>(payload), length, &action)) {
      Serial.printf("[net] ignored bad cmd (%u bytes)\n", length);
      return;
    }
    ev.type = NetEvent::Command;
    ev.action = action;
  } else if (strcmp(topic, kTopicWeather) == 0) {
    if (!awning::parseWeather(reinterpret_cast<const char*>(payload), length, &ev.weather)) {
      Serial.printf("[net] ignored bad weather (%u bytes)\n", length);
      return;
    }
    ev.type = NetEvent::Weather;
  } else {
    return;
  }
  if (!shared::pushEvent(ev)) Serial.println("[net] event queue full, dropped");
}

// Returns true when a new session was established.
bool mqttConnect() {
  Serial.printf("[net] MQTT connecting to %s:%d\n", MQTT_HOST, MQTT_PORT);
  // clean session: the broker must not queue old `cmd` messages for us while we are away
  // (an old "close"/"open" replayed after a reboot would be a surprise).
  const bool ok = g_mqtt.connect(MQTT_CLIENT_ID, MQTT_USER, MQTT_PASSWORD, kTopicStatus,
                                 1 /*will qos*/, true /*will retain*/, "offline", true /*clean*/);
  if (!ok) {
    Serial.printf("[net] MQTT connect failed, rc=%d\n", g_mqtt.state());
    return false;
  }
  // Subscribe BEFORE announcing "online": the backend answers "online" with a weather
  // message, which must not be missed.
  g_mqtt.subscribe(kTopicCmd, 1);
  g_mqtt.subscribe(kTopicWeather, 1);
  g_mqtt.publish(kTopicStatus, "online", true);  // PubSubClient can only publish at QoS 0
  Serial.println("[net] MQTT connected");
  return true;
}

// Returns true only in the iteration where the session was just established.
bool mqttStep(uint32_t now) {
  if (g_mqtt.connected()) {
    g_mqtt.loop();
    return false;
  }
  if (g_mqttTried && awning::elapsedMs(now, g_mqttLastAttemptMs) < g_mqttRetryMs) return false;
  g_mqttTried = true;
  g_mqttLastAttemptMs = now;
  if (mqttConnect()) {
    g_mqttRetryMs = MQTT_RETRY_MIN_MS;
    return true;
  }
  g_mqttRetryMs = minU32(g_mqttRetryMs * 2, MQTT_RETRY_MAX_MS);
  return false;
}

// Returns false only if the shared state was busy (nothing published, caller retries).
bool publishTelemetry() {
  awning::Snapshot snap;
  SensorData sd;
  if (!shared::readState(&snap, &sd)) return false;  // never publish made-up data

  awning::TelemetryData t;
  t.tempValid = sd.tempValid;
  t.temp = sd.temp;
  t.humidityValid = sd.humidityValid;
  t.humidity = sd.humidity;
  t.light = sd.light;
  t.state = snap.state;
  t.mode = snap.mode;
  t.rain = snap.rain;
  t.rainSource = snap.rainSource;
  t.weatherAgeS = snap.weatherAgeS;
  t.failSafe = snap.failSafe;
  t.manualLeftS = snap.manualLeftS;
  t.rssi = WiFi.RSSI();
  t.uptimeS = static_cast<uint32_t>(esp_timer_get_time() / 1000000LL);  // 64-bit: no 49-day wrap

  char buf[384];
  if (awning::formatTelemetry(buf, sizeof(buf), t) == 0) {
    Serial.println("[net] telemetry does not fit the buffer");
    return true;
  }
  if (!g_mqtt.publish(kTopicTelemetry, buf)) Serial.println("[net] telemetry publish failed");
  return true;
}

void netTask(void*) {
  WiFi.persistent(false);  // do not wear the flash with credentials on every boot
  WiFi.mode(WIFI_STA);
  WiFi.setHostname(MQTT_CLIENT_ID);
  WiFi.setSleep(false);  // modem sleep adds up to ~100 ms to every incoming command
  WiFi.setAutoReconnect(true);

  g_mqtt.setServer(MQTT_HOST, MQTT_PORT);
  g_mqtt.setBufferSize(MQTT_BUFFER_BYTES);
  g_mqtt.setKeepAlive(MQTT_KEEPALIVE_S);
  g_mqtt.setSocketTimeout(MQTT_SOCKET_TIMEOUT_S);
  g_mqtt.setCallback(onMessage);

  uint32_t lastTelemetryMs = 0;
  bool changedPending = false;
  NetStatus lastStatus;
  bool statusSent = false;

  for (;;) {
    const uint32_t now = millis();

    wifiStep(now);
    const bool wifiUp = WiFi.status() == WL_CONNECTED;
    bool justConnected = false;
    if (wifiUp) justConnected = mqttStep(now);

    NetStatus st;
    st.wifi = wifiUp;
    st.mqtt = wifiUp && g_mqtt.connected();
    st.rssi = wifiUp ? WiFi.RSSI() : 0;
    if (!statusSent || st.wifi != lastStatus.wifi || st.mqtt != lastStatus.mqtt) {
      shared::setNetStatus(st);
      lastStatus = st;
      statusSent = true;
    }

    if (st.mqtt) {
      const uint32_t sinceLast = awning::elapsedMs(now, lastTelemetryMs);
      const bool periodic = sinceLast >= TELEMETRY_PERIOD_MS;
      const bool onEvent = changedPending && sinceLast >= TELEMETRY_MIN_GAP_MS;
      if ((justConnected || periodic || onEvent) && publishTelemetry()) {
        lastTelemetryMs = now;
        changedPending = false;
      }
    }

    // Sleep up to one control tick; a Changed notification wakes us immediately.
    if (ulTaskNotifyTake(pdTRUE, pdMS_TO_TICKS(CONTROL_TICK_MS)) > 0) changedPending = true;
  }
}

}  // namespace

namespace net {

void start() {
  xTaskCreatePinnedToCore(netTask, "net", 10240, nullptr, 1, &g_task, 0);
}

void notifyChanged() {
  if (g_task != nullptr) xTaskNotifyGive(g_task);
}

}  // namespace net

// Host simulation of the ESP32 platform under the real firmware sources.
// Machine-readable lines on stdout start with "@@"; firmware serial output is "SER <ms> <text>".
#include <sys/types.h>
#include <unistd.h>

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdarg>
#include <deque>
#include <map>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#include "Adafruit_SSD1306.h"
#include "Arduino.h"
#include "DHT.h"
#include "WiFi.h"
#include "Wire.h"
#include "config.h"
#include "esp_system.h"
#include "esp_task_wdt.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/queue.h"
#include "freertos/semphr.h"
#include "freertos/task.h"

using Clock = std::chrono::steady_clock;

namespace {
const Clock::time_point g_start = Clock::now();
std::mutex g_outMutex;
std::atomic<int> g_wifiEpoch{0};

uint64_t hostMs() {
  return std::chrono::duration_cast<std::chrono::milliseconds>(Clock::now() - g_start).count();
}
uint64_t millisOffset() {
  static uint64_t off = getenv("SIM_MILLIS_OFFSET") ? strtoull(getenv("SIM_MILLIS_OFFSET"), nullptr, 10) : 0;
  return off;
}

void emit(const char* fmt, ...) __attribute__((format(printf, 1, 2)));
void emit(const char* fmt, ...) {
  std::lock_guard<std::mutex> lock(g_outMutex);
  va_list ap;
  va_start(ap, fmt);
  vprintf(fmt, ap);
  va_end(ap);
  putchar('\n');
  fflush(stdout);
}

// ---- simulated hardware ------------------------------------------------------
std::mutex g_hw;
int g_dir[64];
int g_outReg[64];
std::atomic<int> g_in[64];
std::atomic<int> g_ldr{2000};
std::atomic<int> g_dhtT10{290}, g_dhtH10{710};
std::atomic<bool> g_dhtNan{false};
std::atomic<int> g_hangReadMs{0};
bool g_relayOn[2] = {false, false};
int64_t g_lastOff[2] = {-1, -1};
bool g_buzzerOn = false;

bool relayLogicalOn(int pin) {
  const int onLevel = RELAY_ACTIVE_LOW ? LOW : HIGH;
  return g_dir[pin] == OUTPUT && g_outReg[pin] == onLevel;
}

void relaysChanged() {  // caller holds g_hw
  const bool on[2] = {relayLogicalOn(PIN_RELAY_CLOSE), relayLogicalOn(PIN_RELAY_OPEN)};
  if (on[0] == g_relayOn[0] && on[1] == g_relayOn[1]) return;
  const int64_t t = static_cast<int64_t>(hostMs());
  for (int i = 0; i < 2; ++i) {
    if (on[i] == g_relayOn[i]) continue;
    if (!on[i]) {
      g_lastOff[i] = t;
    } else {
      const int o = 1 - i;
      if (g_lastOff[o] >= 0 && t - g_lastOff[o] < 190) {
        emit("@@VIOLATION dead_time ch%d on only %lld ms after ch%d off", i + 1,
             static_cast<long long>(t - g_lastOff[o]), o + 1);
        _exit(98);
      }
    }
  }
  g_relayOn[0] = on[0];
  g_relayOn[1] = on[1];
  if (on[0] && on[1]) {
    emit("@@VIOLATION both relays on");
    _exit(99);
  }
  emit("@@RELAY %llu %d %d", static_cast<unsigned long long>(t), on[0] ? 1 : 0, on[1] ? 1 : 0);
}

// ---- FreeRTOS shims ------------------------------------------------------------
thread_local TaskCtl* t_self = nullptr;
}  // namespace

struct TaskCtl {
  std::mutex m;
  std::condition_variable cv;
  uint32_t notify = 0;
};
struct QueueCtl {
  std::mutex m;
  std::deque<std::vector<uint8_t>> q;
  size_t len = 0, itemSize = 0;
};
struct MutexCtl {
  std::timed_mutex m;
};

// ---- Arduino ----------------------------------------------------------------------
SerialClass Serial;
TwoWire Wire;
WiFiClass WiFi;

uint32_t millis() { return static_cast<uint32_t>(millisOffset() + hostMs()); }
void delay(uint32_t ms) { std::this_thread::sleep_for(std::chrono::milliseconds(ms)); }
void yield() { std::this_thread::yield(); }

void pinMode(uint8_t pin, uint8_t mode) {
  std::lock_guard<std::mutex> lock(g_hw);
  g_dir[pin] = (mode == OUTPUT) ? OUTPUT : INPUT;
  if (pin == PIN_RELAY_CLOSE || pin == PIN_RELAY_OPEN) relaysChanged();
}

void digitalWrite(uint8_t pin, uint8_t val) {
  std::lock_guard<std::mutex> lock(g_hw);
  g_outReg[pin] = val;
  if (pin == PIN_RELAY_CLOSE || pin == PIN_RELAY_OPEN) relaysChanged();
  if (pin == PIN_BUZZER) {
    const bool on = g_dir[pin] == OUTPUT && val == HIGH;
    if (on != g_buzzerOn) {
      g_buzzerOn = on;
      emit("@@BUZ %llu %d", static_cast<unsigned long long>(hostMs()), on ? 1 : 0);
    }
  }
}

int digitalRead(uint8_t pin) {
  const int hang = g_hangReadMs.exchange(0);
  if (hang > 0) std::this_thread::sleep_for(std::chrono::milliseconds(hang));  // wedged peripheral
  return g_in[pin].load();
}
int analogRead(uint8_t) { return g_ldr.load(); }
void analogReadResolution(uint8_t) {}
void analogSetPinAttenuation(uint8_t, int) {}

int SerialClass::printf(const char* fmt, ...) {
  char buf[512];
  va_list ap;
  va_start(ap, fmt);
  int n = vsnprintf(buf, sizeof buf, fmt, ap);
  va_end(ap);
  std::string s(buf);
  while (!s.empty() && (s.back() == '\n' || s.back() == '\r')) s.pop_back();
  emit("SER %llu %s", static_cast<unsigned long long>(hostMs()), s.c_str());
  return n;
}
void SerialClass::println(const char* s) { printf("%s\n", s); }
void SerialClass::println() { printf("\n"); }

float DHT::readTemperature() { return g_dhtNan ? NAN : g_dhtT10 / 10.0f; }
float DHT::readHumidity() { return g_dhtNan ? NAN : g_dhtH10 / 10.0f; }

void Adafruit_SSD1306::display() {
  static std::string last;
  std::string line;
  for (auto& kv : rows) {
    if (!line.empty()) line += "|";
    line += kv.second;
  }
  if (line != last) {
    last = line;
    emit("@@OLED %llu %s", static_cast<unsigned long long>(hostMs()), line.c_str());
  }
}

// ---- WiFi ---------------------------------------------------------------------------
namespace {
std::atomic<bool> g_wifiWanted{true};
std::atomic<int64_t> g_wifiBeginMs{-1};
}  // namespace

namespace sim {
int wifiEpoch() { return g_wifiEpoch.load(); }
bool stallConnect() {
  // While the file named by SIM_STALL_FILE exists, every connect blocks 3 s and then fails
  // (a black-holed broker). Deleting the file lets connects succeed again.
  const char* f = getenv("SIM_STALL_FILE");
  if (f && access(f, F_OK) == 0) {
    std::this_thread::sleep_for(std::chrono::milliseconds(3000));
    return true;
  }
  return false;
}
}  // namespace sim

wl_status_t WiFiClass::begin(const char*, const char*) {
  g_wifiBeginMs = static_cast<int64_t>(hostMs());
  return status();
}
bool WiFiClass::disconnect(bool, bool) {
  g_wifiBeginMs = -1;
  return true;
}
wl_status_t WiFiClass::status() {
  if (!g_wifiWanted) return WL_DISCONNECTED;
  const int64_t b = g_wifiBeginMs.load();
  if (b < 0) return WL_DISCONNECTED;
  return static_cast<int64_t>(hostMs()) - b >= 300 ? WL_CONNECTED : WL_IDLE_STATUS;
}

// ---- ESP-IDF ---------------------------------------------------------------------------
esp_reset_reason_t esp_reset_reason(void) { return ESP_RST_POWERON; }
int64_t esp_timer_get_time(void) { return static_cast<int64_t>(hostMs()) * 1000; }

namespace {
std::atomic<uint64_t> g_wdtLast{0};
std::atomic<uint32_t> g_wdtTimeoutS{5};
std::atomic<bool> g_wdtArmed{false};
}  // namespace
esp_err_t esp_task_wdt_init(uint32_t t, bool) {
  g_wdtTimeoutS = t;
  return 0;
}
esp_err_t esp_task_wdt_add(TaskHandle_t) {
  g_wdtLast = hostMs();
  if (!g_wdtArmed.exchange(true)) {
    std::thread([] {
      for (;;) {
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
        if (hostMs() - g_wdtLast > static_cast<uint64_t>(g_wdtTimeoutS) * 1000) {
          emit("@@WDT task watchdog fired, rebooting (exit 3)");
          _exit(3);
        }
      }
    }).detach();
  }
  return 0;
}
esp_err_t esp_task_wdt_reset(void) {
  g_wdtLast = hostMs();
  return 0;
}

// ---- FreeRTOS -----------------------------------------------------------------------------
BaseType_t xTaskCreatePinnedToCore(TaskFunction_t fn, const char*, uint32_t, void* arg, UBaseType_t,
                                   TaskHandle_t* handle, BaseType_t) {
  TaskCtl* ctl = new TaskCtl();
  if (handle) *handle = ctl;
  std::thread([fn, arg, ctl] {
    t_self = ctl;
    fn(arg);
  }).detach();
  return pdTRUE;
}
TickType_t xTaskGetTickCount() { return static_cast<TickType_t>(hostMs()); }
void vTaskDelayUntil(TickType_t* prev, TickType_t inc) {
  const TickType_t next = *prev + inc;
  const int32_t wait = static_cast<int32_t>(next - xTaskGetTickCount());
  if (wait > 0) std::this_thread::sleep_for(std::chrono::milliseconds(wait));
  *prev = next;
}
uint32_t ulTaskNotifyTake(BaseType_t clear, TickType_t wait) {
  if (t_self == nullptr) t_self = new TaskCtl();
  std::unique_lock<std::mutex> lock(t_self->m);
  t_self->cv.wait_for(lock, std::chrono::milliseconds(wait), [] { return t_self->notify > 0; });
  const uint32_t n = t_self->notify;
  if (clear) t_self->notify = 0; else if (n > 0) t_self->notify--;
  return n;
}
BaseType_t xTaskNotifyGive(TaskHandle_t h) {
  std::lock_guard<std::mutex> lock(h->m);
  h->notify++;
  h->cv.notify_one();
  return pdTRUE;
}

QueueHandle_t xQueueCreate(UBaseType_t length, UBaseType_t itemSize) {
  QueueCtl* q = new QueueCtl();
  q->len = length;
  q->itemSize = itemSize;
  return q;
}
BaseType_t xQueueSend(QueueHandle_t q, const void* item, TickType_t) {
  std::lock_guard<std::mutex> lock(q->m);
  if (q->q.size() >= q->len) return pdFALSE;
  q->q.emplace_back(static_cast<const uint8_t*>(item), static_cast<const uint8_t*>(item) + q->itemSize);
  return pdTRUE;
}
BaseType_t xQueueReceive(QueueHandle_t q, void* item, TickType_t) {
  std::lock_guard<std::mutex> lock(q->m);
  if (q->q.empty()) return pdFALSE;
  memcpy(item, q->q.front().data(), q->itemSize);
  q->q.pop_front();
  return pdTRUE;
}

SemaphoreHandle_t xSemaphoreCreateMutex() { return new MutexCtl(); }
BaseType_t xSemaphoreTake(SemaphoreHandle_t m, TickType_t wait) {
  return m->m.try_lock_for(std::chrono::milliseconds(wait)) ? pdTRUE : pdFALSE;
}
BaseType_t xSemaphoreGive(SemaphoreHandle_t m) {
  m->m.unlock();
  return pdTRUE;
}

// ---- stdin control channel ---------------------------------------------------------------
namespace sim {
void startStdinThread() {
  for (int i = 0; i < 64; ++i) g_in[i] = 1;  // pulled-up inputs: released
  if (const char* init = getenv("SIM_INITIAL_IN")) {  // e.g. "32=0,33=1": inputs held at boot
    std::string spec(init);
    size_t pos = 0;
    while (pos < spec.size()) {
      size_t end = spec.find(',', pos);
      if (end == std::string::npos) end = spec.size();
      int pin = 0, v = 1;
      if (sscanf(spec.substr(pos, end - pos).c_str(), "%d=%d", &pin, &v) == 2) g_in[pin] = v;
      pos = end + 1;
    }
  }
  std::thread([] {
    char line[256];
    while (fgets(line, sizeof line, stdin)) {
      int pin, v, ms;
      float t, h;
      if (sscanf(line, "in %d %d", &pin, &v) == 2) {
        g_in[pin] = v;
      } else if (sscanf(line, "ldr %d", &v) == 1) {
        g_ldr = v;
      } else if (sscanf(line, "dht %f %f", &t, &h) == 2) {
        g_dhtNan = false;
        g_dhtT10 = static_cast<int>(t * 10);
        g_dhtH10 = static_cast<int>(h * 10);
      } else if (strncmp(line, "dht nan", 7) == 0) {
        g_dhtNan = true;
      } else if (sscanf(line, "wifi %d", &v) == 1) {
        g_wifiWanted = v != 0;
        if (v == 0) {
          ++g_wifiEpoch;  // sockets die with the link
        } else {
          g_wifiBeginMs = static_cast<int64_t>(hostMs());  // auto-reconnect
        }
      } else if (sscanf(line, "hangread %d", &ms) == 1) {
        g_hangReadMs = ms;
      } else if (strncmp(line, "quit", 4) == 0) {
        _exit(0);
      }
    }
  }).detach();
}
}  // namespace sim

#include "shared.h"

#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/semphr.h>

namespace {

SemaphoreHandle_t g_mutex = nullptr;
QueueHandle_t g_queue = nullptr;
awning::Snapshot g_snap;
SensorData g_sensors;
NetStatus g_net;

// Bounded wait: if the other core somehow holds the lock, skip this update rather than
// stall the control loop.
const TickType_t kLockWait = pdMS_TO_TICKS(5);

}  // namespace

namespace shared {

void init() {
  g_mutex = xSemaphoreCreateMutex();
  g_queue = xQueueCreate(8, sizeof(NetEvent));
}

void publishState(const awning::Snapshot& snap, const SensorData& sensors) {
  if (xSemaphoreTake(g_mutex, kLockWait) != pdTRUE) return;
  g_snap = snap;
  g_sensors = sensors;
  xSemaphoreGive(g_mutex);
}

bool readState(awning::Snapshot* snap, SensorData* sensors) {
  if (xSemaphoreTake(g_mutex, kLockWait) != pdTRUE) return false;
  *snap = g_snap;
  *sensors = g_sensors;
  xSemaphoreGive(g_mutex);
  return true;
}

void setNetStatus(const NetStatus& status) {
  if (xSemaphoreTake(g_mutex, kLockWait) != pdTRUE) return;
  g_net = status;
  xSemaphoreGive(g_mutex);
}

NetStatus netStatus() {
  NetStatus out;
  if (xSemaphoreTake(g_mutex, kLockWait) != pdTRUE) return out;
  out = g_net;
  xSemaphoreGive(g_mutex);
  return out;
}

bool pushEvent(const NetEvent& event) { return xQueueSend(g_queue, &event, 0) == pdTRUE; }

bool popEvent(NetEvent* event) { return xQueueReceive(g_queue, event, 0) == pdTRUE; }

}  // namespace shared

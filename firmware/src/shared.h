// Data shared between the control task (core 1) and the network task (core 0).
// Rule: the control task never waits for the network. All that is shared is a mutex held
// for a struct copy and a non-blocking queue.
#pragma once
#include <Arduino.h>
#include <type_traits>

#include "awning_core.h"
#include "awning_msg.h"

struct SensorData {
  bool tempValid = false;
  float temp = 0.0f;
  bool humidityValid = false;
  float humidity = 0.0f;
  int light = 0;     // normalised: high = bright
  int lightRaw = 0;  // raw ADC average, for LDR calibration
  bool rainValid = false;  // a rain plate reading exists (never true when RAIN_SENSOR_ENABLED is 0)
  int rainLevel = 0;       // normalised: 0..4095, high = wet
  int rainRaw = 0;         // raw ADC average, for calibration
};

struct NetStatus {
  bool wifi = false;
  bool mqtt = false;
  int rssi = 0;
};

// Network -> control loop message.
struct NetEvent {
  enum Type : uint8_t { Command, Weather };
  Type type = Command;
  awning::Action action = awning::Action::Auto;
  awning::WeatherMsg weather;
};
static_assert(std::is_trivially_copyable<NetEvent>::value, "NetEvent is copied by FreeRTOS queues");

namespace shared {

void init();  // call once, before creating tasks

// Control -> network (telemetry).
void publishState(const awning::Snapshot& snap, const SensorData& sensors);
bool readState(awning::Snapshot* snap, SensorData* sensors);  // false: lock busy, outputs untouched

// Network -> control (OLED status line).
void setNetStatus(const NetStatus& status);
NetStatus netStatus();

// Network -> control (commands, weather). Never blocks; pushEvent returns false if the queue is full.
bool pushEvent(const NetEvent& event);
bool popEvent(NetEvent* event);

}  // namespace shared

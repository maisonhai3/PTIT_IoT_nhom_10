// Network task (core 0): WiFi + MQTT. Must never be able to stall the control loop.
#pragma once

namespace net {

// Creates the network task pinned to core 0. Call after shared::init().
void start();

// From the control task: a Changed event happened, publish telemetry now (non-blocking).
void notifyChanged();

}  // namespace net

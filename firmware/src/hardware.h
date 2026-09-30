// Hardware used by the control task: relays, switches, sensors, buzzer, OLED.
// Only ever called from the control task (core 1), so no locking is needed.
#pragma once
#include <Arduino.h>

#include "shared.h"

namespace hw {

// FIRST thing in setup(): drive both relays to OFF before anything else.
void relaysOffEarly();

// Pin modes, ADC, DHT, I2C/OLED.
void begin();

// Switches the channel that must turn off first, then the one that turns on; never both on.
void setRelays(bool close, bool open);

// Raw (not debounced) levels, true = pressed.
bool rawLimitClosed();
bool rawLimitOpen();
bool rawManualButton();

// DHT11 every DHT_PERIOD_MS, LDR every LDR_PERIOD_MS.
void sensorsUpdate(uint32_t nowMs);
SensorData sensors();

// Non-blocking buzzer: call buzzerUpdate() every tick.
void buzzerBeep(uint32_t nowMs);   // triple beep: automatic close started
void buzzerClick(uint32_t nowMs);  // one short beep: long-press acknowledged
void buzzerAlarm(uint32_t nowMs, bool on);  // long repeating beeps until switched off
void buzzerUpdate(uint32_t nowMs);

void oledDraw(const awning::Snapshot& snap, const SensorData& sensors, const NetStatus& net);

}  // namespace hw

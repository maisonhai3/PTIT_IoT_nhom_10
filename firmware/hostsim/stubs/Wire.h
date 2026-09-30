#pragma once
#include "Arduino.h"
struct TwoWire {
  bool begin(int, int) { return true; }
  bool begin() { return true; }
  void beginTransmission(uint8_t) {}
  uint8_t endTransmission() { return 0; }  // pretend the OLED ACKs
};
extern TwoWire Wire;

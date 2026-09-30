#pragma once
#include "Arduino.h"
#define DHT11 11
class DHT {
 public:
  DHT(uint8_t, uint8_t) {}
  void begin() {}
  float readTemperature();
  float readHumidity();
};

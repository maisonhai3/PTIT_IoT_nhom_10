#pragma once
#include <stdint.h>
#include <stdio.h>
#include "Arduino.h"
class IPAddress {
 public:
  IPAddress() { a[0] = a[1] = a[2] = a[3] = 0; }
  IPAddress(uint8_t o1, uint8_t o2, uint8_t o3, uint8_t o4) { a[0] = o1; a[1] = o2; a[2] = o3; a[3] = o4; }
  IPAddress(const uint8_t* p) { for (int i = 0; i < 4; ++i) a[i] = p[i]; }
  String toString() const {
    char b[20];
    snprintf(b, sizeof b, "%d.%d.%d.%d", a[0], a[1], a[2], a[3]);
    return String(std::string(b));
  }
  uint8_t a[4];
};

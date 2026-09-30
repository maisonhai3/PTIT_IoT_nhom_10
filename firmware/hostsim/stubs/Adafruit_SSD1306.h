#pragma once
#include <map>
#include <string>
#include "Arduino.h"
#include "Wire.h"
#define SSD1306_SWITCHCAPVCC 0x02
#define SSD1306_WHITE 1
class Adafruit_SSD1306 {
 public:
  Adafruit_SSD1306(int, int, TwoWire*, int) {}
  bool begin(uint8_t, uint8_t, bool = true, bool = true) { return true; }
  void clearDisplay() { rows.clear(); }
  void display();
  void setTextColor(int) {}
  void setTextSize(int) {}
  void setCursor(int, int y) { cy = y; }
  void print(const char* s) { rows[cy] += s; }
 private:
  std::map<int, std::string> rows;
  int cy = 0;
};

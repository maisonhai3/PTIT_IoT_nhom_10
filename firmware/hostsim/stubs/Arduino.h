// Host stub: just enough of the Arduino API to run the firmware sources on Linux.
#pragma once
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <string>

typedef bool boolean;
typedef uint8_t byte;
#define PROGMEM
#define pgm_read_byte_near(a) (*(const uint8_t*)(a))
#define pgm_read_byte(a) (*(const uint8_t*)(a))
#define HIGH 1
#define LOW 0
#define INPUT 0
#define OUTPUT 1
#define INPUT_PULLUP 2
#define ADC_11db 3

struct String : std::string {
  String() {}
  String(const std::string& s) : std::string(s) {}
};

uint32_t millis();  // 32-bit like the ESP32, wraps
void delay(uint32_t ms);
void yield();
void pinMode(uint8_t pin, uint8_t mode);
void digitalWrite(uint8_t pin, uint8_t val);
int digitalRead(uint8_t pin);
int analogRead(uint8_t pin);
void analogReadResolution(uint8_t bits);
void analogSetPinAttenuation(uint8_t pin, int attenuation);

struct SerialClass {
  void begin(unsigned long) {}
  int printf(const char* fmt, ...) __attribute__((format(printf, 2, 3)));
  void println(const char* s);
  void println();
};
extern SerialClass Serial;

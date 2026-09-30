#pragma once
#include <stddef.h>
#include <stdint.h>
class Print {
 public:
  virtual ~Print() {}
  virtual size_t write(uint8_t) = 0;
  virtual size_t write(const uint8_t* buf, size_t size) {
    size_t n = 0;
    while (size--) n += write(*buf++);
    return n;
  }
};

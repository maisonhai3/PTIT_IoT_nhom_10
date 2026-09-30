// Periodic timer helper for the control loop.
#pragma once
#include <stdint.h>

// True once per periodMs. Uses a signed difference so it survives millis() rollover.
// nextMs starts at 0 => the first call returns true.
inline bool due(uint32_t now, uint32_t& nextMs, uint32_t periodMs) {
  if (static_cast<int32_t>(now - nextMs) < 0) return false;
  nextMs = now + periodMs;
  return true;
}

#include "awning_core.h"

namespace awning {

Button::Gesture Button::update(uint32_t nowMs, bool rawPressed) {
  Gesture g = None;

  if (rawPressed != raw_) {
    raw_ = rawPressed;
    rawSinceMs_ = nowMs;
  }
  // Accept a level only after it has been steady for debounceMs_.
  if (raw_ != stable_ && elapsedMs(nowMs, rawSinceMs_) >= debounceMs_) {
    stable_ = raw_;
    if (stable_) {
      pressStartMs_ = rawSinceMs_;
      longFired_ = false;
    } else if (!longFired_) {
      g = ShortPress;
    }
  }
  if (stable_ && !longFired_ && elapsedMs(nowMs, pressStartMs_) >= longPressMs_) {
    longFired_ = true;
    g = LongPress;
  }
  return g;
}

}  // namespace awning

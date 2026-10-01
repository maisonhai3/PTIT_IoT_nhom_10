// Awning state machine + button gestures.
// Pure C++11: no Arduino/ESP headers, so every rule below is unit-tested on the host.
#pragma once
#include <stdint.h>

namespace awning {

// Unsigned subtraction survives millis() rollover. A timestamp slightly *ahead* of
// `now` (caller used an older clock reading) yields 0 instead of "49 days elapsed".
inline uint32_t elapsedMs(uint32_t now, uint32_t since) {
  const uint32_t d = now - since;
  return d > 0x7FFFFFFFu ? 0u : d;
}

// Fires once per period, rollover-safe (signed difference). The first call fires at once,
// unless startIn() postponed it.
class Every {
 public:
  explicit Every(uint32_t periodMs) : periodMs_(periodMs) {}
  void startIn(uint32_t nowMs, uint32_t delayMs) {
    nextMs_ = nowMs + delayMs;
    armed_ = true;
  }
  bool due(uint32_t nowMs) {
    if (armed_ && static_cast<int32_t>(nowMs - nextMs_) < 0) return false;
    armed_ = true;
    nextMs_ = nowMs + periodMs_;
    return true;
  }

 private:
  uint32_t periodMs_;
  uint32_t nextMs_ = 0;
  bool armed_ = false;
};

enum class State : uint8_t { Open, Closing, Closed, Opening, Error };
enum class Mode : uint8_t { Auto, Manual };
enum class RainSource : uint8_t { None, Api, Sim, Local, Sensor };
enum class Action : uint8_t { Open, Close, Auto, SimulateRain, ClearRain };

// Strings used on the wire (docs/mqtt-topics.md).
const char* toString(State s);
const char* toString(Mode m);
const char* toString(RainSource r);
// Exact, case-sensitive match of the `cmd` action names. `*out` is untouched on failure.
bool parseAction(const char* s, Action* out);

// Bit flags returned by Controller::consumeEvents().
enum Event : uint8_t {
  EvChanged = 1,  // state/mode/rain/rainSource/failSafe/rainSensorWet changed: publish telemetry now
  EvBeep = 2,     // automatic close just started
  EvAlarm = 4     // entered Error
};

// Defaults = docs/mqtt-topics.md. simTravelMs = 0 means "rely on limit switches only".
struct Config {
  uint32_t rainConfirmMs = 30UL * 1000UL;
  uint32_t dryConfirmMs = 15UL * 60UL * 1000UL;
  uint32_t manualTimeoutMs = 10UL * 60UL * 1000UL;
  uint32_t travelTimeoutMs = 30UL * 1000UL;
  uint32_t deadTimeMs = 200;
  uint32_t simTravelMs = 0;
  uint32_t weatherStaleMs = 30UL * 60UL * 1000UL;
  uint32_t firstWeatherGraceMs = 120UL * 1000UL;
  float localHumidityHigh = 85.0f;
  int localDarkBelow = 800;
  // Rain plate (docs/mqtt-topics.md): wet from rainWetAbove up, dry again from rainDryBelow down,
  // unchanged in between. It must stay wet for sensorRainConfirmMs before the awning closes.
  uint32_t sensorRainConfirmMs = 5UL * 1000UL;
  int rainWetAbove = 400;
  int rainDryBelow = 200;
};

struct Inputs {
  uint32_t nowMs = 0;
  bool limitClosed = false;  // debounced, true = pressed
  bool limitOpen = false;
  bool humidityValid = false;
  float humidity = 0.0f;
  int light = 0;  // 0..4095, high = bright (polarity already normalised)
  bool rainSensorValid = false;  // false = no plate fitted / no reading yet: the sensor is ignored
  int rainLevel = 0;             // 0..4095, high = wet (polarity already normalised)
};

struct Snapshot {
  State state = State::Open;
  Mode mode = Mode::Auto;
  bool driveClose = false;  // relay CH1 must be energised
  bool driveOpen = false;   // relay CH2 must be energised
  bool rain = false;        // wet right now, before the confirm timer
  RainSource rainSource = RainSource::None;
  int32_t weatherAgeS = -1;  // -1 = never received
  bool failSafe = false;
  bool rainSensorWet = false;  // the plate's verdict after the hysteresis; false without a sensor
  uint32_t manualLeftS = 0;    // 0 in Auto
};

class Controller {
 public:
  explicit Controller(const Config& cfg = Config());

  // Call once at boot with the debounced limit switches. Resets everything.
  void begin(uint32_t nowMs, bool limitClosed, bool limitOpen);
  // A `weather` message; ageS is the data age at publish time (age_s).
  void onWeather(uint32_t nowMs, bool isRaining, bool rainExpected15m, uint32_t ageS);
  void onCommand(uint32_t nowMs, Action action);
  void onManualButton(uint32_t nowMs);
  // Every control tick (~20 ms). Call before snapshot() in the same tick.
  void update(const Inputs& in);

  Snapshot snapshot(uint32_t nowMs) const;
  uint8_t consumeEvents();  // returns Event flags and clears them
  bool simulatingRain() const { return sim_; }

 private:
  enum class Wet : uint8_t { Unknown, Dry, Wet };

  void foldWeatherAge(uint32_t now);
  uint32_t weatherAgeMsAt(uint32_t now) const;
  void trackWetness(const Inputs& in);
  void progressMotion(const Inputs& in);
  void applyAutoRule(uint32_t now);
  void goManual(uint32_t now, State direction);
  void enterState(uint32_t now, State s);
  bool inDeadTime(uint32_t now) const;
  void noteChanges();
  uint32_t signature() const;

  Config cfg_;
  State state_ = State::Open;
  Mode mode_ = Mode::Auto;
  bool sim_ = false;

  uint32_t bootMs_ = 0;
  uint32_t motionStartMs_ = 0;  // when the current Closing/Opening was requested
  uint32_t manualSinceMs_ = 0;
  bool prevLimitClosed_ = false;
  bool prevLimitOpen_ = false;

  // Weather age is accumulated (saturating) instead of stored as a timestamp,
  // so it can never wrap back to "fresh".
  bool weatherKnown_ = false;
  bool raining_ = false;
  bool rainExpected_ = false;
  uint32_t weatherAgeMs_ = 0;
  uint32_t weatherRefMs_ = 0;

  bool sensorWet_ = false;  // rain plate verdict with hysteresis

  Wet wet_ = Wet::Unknown;
  uint32_t wetSinceMs_ = 0;
  bool confirmed_ = false;  // latched once the current wet_ run reached its confirm time
  bool rain_ = false;
  RainSource rainSource_ = RainSource::None;
  bool failSafe_ = false;

  uint8_t events_ = 0;
  uint32_t sig_ = 0;
};

// Debounce + gesture. Also used for the limit switches (debounced level only).
class Button {
 public:
  enum Gesture : uint8_t { None, ShortPress, LongPress };

  explicit Button(uint32_t debounceMs = 30, uint32_t longPressMs = 3000)
      : debounceMs_(debounceMs), longPressMs_(longPressMs) {}

  // ShortPress fires on release (never after a LongPress); LongPress fires once
  // while still held. Hold time is measured from the first raw edge of the press.
  Gesture update(uint32_t nowMs, bool rawPressed);
  bool pressed() const { return stable_; }

 private:
  uint32_t debounceMs_;
  uint32_t longPressMs_;
  bool raw_ = false;
  bool stable_ = false;
  bool longFired_ = false;
  uint32_t rawSinceMs_ = 0;
  uint32_t pressStartMs_ = 0;
};

}  // namespace awning

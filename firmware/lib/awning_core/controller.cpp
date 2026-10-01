#include <string.h>

#include "awning_core.h"

namespace awning {

namespace {

uint32_t satAdd(uint32_t a, uint32_t b) {
  const uint32_t s = a + b;
  return s < a ? 0xFFFFFFFFu : s;
}

}  // namespace

const char* toString(State s) {
  switch (s) {
    case State::Open: return "OPEN";
    case State::Closing: return "CLOSING";
    case State::Closed: return "CLOSED";
    case State::Opening: return "OPENING";
    case State::Error: break;
  }
  return "ERROR";
}

const char* toString(Mode m) { return m == Mode::Manual ? "MANUAL" : "AUTO"; }

const char* toString(RainSource r) {
  switch (r) {
    case RainSource::Api: return "api";
    case RainSource::Sim: return "sim";
    case RainSource::Local: return "local";
    case RainSource::Sensor: return "sensor";
    case RainSource::None: break;
  }
  return "none";
}

bool parseAction(const char* s, Action* out) {
  static const struct {
    const char* name;
    Action action;
  } kActions[] = {
      {"open", Action::Open},
      {"close", Action::Close},
      {"auto", Action::Auto},
      {"simulate_rain", Action::SimulateRain},
      {"clear_rain", Action::ClearRain},
  };
  if (s == nullptr || out == nullptr) return false;
  for (size_t i = 0; i < sizeof(kActions) / sizeof(kActions[0]); ++i) {
    if (strcmp(s, kActions[i].name) == 0) {
      *out = kActions[i].action;
      return true;
    }
  }
  return false;
}

Controller::Controller(const Config& cfg) : cfg_(cfg) {}

void Controller::begin(uint32_t nowMs, bool limitClosed, bool limitOpen) {
  mode_ = Mode::Auto;
  sim_ = false;
  bootMs_ = nowMs;
  manualSinceMs_ = nowMs;
  prevLimitClosed_ = limitClosed;
  prevLimitOpen_ = limitOpen;

  weatherKnown_ = false;
  raining_ = false;
  rainExpected_ = false;
  weatherAgeMs_ = 0;
  weatherRefMs_ = nowMs;

  sensorWet_ = false;
  wet_ = Wet::Unknown;
  wetSinceMs_ = nowMs;
  confirmed_ = false;
  rain_ = false;
  rainSource_ = RainSource::None;
  failSafe_ = false;

  if (limitClosed) {
    enterState(nowMs, State::Closed);
  } else if (limitOpen) {
    enterState(nowMs, State::Open);
  } else if (cfg_.simTravelMs > 0) {
    // No motor: assume the awning starts open instead of homing forever.
    enterState(nowMs, State::Open);
  } else {
    enterState(nowMs, State::Closing);  // homing: position unknown, go to the closed end
  }
  events_ = 0;
  sig_ = signature();
}

// Restarts the state clock; for Closing/Opening this is also the start of the relay dead time.
void Controller::enterState(uint32_t now, State s) {
  state_ = s;
  motionStartMs_ = now;
}

bool Controller::inDeadTime(uint32_t now) const {
  return (state_ == State::Closing || state_ == State::Opening) &&
         elapsedMs(now, motionStartMs_) < cfg_.deadTimeMs;
}

void Controller::onWeather(uint32_t nowMs, bool isRaining, bool rainExpected15m, uint32_t ageS) {
  weatherKnown_ = true;
  raining_ = isRaining;
  rainExpected_ = rainExpected15m;
  weatherAgeMs_ = ageS > 4294967u ? 0xFFFFFFFFu : ageS * 1000u;  // saturate instead of overflow
  weatherRefMs_ = nowMs;
}

void Controller::goManual(uint32_t now, State direction) {
  mode_ = Mode::Manual;
  manualSinceMs_ = now;
  if (direction == State::Closing) {
    if (state_ != State::Closed && state_ != State::Closing) enterState(now, State::Closing);
  } else {
    if (state_ != State::Open && state_ != State::Opening) enterState(now, State::Opening);
  }
}

void Controller::onCommand(uint32_t nowMs, Action action) {
  switch (action) {
    case Action::Open: goManual(nowMs, State::Opening); break;
    case Action::Close: goManual(nowMs, State::Closing); break;
    case Action::Auto: mode_ = Mode::Auto; break;
    case Action::SimulateRain: sim_ = true; break;
    case Action::ClearRain: sim_ = false; break;
  }
  noteChanges();
}

void Controller::onManualButton(uint32_t nowMs) {
  const bool wantClose = state_ == State::Open || state_ == State::Opening || state_ == State::Error;
  goManual(nowMs, wantClose ? State::Closing : State::Opening);
  noteChanges();
}

// --- weather age --------------------------------------------------------------

void Controller::foldWeatherAge(uint32_t now) {
  if (!weatherKnown_) return;
  weatherAgeMs_ = satAdd(weatherAgeMs_, elapsedMs(now, weatherRefMs_));
  weatherRefMs_ = now;
}

uint32_t Controller::weatherAgeMsAt(uint32_t now) const {
  return satAdd(weatherAgeMs_, elapsedMs(now, weatherRefMs_));
}

// --- wetness ------------------------------------------------------------------

void Controller::trackWetness(const Inputs& in) {
  const uint32_t now = in.nowMs;

  // The rain plate: wet from rainWetAbove up, dry again from rainDryBelow down, unchanged in between.
  // Without a valid reading there is no verdict at all: never keep claiming a wet plate.
  if (!in.rainSensorValid) {
    sensorWet_ = false;
  } else if (!sensorWet_ && in.rainLevel >= cfg_.rainWetAbove) {
    sensorWet_ = true;
  } else if (sensorWet_ && in.rainLevel <= cfg_.rainDryBelow) {
    sensorWet_ = false;
  }

  const bool weatherFresh = weatherKnown_ && weatherAgeMs_ <= cfg_.weatherStaleMs;
  // Right after boot give the backend time to send the first message before falling back to local sensors.
  const bool inGrace = !weatherKnown_ && elapsedMs(now, bootMs_) <= cfg_.firstWeatherGraceMs;
  const bool weatherLost = !weatherFresh && !inGrace;

  Wet w = Wet::Unknown;
  RainSource src = RainSource::None;
  bool failSafe = false;

  if (sim_) {
    w = Wet::Wet;
    src = RainSource::Sim;
  } else {
    failSafe = weatherLost;  // reports the weather feed, whatever the plate says
    if (sensorWet_) {
      // Physical evidence: it counts with or without weather, and outranks the forecast and the local rule.
      w = Wet::Wet;
      src = RainSource::Sensor;
    } else if (weatherFresh) {
      if (raining_ || rainExpected_) {
        w = Wet::Wet;
        src = RainSource::Api;
      } else {
        w = Wet::Dry;
      }
    } else if (weatherLost && in.humidityValid && in.humidity >= cfg_.localHumidityHigh &&
               in.light < cfg_.localDarkBelow) {
      w = Wet::Wet;
      src = RainSource::Local;
    }
    // Otherwise UNKNOWN: a dry plate says nothing about a missing forecast, so nothing is counted.
  }

  // Any change of the tri-state restarts the run; UNKNOWN therefore resets both timers.
  if (w != wet_) {
    wet_ = w;
    wetSinceMs_ = now;
    confirmed_ = false;
  }
  if (!confirmed_ && w != Wet::Unknown) {
    uint32_t need = cfg_.dryConfirmMs;
    if (w == Wet::Wet) {
      need = src == RainSource::Sim ? 0u : src == RainSource::Sensor ? cfg_.sensorRainConfirmMs : cfg_.rainConfirmMs;
    }
    if (elapsedMs(now, wetSinceMs_) >= need) confirmed_ = true;  // latched: immune to wrap-around
  }

  rain_ = (w == Wet::Wet);
  rainSource_ = rain_ ? src : RainSource::None;
  failSafe_ = failSafe;
}

// --- motion -------------------------------------------------------------------

void Controller::progressMotion(const Inputs& in) {
  const uint32_t t = elapsedMs(in.nowMs, motionStartMs_);
  switch (state_) {
    case State::Closing:
      if (in.limitClosed || (cfg_.simTravelMs > 0 && t >= cfg_.simTravelMs)) {
        state_ = State::Closed;
      } else if (t >= cfg_.travelTimeoutMs) {
        state_ = State::Error;
        events_ |= EvAlarm;
      }
      break;
    case State::Opening:
      if (in.limitOpen || (cfg_.simTravelMs > 0 && t >= cfg_.simTravelMs)) {
        state_ = State::Open;
      } else if (t >= cfg_.travelTimeoutMs) {
        state_ = State::Error;
        events_ |= EvAlarm;
      }
      break;
    case State::Error:
      // A *new* press tells us where the awning is. Level-triggered would loop forever
      // on a stuck switch (Error -> Closed -> auto-open -> Error ...).
      if (in.limitClosed && !prevLimitClosed_) {
        state_ = State::Closed;
      } else if (in.limitOpen && !prevLimitOpen_) {
        state_ = State::Open;
      }
      break;
    case State::Open:
    case State::Closed:
      break;
  }
}

void Controller::applyAutoRule(uint32_t now) {
  if (mode_ != Mode::Auto || state_ == State::Error || inDeadTime(now) || !confirmed_) return;
  if (wet_ == Wet::Wet && (state_ == State::Open || state_ == State::Opening)) {
    enterState(now, State::Closing);
    events_ |= EvBeep;
  } else if (wet_ == Wet::Dry && (state_ == State::Closed || state_ == State::Closing)) {
    enterState(now, State::Opening);
  }
}

void Controller::update(const Inputs& in) {
  const uint32_t now = in.nowMs;
  foldWeatherAge(now);
  trackWetness(in);
  if (mode_ == Mode::Manual && elapsedMs(now, manualSinceMs_) >= cfg_.manualTimeoutMs) {
    mode_ = Mode::Auto;
  }
  progressMotion(in);
  applyAutoRule(now);
  prevLimitClosed_ = in.limitClosed;
  prevLimitOpen_ = in.limitOpen;
  noteChanges();
}

// --- outputs ------------------------------------------------------------------

Snapshot Controller::snapshot(uint32_t nowMs) const {
  Snapshot s;
  s.state = state_;
  s.mode = mode_;
  // Outputs are derived from state + time: only one direction can ever be true, and
  // neither is true during the dead time after a start/reversal.
  const bool relayLive = elapsedMs(nowMs, motionStartMs_) >= cfg_.deadTimeMs;
  s.driveClose = state_ == State::Closing && relayLive;
  s.driveOpen = state_ == State::Opening && relayLive;
  s.rain = rain_;
  s.rainSource = rainSource_;
  s.failSafe = failSafe_;
  s.rainSensorWet = sensorWet_;
  s.weatherAgeS = weatherKnown_ ? static_cast<int32_t>(weatherAgeMsAt(nowMs) / 1000u) : -1;
  if (mode_ == Mode::Manual) {
    const uint32_t e = elapsedMs(nowMs, manualSinceMs_);
    const uint32_t remain = e >= cfg_.manualTimeoutMs ? 0u : cfg_.manualTimeoutMs - e;
    s.manualLeftS = (remain + 999u) / 1000u;
  }
  return s;
}

uint32_t Controller::signature() const {
  return static_cast<uint32_t>(state_) | (static_cast<uint32_t>(mode_) << 3) |
         (static_cast<uint32_t>(rain_) << 4) | (static_cast<uint32_t>(rainSource_) << 5) |
         (static_cast<uint32_t>(failSafe_) << 8) | (static_cast<uint32_t>(sensorWet_) << 9);
}

void Controller::noteChanges() {
  const uint32_t sig = signature();
  if (sig != sig_) {
    sig_ = sig;
    events_ |= EvChanged;
  }
}

uint8_t Controller::consumeEvents() {
  const uint8_t e = events_;
  events_ = 0;
  return e;
}

}  // namespace awning

// Maps the macros of config.h onto awning::Config for the Controller.
// Kept in its own Arduino-free header so the host tests can check the mapping.
#pragma once
#include "awning_core.h"
#include "config.h"

inline awning::Config appConfig() {
  awning::Config c;
  c.rainConfirmMs = RAIN_CONFIRM_MS;
  c.dryConfirmMs = DRY_CONFIRM_MS;
  c.manualTimeoutMs = MANUAL_TIMEOUT_MS;
  c.travelTimeoutMs = TRAVEL_TIMEOUT_MS;
  c.deadTimeMs = RELAY_DEAD_TIME_MS;
  c.simTravelMs = SIM_TRAVEL_MS;
  c.weatherStaleMs = WEATHER_STALE_MS;
  c.firstWeatherGraceMs = FIRST_WEATHER_GRACE_MS;
  c.localHumidityHigh = LOCAL_HUMIDITY_HIGH;
  c.localDarkBelow = LOCAL_DARK_BELOW;
  return c;
}

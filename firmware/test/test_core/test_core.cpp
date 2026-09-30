// Host tests for lib/awning_core (run: pio test -e native).
// Time is simulated: every test drives Controller::update() itself in 20 ms ticks,
// exactly like the firmware's control loop.
#include <stdint.h>
#include <string.h>
#include <unity.h>

#include <vector>

#include "app_config.h"
#include "awning_core.h"
#include "config.h"

using namespace awning;

void setUp(void) {}
void tearDown(void) {}

#define ASSERT_STATE(expected, snap) \
  TEST_ASSERT_EQUAL_STRING(toString(expected), toString((snap).state))
#define ASSERT_MODE(expected, snap) \
  TEST_ASSERT_EQUAL_STRING(toString(expected), toString((snap).mode))
#define ASSERT_SRC(expected, snap) \
  TEST_ASSERT_EQUAL_STRING(toString(expected), toString((snap).rainSource))

namespace {

const uint32_t kStart = 100000;       // arbitrary non-zero boot time
const uint32_t kMinute = 60UL * 1000UL;

Config makeCfg(uint32_t simTravelMs) {
  Config c;  // contract defaults (docs/mqtt-topics.md)
  c.simTravelMs = simTravelMs;
  return c;
}

struct Rig {
  Config cfg;
  Controller c;
  Inputs in;
  uint32_t now;

  explicit Rig(const Config& cfg_, uint32_t start = kStart) : cfg(cfg_), c(cfg_), now(start) {}

  void boot(bool limitClosed = false, bool limitOpen = false) {
    in.limitClosed = limitClosed;
    in.limitOpen = limitOpen;
    in.nowMs = now;
    c.begin(now, limitClosed, limitOpen);
  }
  void tick(uint32_t stepMs = 20) {
    now += stepMs;  // wraps like millis()
    in.nowMs = now;
    c.update(in);
  }
  // Advance exactly `ms` in steps of at most `stepMs`.
  void run(uint32_t ms, uint32_t stepMs = 20) {
    while (ms > 0) {
      const uint32_t s = ms < stepMs ? ms : stepMs;
      tick(s);
      ms -= s;
    }
  }
  Snapshot snap() const { return c.snapshot(now); }
  // Deliver a weather message and let the controller observe it at this very instant.
  void weather(bool raining, bool expected = false, uint32_t ageS = 0) {
    c.onWeather(now, raining, expected, ageS);
    tick(0);
  }
  void cmd(Action a) { c.onCommand(now, a); }
  void button() { c.onManualButton(now); }
  void humid(float humidity, int light) {
    in.humidityValid = true;
    in.humidity = humidity;
    in.light = light;
  }
};

}  // namespace

// ---------------------------------------------------------------------------------
// Strings, parsing, config
// ---------------------------------------------------------------------------------

void test_strings_and_parse_action() {
  TEST_ASSERT_EQUAL_STRING("OPEN", toString(State::Open));
  TEST_ASSERT_EQUAL_STRING("CLOSING", toString(State::Closing));
  TEST_ASSERT_EQUAL_STRING("CLOSED", toString(State::Closed));
  TEST_ASSERT_EQUAL_STRING("OPENING", toString(State::Opening));
  TEST_ASSERT_EQUAL_STRING("ERROR", toString(State::Error));
  TEST_ASSERT_EQUAL_STRING("AUTO", toString(Mode::Auto));
  TEST_ASSERT_EQUAL_STRING("MANUAL", toString(Mode::Manual));
  TEST_ASSERT_EQUAL_STRING("none", toString(RainSource::None));
  TEST_ASSERT_EQUAL_STRING("api", toString(RainSource::Api));
  TEST_ASSERT_EQUAL_STRING("sim", toString(RainSource::Sim));
  TEST_ASSERT_EQUAL_STRING("local", toString(RainSource::Local));

  const struct {
    const char* name;
    Action action;
  } good[] = {{"open", Action::Open},
              {"close", Action::Close},
              {"auto", Action::Auto},
              {"simulate_rain", Action::SimulateRain},
              {"clear_rain", Action::ClearRain}};
  for (size_t i = 0; i < sizeof(good) / sizeof(good[0]); ++i) {
    Action a = Action::Auto;
    if (good[i].action == Action::Auto) a = Action::Open;  // make sure it really gets written
    TEST_ASSERT_TRUE(parseAction(good[i].name, &a));
    TEST_ASSERT_EQUAL_INT(static_cast<int>(good[i].action), static_cast<int>(a));
  }

  const char* bad[] = {"", "OPEN", "Open", " open", "open ", "toggle", "simulate", "clear-rain", "opens"};
  for (size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); ++i) {
    Action a = Action::Close;
    TEST_ASSERT_FALSE(parseAction(bad[i], &a));
    TEST_ASSERT_EQUAL_INT(static_cast<int>(Action::Close), static_cast<int>(a));  // untouched
  }
  Action a = Action::Close;
  TEST_ASSERT_FALSE(parseAction(nullptr, &a));
  TEST_ASSERT_FALSE(parseAction("open", nullptr));
}

// Guards docs/mqtt-topics.md, docs/wiring.md against silent edits of include/config.h.
#ifndef DEMO_FAST_TIMERS
void test_config_h_matches_contract() {
  const Config d;  // defaults in awning_core.h = documented contract values
  TEST_ASSERT_EQUAL_UINT32(d.rainConfirmMs, RAIN_CONFIRM_MS);
  TEST_ASSERT_EQUAL_UINT32(d.dryConfirmMs, DRY_CONFIRM_MS);
  TEST_ASSERT_EQUAL_UINT32(d.manualTimeoutMs, MANUAL_TIMEOUT_MS);
  TEST_ASSERT_EQUAL_UINT32(d.travelTimeoutMs, TRAVEL_TIMEOUT_MS);
  TEST_ASSERT_EQUAL_UINT32(d.deadTimeMs, RELAY_DEAD_TIME_MS);
  TEST_ASSERT_EQUAL_UINT32(d.weatherStaleMs, WEATHER_STALE_MS);
  TEST_ASSERT_EQUAL_UINT32(d.firstWeatherGraceMs, FIRST_WEATHER_GRACE_MS);
  TEST_ASSERT_EQUAL_FLOAT(d.localHumidityHigh, LOCAL_HUMIDITY_HIGH);
  TEST_ASSERT_EQUAL_INT(d.localDarkBelow, LOCAL_DARK_BELOW);
  TEST_ASSERT_EQUAL_UINT32(8000, SIM_TRAVEL_MS);
  TEST_ASSERT_EQUAL_UINT32(5000, TELEMETRY_PERIOD_MS);
  TEST_ASSERT_EQUAL_INT(1, RELAY_ACTIVE_LOW);
  TEST_ASSERT_EQUAL_INT(0, LDR_INVERT);
  TEST_ASSERT_EQUAL_STRING("pkg/awning01/", MQTT_TOPIC_PREFIX);
  TEST_ASSERT_EQUAL_STRING("esp32-awning01", MQTT_CLIENT_ID);
  // Pins: docs/wiring.md
  TEST_ASSERT_EQUAL_INT(4, PIN_DHT);
  TEST_ASSERT_EQUAL_INT(34, PIN_LDR);
  TEST_ASSERT_EQUAL_INT(21, PIN_I2C_SDA);
  TEST_ASSERT_EQUAL_INT(22, PIN_I2C_SCL);
  TEST_ASSERT_EQUAL_INT(26, PIN_RELAY_CLOSE);
  TEST_ASSERT_EQUAL_INT(27, PIN_RELAY_OPEN);
  TEST_ASSERT_EQUAL_INT(32, PIN_LIMIT_CLOSED);
  TEST_ASSERT_EQUAL_INT(33, PIN_LIMIT_OPEN);
  TEST_ASSERT_EQUAL_INT(25, PIN_BTN_MANUAL);
  TEST_ASSERT_EQUAL_INT(23, PIN_BUZZER);
}
#endif

// The Config that main.cpp really feeds to the controller (macros in config.h -> struct).
#ifndef DEMO_FAST_TIMERS
void test_app_config_maps_every_field_to_the_documented_value() {
  const Config c = appConfig();
  TEST_ASSERT_EQUAL_UINT32(30000, c.rainConfirmMs);
  TEST_ASSERT_EQUAL_UINT32(900000, c.dryConfirmMs);
  TEST_ASSERT_EQUAL_UINT32(600000, c.manualTimeoutMs);
  TEST_ASSERT_EQUAL_UINT32(30000, c.travelTimeoutMs);
  TEST_ASSERT_EQUAL_UINT32(200, c.deadTimeMs);
  TEST_ASSERT_EQUAL_UINT32(8000, c.simTravelMs);
  TEST_ASSERT_EQUAL_UINT32(1800000, c.weatherStaleMs);
  TEST_ASSERT_EQUAL_UINT32(120000, c.firstWeatherGraceMs);
  TEST_ASSERT_EQUAL_FLOAT(85.0f, c.localHumidityHigh);
  TEST_ASSERT_EQUAL_INT(800, c.localDarkBelow);
}
#endif

// ---------------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------------

void test_boot_closed_limit_pressed() {
  Rig r(makeCfg(8000));
  r.boot(true, false);
  const Snapshot s = r.snap();
  ASSERT_STATE(State::Closed, s);
  ASSERT_MODE(Mode::Auto, s);
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);
  TEST_ASSERT_EQUAL_INT32(-1, s.weatherAgeS);
  TEST_ASSERT_EQUAL_UINT8(0, r.c.consumeEvents());
}

void test_boot_open_limit_pressed() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  ASSERT_STATE(State::Open, r.snap());
}

void test_boot_both_limits_prefers_closed() {
  Rig r(makeCfg(0));
  r.boot(true, true);
  ASSERT_STATE(State::Closed, r.snap());
}

void test_boot_no_limit_with_sim_travel_assumes_open() {
  Rig r(makeCfg(8000));
  r.boot();
  const Snapshot s = r.snap();
  ASSERT_STATE(State::Open, s);
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);
}

void test_boot_no_limit_no_sim_homes_to_closed() {
  Rig r(makeCfg(0));
  r.boot();
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closing, s);
  TEST_ASSERT_FALSE(s.driveClose);  // dead time first
  r.run(200);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.run(2000);
  r.in.limitClosed = true;
  r.tick();
  s = r.snap();
  ASSERT_STATE(State::Closed, s);
  TEST_ASSERT_FALSE(s.driveClose);
}

// ---------------------------------------------------------------------------------
// Rain rule: confirm timers
// ---------------------------------------------------------------------------------

void test_wet_confirmed_after_30s_then_closes_with_beep() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true);  // WET(api) observed now
  Snapshot s = r.snap();
  TEST_ASSERT_TRUE(s.rain);
  ASSERT_SRC(RainSource::Api, s);
  ASSERT_STATE(State::Open, s);
  TEST_ASSERT_EQUAL_UINT8(EvChanged, r.c.consumeEvents());  // rain flag changed, nothing else yet

  r.run(29980);
  ASSERT_STATE(State::Open, r.snap());
  TEST_ASSERT_EQUAL_UINT8(0, r.c.consumeEvents());
  r.tick(20);  // exactly 30 s of continuous rain
  s = r.snap();
  ASSERT_STATE(State::Closing, s);
  TEST_ASSERT_FALSE(s.driveClose);  // relay dead time
  const uint8_t ev = r.c.consumeEvents();
  TEST_ASSERT_TRUE(ev & EvBeep);
  TEST_ASSERT_TRUE(ev & EvChanged);
  TEST_ASSERT_FALSE(ev & EvAlarm);
  r.run(200);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  TEST_ASSERT_FALSE(r.snap().driveOpen);
}

void test_rain_expected_15m_counts_as_wet() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(false, true);
  const Snapshot s = r.snap();
  TEST_ASSERT_TRUE(s.rain);
  ASSERT_SRC(RainSource::Api, s);
  r.run(30000);
  ASSERT_STATE(State::Closing, r.snap());
}

void test_wet_flicker_resets_the_confirm_timer() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true);
  r.run(20000);
  r.weather(false);  // DRY for a moment
  TEST_ASSERT_FALSE(r.snap().rain);
  r.run(1000);
  r.weather(true);  // wet again: the 30 s starts over
  r.run(20000);     // 40 s of wet in total, but only 20 s since the flicker
  ASSERT_STATE(State::Open, r.snap());
  r.run(9980);
  ASSERT_STATE(State::Open, r.snap());
  r.tick(20);       // 30 s since the flicker
  ASSERT_STATE(State::Closing, r.snap());
}

void test_sim_rain_closes_immediately() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::SimulateRain);
  TEST_ASSERT_TRUE(r.c.simulatingRain());
  r.tick();
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closing, s);
  ASSERT_MODE(Mode::Auto, s);  // sim does not switch to manual
  TEST_ASSERT_TRUE(s.rain);
  ASSERT_SRC(RainSource::Sim, s);
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvBeep);

  r.run(180);
  TEST_ASSERT_FALSE(r.snap().driveClose);
  r.tick(20);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.run(7780);
  ASSERT_STATE(State::Closing, r.snap());  // 7980 ms since it started
  r.tick(20);                              // SIM_TRAVEL 8000 ms
  s = r.snap();
  ASSERT_STATE(State::Closed, s);
  TEST_ASSERT_FALSE(s.driveClose);
}

void test_sim_rain_clear_then_dry_reopens_after_15_min() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::SimulateRain);
  r.run(9000);
  ASSERT_STATE(State::Closed, r.snap());
  r.c.consumeEvents();  // drop the beep of the sim-rain close
  r.cmd(Action::ClearRain);
  r.weather(false);  // backend says dry: DRY starts being counted right here
  const Snapshot s = r.snap();
  TEST_ASSERT_FALSE(s.rain);
  ASSERT_SRC(RainSource::None, s);
  r.run(15 * kMinute - 20);
  ASSERT_STATE(State::Closed, r.snap());
  r.tick(20);  // 15 min of continuous dry
  ASSERT_STATE(State::Opening, r.snap());
  TEST_ASSERT_FALSE(r.c.consumeEvents() & EvBeep);  // only the auto *close* beeps
}

void test_dry_needs_continuous_15_minutes() {
  Rig r(makeCfg(8000));
  r.boot(true, false);  // Closed
  r.weather(false);
  r.run(10 * kMinute);
  r.weather(true);  // 5 s of rain: not confirmed, but it breaks the dry run
  r.run(5000);
  r.weather(false);
  r.run(15 * kMinute - 20);
  ASSERT_STATE(State::Closed, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Opening, r.snap());
}

void test_weather_heartbeats_keep_the_state_machine_running() {
  // Backend publishes every 60 s; the timers must not restart on same-value repeats.
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true, false, 3);
  for (int i = 0; i < 4; ++i) {
    r.run(9000);
    r.weather(true, false, 3);
  }
  ASSERT_STATE(State::Closing, r.snap());  // 36 s of continuous rain, repeated messages
}

// ---------------------------------------------------------------------------------
// Motion: end of travel, timeout, dead time
// ---------------------------------------------------------------------------------

void test_limit_switch_ends_travel() {
  Rig r(makeCfg(0));
  r.boot(false, true);  // Open
  r.in.limitOpen = false;
  r.cmd(Action::Close);
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closing, s);
  r.run(3000);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.in.limitClosed = true;
  r.tick();
  s = r.snap();
  ASSERT_STATE(State::Closed, s);
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);

  r.in.limitClosed = false;
  r.cmd(Action::Open);
  r.run(2000);
  TEST_ASSERT_TRUE(r.snap().driveOpen);
  r.in.limitOpen = true;
  r.tick();
  ASSERT_STATE(State::Open, r.snap());
  TEST_ASSERT_FALSE(r.snap().driveOpen);
}

void test_wrong_limit_switch_is_ignored_while_moving() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  r.in.limitOpen = false;
  r.cmd(Action::Close);
  r.run(1000);
  r.in.limitOpen = true;  // "open" limit pressed while closing: ignore
  r.run(1000);
  ASSERT_STATE(State::Closing, r.snap());
  TEST_ASSERT_TRUE(r.snap().driveClose);
}

void test_timer_ends_travel_after_sim_travel_ms() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::Close);
  r.run(7980);
  ASSERT_STATE(State::Closing, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Closed, r.snap());

  r.cmd(Action::Open);
  r.run(7980);
  ASSERT_STATE(State::Opening, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Open, r.snap());
}

void test_no_limit_within_30s_is_error_and_alarm() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  r.in.limitOpen = false;
  r.cmd(Action::Close);
  r.c.consumeEvents();
  r.run(29980);
  ASSERT_STATE(State::Closing, r.snap());
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.tick(20);
  const Snapshot s = r.snap();
  ASSERT_STATE(State::Error, s);
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);
  const uint8_t ev = r.c.consumeEvents();
  TEST_ASSERT_TRUE(ev & EvAlarm);
  TEST_ASSERT_TRUE(ev & EvChanged);
  TEST_ASSERT_EQUAL_UINT8(0, r.c.consumeEvents());  // alarm fires once, on entry
}

void test_opening_timeout_is_error_too() {
  Rig r(makeCfg(0));
  r.boot(true, false);
  r.in.limitClosed = false;
  r.cmd(Action::Open);
  r.run(30000);
  ASSERT_STATE(State::Error, r.snap());
}

void test_sim_travel_end_beats_the_timeout() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::Close);
  r.run(40000);
  ASSERT_STATE(State::Closed, r.snap());  // ended at 8 s, never reached Error
}

void test_reversal_has_dead_time_on_both_relays() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  r.in.limitOpen = false;
  r.cmd(Action::Close);
  r.run(1000);
  Snapshot s = r.snap();
  TEST_ASSERT_TRUE(s.driveClose);

  r.cmd(Action::Open);  // reverse mid-travel
  s = r.snap();
  ASSERT_STATE(State::Opening, s);
  TEST_ASSERT_FALSE(s.driveClose);  // old direction off immediately
  TEST_ASSERT_FALSE(s.driveOpen);   // new direction not before the dead time
  r.run(180);
  s = r.snap();
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);
  r.tick(20);  // 200 ms
  s = r.snap();
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_TRUE(s.driveOpen);
}

void test_auto_does_not_reverse_inside_the_dead_time() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::Close);  // Manual close, Closing (dead time running)
  r.cmd(Action::Auto);
  r.weather(false);      // will be DRY, but not confirmed for 15 min
  r.cmd(Action::SimulateRain);
  r.tick(0);             // WET(sim) confirmed instantly, state Closing: nothing to reverse
  ASSERT_STATE(State::Closing, r.snap());
  r.cmd(Action::ClearRain);
  r.cmd(Action::Open);   // manual reversal at t0, dead time starts
  r.cmd(Action::Auto);
  r.cmd(Action::SimulateRain);
  r.tick(100);           // confirmed wet, state Opening, only 100 ms into its dead time
  ASSERT_STATE(State::Opening, r.snap());
  r.tick(100);           // dead time over: now the auto rule may reverse
  ASSERT_STATE(State::Closing, r.snap());
  TEST_ASSERT_FALSE(r.snap().driveOpen);
  TEST_ASSERT_FALSE(r.snap().driveClose);
}

// ---------------------------------------------------------------------------------
// Manual mode
// ---------------------------------------------------------------------------------

void test_manual_open_close_commands_and_countdown() {
  Rig r(makeCfg(8000));
  r.boot(true, false);  // Closed
  r.cmd(Action::Open);
  Snapshot s = r.snap();
  ASSERT_MODE(Mode::Manual, s);
  ASSERT_STATE(State::Opening, s);
  TEST_ASSERT_EQUAL_UINT32(600, s.manualLeftS);
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvChanged);

  r.run(100000);
  TEST_ASSERT_EQUAL_UINT32(500, r.snap().manualLeftS);
  r.run(499980);  // 599.98 s
  s = r.snap();
  ASSERT_MODE(Mode::Manual, s);
  TEST_ASSERT_EQUAL_UINT32(1, s.manualLeftS);
  r.c.consumeEvents();
  r.tick(20);  // 600 s: back to auto
  s = r.snap();
  ASSERT_MODE(Mode::Auto, s);
  TEST_ASSERT_EQUAL_UINT32(0, s.manualLeftS);
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvChanged);
}

void test_manual_command_restarts_the_countdown() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::Close);
  r.run(300000);
  TEST_ASSERT_EQUAL_UINT32(300, r.snap().manualLeftS);
  r.cmd(Action::Open);
  TEST_ASSERT_EQUAL_UINT32(600, r.snap().manualLeftS);
}

void test_manual_command_at_the_end_position_does_nothing() {
  Rig r(makeCfg(8000));
  r.boot(true, false);  // Closed
  r.cmd(Action::Close);
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closed, s);
  ASSERT_MODE(Mode::Manual, s);  // still switches the mode
  TEST_ASSERT_FALSE(s.driveClose);
  r.cmd(Action::Auto);
  r.boot(false, true);  // Open (re-boot the same rig)
  r.cmd(Action::Open);
  ASSERT_STATE(State::Open, r.snap());
}

void test_repeating_the_running_direction_does_not_restart_the_motion() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  r.in.limitOpen = false;
  r.cmd(Action::Close);
  r.run(1000);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.cmd(Action::Close);  // already closing: must not drop the relay for a new dead time
  TEST_ASSERT_TRUE(r.snap().driveClose);
}

void test_auto_command_returns_to_auto() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::Close);
  ASSERT_MODE(Mode::Manual, r.snap());
  r.cmd(Action::Auto);
  const Snapshot s = r.snap();
  ASSERT_MODE(Mode::Auto, s);
  TEST_ASSERT_EQUAL_UINT32(0, s.manualLeftS);
}

void test_auto_command_leaves_simulated_rain_alone() {
  Rig r(makeCfg(8000));
  r.boot();
  r.cmd(Action::SimulateRain);
  r.cmd(Action::Close);  // manual
  r.cmd(Action::Auto);
  TEST_ASSERT_TRUE(r.c.simulatingRain());
  r.tick();
  ASSERT_SRC(RainSource::Sim, r.snap());
  r.cmd(Action::Open);  // manual open: sim rain must not block a manual command
  ASSERT_STATE(State::Opening, r.snap());
  TEST_ASSERT_TRUE(r.c.simulatingRain());
}

void test_manual_blocks_the_auto_rule_but_timers_keep_running() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true);
  r.run(31000);  // rain confirmed: auto closed the awning
  r.run(9000);
  ASSERT_STATE(State::Closed, r.snap());

  r.cmd(Action::Open);  // user opens it despite the rain
  r.run(9000);
  ASSERT_STATE(State::Open, r.snap());
  r.weather(true);
  r.run(5 * kMinute);
  ASSERT_STATE(State::Open, r.snap());  // manual wins while it lasts
  TEST_ASSERT_TRUE(r.snap().rain);

  r.c.consumeEvents();
  r.weather(true);
  r.run(5 * kMinute - 9000 - 20 + 20);  // reach the manual timeout (10 min after the command)
  r.weather(true);
  r.run(20);
  // Back to AUTO, and the rain has been confirmed for minutes: close at once, with a beep.
  const Snapshot s = r.snap();
  ASSERT_MODE(Mode::Auto, s);
  ASSERT_STATE(State::Closing, s);
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvBeep);
}

void test_manual_button_toggles_direction() {
  Rig r(makeCfg(0));
  r.boot(false, true);  // Open
  r.in.limitOpen = false;
  r.button();
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closing, s);
  ASSERT_MODE(Mode::Manual, s);
  r.run(500);
  r.button();  // Closing -> Opening
  ASSERT_STATE(State::Opening, r.snap());
  r.button();  // Opening -> Closing
  ASSERT_STATE(State::Closing, r.snap());
  r.in.limitClosed = true;
  r.tick();
  ASSERT_STATE(State::Closed, r.snap());
  r.in.limitClosed = false;
  r.button();  // Closed -> Opening
  ASSERT_STATE(State::Opening, r.snap());
}

// ---------------------------------------------------------------------------------
// Error and recovery
// ---------------------------------------------------------------------------------

static void driveIntoError(Rig& r, bool closing) {
  r.in.limitClosed = false;
  r.in.limitOpen = false;
  r.cmd(closing ? Action::Close : Action::Open);
  r.run(30000);
  ASSERT_STATE(State::Error, r.snap());
  r.c.consumeEvents();
}

void test_error_recovers_on_a_new_limit_press() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  driveIntoError(r, true);
  r.in.limitClosed = true;
  r.tick();
  ASSERT_STATE(State::Closed, r.snap());  // position known again
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvChanged);

  Rig r2(makeCfg(0));
  r2.boot(true, false);
  driveIntoError(r2, false);
  r2.in.limitOpen = true;
  r2.tick();
  ASSERT_STATE(State::Open, r2.snap());
}

void test_error_recovers_with_manual_command_or_button() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  driveIntoError(r, true);
  r.cmd(Action::Close);  // retry
  Snapshot s = r.snap();
  ASSERT_STATE(State::Closing, s);
  ASSERT_MODE(Mode::Manual, s);
  r.run(200);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.in.limitClosed = true;
  r.tick();
  ASSERT_STATE(State::Closed, r.snap());

  Rig r2(makeCfg(0));
  r2.boot(false, true);
  driveIntoError(r2, true);
  r2.cmd(Action::Open);
  ASSERT_STATE(State::Opening, r2.snap());

  Rig r3(makeCfg(0));
  r3.boot(false, true);
  driveIntoError(r3, true);
  r3.button();  // Error -> Close
  s = r3.snap();
  ASSERT_STATE(State::Closing, s);
  ASSERT_MODE(Mode::Manual, s);
}

void test_auto_never_retries_out_of_error() {
  Rig r(makeCfg(0));
  r.boot(false, true);
  driveIntoError(r, true);
  r.cmd(Action::Auto);
  r.weather(true);
  for (int i = 0; i < 20; ++i) {  // an hour of rain heartbeats
    r.run(3 * kMinute);
    r.weather(true);
  }
  ASSERT_STATE(State::Error, r.snap());
  r.cmd(Action::SimulateRain);
  r.run(kMinute);
  ASSERT_STATE(State::Error, r.snap());
  r.cmd(Action::ClearRain);
  r.weather(false);
  for (int i = 0; i < 20; ++i) {  // and an hour of dry weather
    r.run(3 * kMinute);
    r.weather(false);
  }
  const Snapshot s = r.snap();
  ASSERT_STATE(State::Error, s);
  TEST_ASSERT_FALSE(s.driveClose);
  TEST_ASSERT_FALSE(s.driveOpen);
}

void test_stuck_limit_switch_does_not_bounce_out_of_error() {
  Rig r(makeCfg(0));
  r.boot(true, false);        // Closed
  r.in.limitClosed = true;    // stuck pressed from now on
  r.cmd(Action::Open);        // opening, but the 'open' limit is never reached
  r.run(30000);
  ASSERT_STATE(State::Error, r.snap());
  r.run(kMinute);
  ASSERT_STATE(State::Error, r.snap());  // level alone is not a recovery
  r.in.limitClosed = false;
  r.run(100);
  ASSERT_STATE(State::Error, r.snap());
  r.in.limitClosed = true;   // a fresh press is
  r.tick();
  ASSERT_STATE(State::Closed, r.snap());
}

// ---------------------------------------------------------------------------------
// Weather age, staleness, fail-safe, UNKNOWN
// ---------------------------------------------------------------------------------

void test_weather_age_is_reported_age_plus_elapsed() {
  Rig r(makeCfg(8000));
  r.boot();
  TEST_ASSERT_EQUAL_INT32(-1, r.snap().weatherAgeS);
  r.weather(false, false, 100);
  TEST_ASSERT_EQUAL_INT32(100, r.snap().weatherAgeS);
  r.run(50000);
  TEST_ASSERT_EQUAL_INT32(150, r.snap().weatherAgeS);
  r.weather(false, false, 7);  // a newer message replaces it
  TEST_ASSERT_EQUAL_INT32(7, r.snap().weatherAgeS);
  // snapshot() alone (no update) still accounts for elapsed time
  TEST_ASSERT_EQUAL_INT32(17, r.c.snapshot(r.now + 10000).weatherAgeS);
}

void test_weather_becomes_stale_only_after_30_minutes() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(false, false, 0);
  r.run(30 * kMinute);  // age == 1800 s exactly: still fresh
  Snapshot s = r.snap();
  TEST_ASSERT_FALSE(s.failSafe);
  TEST_ASSERT_EQUAL_INT32(1800, s.weatherAgeS);
  r.tick(20);           // age > 30 min
  TEST_ASSERT_TRUE(r.snap().failSafe);
  TEST_ASSERT_TRUE(r.c.consumeEvents() & EvChanged);

  Rig r2(makeCfg(8000));
  r2.boot();
  r2.weather(false, false, 1790);  // backend already had 1790 s old data
  r2.run(10000);
  TEST_ASSERT_FALSE(r2.snap().failSafe);
  r2.tick(20);
  TEST_ASSERT_TRUE(r2.snap().failSafe);
}

void test_message_older_than_30_minutes_is_stale_immediately() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true, true, 1801);  // "raining", but the data is stale => not trusted
  const Snapshot s = r.snap();
  TEST_ASSERT_TRUE(s.failSafe);
  TEST_ASSERT_FALSE(s.rain);
  ASSERT_SRC(RainSource::None, s);
}

void test_first_weather_grace_after_boot() {
  Rig r(makeCfg(8000));
  r.boot();
  r.humid(95, 100);  // humid and dark would trigger the local rule ...
  r.run(120000);     // ... but not inside the 2 minute grace
  Snapshot s = r.snap();
  TEST_ASSERT_FALSE(s.failSafe);
  TEST_ASSERT_FALSE(s.rain);
  TEST_ASSERT_EQUAL_INT32(-1, s.weatherAgeS);
  r.tick(20);
  s = r.snap();
  TEST_ASSERT_TRUE(s.failSafe);
  TEST_ASSERT_TRUE(s.rain);
  ASSERT_SRC(RainSource::Local, s);
}

void test_local_rule_thresholds_are_inclusive_humidity_exclusive_light() {
  Rig r(makeCfg(8000));
  r.boot();
  r.run(121000);  // never any weather: fail-safe
  r.humid(84.9f, 799);
  r.tick();
  TEST_ASSERT_FALSE(r.snap().rain);
  r.humid(85.0f, 799);
  r.tick();
  TEST_ASSERT_TRUE(r.snap().rain);
  r.humid(85.0f, 800);
  r.tick();
  TEST_ASSERT_FALSE(r.snap().rain);
  r.humid(100.0f, 0);
  r.tick();
  TEST_ASSERT_TRUE(r.snap().rain);
  r.in.humidityValid = false;  // DHT lost: cannot claim rain
  r.tick();
  TEST_ASSERT_FALSE(r.snap().rain);
  TEST_ASSERT_TRUE(r.snap().failSafe);
}

void test_failsafe_local_rain_closes_and_then_holds_without_reopening() {
  Rig r(makeCfg(8000));
  r.boot();
  r.humid(90, 300);
  r.run(2 * kMinute + 20);  // grace over, humid + dark
  ASSERT_SRC(RainSource::Local, r.snap());
  r.run(29980);
  ASSERT_STATE(State::Open, r.snap());
  r.tick(20);  // 30 s of local wetness
  ASSERT_STATE(State::Closing, r.snap());
  r.run(9000);
  ASSERT_STATE(State::Closed, r.snap());

  // Sensors say "not wet": the fail-safe must hold the current state, never reopen.
  r.humid(50, 3000);
  r.run(5 * 3600 * 1000UL, 1000);
  const Snapshot s = r.snap();
  ASSERT_STATE(State::Closed, s);
  TEST_ASSERT_TRUE(s.failSafe);
  TEST_ASSERT_FALSE(s.rain);

  // A fresh dry weather message ends the fail-safe; reopening needs the normal 15 minutes.
  r.weather(false);
  TEST_ASSERT_FALSE(r.snap().failSafe);
  r.run(15 * kMinute - 20);
  ASSERT_STATE(State::Closed, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Opening, r.snap());
}

void test_unknown_resets_both_timers_and_holds_the_state() {
  Rig r(makeCfg(8000));
  r.boot(true, false);  // Closed
  r.weather(false, false, 1790);  // dry, but the data goes stale in 10 s
  r.run(9000);                    // 9 s of DRY counted
  ASSERT_STATE(State::Closed, r.snap());
  r.run(2000);                    // stale => UNKNOWN (no local evidence): timers reset
  TEST_ASSERT_TRUE(r.snap().failSafe);
  r.run(20 * kMinute);            // far beyond 15 min, but UNKNOWN never counts
  ASSERT_STATE(State::Closed, r.snap());

  r.weather(false);               // fresh dry message: DRY counting starts *now*
  r.run(15 * kMinute - 20);
  ASSERT_STATE(State::Closed, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Opening, r.snap());
}

void test_unknown_interrupts_wet_confirmation() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(true, false, 1790);   // rain, data stale in 10 s
  r.run(9000);
  TEST_ASSERT_TRUE(r.snap().rain);
  r.run(2000);                    // stale and dry-local: UNKNOWN, WET timer reset
  TEST_ASSERT_FALSE(r.snap().rain);
  r.humid(90, 100);               // local evidence appears: a brand new WET run
  r.tick();
  TEST_ASSERT_TRUE(r.snap().rain);
  ASSERT_STATE(State::Open, r.snap());  // 11 s of earlier rain do not count any more
  r.run(29980);
  ASSERT_STATE(State::Open, r.snap());
  r.tick(20);                           // 30 s since the new run began
  ASSERT_STATE(State::Closing, r.snap());
}

void test_no_weather_ever_never_moves_an_idle_awning() {
  Rig r(makeCfg(8000));
  r.boot();
  r.run(6 * 3600 * 1000UL, 1000);
  ASSERT_STATE(State::Open, r.snap());
  TEST_ASSERT_TRUE(r.snap().failSafe);

  Rig r2(makeCfg(8000));
  r2.boot();
  r2.cmd(Action::Close);
  r2.run(9000);
  r2.cmd(Action::Auto);
  r2.run(6 * 3600 * 1000UL, 1000);
  ASSERT_STATE(State::Closed, r2.snap());  // no evidence of dry weather: stays closed
}

void test_sim_rain_hides_failsafe_and_wins_over_the_api() {
  Rig r(makeCfg(8000));
  r.boot();
  r.run(121000);
  TEST_ASSERT_TRUE(r.snap().failSafe);
  r.cmd(Action::SimulateRain);
  r.tick();
  Snapshot s = r.snap();
  TEST_ASSERT_FALSE(s.failSafe);
  ASSERT_SRC(RainSource::Sim, s);
  r.weather(true);  // API also says rain: the source stays "sim"
  ASSERT_SRC(RainSource::Sim, r.snap());
  r.cmd(Action::ClearRain);
  r.tick();
  ASSERT_SRC(RainSource::Api, r.snap());
}

void test_weather_age_saturates_instead_of_wrapping() {
  Rig r(makeCfg(8000));
  r.boot();
  r.weather(false, false, 0);
  int32_t last = 0;
  for (int day = 0; day < 60; ++day) {  // 60 days, one update per day (wraps millis twice)
    r.run(24UL * 3600UL * 1000UL, 24UL * 3600UL * 1000UL);
    const Snapshot s = r.snap();
    TEST_ASSERT_TRUE(s.weatherAgeS >= last);
    TEST_ASSERT_TRUE(s.failSafe);
    last = s.weatherAgeS;
  }
  TEST_ASSERT_TRUE(last > 4000000);  // ~49.7 days worth of seconds, not a small wrapped number
  r.weather(false, false, 0xFFFFFFFFu);  // absurd age from the wire is clamped, not overflowed
  TEST_ASSERT_TRUE(r.snap().weatherAgeS > 4000000);
}

// ---------------------------------------------------------------------------------
// millis() rollover
// ---------------------------------------------------------------------------------

// Weeks of unchanged weather must not un-confirm it (a naive "elapsed >= N" on a 32-bit
// clock breaks after 24.8 days, or wraps after 49.7).
void test_confirmation_survives_weeks_of_unchanged_weather() {
  for (int wet = 0; wet < 2; ++wet) {
    Rig r(makeCfg(8000), 0xFFFFFFFFu - 3 * 24 * 3600 * 1000UL);
    r.boot(wet == 0, wet != 0);  // dry test starts Closed, rain test starts Open
    for (int i = 0; i < 30 * 24 * 3; ++i) {  // 30 days of heartbeats every 20 minutes
      r.weather(wet == 1, false, 5);
      r.run(20 * kMinute, kMinute);
    }
    r.weather(wet == 1, false, 5);
    // Confirmed long ago. A manual override that expires must be undone at once.
    r.cmd(wet == 1 ? Action::Open : Action::Close);
    r.run(9000);
    ASSERT_STATE(wet == 1 ? State::Open : State::Closed, r.snap());
    r.run(10 * kMinute - 9000 - 20);
    r.weather(wet == 1, false, 5);
    r.run(20);
    const Snapshot s = r.snap();
    ASSERT_MODE(Mode::Auto, s);
    ASSERT_STATE(wet == 1 ? State::Closing : State::Opening, s);
  }
}

namespace {

struct TracePoint {
  uint32_t rel;
  State state;
  Mode mode;
  bool rain;
  RainSource src;
  bool failSafe;
  int32_t ageS;
  uint32_t manualLeft;
  bool driveClose;
  bool driveOpen;
};

// One long scenario touching every timer (rain confirm, travel, dry confirm, manual,
// weather age, staleness, dead time). Returns the trace of everything observable.
std::vector<TracePoint> runScenario(uint32_t startMs) {
  std::vector<TracePoint> trace;
  Rig r(makeCfg(8000), startMs);
  r.boot();
  const uint32_t t0 = r.now;
  bool first = true;
  Snapshot prev;

  struct Local {
    static void record(Rig& rig, uint32_t t0_, std::vector<TracePoint>& tr, bool force, bool& first_, Snapshot& prev_) {
      const Snapshot s = rig.snap();
      const bool changed = first_ || s.state != prev_.state || s.mode != prev_.mode ||
                           s.rain != prev_.rain || s.rainSource != prev_.rainSource ||
                           s.failSafe != prev_.failSafe || s.driveClose != prev_.driveClose ||
                           s.driveOpen != prev_.driveOpen;
      if (changed || force) {
        TracePoint p;
        p.rel = rig.now - t0_;
        p.state = s.state;
        p.mode = s.mode;
        p.rain = s.rain;
        p.src = s.rainSource;
        p.failSafe = s.failSafe;
        p.ageS = s.weatherAgeS;
        p.manualLeft = s.manualLeftS;
        p.driveClose = s.driveClose;
        p.driveOpen = s.driveOpen;
        tr.push_back(p);
      }
      prev_ = s;
      first_ = false;
    }
  };

  // Script: (relative time in ms) -> action. Ticks are 20 ms; a checkpoint every 100 s.
  const uint32_t total = 75 * kMinute;
  for (uint32_t rel = 0; rel < total; rel += 20) {
    if (rel == 5000 || (rel >= 60000 && rel < 5 * kMinute && rel % 60000 == 0)) r.c.onWeather(r.now, true, false, 2);
    if (rel >= 5 * kMinute && rel < 40 * kMinute && rel % 60000 == 0) r.c.onWeather(r.now, false, false, 5);
    if (rel == 20 * kMinute + 500) r.cmd(Action::Close);         // manual close while dry
    if (rel == 22 * kMinute) r.cmd(Action::Open);                // manual reversal
    if (rel == 41 * kMinute) r.cmd(Action::SimulateRain);        // sim: immediate close
    if (rel == 45 * kMinute) r.cmd(Action::ClearRain);
    if (rel == 46 * kMinute) r.button();                         // manual button
    if (rel >= 47 * kMinute && rel < 60 * kMinute && rel % 60000 == 0) r.c.onWeather(r.now, false, false, 0);
    // after 60 min: silence => stale at 90 min of data age (not reached), grace irrelevant
    r.tick(20);
    Local::record(r, t0, trace, rel % (100 * 1000) == 0, first, prev);
  }
  return trace;
}

}  // namespace

void test_rollover_gives_identical_behaviour_at_any_start_time() {
  const std::vector<TracePoint> base = runScenario(1000);
  TEST_ASSERT_TRUE(base.size() > 40);  // the scenario really does something

  const uint32_t starts[] = {
      0xFFFFFFFFu - 20 * kMinute,  // wraps in the middle of the rain/dry phases
      0xFFFFFFFFu - 500000u,       // wraps during the first weather heartbeats
      0xFFFFFFFFu - 100,           // wraps right at the start
      0xFFFFFFFFu,
      0u,
      0x7FFFFFFFu - 30 * kMinute,  // signed rollover in the middle
      0x80000000u - 50,
      4200000000u,
  };
  for (size_t i = 0; i < sizeof(starts) / sizeof(starts[0]); ++i) {
    const std::vector<TracePoint> t = runScenario(starts[i]);
    TEST_ASSERT_EQUAL_UINT32_MESSAGE(base.size(), t.size(), "trace length differs");
    for (size_t k = 0; k < base.size(); ++k) {
      TEST_ASSERT_EQUAL_UINT32(base[k].rel, t[k].rel);
      TEST_ASSERT_EQUAL_INT(static_cast<int>(base[k].state), static_cast<int>(t[k].state));
      TEST_ASSERT_EQUAL_INT(static_cast<int>(base[k].mode), static_cast<int>(t[k].mode));
      TEST_ASSERT_EQUAL_INT(base[k].rain, t[k].rain);
      TEST_ASSERT_EQUAL_INT(static_cast<int>(base[k].src), static_cast<int>(t[k].src));
      TEST_ASSERT_EQUAL_INT(base[k].failSafe, t[k].failSafe);
      TEST_ASSERT_EQUAL_INT32(base[k].ageS, t[k].ageS);
      TEST_ASSERT_EQUAL_UINT32(base[k].manualLeft, t[k].manualLeft);
      TEST_ASSERT_EQUAL_INT(base[k].driveClose, t[k].driveClose);
      TEST_ASSERT_EQUAL_INT(base[k].driveOpen, t[k].driveOpen);
    }
  }
}

void test_rollover_specific_timers_across_the_wrap() {
  // Wet confirm, sim travel, dead time and manual countdown, each straddling 2^32.
  Rig r(makeCfg(8000), 0xFFFFFFFFu - 10000u);
  r.boot();
  r.weather(true);
  r.run(29980);  // crosses the wrap after 10 s
  ASSERT_STATE(State::Open, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Closing, r.snap());
  r.run(180);
  TEST_ASSERT_FALSE(r.snap().driveClose);
  r.tick(20);
  TEST_ASSERT_TRUE(r.snap().driveClose);
  r.run(7780);
  ASSERT_STATE(State::Closing, r.snap());
  r.tick(20);
  ASSERT_STATE(State::Closed, r.snap());

  Rig m(makeCfg(8000), 0xFFFFFFFFu - 1000u);
  m.boot();
  m.cmd(Action::Close);  // manual timer starts 1 s before the wrap
  m.run(599980);
  ASSERT_MODE(Mode::Manual, m.snap());
  TEST_ASSERT_EQUAL_UINT32(1, m.snap().manualLeftS);
  m.tick(20);
  ASSERT_MODE(Mode::Auto, m.snap());
}

// ---------------------------------------------------------------------------------
// Fuzz: invariants that must hold whatever the world throws at the controller
// ---------------------------------------------------------------------------------

namespace {

struct Rng {
  uint32_t s;
  explicit Rng(uint32_t seed) : s(seed ? seed : 0x9E3779B9u) {}
  uint32_t next() {
    s ^= s << 13;
    s ^= s >> 17;
    s ^= s << 5;
    return s;
  }
  uint32_t below(uint32_t n) { return next() % n; }
  bool chance(uint32_t oneIn) { return below(oneIn) == 0; }
};

bool validTransition(State from, State to) {
  if (from == to) return true;
  switch (from) {
    case State::Open: return to == State::Closing;
    case State::Closing: return to == State::Closed || to == State::Opening || to == State::Error;
    case State::Closed: return to == State::Opening;
    case State::Opening: return to == State::Open || to == State::Closing || to == State::Error;
    case State::Error:
      return to == State::Closed || to == State::Open || to == State::Closing || to == State::Opening;
  }
  return false;
}

// How much of the state space the fuzzer really visited (guards against a vacuous run).
struct Coverage {
  long errors = 0, alarms = 0, beeps = 0, changed = 0, reversals = 0, manualReverts = 0;
  long sim = 0, api = 0, local = 0, failSafe = 0, errorLimitExit = 0, errorCmdExit = 0;
  long closings = 0, openings = 0, closedReached = 0, openReached = 0;
};
Coverage g_cov;

struct Fuzzer {
  Rig rig;
  Rng rng;
  Snapshot prev;
  uint32_t enterMs;         // when the current Closing/Opening began
  bool haveCloseOn = false, haveOpenOn = false;
  uint32_t lastCloseOnMs = 0, lastOpenOnMs = 0;
  int32_t prevAge = -1;
  bool weatherArrived = false;
  uint32_t maxStepMs = 20;
  long checks = 0;

  uint32_t stuckUntilMs = 0;  // limit switches frozen (motor jammed / switch dead) until then
  bool stuck = false;
  bool lastLimitClosed = false;  // inputs seen by the previous update()
  bool lastLimitOpen = false;

  Fuzzer(const Config& cfg, uint32_t seed, uint32_t start) : rig(cfg, start), rng(seed), enterMs(start) {}

  enum Call { CallUpdate, CallCommand, CallButton, CallWeather };

  // Check everything that must hold after one controller call.
  void check(Call call) {
    const Snapshot s = rig.snap();
    const uint8_t ev = rig.c.consumeEvents();
    ++checks;

    // 1. Relay interlock: never both, only in the matching state, never in Error.
    TEST_ASSERT_FALSE(s.driveClose && s.driveOpen);
    if (s.driveClose) TEST_ASSERT_TRUE(s.state == State::Closing);
    if (s.driveOpen) TEST_ASSERT_TRUE(s.state == State::Opening);
    if (s.state == State::Error) TEST_ASSERT_FALSE(s.driveClose || s.driveOpen);

    // 2. Only legal state transitions.
    TEST_ASSERT_TRUE(validTransition(prev.state, s.state));

    // 3. Dead time: a relay may only come on >= deadTimeMs after the other was last on.
    if (s.driveClose && !prev.driveClose && haveOpenOn) {
      TEST_ASSERT_TRUE(elapsedMs(rig.now, lastOpenOnMs) >= rig.cfg.deadTimeMs);
    }
    if (s.driveOpen && !prev.driveOpen && haveCloseOn) {
      TEST_ASSERT_TRUE(elapsedMs(rig.now, lastCloseOnMs) >= rig.cfg.deadTimeMs);
    }
    if (s.driveClose) { haveCloseOn = true; lastCloseOnMs = rig.now; }
    if (s.driveOpen) { haveOpenOn = true; lastOpenOnMs = rig.now; }

    // 4. Manual countdown consistent with mode.
    if (s.mode == Mode::Auto) TEST_ASSERT_EQUAL_UINT32(0, s.manualLeftS);
    else TEST_ASSERT_TRUE(s.manualLeftS >= 1 && s.manualLeftS <= (rig.cfg.manualTimeoutMs + 999) / 1000);

    // 5. Travel never lasts longer than the timeout (plus one stall).
    const bool moving = s.state == State::Closing || s.state == State::Opening;
    if (moving && (prev.state != s.state)) enterMs = rig.now;
    if (moving) TEST_ASSERT_TRUE(elapsedMs(rig.now, enterMs) <= rig.cfg.travelTimeoutMs + maxStepMs);

    // 6. Weather age never goes backwards between messages.
    if (call == CallWeather) weatherArrived = true;
    if (s.weatherAgeS >= 0 && prevAge >= 0 && !weatherArrived) TEST_ASSERT_TRUE(s.weatherAgeS >= prevAge);
    if (call != CallWeather && call != CallUpdate) { /* commands do not touch the age */ }
    prevAge = s.weatherAgeS;
    weatherArrived = false;
    TEST_ASSERT_TRUE(s.weatherAgeS >= -1);

    // 7. Events match the observable change.
    const bool visibleChange = s.state != prev.state || s.mode != prev.mode || s.rain != prev.rain ||
                               s.rainSource != prev.rainSource || s.failSafe != prev.failSafe;
    TEST_ASSERT_EQUAL_INT(visibleChange, (ev & EvChanged) != 0);
    TEST_ASSERT_EQUAL_INT(prev.state != State::Error && s.state == State::Error, (ev & EvAlarm) != 0);
    if (ev & EvBeep) {  // the beep belongs to an automatic close, nothing else
      TEST_ASSERT_TRUE(call == CallUpdate);
      TEST_ASSERT_TRUE(s.state == State::Closing && s.mode == Mode::Auto && s.rain);
    }

    // 8. update() starts a motion only through the automatic rule, and only leaves Error
    //    when a limit switch was freshly pressed (auto alone never retries out of Error).
    if (call == CallUpdate) {
      const bool limitEdge = (rig.in.limitClosed && !lastLimitClosed) || (rig.in.limitOpen && !lastLimitOpen);
      if (s.state == State::Closing && prev.state != State::Closing) {
        TEST_ASSERT_TRUE(s.mode == Mode::Auto && s.rain && (ev & EvBeep));
      }
      if (s.state == State::Opening && prev.state != State::Opening) {
        TEST_ASSERT_TRUE(s.mode == Mode::Auto && !s.rain);
      }
      if (prev.state == State::Error && s.state != State::Error) TEST_ASSERT_TRUE(limitEdge);
      lastLimitClosed = rig.in.limitClosed;
      lastLimitOpen = rig.in.limitOpen;
    }
    // Coverage bookkeeping.
    if (s.state == State::Error && prev.state != State::Error) ++g_cov.errors;
    if (ev & EvAlarm) ++g_cov.alarms;
    if (ev & EvBeep) ++g_cov.beeps;
    if (ev & EvChanged) ++g_cov.changed;
    if ((prev.state == State::Closing && s.state == State::Opening) ||
        (prev.state == State::Opening && s.state == State::Closing)) ++g_cov.reversals;
    if (prev.mode == Mode::Manual && s.mode == Mode::Auto && call == CallUpdate) ++g_cov.manualReverts;
    if (s.rain && s.rainSource == RainSource::Sim) ++g_cov.sim;
    if (s.rain && s.rainSource == RainSource::Api) ++g_cov.api;
    if (s.rain && s.rainSource == RainSource::Local) ++g_cov.local;
    if (s.failSafe) ++g_cov.failSafe;
    if (prev.state == State::Error && s.state != State::Error) {
      if (call == CallUpdate) ++g_cov.errorLimitExit; else ++g_cov.errorCmdExit;
    }
    if (s.state == State::Closing && prev.state != State::Closing) ++g_cov.closings;
    if (s.state == State::Opening && prev.state != State::Opening) ++g_cov.openings;
    if (s.state == State::Closed && prev.state != State::Closed) ++g_cov.closedReached;
    if (s.state == State::Open && prev.state != State::Open) ++g_cov.openReached;
    prev = s;
  }

  void randomInputs() {
    Inputs& in = rig.in;
    if (!stuck && rng.chance(20000)) {  // freeze the limit switches for 10..70 s
      stuck = true;
      stuckUntilMs = rig.now + 10000 + rng.below(60000);
    }
    if (stuck && static_cast<int32_t>(rig.now - stuckUntilMs) >= 0) stuck = false;
    if (!stuck && rng.chance(150)) in.limitClosed = !in.limitClosed;
    if (!stuck && rng.chance(150)) in.limitOpen = !in.limitOpen;
    if (rng.chance(400)) in.humidityValid = rng.below(5) != 0;
    if (rng.chance(200)) in.humidity = static_cast<float>(40 + rng.below(61));
    if (rng.chance(200)) in.light = static_cast<int>(rng.below(4096));
  }

  void randomEvents(int& phase) {
    if (rng.chance(9000)) phase = static_cast<int>(rng.below(4));  // weather regime changes every ~3 min
    if (rng.chance(3000)) {                                        // ~ once a minute
      switch (phase) {
        case 0: break;  // silent backend
        case 1: rig.c.onWeather(rig.now, false, false, rng.below(30)); check(CallWeather); break;
        case 2: rig.c.onWeather(rig.now, true, rng.chance(3), rng.below(30)); check(CallWeather); break;
        default:
          rig.c.onWeather(rig.now, rng.chance(2), rng.chance(4),
                          rng.chance(5) ? rng.below(6000) : rng.below(60));
          check(CallWeather);
          break;
      }
    }
    if (rng.chance(2500)) {
      rig.c.onCommand(rig.now, static_cast<Action>(rng.below(5)));
      check(CallCommand);
    }
    if (rng.chance(4000)) {
      rig.c.onManualButton(rig.now);
      check(CallButton);
    }
  }

  void run(uint32_t totalMs) {
    rig.boot(rng.chance(2), rng.chance(2));
    lastLimitClosed = rig.in.limitClosed;
    lastLimitOpen = rig.in.limitOpen;
    prev = rig.snap();
    enterMs = rig.now;
    int phase = static_cast<int>(rng.below(4));
    for (uint32_t elapsed = 0; elapsed < totalMs;) {
      uint32_t step = 20;
      if (rng.chance(700)) step += rng.below(2500);  // occasional stall of the control loop
      if (step > maxStepMs) maxStepMs = step;
      randomInputs();
      randomEvents(phase);
      rig.tick(step);
      elapsed += step;
      check(CallUpdate);
    }
  }
};

Config fastCfg(uint32_t sim) {
  Config c;
  c.rainConfirmMs = 3000;
  c.dryConfirmMs = 20000;
  c.manualTimeoutMs = 60000;
  c.weatherStaleMs = 5 * 60000UL;
  c.firstWeatherGraceMs = 20000;
  c.simTravelMs = sim;
  return c;
}

}  // namespace

void test_fuzz_interlock_and_invariants() {
  const uint32_t hours = 3;
  const uint32_t starts[] = {1000, 0xFFFFFFFFu - 45 * 60 * 1000UL, 0x7FFFFFFFu - 5000};
  long total = 0;
  for (uint32_t seed = 1; seed <= 12; ++seed) {
    for (int variant = 0; variant < 4; ++variant) {
      const Config cfg = (variant & 1) ? makeCfg(variant >= 2 ? 8000 : 0) : fastCfg(variant >= 2 ? 4000 : 0);
      Fuzzer f(cfg, seed * 7919u + static_cast<uint32_t>(variant), starts[seed % 3]);
      f.run(hours * 3600UL * 1000UL);
      total += f.checks;
    }
  }
  TEST_ASSERT_TRUE(total > 1000000);
  // The run must have exercised the interesting corners, not just idled.
  TEST_ASSERT_TRUE(g_cov.errors > 20);
  TEST_ASSERT_TRUE(g_cov.alarms == g_cov.errors);
  TEST_ASSERT_TRUE(g_cov.errorLimitExit > 0);
  TEST_ASSERT_TRUE(g_cov.errorCmdExit > 0);
  TEST_ASSERT_TRUE(g_cov.beeps > 100);
  TEST_ASSERT_TRUE(g_cov.reversals > 100);
  TEST_ASSERT_TRUE(g_cov.manualReverts > 100);
  TEST_ASSERT_TRUE(g_cov.sim > 1000 && g_cov.api > 1000 && g_cov.local > 1000 && g_cov.failSafe > 1000);
  TEST_ASSERT_TRUE(g_cov.closings > 500 && g_cov.openings > 500);
  TEST_ASSERT_TRUE(g_cov.closedReached > 500 && g_cov.openReached > 500);
  TEST_ASSERT_TRUE(g_cov.changed > 1000);
}

// ---------------------------------------------------------------------------------
// Button
// ---------------------------------------------------------------------------------

namespace {

// Feed a constant raw level every 5 ms for `ms`, collecting the gestures.
struct Feed {
  Button b;
  uint32_t now;
  int shorts = 0;
  int longs = 0;
  explicit Feed(uint32_t start = 5000) : now(start) {}
  void hold(bool raw, uint32_t ms) {
    for (uint32_t t = 0; t < ms; t += 5) {
      now += 5;
      const Button::Gesture g = b.update(now, raw);
      if (g == Button::ShortPress) ++shorts;
      if (g == Button::LongPress) ++longs;
    }
  }
};

}  // namespace

void test_button_ignores_glitches_shorter_than_debounce() {
  Feed f;
  f.hold(false, 100);
  f.hold(true, 20);  // < 30 ms
  f.hold(false, 100);
  TEST_ASSERT_FALSE(f.b.pressed());
  TEST_ASSERT_EQUAL_INT(0, f.shorts);
  TEST_ASSERT_EQUAL_INT(0, f.longs);
}

void test_button_bounce_on_press_and_release_is_one_short_press() {
  Feed f;
  f.hold(false, 100);
  const bool bounce[] = {true, false, true, false, true};  // 5 ms chatter, then steady
  for (size_t i = 0; i < sizeof(bounce); ++i) f.hold(bounce[i], 5);
  f.hold(true, 200);
  TEST_ASSERT_TRUE(f.b.pressed());
  TEST_ASSERT_EQUAL_INT(0, f.shorts);  // nothing until release
  const bool bounce2[] = {false, true, false, true, false};
  for (size_t i = 0; i < sizeof(bounce2); ++i) f.hold(bounce2[i], 5);
  f.hold(false, 100);
  TEST_ASSERT_FALSE(f.b.pressed());
  TEST_ASSERT_EQUAL_INT(1, f.shorts);
  TEST_ASSERT_EQUAL_INT(0, f.longs);
}

void test_button_short_press_fires_on_release() {
  Feed f;
  f.hold(false, 100);
  f.hold(true, 500);
  TEST_ASSERT_EQUAL_INT(0, f.shorts);
  f.hold(false, 20);  // released, but still inside the debounce window
  TEST_ASSERT_EQUAL_INT(0, f.shorts);
  TEST_ASSERT_TRUE(f.b.pressed());
  f.hold(false, 30);
  TEST_ASSERT_EQUAL_INT(1, f.shorts);
  TEST_ASSERT_FALSE(f.b.pressed());
}

void test_button_long_press_fires_once_and_suppresses_short_press() {
  Feed f;
  f.hold(false, 100);
  f.hold(true, 2990);
  TEST_ASSERT_EQUAL_INT(0, f.longs);
  f.hold(true, 15);  // crosses 3000 ms since the first raw edge
  TEST_ASSERT_EQUAL_INT(1, f.longs);
  f.hold(true, 7000);  // still held: no repeat
  TEST_ASSERT_EQUAL_INT(1, f.longs);
  f.hold(false, 200);
  TEST_ASSERT_EQUAL_INT(1, f.longs);
  TEST_ASSERT_EQUAL_INT(0, f.shorts);
  // and a normal short press still works afterwards
  f.hold(true, 300);
  f.hold(false, 100);
  TEST_ASSERT_EQUAL_INT(1, f.shorts);
  TEST_ASSERT_EQUAL_INT(1, f.longs);
}

void test_button_press_just_under_3s_is_a_short_press() {
  Feed f;
  f.hold(false, 100);
  f.hold(true, 2900);
  f.hold(false, 100);
  TEST_ASSERT_EQUAL_INT(1, f.shorts);
  TEST_ASSERT_EQUAL_INT(0, f.longs);
}

void test_button_pressed_reflects_debounced_level_for_limit_switches() {
  Feed f;
  f.hold(false, 100);
  f.hold(true, 25);
  TEST_ASSERT_FALSE(f.b.pressed());
  f.hold(true, 20);
  TEST_ASSERT_TRUE(f.b.pressed());
  f.hold(false, 25);
  TEST_ASSERT_TRUE(f.b.pressed());
  f.hold(false, 20);
  TEST_ASSERT_FALSE(f.b.pressed());
}

void test_button_works_across_millis_rollover() {
  Feed f(0xFFFFFFFFu - 1000u);
  f.hold(false, 100);
  f.hold(true, 3500);  // press straddles the wrap
  TEST_ASSERT_EQUAL_INT(1, f.longs);
  f.hold(false, 100);
  f.hold(true, 300);
  f.hold(false, 100);
  TEST_ASSERT_EQUAL_INT(1, f.shorts);
}

// ---------------------------------------------------------------------------------

int main(int, char**) {
  UNITY_BEGIN();
  RUN_TEST(test_strings_and_parse_action);
#ifndef DEMO_FAST_TIMERS
  RUN_TEST(test_config_h_matches_contract);
#endif
#ifndef DEMO_FAST_TIMERS
  RUN_TEST(test_app_config_maps_every_field_to_the_documented_value);
#endif
  RUN_TEST(test_boot_closed_limit_pressed);
  RUN_TEST(test_boot_open_limit_pressed);
  RUN_TEST(test_boot_both_limits_prefers_closed);
  RUN_TEST(test_boot_no_limit_with_sim_travel_assumes_open);
  RUN_TEST(test_boot_no_limit_no_sim_homes_to_closed);
  RUN_TEST(test_wet_confirmed_after_30s_then_closes_with_beep);
  RUN_TEST(test_rain_expected_15m_counts_as_wet);
  RUN_TEST(test_wet_flicker_resets_the_confirm_timer);
  RUN_TEST(test_sim_rain_closes_immediately);
  RUN_TEST(test_sim_rain_clear_then_dry_reopens_after_15_min);
  RUN_TEST(test_dry_needs_continuous_15_minutes);
  RUN_TEST(test_weather_heartbeats_keep_the_state_machine_running);
  RUN_TEST(test_limit_switch_ends_travel);
  RUN_TEST(test_wrong_limit_switch_is_ignored_while_moving);
  RUN_TEST(test_timer_ends_travel_after_sim_travel_ms);
  RUN_TEST(test_no_limit_within_30s_is_error_and_alarm);
  RUN_TEST(test_opening_timeout_is_error_too);
  RUN_TEST(test_sim_travel_end_beats_the_timeout);
  RUN_TEST(test_reversal_has_dead_time_on_both_relays);
  RUN_TEST(test_auto_does_not_reverse_inside_the_dead_time);
  RUN_TEST(test_manual_open_close_commands_and_countdown);
  RUN_TEST(test_manual_command_restarts_the_countdown);
  RUN_TEST(test_manual_command_at_the_end_position_does_nothing);
  RUN_TEST(test_repeating_the_running_direction_does_not_restart_the_motion);
  RUN_TEST(test_auto_command_returns_to_auto);
  RUN_TEST(test_auto_command_leaves_simulated_rain_alone);
  RUN_TEST(test_manual_blocks_the_auto_rule_but_timers_keep_running);
  RUN_TEST(test_manual_button_toggles_direction);
  RUN_TEST(test_error_recovers_on_a_new_limit_press);
  RUN_TEST(test_error_recovers_with_manual_command_or_button);
  RUN_TEST(test_auto_never_retries_out_of_error);
  RUN_TEST(test_stuck_limit_switch_does_not_bounce_out_of_error);
  RUN_TEST(test_weather_age_is_reported_age_plus_elapsed);
  RUN_TEST(test_weather_becomes_stale_only_after_30_minutes);
  RUN_TEST(test_message_older_than_30_minutes_is_stale_immediately);
  RUN_TEST(test_first_weather_grace_after_boot);
  RUN_TEST(test_local_rule_thresholds_are_inclusive_humidity_exclusive_light);
  RUN_TEST(test_failsafe_local_rain_closes_and_then_holds_without_reopening);
  RUN_TEST(test_unknown_resets_both_timers_and_holds_the_state);
  RUN_TEST(test_unknown_interrupts_wet_confirmation);
  RUN_TEST(test_no_weather_ever_never_moves_an_idle_awning);
  RUN_TEST(test_sim_rain_hides_failsafe_and_wins_over_the_api);
  RUN_TEST(test_weather_age_saturates_instead_of_wrapping);
  RUN_TEST(test_confirmation_survives_weeks_of_unchanged_weather);
  RUN_TEST(test_rollover_gives_identical_behaviour_at_any_start_time);
  RUN_TEST(test_rollover_specific_timers_across_the_wrap);
  RUN_TEST(test_fuzz_interlock_and_invariants);
  RUN_TEST(test_button_ignores_glitches_shorter_than_debounce);
  RUN_TEST(test_button_bounce_on_press_and_release_is_one_short_press);
  RUN_TEST(test_button_short_press_fires_on_release);
  RUN_TEST(test_button_long_press_fires_once_and_suppresses_short_press);
  RUN_TEST(test_button_press_just_under_3s_is_a_short_press);
  RUN_TEST(test_button_pressed_reflects_debounced_level_for_limit_switches);
  RUN_TEST(test_button_works_across_millis_rollover);
  return UNITY_END();
}

#!/usr/bin/env python3
"""End-to-end check of the REAL firmware sources (host build, stubbed Arduino/FreeRTOS/WiFi)
against a REAL Mosquitto using the repo's ACL. Plays the role of the Go backend.
Usage: python3 e2e.py [s1 s2 ... s13]   (no argument = every scenario, ~9 minutes)"""
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import threading
import time

import paho.mqtt.client as mqtt

HERE = os.path.dirname(os.path.abspath(__file__))
FW = os.path.dirname(HERE)                        # firmware/
REPO = os.path.dirname(FW)                        # repo root (deploy/mosquitto/acl lives here)
OUT = os.path.join(FW, ".pio", "hostsim")         # binaries and runtime files (gitignored)
HOST, PORT = "127.0.0.1", 28830
PFX = "pkg/awning01/"
KEYS = ["temp", "humidity", "light", "rain_level", "rain_wet", "state", "mode", "rain", "rain_source",
        "weather_age_s", "fail_safe", "manual_left_s", "rssi", "uptime_s"]

results = []


def check(name, cond, detail=""):
    results.append((name, bool(cond), detail))
    print(("  PASS  " if cond else "  FAIL  ") + name + (f"   [{detail}]" if detail else ""), flush=True)
    return bool(cond)


class Backend:
    """Stands in for the Go backend: subscribes to everything, publishes cmd/weather."""

    def __init__(self):
        self.lock = threading.Lock()
        self.msgs = []
        self.c = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="sim-backend")
        self.c.username_pw_set("backend", "sim-backend-pass")
        self.c.on_message = self._on_message
        self.c.connect(HOST, PORT, keepalive=30)
        self.c.subscribe(PFX + "#", qos=1)
        self.c.loop_start()
        time.sleep(0.3)

    def _on_message(self, c, u, m):
        with self.lock:
            self.msgs.append({"t": time.time(), "topic": m.topic, "raw": m.payload.decode(errors="replace"),
                              "retain": bool(m.retain), "qos": m.qos})

    def clear(self):
        with self.lock:
            self.msgs = []

    def pub(self, sub, payload, qos=1):
        if not isinstance(payload, str):
            payload = json.dumps(payload)
        self.c.publish(PFX + sub, payload, qos=qos, retain=False)

    def weather(self, raining, expected=False, age=0):
        self.pub("weather", {"age_s": age, "is_raining": raining, "rain_expected_15m": expected})

    def cmd(self, action):
        self.pub("cmd", {"action": action})

    def telemetry(self):
        with self.lock:
            out = []
            for m in self.msgs:
                if m["topic"] == PFX + "telemetry":
                    try:
                        d = json.loads(m["raw"])
                    except ValueError:
                        continue
                    d["_t"] = m["t"]
                    d["_raw"] = m["raw"]
                    out.append(d)
            return out

    def status(self):
        with self.lock:
            return [(m["t"], m["raw"], m["retain"], m["qos"]) for m in self.msgs if m["topic"] == PFX + "status"]

    def wait_tel(self, pred, timeout, since=None):
        """First telemetry (received after `since`, default: now) satisfying pred, else None."""
        since = time.time() if since is None else since
        end = time.time() + timeout
        seen = 0
        while time.time() < end:
            tel = [t for t in self.telemetry() if t["_t"] >= since]
            for t in tel:
                if pred(t):
                    return t
            time.sleep(0.05)
        return None

    def wait_status(self, value, timeout, since=None):
        since = time.time() if since is None else since
        end = time.time() + timeout
        while time.time() < end:
            for t, raw, retain, qos in self.status():
                if t >= since and raw == value:
                    return t
            time.sleep(0.05)
        return None

    def last(self):
        tel = self.telemetry()
        return tel[-1] if tel else None

    def close(self):
        self.c.loop_stop()
        self.c.disconnect()


class Firmware:
    def __init__(self, exe, env=None):
        e = dict(os.environ)
        e.update(env or {})
        self.p = subprocess.Popen([os.path.join(OUT, exe)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=subprocess.STDOUT, text=True, bufsize=1, env=e)
        self.lock = threading.Lock()
        self.lines = []       # (wall, text)
        self.relay = []       # (wall, ch1, ch2)
        self.buz = []         # (wall, on)
        self.rainpwr = []     # (firmware seconds, on): RAIN_PWR_PIN edges, only in the fwsim_pwr build
        self.oled = []        # (wall, text)
        self.violation = None
        self.t0 = time.time()
        self.th = threading.Thread(target=self._read, daemon=True)
        self.th.start()

    def _read(self):
        for line in self.p.stdout:
            line = line.rstrip("\n")
            now = time.time()
            with self.lock:
                self.lines.append((now, line))
                if line.startswith("@@RELAY"):
                    _, _, a, b = line.split()
                    self.relay.append((now, int(a), int(b)))
                elif line.startswith("@@BUZ"):
                    self.buz.append((now, int(line.split()[2])))
                elif line.startswith("@@RAINPWR"):
                    _, ms, on = line.split()
                    self.rainpwr.append((int(ms) / 1000.0, int(on)))
                elif line.startswith("@@OLED"):
                    self.oled.append((now, line.split(" ", 2)[2] if line.count(" ") >= 2 else ""))
                elif line.startswith("@@VIOLATION"):
                    self.violation = line

    def send(self, s):
        try:
            self.p.stdin.write(s + "\n")
            self.p.stdin.flush()
        except (BrokenPipeError, ValueError):
            pass

    def press(self, pin):
        self.send(f"in {pin} 0")

    def release(self, pin):
        self.send(f"in {pin} 1")

    def relays(self):
        with self.lock:
            return (self.relay[-1][1], self.relay[-1][2]) if self.relay else (0, 0)

    def wait_relay(self, pred, timeout, since=None):
        since = time.time() if since is None else since
        end = time.time() + timeout
        while time.time() < end:
            with self.lock:
                for w, a, b in self.relay:
                    if w >= since and pred(a, b):
                        return w
            time.sleep(0.005)
        return None

    def wait_line(self, regex, timeout, since=None):
        since = time.time() if since is None else since
        rx = re.compile(regex)
        end = time.time() + timeout
        while time.time() < end:
            with self.lock:
                for w, l in self.lines:
                    if w >= since and rx.search(l):
                        return w, l
            time.sleep(0.02)
        return None

    def ser_lines(self, regex, since=0):
        rx = re.compile(regex)
        with self.lock:
            return [(w, l) for w, l in self.lines if w >= since and rx.search(l)]

    def alive(self):
        return self.p.poll() is None

    def kill9(self):
        os.kill(self.p.pid, signal.SIGKILL)
        self.p.wait()

    def stop(self):
        if self.alive():
            self.send("quit")
            try:
                self.p.wait(timeout=3)
            except subprocess.TimeoutExpired:
                self.p.kill()
        time.sleep(0.1)


def boot(be, exe="fwsim_demo", env=None, settle=True):
    be.clear()
    t = time.time()
    fw = Firmware(exe, env)
    if settle:
        assert be.wait_status("online", 6, since=t), "device never came online"
        be.wait_tel(lambda d: True, 3, since=t)
    return fw


# ------------------------------------------------------------------------------------------
def s1_boot_and_contract(be):
    print("\n[S1] boot, online status, telemetry contract, retained status")
    t0 = time.time()
    be.clear()
    fw = Firmware("fwsim_demo")
    ts = be.wait_status("online", 6, since=t0)
    check("status 'online' published after boot", ts is not None)
    t1 = be.wait_tel(lambda d: True, 3, since=t0)
    check("telemetry published immediately after connect", t1 is not None)
    if t1:
        check("telemetry key set and order match docs/mqtt-topics.md", list(json.loads(t1["_raw"]).keys()) == KEYS,
              ",".join(json.loads(t1["_raw"]).keys()))
        check("boot state OPEN/AUTO, weather_age_s -1, no fail_safe",
              (t1["state"], t1["mode"], t1["weather_age_s"], t1["fail_safe"]) == ("OPEN", "AUTO", -1, False))
        check("dht not read yet at first telemetry -> null", t1["temp"] is None and t1["humidity"] is None)
    # periodic: 3 consecutive periodic messages ~5 s apart
    time.sleep(11.5)
    tel = [t for t in be.telemetry() if t["_t"] >= t0]
    gaps = [round(b["_t"] - a["_t"], 2) for a, b in zip(tel, tel[1:])]
    check("telemetry period ~5 s", len(gaps) >= 2 and all(4.6 <= g <= 5.4 for g in gaps[1:]), f"gaps={gaps}")
    later = tel[-1]
    check("temp/humidity numbers after DHT read", later["temp"] == 29 and later["humidity"] == 71,
          f"{later['temp']},{later['humidity']}")
    check("dry rain plate: rain_level 0, rain_wet false", later["rain_level"] == 0 and later["rain_wet"] is False,
          f"{later['rain_level']},{later['rain_wet']}")
    # retained status seen by a late subscriber
    got = []
    late = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="late-sub")
    late.username_pw_set("backend", "sim-backend-pass")
    late.on_message = lambda c, u, m: got.append((m.topic, m.payload.decode(), m.retain))
    late.connect(HOST, PORT)
    late.subscribe(PFX + "status", qos=1)
    late.loop_start()
    time.sleep(0.6)
    late.loop_stop()
    late.disconnect()
    check("late subscriber gets retained 'online'", (PFX + "status", "online", True) in got, str(got))
    # LWT on abrupt death
    tk = time.time()
    fw.kill9()
    ts = be.wait_status("offline", 5, since=tk)
    check("LWT 'offline' delivered after kill -9", ts is not None, f"{(ts - tk):.2f}s" if ts else "")
    got = []
    late = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="late-sub2")
    late.username_pw_set("backend", "sim-backend-pass")
    late.on_message = lambda c, u, m: got.append((m.topic, m.payload.decode(), m.retain))
    late.connect(HOST, PORT)
    late.subscribe(PFX + "status", qos=1)
    late.loop_start()
    time.sleep(0.6)
    late.loop_stop()
    late.disconnect()
    check("LWT is retained (late subscriber sees 'offline')", (PFX + "status", "offline", True) in got, str(got))
    fw.stop()


def s2_rain_close_reopen(be):
    print("\n[S2] API rain -> confirm -> close (beeps, relay dead time) ; dry -> reopen")
    fw = boot(be)
    t0 = time.time()
    be.weather(True)
    t = be.wait_tel(lambda d: d["rain"], 3, since=t0)
    check("telemetry shows rain=true source=api quickly", t is not None and t["rain_source"] == "api",
          f"{(t['_t'] - t0):.2f}s" if t else "no msg")
    t = be.wait_tel(lambda d: d["state"] == "CLOSING", 6, since=t0)
    dt = (t["_t"] - t0) if t else None
    check("CLOSING ~3 s after rain (demo confirm), published immediately", t is not None and 2.8 <= dt <= 4.3, f"{dt:.2f}s" if dt else "")
    tc = fw.wait_relay(lambda a, b: a == 1 and b == 0, 3, since=t0)
    check("relay CH1 (close) energised after the dead time", tc is not None and (tc - (t["_t"] if t else t0)) < 1.0)
    # triple beep: three ON pulses around the close start
    time.sleep(0.9)
    pulses = [b for (w, b) in fw.buz if w >= t0 + 2.5 and w <= t0 + 6 and b == 1]
    check("buzzer: 3 beeps when auto close starts", len(pulses) == 3, f"pulses={len(pulses)}")
    t = be.wait_tel(lambda d: d["state"] == "CLOSED", 7, since=t0)
    check("CLOSED after simulated travel (4 s)", t is not None)
    check("relay CH1 off when CLOSED", fw.wait_relay(lambda a, b: a == 0 and b == 0, 1, since=t0) is not None)
    # dry heartbeats -> reopen after 20 s (demo)
    td = time.time()
    be.weather(False)
    reopened = None
    for i in range(6):
        t = be.wait_tel(lambda d: d["state"] in ("OPENING", "OPEN"), 5, since=td)
        if t:
            reopened = t
            break
        be.weather(False)  # backend heartbeat
    check("reopens ~20 s after continuous dry (demo timing)", reopened is not None and 18 <= reopened["_t"] - td <= 27,
          f"{(reopened['_t'] - td):.1f}s" if reopened else "")
    tc = fw.wait_relay(lambda a, b: a == 0 and b == 1, 3, since=td)
    check("relay CH2 (open) energised", tc is not None)
    t = be.wait_tel(lambda d: d["state"] == "OPEN", 6, since=td)
    check("OPEN again", t is not None)
    check("no relay violation observed", fw.violation is None, str(fw.violation))
    fw.stop()


def s3_commands(be):
    print("\n[S3] manual commands, garbage payloads, simulate/clear rain, manual button")
    fw = boot(be)
    t0 = time.time()
    be.cmd("close")
    t = be.wait_tel(lambda d: d["state"] == "CLOSING" and d["mode"] == "MANUAL", 3, since=t0)
    check("cmd close -> CLOSING + MANUAL, published immediately", t is not None and t["_t"] - t0 < 1.2,
          f"{(t['_t'] - t0):.2f}s" if t else "")
    if t:
        check("manual_left_s counts from 60 (demo)", 55 <= t["manual_left_s"] <= 60, str(t["manual_left_s"]))
    be.wait_tel(lambda d: d["state"] == "CLOSED", 7, since=t0)
    # garbage
    tg = time.time()
    for payload in ["not json", "{\"action\":\"dance\"}", "{\"action\":1}", "{}", "[]", "{\"action\":\"OPEN\"}", ""]:
        be.pub("cmd", payload)
    time.sleep(1.0)
    fw_alive = fw.alive()
    last = be.last()
    check("garbage/unknown cmd ignored, device alive, state unchanged", fw_alive and last and last["state"] == "CLOSED"
          and not [t for t in be.telemetry() if t["_t"] >= tg and t["state"] in ("OPENING", "OPEN")])
    t1 = time.time()
    be.cmd("auto")
    t = be.wait_tel(lambda d: d["mode"] == "AUTO", 3, since=t1)
    check("cmd auto -> AUTO", t is not None and t["manual_left_s"] == 0)
    t1 = time.time()
    be.cmd("simulate_rain")
    t = be.wait_tel(lambda d: d["rain_source"] == "sim", 3, since=t1)
    check("simulate_rain -> rain_source sim", t is not None)
    be.cmd("open")
    t = be.wait_tel(lambda d: d["state"] == "OPENING", 3, since=t1)
    check("manual open works while sim rain active (manual beats auto)", t is not None)
    be.wait_tel(lambda d: d["state"] == "OPEN", 6, since=t1)
    t2 = time.time()
    be.cmd("clear_rain")
    t = be.wait_tel(lambda d: d["rain_source"] == "none" and not d["rain"], 3, since=t2)
    check("clear_rain -> rain_source none", t is not None)
    # physical manual button: short press = toggle
    t3 = time.time()
    fw.press(25)
    time.sleep(0.25)
    fw.release(25)
    t = be.wait_tel(lambda d: d["state"] == "CLOSING", 3, since=t3)
    check("manual button short press toggles direction (OPEN -> CLOSING)", t is not None)
    be.wait_tel(lambda d: d["state"] == "CLOSED", 7, since=t3)
    # long press = sim rain toggle
    t4 = time.time()
    fw.press(25)
    time.sleep(3.3)
    fw.release(25)
    t = be.wait_tel(lambda d: d["rain_source"] == "sim", 2, since=t4)
    check("manual button held 3 s -> simulated rain ON", t is not None)
    check("long press acknowledged with a beep", any(w >= t4 + 2.9 and b == 1 for (w, b) in fw.buz))
    time.sleep(0.3)  # a real finger needs a moment between two presses
    t5 = time.time()
    fw.press(25)
    time.sleep(3.3)
    fw.release(25)
    t = be.wait_tel(lambda d: d["rain_source"] == "none", 2, since=t5)
    check("held again -> simulated rain OFF", t is not None)
    check("no relay violation observed", fw.violation is None, str(fw.violation))
    fw.stop()


def s4_limit_switch_latency(be):
    print("\n[S4] limit switch stops the relay quickly (real switches, production sim-travel disabled by early press)")
    fw = boot(be)
    t0 = time.time()
    be.cmd("close")
    fw.wait_relay(lambda a, b: a == 1, 2, since=t0)
    time.sleep(1.0)
    tp = time.time()
    fw.press(32)  # closed limit
    tr = fw.wait_relay(lambda a, b: a == 0 and b == 0, 1, since=tp)
    check("CH1 off within 150 ms of pressing the closed-limit switch", tr is not None and tr - tp < 0.15,
          f"{(tr - tp) * 1000:.0f} ms" if tr else "no change")
    t = be.wait_tel(lambda d: d["state"] == "CLOSED", 2, since=t0)
    check("CLOSED published long before SIM_TRAVEL (limit beats timer)", t is not None and t["_t"] - t0 < 3.0)
    fw.release(32)
    fw.stop()


def s5_stale_failsafe_hold(be):
    print("\n[S5] stale weather -> fail_safe, local rule, hold (no reopen), fresh weather ends it")
    fw = boot(be)
    t0 = time.time()
    be.weather(False, False, age=1799)
    t = be.wait_tel(lambda d: d["weather_age_s"] >= 1799, 3, since=t0)
    check("weather_age_s reflects age_s + elapsed", t is not None and 1799 <= t["weather_age_s"] <= 1800, str(t and t["weather_age_s"]))
    t = be.wait_tel(lambda d: d["fail_safe"], 5, since=t0)
    check("fail_safe becomes true once the data is older than 30 min", t is not None and 1.0 <= t["_t"] - t0 <= 3.5,
          f"{(t['_t'] - t0):.2f}s" if t else "")
    ts = time.time()
    fw.send("dht 30 92")
    fw.send("ldr 300")
    t = be.wait_tel(lambda d: d["rain"] and d["rain_source"] == "local", 8, since=ts)
    check("local rule: humid + dark => rain_source local", t is not None)
    t = be.wait_tel(lambda d: d["state"] == "CLOSING", 8, since=ts)
    check("local rain closes the awning after the confirm time", t is not None)
    be.wait_tel(lambda d: d["state"] == "CLOSED", 8, since=ts)
    th = time.time()
    fw.send("dht 30 50")
    fw.send("ldr 3000")
    t = be.wait_tel(lambda d: not d["rain"], 5, since=th)
    check("evidence gone -> rain false", t is not None)
    time.sleep(27)  # > 20 s demo dry-confirm
    last = be.last()
    check("fail-safe holds: still CLOSED after 27 s without rain evidence", last["state"] == "CLOSED" and last["fail_safe"],
          f"{last['state']} fs={last['fail_safe']}")
    tf = time.time()
    be.weather(False)
    t = be.wait_tel(lambda d: not d["fail_safe"], 3, since=tf)
    check("fresh weather message clears fail_safe", t is not None)
    fw.stop()


def s6_sensor_failure(be):
    print("\n[S6] DHT failure handling")
    fw = boot(be)
    fw.send("dht nan")
    t0 = time.time()
    t = be.wait_tel(lambda d: d["temp"] is None and d["humidity"] is None, 14, since=t0)
    check("temp/humidity become null after repeated DHT failures", t is not None, f"{(t['_t'] - t0):.1f}s" if t else "")
    t1 = time.time()
    fw.send("dht 25 60")
    t = be.wait_tel(lambda d: d["temp"] == 25 and d["humidity"] == 60, 12, since=t1)
    check("readings return when the sensor recovers", t is not None)
    t2 = time.time()
    fw.send("ldr 4095")
    t = be.wait_tel(lambda d: d["light"] >= 3900, 8, since=t2 + 1.5)  # smoothed: needs ~1.2 s to settle
    check("light follows the ADC (4095) after smoothing", t is not None, str(t and t["light"]))
    fw.send("ldr 0")
    t = be.wait_tel(lambda d: d["light"] <= 100, 8, since=time.time() + 1.5)
    check("light follows the ADC (0) after smoothing", t is not None, str(t and t["light"]))
    fw.stop()


def s7_wifi_drop_and_reconnect(be):
    print("\n[S7] WiFi drop: control keeps working, LWT via keepalive, automatic recovery")
    fw = boot(be)
    t0 = time.time()
    fw.send("wifi 0")
    r = fw.wait_line(r"\[net\] WiFi lost", 3, since=t0)
    check("firmware notices WiFi loss", r is not None)
    # control still works offline
    tb = time.time()
    fw.press(25)
    time.sleep(0.25)
    fw.release(25)
    tr = fw.wait_relay(lambda a, b: a == 1, 2, since=tb)
    check("manual button still moves the awning with WiFi down", tr is not None)
    o = fw.oled[-1][1] if fw.oled else ""
    time.sleep(1.0)
    o = fw.oled[-1][1] if fw.oled else ""
    check("OLED shows WiFi:-- MQTT:--", "WiFi:-- MQTT:--" in o, o)
    ts = be.wait_status("offline", 40, since=t0)
    check("broker publishes LWT offline via keepalive timeout (~22 s)", ts is not None and 15 <= ts - t0 <= 35,
          f"{(ts - t0):.1f}s" if ts else "")
    tr0 = time.time()
    fw.send("wifi 1")
    ts = be.wait_status("online", 60, since=tr0)
    check("firmware reconnects and publishes online again", ts is not None, f"{(ts - tr0):.1f}s" if ts else "")
    t = be.wait_tel(lambda d: True, 3, since=tr0)
    check("telemetry resumes right after reconnect", t is not None)
    fw.stop()


def s9_watchdog(be):
    print("\n[S9] watchdog: a hung control loop reboots (sim: exit code 3)")
    fw = boot(be)
    fw.send("hangread 9000")
    t0 = time.time()
    end = time.time() + 12
    while fw.alive() and time.time() < end:
        time.sleep(0.1)
    check("process exits via watchdog within 5-7 s of the hang", (not fw.alive()) and fw.p.returncode == 3 and 4.5 <= time.time() - t0 <= 7.5,
          f"rc={fw.p.returncode} after {time.time() - t0:.1f}s")


def s10_boot_states(be):
    print("\n[S10] boot state from limit switches (production sim travel disabled? no: SIM_TRAVEL_MS=8000)")
    for env, expect in (({"SIM_INITIAL_IN": "32=0"}, "CLOSED"), ({"SIM_INITIAL_IN": "33=0"}, "OPEN"), ({"SIM_INITIAL_IN": "32=0,33=0"}, "CLOSED")):
        fw = boot(be, "fwsim_prod", env)
        t = be.last()
        check(f"boot with {env['SIM_INITIAL_IN']} -> {expect}", t is not None and t["state"] == expect, str(t and t["state"]))
        check("relays stayed off during boot (no glitch)", fw.relays() == (0, 0) and len(fw.relay) == 0, str(fw.relay[:3]))
        fw.stop()


def s11_rollover(be):
    print("\n[S11] millis() rollover 12 s after start, whole firmware (control, net timers, telemetry)")
    off = str(2 ** 32 - 12000)
    fw = boot(be, "fwsim_demo", {"SIM_MILLIS_OFFSET": off})
    t0 = time.time()
    time.sleep(6)
    be.weather(True)                                  # confirm 3 s -> CLOSING at ~9 s, CLOSED ~13 s (after the wrap at 12 s)
    t = be.wait_tel(lambda d: d["state"] == "CLOSING", 6, since=t0)
    check("auto close starts before the wrap", t is not None)
    t = be.wait_tel(lambda d: d["state"] == "CLOSED", 8, since=t0)
    check("travel timer completes across the wrap (CLOSED)", t is not None)
    time.sleep(14)
    tel = [x for x in be.telemetry() if x["_t"] >= t0]
    gaps = [b["_t"] - a["_t"] for a, b in zip(tel, tel[1:])]
    logs = fw.ser_lines(r"\[ctl\] light_raw", since=t0)
    lg = [b[0] - a[0] for a, b in zip(logs, logs[1:])]
    check("telemetry cadence unbroken across the wrap", all(g < 5.6 for g in gaps), f"max gap {max(gaps):.2f}s")
    check("1 Hz control log unbroken across the wrap", max(lg) < 1.3, f"max gap {max(lg):.2f}s")
    check("no reconnects/WiFi loss across the wrap", not fw.ser_lines(r"WiFi lost|MQTT connect failed|MQTT connecting") or len(fw.ser_lines(r"MQTT connecting")) == 1)
    t1 = time.time()
    be.cmd("open")
    t = be.wait_tel(lambda d: d["state"] == "OPEN", 8, since=t1)
    check("commands still work after the wrap", t is not None)
    fw.stop()


def log_wet(fw, level, wet, timeout, since):
    """The 1 Hz control log line carries the controller's verdict for the plate (what the web cannot show between frames)."""
    return fw.wait_line(rf"rain_level={level} wet={1 if wet else 0}\b", timeout, since=since)


def s12_rain_plate(be):
    print("\n[S12] rain plate on GPIO35: wet -> source sensor -> close, no weather needed; hysteresis; splash; dry is not proof")
    # The ADC value is what the YL-83 puts on AO: ~4095 dry, falling when wet. Firmware reports level = 4095 - raw.
    fw = boot(be)                                        # note: the backend sends NO weather in this part
    t = be.wait_tel(lambda d: d["rain_level"] is not None, 4, since=time.time() - 3)
    check("dry plate: rain_level 0, rain_wet false, source none", t is not None and (t["rain_level"], t["rain_wet"], t["rain_source"]) == (0, False, "none"),
          str(t and (t["rain_level"], t["rain_wet"], t["rain_source"])))

    ta = time.time()
    fw.send("rain 1500")                                 # level 2595
    t = be.wait_tel(lambda d: d["rain_wet"] is True, 3, since=ta)
    check("wet plate published at once (change-triggered frame): level 2595, rain, source sensor",
          t is not None and t["_t"] - ta < 1.6 and (t["rain_level"], t["rain"], t["rain_source"]) == (2595, True, "sensor"),
          f"{(t['_t'] - ta):.2f}s {t['rain_level']} {t['rain']} {t['rain_source']}" if t else "no frame")
    check("no weather needed: weather_age_s -1 and no fail_safe inside the first-weather grace",
          t is not None and t["weather_age_s"] == -1 and t["fail_safe"] is False)
    t = be.wait_tel(lambda d: d["state"] == "CLOSING", 6, since=ta)
    dt = (t["_t"] - ta) if t else None
    check("CLOSING ~2 s after the plate got wet (sensor confirm, not the 3 s forecast one)", t is not None and 1.9 <= dt <= 2.9,
          f"{dt:.2f}s" if dt else "")
    time.sleep(0.9)
    pulses = [b for (w, b) in fw.buz if ta + 1.5 <= w <= ta + 6 and b == 1]
    check("buzzer: 3 beeps when the sensor starts the close", len(pulses) == 3, f"pulses={len(pulses)}")
    check("CLOSED after the simulated travel", be.wait_tel(lambda d: d["state"] == "CLOSED", 7, since=ta) is not None)

    # Hysteresis on the closed awning: the verdict is visible in the 1 Hz control log.
    th = time.time()
    fw.send("rain 3800")                                 # level 295: between the thresholds -> stays wet
    check("295 (between 200 and 400) keeps a wet plate wet", log_wet(fw, 295, True, 3, th) is not None)
    th = time.time()
    fw.send("rain 3896")                                 # level 199 <= 200 -> dry
    check("199 (<= RAIN_DRY_BELOW) turns it dry", log_wet(fw, 199, False, 3, th) is not None)
    th = time.time()
    fw.send("rain 3800")
    check("295 keeps a dry plate dry", log_wet(fw, 295, False, 3, th) is not None)
    th = time.time()
    fw.send("rain 3695")                                 # level 400 >= RAIN_WET_ABOVE -> wet (inclusive)
    check("400 (>= RAIN_WET_ABOVE) turns it wet", log_wet(fw, 400, True, 3, th) is not None)

    # A dry plate is no proof of dry weather: with no forecast the awning stays closed however long it is dry.
    fw.send("rain 4095")
    td = time.time()
    t = be.wait_tel(lambda d: not d["rain"], 3, since=td)
    check("plate dry again: rain false, source none", t is not None and t["rain_source"] == "none")
    time.sleep(23)                                       # > 20 s demo dry-confirm
    last = be.last()
    check("no forecast + dry plate: the awning is NOT reopened (dry plate is no evidence)", last["state"] == "CLOSED",
          last["state"])
    # A fresh forecast saying dry is evidence: reopen after the dry confirm.
    tf = time.time()
    be.weather(False)
    t = be.wait_tel(lambda d: d["state"] in ("OPENING", "OPEN"), 26, since=tf)
    check("fresh dry forecast + dry plate: reopens after the dry confirmation (~20 s)", t is not None and 18 <= t["_t"] - tf <= 25,
          f"{(t['_t'] - tf):.1f}s" if t else "")
    check("OPEN again", be.wait_tel(lambda d: d["state"] == "OPEN", 7, since=tf) is not None)

    # A splash shorter than the confirm time must not move the awning.
    ts = time.time()
    fw.send("rain 1500")
    time.sleep(1.0)
    fw.send("rain 4095")
    time.sleep(4.5)
    tel = [x for x in be.telemetry() if x["_t"] >= ts]
    check("splash of ~1 s: seen as wet but the awning never moved",
          any(x["rain_wet"] for x in tel) and not any(x["state"] != "OPEN" for x in tel),
          str([(x["rain_wet"], x["state"]) for x in tel]))

    # An out-of-range reading is clamped instead of producing a frame the backend would reject (level would be negative).
    fw.send("rain 3000")                                 # level 1095, wet for ~1 s: still under the 2 s confirm
    time.sleep(1.2)
    tc = time.time()
    fw.send("rain 99999")
    check("a reading above 4095 is clamped: rain_level 0, not negative", log_wet(fw, 0, False, 3, tc) is not None)
    check("no relay violation observed", fw.violation is None, str(fw.violation))
    fw.stop()


def s13_rain_power_gate(be):
    print("\n[S13] RAIN_PWR_PIN=18: the module has power only while it is being measured")
    fw = boot(be, "fwsim_pwr")
    be.weather(False)
    t0 = time.time()
    time.sleep(7)
    on = [w for (w, v) in fw.rainpwr if v == 1]
    off = [w for (w, v) in fw.rainpwr if v == 0]
    check("the pin is pulsed regularly (>= 10 power-ups in ~7 s)", len(on) >= 10, f"{len(on)} power-ups")
    pairs = []
    for w, v in fw.rainpwr:
        if v == 1:
            pairs.append([w, None])
        elif pairs and pairs[-1][1] is None:
            pairs[-1][1] = w
    done = [(a, b) for a, b in pairs if b is not None]
    lengths = [b - a for a, b in done]
    gaps = [b[0] - a[0] for a, b in zip(pairs, pairs[1:])]
    check("each power-up lasts at least RAIN_SETTLE_MS (20 ms) and not much longer", lengths and all(0.019 <= d <= 0.15 for d in lengths),
          f"min={min(lengths) * 1000:.0f} ms max={max(lengths) * 1000:.0f} ms" if lengths else "no pulses")
    check("one reading every ~500 ms", gaps and all(0.45 <= g <= 0.8 for g in gaps), f"min={min(gaps):.2f}s max={max(gaps):.2f}s" if gaps else "")
    span = pairs[-1][0] - pairs[0][0] if len(pairs) > 1 else 0
    duty = sum(lengths) / span if span else 1
    check("the module is unpowered >= 85% of the time", duty < 0.15, f"duty {duty * 100:.1f}%")
    tel = [x for x in be.telemetry() if x["_t"] >= t0]
    check("every reading was taken while powered: a dry plate never looked wet (an unpowered AO reads ~0 = soaked)",
          tel and all(x["rain_wet"] is not True and x["rain_level"] in (None, 0) for x in tel),
          str([(x["rain_wet"], x["rain_level"]) for x in tel if x["rain_wet"] is not False]))

    ta = time.time()
    fw.send("rain 1500")
    t = be.wait_tel(lambda d: d["rain_wet"] is True, 3, since=ta)
    check("wet plate through the gated supply: level 2595, source sensor", t is not None and (t["rain_level"], t["rain_source"]) == (2595, "sensor"),
          str(t and (t["rain_level"], t["rain_source"])))
    check("AUTO closes the awning", be.wait_tel(lambda d: d["state"] == "CLOSING", 6, since=ta) is not None)
    check("no relay violation observed", fw.violation is None, str(fw.violation))
    fw.stop()


class Mosq:
    """Throw-away Mosquitto on 127.0.0.1:28830 using the repo's real ACL (deploy/mosquitto/acl)."""

    def __init__(self):
        self.p = None
        d = os.path.join(OUT, "mosq")
        os.makedirs(d, exist_ok=True)
        passwd = os.path.join(d, "passwd")
        if os.path.exists(passwd):
            os.remove(passwd)
        for i, (user, pw) in enumerate((("esp32-awning01", "sim-esp-pass"), ("backend", "sim-backend-pass"))):
            subprocess.run(["mosquitto_passwd", "-b"] + (["-c"] if i == 0 else []) + [passwd, user, pw],
                           check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        shutil.copy(os.path.join(REPO, "deploy", "mosquitto", "acl"), os.path.join(d, "acl"))
        self.conf = os.path.join(d, "mosquitto.conf")
        with open(self.conf, "w") as f:
            f.write(f"listener {PORT} 127.0.0.1\nallow_anonymous false\n"
                    f"password_file {passwd}\nacl_file {os.path.join(d, 'acl')}\n"
                    f"persistence false\nlog_dest stderr\nlog_type error\nlog_type warning\nuser root\n")

    def start(self):
        self.p = subprocess.Popen(["mosquitto", "-c", self.conf], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        time.sleep(0.8)

    def stop(self):
        if self.p and self.p.poll() is None:
            self.p.terminate()
            self.p.wait(timeout=5)


def main():
    only = sys.argv[1:]
    mosq = Mosq()
    mosq.start()
    be = Backend()
    scenarios = [("s1", s1_boot_and_contract), ("s2", s2_rain_close_reopen), ("s3", s3_commands),
                 ("s4", s4_limit_switch_latency), ("s5", s5_stale_failsafe_hold), ("s6", s6_sensor_failure),
                 ("s7", s7_wifi_drop_and_reconnect), ("s8", None), ("s9", s9_watchdog), ("s10", s10_boot_states),
                 ("s11", s11_rollover), ("s12", s12_rain_plate), ("s13", s13_rain_power_gate)]
    try:
        for name, fn in scenarios:
            if only and name not in only:
                continue
            if name == "s8":
                be.close()
                mosq_stop_start_scenario(mosq)  # S8 restarts the broker, so the backend client is re-created
                be = Backend()
            else:
                fn(be)
    finally:
        try:
            be.close()
        except Exception:
            pass
        mosq.stop()
    bad = [r for r in results if not r[1]]
    print(f"\n==== {len(results) - len(bad)}/{len(results)} checks passed ====")
    for n, ok, d in bad:
        print("FAILED:", n, d)
    sys.exit(1 if bad else 0)


def mosq_stop_start_scenario(mosq):
    """S8 with its own throw-away backend, because the broker goes down in the middle."""
    print("\n[S8] control loop never blocks on the network (broker down, every connect blocks 3 s)")
    stall = os.path.join(OUT, "stall.flag")
    open(stall, "w").close()
    mosq.stop()
    time.sleep(0.5)
    fw = Firmware("fwsim_demo", {"SIM_STALL_FILE": stall})
    time.sleep(2.0)
    ser0 = time.time()
    tb = time.time()
    fw.press(25)
    time.sleep(0.25)
    fw.release(25)
    tr = fw.wait_relay(lambda a, b: a == 1, 2, since=tb)
    check("relay CH1 on after button+dead time while MQTT connect is stalled", tr is not None and tr - tb < 0.8,
          f"{(tr - tb) * 1000:.0f} ms" if tr else "")
    time.sleep(1.2)
    tp = time.time()
    fw.press(32)
    tr = fw.wait_relay(lambda a, b: a == 0 and b == 0, 1, since=tp)
    check("CH1 off within 150 ms of the limit switch during a network stall", tr is not None and tr - tp < 0.15,
          f"{(tr - tp) * 1000:.0f} ms" if tr else "")
    fw.release(32)
    time.sleep(8)
    logs = fw.ser_lines(r"\[ctl\] light_raw", since=ser0)
    times = [w for w, _ in logs]
    gaps = [b - a for a, b in zip(times, times[1:])]
    check("1 Hz control log never gaps (max < 1.3 s) over ~12 s of stalled connects", len(gaps) >= 9 and max(gaps) < 1.3,
          f"n={len(logs)} max_gap={max(gaps):.2f}s" if gaps else "no logs")
    fails = fw.ser_lines(r"MQTT connect failed", since=ser0)
    check("the net task really was failing/stalling meanwhile", len(fails) >= 1, f"{len(fails)} failed attempts")
    os.remove(stall)   # network works again ...
    mosq.start()       # ... and so does the broker
    be2 = Backend()
    tr0 = time.time()
    ok = be2.wait_status("online", 50, since=tr0)
    check("recovers by itself once the broker is back (backoff <= 30 s)", ok is not None, f"{(ok - tr0):.1f}s" if ok else "")
    check("no relay/dead-time violation observed", fw.violation is None, str(fw.violation))
    be2.close()
    fw.stop()


if __name__ == "__main__":
    main()

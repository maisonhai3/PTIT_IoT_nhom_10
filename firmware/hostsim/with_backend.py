#!/usr/bin/env python3
"""Firmware thật (build cho Linux) + backend Go thật + Mosquitto thật (ACL của repo) + Open-Meteo giả.

Khác e2e.py (dùng script Python đóng vai backend), ở đây đầu bên kia là chính `backend/cmd/server`, nên kiểm tra được
hai phía hiểu nhau thật: telemetry của firmware qua được bước kiểm tra của backend, lệnh và thời tiết của backend
được firmware hiểu, và luật tự động chạy trên dữ liệu thời tiết đi qua cả chuỗi.

Cần: g++ (để ./build.sh), go, mosquitto, mosquitto_passwd, python3 (không cần thư viện ngoài).
Chạy: ./build.sh && python3 with_backend.py        (khoảng 4 phút vì backend hỏi thời tiết mỗi 1 phút)
"""
import http.server
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
FW = os.path.dirname(HERE)
REPO = os.path.dirname(FW)
FW_BIN = os.path.join(FW, ".pio", "hostsim", "fwsim_demo")  # bản thời gian rút ngắn, do build.sh tạo
MQTT_PORT = 28830  # khớp stubs/secrets.h
HTTP_PORT, WX_PORT = 28831, 28832
BASE = f"http://127.0.0.1:{HTTP_PORT}"

results = []
procs = []


def check(name, cond, detail=""):
    results.append(bool(cond))
    print(("  PASS  " if cond else "  FAIL  ") + name + (f"  [{detail}]" if detail else ""), flush=True)
    return bool(cond)


def need(cmd):
    if shutil.which(cmd) is None:
        sys.exit(f"Thiếu lệnh '{cmd}' (xem README.md, mục 'Cần có').")


# ---- Open-Meteo giả -------------------------------------------------------------------------
class Weather(http.server.BaseHTTPRequestHandler):
    raining = False

    def log_message(self, *a):
        pass

    def do_GET(self):
        now = int(time.time())
        r = Weather.raining
        q15, hour = now - now % 900 + 900, now - now % 3600
        body = {
            "current": {"time": now, "interval": 900, "temperature_2m": 27.5 if r else 31.0, "relative_humidity_2m": 90 if r else 65,
                        "precipitation": 1.2 if r else 0.0, "rain": 1.2 if r else 0.0, "showers": 0.0, "weather_code": 63 if r else 1},
            "minutely_15": {"time": [q15 + 900 * i for i in range(-1, 6)], "precipitation": [0.0] * 7},
            "hourly": {"time": [hour + 3600 * i for i in range(-1, 4)], "precipitation_probability": [10, 20, 30, 40, 50], "precipitation": [0.0] * 5},
        }
        data = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


# ---- helpers ---------------------------------------------------------------------------------
def get(path):
    with urllib.request.urlopen(BASE + path, timeout=5) as r:
        return json.load(r)


def post(action):
    req = urllib.request.Request(BASE + "/api/command", data=json.dumps({"action": action}).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=5) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code


def tel():
    return get("/api/state")["telemetry"]


def wait(pred, timeout=20):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            if pred():
                return time.time() - t0
        except Exception:
            pass
        time.sleep(0.2)
    return None


def start(cmd, **kw):
    p = subprocess.Popen(cmd, **kw)
    procs.append(p)
    return p


def main():
    for c in ("go", "mosquitto", "mosquitto_passwd"):
        need(c)
    if not os.path.exists(FW_BIN):
        sys.exit(f"Chưa có {FW_BIN}: chạy ./build.sh trước.")
    tmp = tempfile.mkdtemp(prefix="awning-withbackend-")
    try:
        run(tmp)
    finally:
        for p in reversed(procs):
            if p.poll() is None:
                p.terminate()
        for p in procs:
            try:
                p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                p.kill()
        shutil.rmtree(tmp, ignore_errors=True)
    print(f"\n{sum(results)}/{len(results)} kiểm tra đạt")
    sys.exit(0 if results and all(results) else 1)


def run(tmp):
    # 1. Mosquitto với đúng ACL của repo
    passwd = os.path.join(tmp, "passwd")
    for i, (user, pw) in enumerate((("esp32-awning01", "sim-esp-pass"), ("backend", "sim-backend-pass"))):
        subprocess.run(["mosquitto_passwd", "-b"] + (["-c"] if i == 0 else []) + [passwd, user, pw],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    shutil.copy(os.path.join(REPO, "deploy", "mosquitto", "acl"), os.path.join(tmp, "acl"))
    conf = os.path.join(tmp, "mosquitto.conf")
    with open(conf, "w") as f:
        f.write(f"listener {MQTT_PORT} 127.0.0.1\nallow_anonymous false\npassword_file {passwd}\nacl_file {os.path.join(tmp, 'acl')}\n"
                "persistence false\nlog_dest stderr\nlog_type error\nuser root\n")
    start(["mosquitto", "-c", conf], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    # 2. Open-Meteo giả
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", WX_PORT), Weather)
    threading.Thread(target=srv.serve_forever, daemon=True).start()

    # 3. Backend Go thật
    server_bin = os.path.join(tmp, "server")
    subprocess.run(["go", "build", "-o", server_bin, "./cmd/server"], cwd=os.path.join(REPO, "backend"), check=True)
    env = dict(os.environ, HTTP_ADDR=f"127.0.0.1:{HTTP_PORT}", MQTT_URL=f"tcp://127.0.0.1:{MQTT_PORT}", MQTT_USER="backend",
               MQTT_PASSWORD="sim-backend-pass", DB_PATH=os.path.join(tmp, "awning.db"), WEATHER_BASE_URL=f"http://127.0.0.1:{WX_PORT}",
               WEATHER_POLL_INTERVAL="1m", WEATHER_PUBLISH_INTERVAL="5s", DEVICE_TIMEOUT="5s", HISTORY_INTERVAL="2s", STATIC_DIR="")
    log = open(os.path.join(tmp, "backend.log"), "w")
    start([server_bin], env=env, stdout=log, stderr=log)
    time.sleep(1.5)

    # 4. Firmware thật (phần cứng giả, lệnh qua stdin)
    fw = start([FW_BIN], stdin=subprocess.PIPE, stdout=open(os.path.join(tmp, "firmware.log"), "w"), stderr=subprocess.STDOUT, text=True)

    def fwcmd(line):
        fw.stdin.write(line + "\n")
        fw.stdin.flush()

    print("0. khởi động")
    ok = wait(lambda: get("/api/state")["online"], 20) is not None
    check("backend thấy firmware online qua Mosquitto có ACL", ok)
    if not ok:
        print(f"    (log trong {tmp} bị xoá khi thoát; chạy lại và xem backend.log/firmware.log nếu cần)")
        return
    check("firmware nhận thời tiết từ backend (weather_age_s >= 0)", wait(lambda: tel()["weather_age_s"] >= 0, 15) is not None)

    print("1. lệnh từ web -> firmware -> web")
    check("close được nhận", post("close") == 202)
    check("đi tới CLOSED, chế độ MANUAL", wait(lambda: tel()["state"] == "CLOSED" and tel()["mode"] == "MANUAL", 15) is not None)
    check("manual_left_s đếm ngược từ ~60", 30 < tel()["manual_left_s"] <= 60, str(tel()["manual_left_s"]))
    post("open")
    check("open rồi tới OPEN", wait(lambda: tel()["state"] == "OPEN", 15) is not None)
    post("auto")
    check("auto về chế độ AUTO", wait(lambda: tel()["mode"] == "AUTO", 6) is not None)

    print("2. công tắc hành trình dừng hành trình sớm (chân 32, bấm = 0)")
    post("close")
    wait(lambda: tel()["state"] == "CLOSING", 6)
    fwcmd("in 32 0")
    d = wait(lambda: tel()["state"] == "CLOSED", 5)
    check("bấm công tắc 'đã thu' kết thúc CLOSING ngay (< 3 s, hành trình giả lập là 4 s)", d is not None and d < 3, f"{d and round(d, 1)}s")
    fwcmd("in 32 1")
    post("open")
    wait(lambda: tel()["state"] == "OPEN", 15)
    post("auto")
    wait(lambda: tel()["mode"] == "AUTO", 6)

    print("3. nút tay (chân 25)")
    fwcmd("in 25 0"); time.sleep(0.15); fwcmd("in 25 1")
    check("bấm ngắn: đảo sang thu, chế độ MANUAL", wait(lambda: tel()["state"] in ("CLOSING", "CLOSED") and tel()["mode"] == "MANUAL", 6) is not None)
    wait(lambda: tel()["state"] == "CLOSED", 15)
    fwcmd("in 25 0"); time.sleep(0.15); fwcmd("in 25 1")
    check("bấm ngắn lần nữa: đảo sang mở", wait(lambda: tel()["state"] in ("OPENING", "OPEN"), 6) is not None)
    wait(lambda: tel()["state"] == "OPEN", 15)
    post("auto")
    wait(lambda: tel()["mode"] == "AUTO", 6)

    print("4. giữ nút 3 giây = giả lập mưa")
    fwcmd("in 25 0"); time.sleep(3.3); fwcmd("in 25 1")
    check("rain_source = sim", wait(lambda: tel()["rain_source"] == "sim", 6) is not None)
    check("AUTO thu giàn ngay", wait(lambda: tel()["state"] in ("CLOSING", "CLOSED") and tel()["mode"] == "AUTO", 6) is not None)
    wait(lambda: tel()["state"] == "CLOSED", 15)
    check("clear_rain từ web được nhận và có hiệu lực", post("clear_rain") == 202 and wait(lambda: tel()["rain_source"] != "sim", 6) is not None)
    check("simulate_rain từ web có hiệu lực", post("simulate_rain") == 202 and wait(lambda: tel()["rain_source"] == "sim", 6) is not None)
    post("clear_rain")
    wait(lambda: tel()["rain_source"] != "sim", 6)

    print("5. cảm biến tới web đúng dạng")
    fwcmd("dht 33.5 88")
    check("DHT11 33,5 °C / 88 %", wait(lambda: tel()["temp"] == 33.5 and tel()["humidity"] == 88, 12) is not None)
    fwcmd("ldr 3500")
    check("ánh sáng theo quang trở (đã làm mượt)", wait(lambda: tel()["light"] > 3000, 8) is not None)
    fwcmd("dht nan")
    check("3 lần đọc DHT lỗi: temp/humidity là JSON null", wait(lambda: tel()["temp"] is None and tel()["humidity"] is None, 25) is not None)
    fwcmd("dht 29 71")
    wait(lambda: tel()["temp"] == 29, 12)

    print("6. mưa từ Open-Meteo làm firmware thật tự thu, hết mưa thì tự mở (chờ chu kỳ hỏi thời tiết 1 phút)")
    post("open")
    wait(lambda: tel()["state"] == "OPEN", 15)
    post("auto")
    wait(lambda: tel()["mode"] == "AUTO", 6)
    Weather.raining = True
    d = wait(lambda: tel()["state"] == "CLOSED" and tel()["rain_source"] == "api" and tel()["mode"] == "AUTO", 100)
    check("Open-Meteo báo mưa: AUTO tự thu (nguồn api)", d is not None, f"{d and round(d)}s")
    Weather.raining = False
    d = wait(lambda: tel()["state"] == "OPEN" and tel()["rain_source"] == "none", 150)
    check("hết mưa: tự mở lại sau thời gian xác nhận khô (bản demo)", d is not None, f"{d and round(d)}s")

    print("7. mất WiFi rồi hồi phục")
    fwcmd("wifi 0")
    d = wait(lambda: get("/api/state")["online"] is False, 30)
    check("backend đánh dấu thiết bị offline khi đường truyền chết", d is not None, f"{d and round(d, 1)}s")
    check("lệnh gửi thiết bị offline bị từ chối 409", post("close") == 409)
    fwcmd("wifi 1")
    d = wait(lambda: get("/api/state")["online"] is True, 60)
    check("thiết bị tự kết nối lại", d is not None, f"{d and round(d, 1)}s")


if __name__ == "__main__":
    main()

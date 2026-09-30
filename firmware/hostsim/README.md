# hostsim: chạy firmware thật trên máy tính, nói chuyện với Mosquitto thật

Biên dịch **chính các file `src/` và `lib/` của firmware** cho Linux, thay Arduino/FreeRTOS/WiFi/DHT/OLED bằng stub trong `stubs/` (thư viện `PubSubClient` và `ArduinoJson` là bản thật). Chương trình kết nối vào một Mosquitto thật (dùng đúng ACL của `deploy/mosquitto/acl`) và `e2e.py` đóng vai backend Go: gửi `cmd`/`weather`, đọc `telemetry`/`status`, đo độ trễ relay.

Đây là kiểm thử tích hợp cho **logic** (điều khiển, MQTT, JSON, timer, tràn `millis()`), **không** thay thế việc thử trên ESP32 thật (WiFi thật, cực tính quang trở, mức kích relay...).

## Cần có
- `g++` (C++11), `python3` với `pip install paho-mqtt`, và lệnh `mosquitto`, `mosquitto_passwd` (gói `mosquitto`).
- Đã chạy `pio run` và `pio test -e native` một lần để `.pio/libdeps` có `PubSubClient` và `ArduinoJson`.

## Chạy
```bash
cd firmware/hostsim
./build.sh                 # tạo ../.pio/hostsim/fwsim_demo (thời gian rút ngắn) và fwsim_prod
python3 e2e.py             # toàn bộ kịch bản s1..s11, khoảng 7 phút
python3 e2e.py s2 s8       # chỉ vài kịch bản
```
`e2e.py` tự dựng một Mosquitto tạm ở `127.0.0.1:28830` (user `esp32-awning01` và `backend`, mật khẩu giả trong `stubs/secrets.h`), không đụng tới broker thật của bạn. File chạy và log nằm trong `../.pio/hostsim/` (đã `.gitignore`).

| Kịch bản | Kiểm tra |
|---|---|
| s1 | `online` retained, telemetry đúng bộ key và thứ tự, chu kỳ 5 s, LWT `offline` retained khi tiến trình bị kill -9 |
| s2 | mưa từ API, xác nhận, thu giàn, bíp 3 tiếng, thời gian chết relay, mở lại sau khi khô |
| s3 | lệnh tay, payload rác bị bỏ qua, giả lập mưa, nút tay bấm ngắn và giữ 3 s |
| s4 | công tắc hành trình tắt relay trong dưới 150 ms |
| s5 | thời tiết cũ, fail-safe, luật cảm biến tại chỗ, giữ nguyên (không tự mở) |
| s6 | DHT hỏng và phục hồi, quang trở |
| s7 | mất WiFi: vẫn điều khiển được, LWT theo keepalive, tự kết nối lại |
| s8 | broker chết và mọi lần connect chặn 3 s: vòng điều khiển không khựng, tự hồi phục khi broker về |
| s9 | vòng điều khiển treo thì watchdog reset (mô phỏng: thoát mã 3) |
| s10 | trạng thái khi khởi động theo công tắc hành trình, relay không giật |
| s11 | `millis()` tràn 12 s sau khi khởi động, toàn bộ firmware chạy tiếp bình thường |

Chương trình còn tự kiểm tra relay: in `@@VIOLATION` và thoát nếu hai relay cùng bật hoặc kênh này bật chưa đủ 190 ms sau khi kênh kia tắt.

## Firmware thật + backend Go thật: `with_backend.py`
`e2e.py` dùng một script Python đóng vai backend, nên nó chỉ chứng minh firmware khớp với **bản mô tả** contract trong đầu người viết script.
`with_backend.py` thay đầu bên kia bằng chính `backend/cmd/server` (build từ mã nguồn), nên kiểm tra hai phía hiểu nhau thật: telemetry của firmware
qua được bước kiểm tra của backend, lệnh và thời tiết của backend được firmware hiểu, và luật tự động chạy trên dữ liệu thời tiết đi qua cả chuỗi.

```bash
./build.sh && python3 with_backend.py      # khoảng 4 phút, không cần thư viện Python ngoài
```
Cần thêm `go`. Script dựng Mosquitto tạm (ACL của repo, cổng 28830), một Open-Meteo giả (cổng 28832, công tắc `Weather.raining`) và backend ở cổng 28831,
rồi kiểm tra lần lượt: thiết bị online qua broker có ACL; lệnh web (`close`/`open`/`auto`) và đếm ngược chế độ thủ công; công tắc hành trình (GPIO32) dừng hành trình sớm;
nút tay GPIO25 bấm ngắn và giữ 3 giây; `simulate_rain`/`clear_rain`; DHT11 và quang trở tới web đúng dạng (đọc lỗi thành `null`);
Open-Meteo báo mưa thì AUTO tự thu, hết mưa thì tự mở lại; mất WiFi thì backend báo offline, lệnh bị từ chối `409`, rồi tự nối lại.
Mọi tiến trình con được dọn khi kết thúc hoặc bị ngắt.

Hai script dùng chung cổng 28830 nên **không chạy đồng thời**.

## Điều khiển phần cứng giả qua stdin
Chạy tay: `SIM_INITIAL_IN=32=0 ../.pio/hostsim/fwsim_demo` rồi gõ lệnh (mỗi dòng một lệnh):

| Lệnh | Ý nghĩa |
|---|---|
| `in <pin> <0\|1>` | mức chân đầu vào; nút/công tắc là INPUT_PULLUP nên `0` = đang bấm (25 nút tay, 32 đã thu, 33 đã mở) |
| `ldr <0..4095>` | giá trị ADC của quang trở |
| `dht <nhiệt> <ẩm>` hoặc `dht nan` | số đọc DHT11 hoặc lỗi đọc |
| `wifi <0\|1>` | tắt/bật WiFi giả (tắt thì mọi socket chết) |
| `hangread <ms>` | lần đọc GPIO kế tiếp bị treo bấy nhiêu ms (thử watchdog) |
| `quit` | thoát |

Biến môi trường: `SIM_INITIAL_IN="32=0,33=1"` (mức chân lúc khởi động), `SIM_MILLIS_OFFSET=<số>` (`millis()` bắt đầu từ giá trị này, ví dụ `4294955296` để tràn sau 12 s), `SIM_STALL_FILE=<đường dẫn>` (khi file tồn tại, mọi `connect` MQTT chặn 3 s rồi thất bại).

Đầu ra: dòng bắt đầu bằng `SER` là serial của firmware; `@@RELAY <ms> <ch1> <ch2>`, `@@BUZ`, `@@OLED` là trạng thái phần cứng giả.

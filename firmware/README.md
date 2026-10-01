# Firmware ESP32 – giàn phơi thông minh

ESP32 DEVKIT V1 điều khiển giàn phơi giả lập (2 relay + LED, 2 nút làm công tắc hành trình). Tự thu giàn khi có mưa theo bản tin thời tiết do backend gửi qua MQTT; nếu mất tin thì dùng cảm biến tại chỗ (DHT11 + quang trở). Contract: [`docs/mqtt-topics.md`](../docs/mqtt-topics.md), nối dây: [`docs/wiring.md`](../docs/wiring.md).

## Chuẩn bị
1. Cài [PlatformIO](https://platformio.org/install) (extension VS Code hoặc `pip install platformio`).
2. `cp include/secrets.h.example include/secrets.h`, điền WiFi và **IP LAN của máy chạy Mosquitto**. File này đã bị `.gitignore`, đừng commit. ESP32 chỉ bắt WiFi 2.4 GHz.
3. Nối dây theo `docs/wiring.md`.

## Cấu trúc
```
include/config.h      mọi chân, ngưỡng, thời gian (sửa ở đây)
lib/awning_core/      máy trạng thái + nút bấm (C++11 thuần, test trên máy tính)
lib/awning_msg/       JSON: lệnh, thời tiết, telemetry (ArduinoJson v7)
src/                  main.cpp (điều khiển), net.cpp (WiFi/MQTT), hardware.cpp, shared.cpp
test/                 test Unity chạy bằng `pio test -e native`
hostsim/              chạy chính firmware này trên máy tính với Mosquitto thật (xem hostsim/README.md)
```

## Build, nạp, theo dõi
```bash
cd firmware
pio run                              # biên dịch (env mặc định esp32dev)
pio run -t upload                    # nạp qua USB
pio device monitor                   # serial 115200, Ctrl+C để thoát
pio run -e esp32dev-demo -t upload   # bản demo (xem dưới)
```
| | `esp32dev` (chạy thật) | `esp32dev-demo` |
|---|---|---|
| Xác nhận mưa | 30 s | 3 s |
| Xác nhận khô | 15 phút | 20 s |
| MANUAL tự về AUTO | 10 phút | 60 s |
| Hành trình giả lập (`SIM_TRAVEL_MS`) | 8 s | 4 s |

Serial in mỗi giây một dòng trạng thái (có `light_raw`) và ngay khi trạng thái đổi. Lúc khởi động in lý do reset (`task_wdt` nghĩa là vòng điều khiển từng treo quá 5 s).

## Test trên máy tính (không cần board)
```bash
pio test -e native
```
Cần trình biên dịch C++ của máy (g++/clang; Windows nên dùng WSL). Chỉ biên dịch `lib/awning_core` (máy trạng thái, nút bấm) và `lib/awning_msg` (JSON), là C++11 thuần không dính Arduino. Test bao gồm: khởi động, xác nhận mưa/khô, giả lập mưa, kết thúc hành trình, ERROR và phục hồi, chế độ tay, thời tiết cũ/fail-safe, `millis()` tràn, interlock relay (fuzz nhiều giờ mô phỏng) và JSON (đúng bộ key, từ chối payload sai). `test_config_h_matches_contract` báo ngay nếu `config.h` lệch khỏi contract.

## Thử nhanh không cần backend
```bash
mosquitto_sub -h <ip> -u backend -P <pass> -t 'pkg/awning01/#' -v
mosquitto_pub -h <ip> -u backend -P <pass> -t pkg/awning01/cmd     -m '{"action":"close"}'
mosquitto_pub -h <ip> -u backend -P <pass> -t pkg/awning01/weather -m '{"age_s":0,"is_raining":true,"rain_expected_15m":false}'
```
Giữ nút GPIO25 3 giây để bật/tắt giả lập mưa ngay trên board (bíp một tiếng).

## Hành vi
- **Trạng thái**: `OPEN`, `CLOSING` (relay CH1), `CLOSED`, `OPENING` (relay CH2), `ERROR` (không chạm công tắc hành trình trong 30 s: tắt relay, buzzer kêu dài tới khi thoát).
- **Khởi động**: công tắc "đã thu" đang bấm là `CLOSED`, "đã mở" là `OPEN`. Không có công tắc nào: `SIM_TRAVEL_MS > 0` thì coi là `OPEN`, ngược lại chạy thu về (homing).
- **AUTO**: mưa (`is_raining` hoặc `rain_expected_15m` hoặc giả lập) liên tục ≥ 30 s thì thu và bíp 3 tiếng; khô liên tục ≥ 15 phút thì mở. Giả lập mưa thu ngay.
- **Thời tiết cũ** (tuổi > 30 phút, hoặc chưa có bản tin nào sau 2 phút kể từ khi khởi động): `fail_safe = true`. Độ ẩm ≥ 85% **và** `light` < 800 thì coi là mưa (`local`); ngược lại "không biết": hai bộ đếm bị reset, giàn giữ nguyên, không tự mở.
- **MANUAL**: lệnh `open`/`close` hoặc bấm nút tay; tự về AUTO sau 10 phút hoặc khi nhận `auto`. Bộ đếm mưa/khô vẫn chạy trong lúc MANUAL nên hết giờ là luật AUTO áp dụng ngay.
- **Thoát `ERROR`**: lệnh `open`/`close`, nút tay (thử thu lại), hoặc **bấm mới** một công tắc hành trình (cho biết vị trí). AUTO không bao giờ tự thử lại. Công tắc kẹt ở mức bấm không tính là bấm mới.
- **Relay**: đảo chiều thì tắt kênh cũ, chờ 200 ms mới bật kênh mới; không bao giờ bật cả hai.
- **OLED** (2 Hz): `DA MO` / `DANG THU` / `DA THU` / `DANG MO` / `LOI`, nhiệt độ, độ ẩm, ánh sáng, chế độ, nguồn mưa, tuổi bản tin, cờ WiFi/MQTT.

## Hiệu chỉnh quang trở
Module quang trở của kit chỉ có ngõ số **DO** (không có AO), nên `light_raw` chỉ ở gần 0 hoặc gần 4095. Quy ước của firmware: `light` cao = sáng.
1. Vặn biến trở xanh trên module cho tới khi đèn DO đổi trạng thái đúng lúc bạn che cảm biến.
2. Mở serial monitor, xem `light_raw` khi sáng và khi che.
3. Che tối mà `light_raw` **tăng** thì đặt `LDR_INVERT` = 1 trong `include/config.h`, nạp lại. Khi đó `light` về gần 0 lúc tối.
Chi tiết và cách dùng module có AO: [`docs/wiring.md`](../docs/wiring.md).

## Cấu hình (`include/config.h`)
Mọi chân, ngưỡng, thời gian nằm ở đây. Các giá trị theo contract (30 s / 15 phút / 10 phút / 30 s / 200 ms / 30 phút / 2 phút / 85% / 800) phải khớp `docs/mqtt-topics.md`; sửa thì sửa cả hai nơi.

| Macro | Mặc định | Ý nghĩa |
|---|---|---|
| `RELAY_ACTIVE_LOW` | 1 | Relay của kit hút khi IN = LOW |
| `LDR_INVERT` | 0 | Đảo cực tính quang trở (module DO của kit thường cần 1) |
| `SIM_TRAVEL_MS` | 8000 | Chưa có motor: coi như hành trình xong sau bấy nhiêu ms (công tắc hành trình vẫn có tác dụng sớm hơn). 0 = chỉ tin công tắc |
| `TELEMETRY_PERIOD_MS` / `TELEMETRY_MIN_GAP_MS` | 5000 / 500 | Chu kỳ telemetry / khoảng cách tối thiểu giữa hai bản tin khi trạng thái đổi dồn dập |
| `WDT_TIMEOUT_S` | 5 | Vòng điều khiển treo quá lâu thì reset về trạng thái relay tắt |
| `OLED_I2C_ADDR` | 0x3C | Đổi thành 0x3D nếu module của bạn là địa chỉ đó |

## Kiến trúc hai core và lý do
```
core 1: loop() = task điều khiển, 20 ms             core 0: task "net"
  đọc nút/công tắc (chống dội 30 ms)                  WiFi: kết nối lại, lùi dần 15..60 s
  DHT11 mỗi 2,5 s, quang trở mỗi 100 ms               MQTT (PubSubClient, buffer 512, LWT)
  lấy lệnh/thời tiết từ hàng đợi  <──── queue ─────   parse JSON, đẩy vào hàng đợi
  Controller::update -> relay, buzzer, OLED           telemetry mỗi 5 s và khi có Changed
  ghi snapshot (mutex)  ────────── mutex ───────────> đọc snapshot rồi publish
```
`WiFi.begin`, phân giải DNS và `mqtt.connect` có thể chặn hàng giây. Nếu chạy chung một vòng lặp thì relay không dừng kịp khi giàn chạm công tắc. Vì vậy vòng điều khiển **không bao giờ gọi hàm mạng** và chỉ dùng hàng đợi không chặn và mutex chờ tối đa 5 ms. Task mạng treo thì giàn vẫn tự chạy theo cảm biến. Ngược lại, nếu vòng điều khiển treo thì watchdog reset và relay về OFF ngay ở đầu `setup()`.

Lõi (`lib/awning_core`) chỉ nhận thời gian và đầu vào qua tham số, không gọi `millis()` hay chân GPIO, nên test được trên máy tính.

## Sự cố thường gặp
| Hiện tượng | Cách xử lý |
|---|---|
| Lỗi biên dịch "Missing include/secrets.h" | Chưa copy `secrets.h.example` thành `secrets.h` |
| Log `WiFi connecting` mãi | SSID/mật khẩu sai, hoặc WiFi 5 GHz (ESP32 chỉ 2.4 GHz) |
| WiFi lên nhưng `MQTT connect failed, rc=-2` (hoặc `-4`) | Không tới được broker: sai IP, firewall chặn cổng 1883, hoặc WiFi bật **AP isolation** (WiFi trường/quán). Dùng hotspot điện thoại hoặc router riêng |
| `rc=4` hoặc `rc=5` | Sai user/mật khẩu (`secrets.h` so với `deploy/mosquitto/passwd`), hoặc ACL từ chối |
| Cả hai LED sáng sẵn lúc đứng yên, tắt khi relay hút | Dây LED đang ở đầu NC của relay: chuyển sang NO (đầu ngoài còn lại, bên kia chân giữa COM). Hai đèn nhỏ trên board relay tắt lúc đứng yên thì là lỗi dây này, không phải `RELAY_ACTIVE_LOW` |
| Relay chạy ngược (hút khi lẽ ra nhả, kêu ngay lúc khởi động) | Đổi `RELAY_ACTIVE_LOW`. Module kích mức cao thì đặt 0 |
| Serial in `OLED not found at 0x3C` | Kiểm tra SDA = GPIO21, SCL = GPIO22, VCC 3.3 V; thử `OLED_I2C_ADDR` 0x3D. Firmware vẫn chạy không cần OLED |
| Nhiệt độ/độ ẩm `--` (telemetry `null`) | DHT11 lỗi 3 lần liên tiếp: kiểm tra dây DATA GPIO4 và nguồn 3.3 V |
| Trời tối mà `rain_source` không thành `local` | Chỉ áp dụng khi thời tiết đã cũ. Kiểm tra `light_raw` và `LDR_INVERT` |
| Giàn báo `ERROR` khi `SIM_TRAVEL_MS = 0` | 30 s không chạm công tắc hành trình: bấm công tắc, hoặc gửi `open`/`close`, hoặc bấm nút tay |
| Board tự reset lặp lại | Xem lý do reset lúc khởi động (`brownout` = nguồn USB yếu, `task_wdt` = vòng điều khiển treo) |

## Giới hạn đã biết
- PubSubClient chỉ **publish ở QoS 0**: bản tin `online` (retained) đi QoS 0. Đăng ký `cmd`/`weather` và LWT `offline` vẫn ở QoS 1. Muốn publish QoS 1 thật sự thì phải đổi thư viện MQTT.
- Kết nối MQTT dùng clean session (broker không giữ phiên), nên lệnh `cmd` phát lúc ESP32 offline sẽ không bị phát lại khi nó online trở lại.
- Arduino core 2.x được ghim (`espressif32@6.9.0`). API watchdog trong `main.cpp` là của IDF 4.4, nâng lên core 3.x phải sửa.

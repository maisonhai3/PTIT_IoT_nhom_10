# MQTT topics

Broker: Mosquitto chạy local bằng Docker (`deploy/`), port 1883, **không TLS** (chỉ dùng trong LAN, không đưa ra Internet).
Có xác thực user/password và ACL (`deploy/mosquitto/acl`). Prefix mọi topic: `pkg/awning01/`.

| Topic | Chiều | QoS | Retained | Payload |
|---|---|---|---|---|
| `telemetry` | ESP32 → backend | 0 | không | JSON, mỗi 5 giây và ngay khi trạng thái đổi |
| `status` | ESP32 → backend | 1 | có | `online` / `offline`. `offline` là **LWT**: broker tự publish khi ESP32 mất kết nối |
| `cmd` | backend → ESP32 | 1 | không | `{"action":"open"}` |
| `weather` | backend → ESP32 | 1 | không | JSON, mỗi 60 giây (heartbeat) và ngay khi thiết bị vừa `online` |

## Payload

### `telemetry` (ESP32 → backend)
```json
{
  "temp": 29.5,
  "humidity": 71,
  "light": 2300,
  "state": "OPEN",
  "mode": "AUTO",
  "rain": false,
  "rain_source": "none",
  "weather_age_s": 45,
  "fail_safe": false,
  "manual_left_s": 0,
  "rssi": -58,
  "uptime_s": 1234
}
```
- `temp`, `humidity`: `null` nếu DHT11 đọc lỗi. `light`: 0..4095, **cao = sáng** (firmware đã đảo cực tính nếu cần).
- `state`: `OPEN | CLOSING | CLOSED | OPENING | ERROR`. `mode`: `AUTO | MANUAL`.
- `rain_source`: `none | api | sim | local`. `weather_age_s`: `-1` nếu chưa nhận bản tin thời tiết nào.
- Không có `ts`: backend gán thời gian khi nhận.

### `cmd` (backend → ESP32)
`{"action":"open|close|auto|simulate_rain|clear_rain"}`. Payload lạ hoặc `action` lạ bị ESP32 bỏ qua.

### `weather` (backend → ESP32)
```json
{ "age_s": 42, "is_raining": false, "rain_expected_15m": true }
```
`age_s` là tuổi của dữ liệu Open-Meteo tại thời điểm backend publish (giây).

## Vì sao thiết kế như vậy
- **`weather` không retained + có `age_s`.** Nếu retained, khi ESP32 khởi động lại nó sẽ nhận ngay một bản tin *cũ* mà không biết cũ bao lâu,
  và nếu tính tuổi từ lúc nhận thì bản tin cũ trông như mới. Với `age_s`, ESP32 tính tuổi = `age_s` + thời gian đã trôi từ lúc nhận (bằng `millis()`),
  không cần NTP. Backend publish lại mỗi 60 giây và ngay khi thấy `status = online`. Nếu Open-Meteo hỏng, backend vẫn publish bản cache nhưng `age_s` tăng dần.
- **`telemetry` không retained.** Nếu retained, backend khởi động lại sẽ nhận số liệu cũ và gán `ts` là bây giờ, làm sai `last_seen`.
- **`cmd` không retained.** Nếu ESP32 offline rồi online lại, nó không được chạy lại lệnh cũ (tránh giàn tự chạy vì lệnh từ hôm qua).
  Vì vậy backend trả 409 khi thiết bị offline thay vì im lặng làm mất lệnh.
- **`status` retained + LWT.** Backend khởi động sau vẫn biết ngay thiết bị đang online hay offline.

## Luật tự động (chạy trên ESP32, không phụ thuộc mạng)
| Hằng số | Giá trị | Ý nghĩa |
|---|---|---|
| Xác nhận mưa | 30 giây | Mưa liên tục ≥ 30 giây mới thu giàn. Giả lập mưa (`sim`) thu ngay |
| Xác nhận khô | 15 phút | Khô liên tục ≥ 15 phút mới mở lại |
| Thời tiết cũ | 30 phút | `age` > 30 phút thì vào fail-safe. Chưa từng nhận bản tin nào thì chờ tối đa 2 phút sau khi khởi động |
| Fail-safe | độ ẩm ≥ 85% **và** `light` < 800 | Coi là mưa (`rain_source = local`). Ngược lại giữ nguyên trạng thái, không tự mở lại |
| Thủ công | 10 phút | `open`/`close` chuyển sang `MANUAL`, hết giờ tự về `AUTO` (hoặc nhận `auto`) |
| Hành trình tối đa | 30 giây | Không chạm công tắc hành trình trong 30 giây → `ERROR`, tắt relay |
| Dead time relay | 200 ms | Đảo chiều: tắt kênh này, đợi 200 ms mới bật kênh kia. Không bao giờ bật cả hai |

Mưa = `weather.is_raining` **hoặc** `weather.rain_expected_15m` **hoặc** đang giả lập mưa.
Ở trạng thái "không biết" (thời tiết cũ nhưng chưa đủ điều kiện fail-safe) cả hai bộ đếm thời gian được reset, giàn giữ nguyên.

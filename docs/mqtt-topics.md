# MQTT topics

Broker: HiveMQ Cloud (TLS, port 8883). Prefix mọi topic: `pkg/awning01/`.

| Topic | Chiều | QoS | Retained | Payload |
|---|---|---|---|---|
| `telemetry` | ESP32 → backend | 0 | có | JSON `Telemetry` (xem `openapi.yaml`), mỗi 5 giây và khi state đổi |
| `status` | ESP32 → backend | 1 | có | `online` / `offline`. `offline` là **LWT**: broker tự publish khi ESP32 mất kết nối |
| `cmd` | backend → ESP32 | 1 | không | `{"action":"open\|close\|auto\|simulate_rain\|clear_rain"}` |
| `weather` | backend → ESP32 | 1 | có | `{"fetched_at":"<RFC3339>","is_raining":bool,"rain_expected_15m":bool,"humidity":number}` |

## Quy ước
- Payload luôn là JSON UTF-8, trừ `status`.
- `ts` do **backend** gán khi nhận (ESP32 không có RTC đáng tin cậy); ESP32 chỉ gửi `uptime_s` nếu cần debug.
- `cmd` **không retained**: nếu ESP32 offline, lệnh cũ không được thực thi lại khi nó online (tránh giàn tự chạy do lệnh từ hôm qua).
- `weather` retained: ESP32 khởi động lại vẫn có bản tin gần nhất ngay. ESP32 tự tính tuổi bản tin từ `fetched_at`
  (hoặc từ lúc nhận nếu chưa đồng bộ NTP) và gửi lại trong `weather_age_s`.

## Luật auto trên ESP32 (tóm tắt)
- Mưa = `weather.is_raining` (hoặc `rain_expected_15m`) hoặc đang `simulate_rain`.
- Mưa liên tục >= 30 s → thu giàn. Khô liên tục >= 15 phút → mở lại.
- Bản tin thời tiết cũ > 30 phút → fail-safe (quyết định ở M4; mặc định: thu khi độ ẩm cao và trời tối, ngược lại giữ nguyên + báo cảnh báo).
- Lệnh tay (`open`/`close`) chuyển sang `MANUAL`; tự về `AUTO` sau N phút hoặc khi nhận `auto`.

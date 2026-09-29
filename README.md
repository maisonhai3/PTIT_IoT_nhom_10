# Giàn phơi thông minh (PTIT IoT – nhóm 10)

Hệ thống IoT giám sát thời tiết và tự động thu giàn phơi khi trời mưa, kèm website hiển thị trạng thái, thời tiết và điều khiển giàn.

```
ESP32 ──MQTT(LAN :1883)──> Mosquitto local <──> Backend (Go) ──REST/WebSocket──> Front-end
                                          └─ Open-Meteo (thời tiết)
```

## Thư mục
| Thư mục | Nội dung | Phụ trách |
|---|---|---|
| `firmware/` | Code ESP32 (PlatformIO/Arduino) | ESP32 + backend |
| `backend/` | API + MQTT client + poller thời tiết + SQLite (Go) | ESP32 + backend |
| `frontend/` | Website | Front-end |
| `docs/` | Contract và sơ đồ nối dây | Cả nhóm |

## Dành cho front-end
Contract nằm ở [`docs/openapi.yaml`](docs/openapi.yaml). Có thể chạy mock server để làm việc khi backend chưa xong:

```bash
npx @stoplight/prism-cli mock docs/openapi.yaml -p 8080
```

Sinh TypeScript types: `npx openapi-typescript docs/openapi.yaml -o frontend/src/api.d.ts`.
Thay đổi contract phải báo cho người làm backend.

## Tài liệu
- [`docs/openapi.yaml`](docs/openapi.yaml): REST + WebSocket API
- [`docs/mqtt-topics.md`](docs/mqtt-topics.md): topic MQTT và luật auto
- [`docs/wiring.md`](docs/wiring.md): sơ đồ nối dây ESP32

## Chạy broker local
```bash
cd deploy
./mosquitto/gen-passwd.sh        # tạo user esp32-awning01 và backend (file passwd bị .gitignore)
docker compose up -d
mosquitto_sub -h localhost -u backend -P <pass> -t 'pkg/awning01/#' -v   # debug
```
Lưu ý mạng: ESP32 và máy chạy broker phải cùng LAN, mở firewall cổng 1883 (và 8080 cho backend).
Wifi trường/công cộng thường bật AP isolation nên ESP32 không tới được broker; dùng hotspot điện thoại hoặc router riêng.

## Cấu hình bí mật
Không commit credential. Copy `firmware/include/secrets.h.example` → `secrets.h` (điền IP LAN của máy chạy broker) và `backend/.env.example` → `.env`.

## Lộ trình
M0 contract + khung repo (hiện tại) → M1 ESP32 standalone → M2 WiFi + MQTT → M3 backend → M4 thời tiết + luật auto → M5 tích hợp front-end → M6 hoàn thiện.

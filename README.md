# Giàn phơi thông minh (PTIT IoT – nhóm 10)

Hệ thống IoT giám sát thời tiết và tự động thu giàn phơi khi trời mưa, kèm website hiển thị trạng thái, thời tiết và điều khiển giàn.

```
ESP32 ──MQTT(TLS)──> HiveMQ Cloud <──> Backend (Go) ──REST/WebSocket──> Front-end
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

## Cấu hình bí mật
Không commit credential. Copy `firmware/include/secrets.h.example` → `secrets.h` và `backend/.env.example` → `.env`.

## Lộ trình
M0 contract + khung repo (hiện tại) → M1 ESP32 standalone → M2 WiFi + MQTT → M3 backend → M4 thời tiết + luật auto → M5 tích hợp front-end → M6 hoàn thiện.

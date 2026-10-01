# Backend (Go)

Cầu nối giữa ESP32 (MQTT) và trình duyệt (REST + WebSocket). Cũng lấy thời tiết từ Open-Meteo, lưu lịch sử vào SQLite
và phục vụ luôn giao diện web ở `../frontend`.

```
ESP32 ──MQTT──> Mosquitto <──MQTT──> backend ──REST/WebSocket──> trình duyệt
                                        └──> Open-Meteo (thời tiết)
```

Contract nằm ở [`../docs/openapi.yaml`](../docs/openapi.yaml) (REST/WS) và [`../docs/mqtt-topics.md`](../docs/mqtt-topics.md) (MQTT).

## Chạy thử không cần phần cứng
```bash
cd backend
go run ./cmd/demo          # broker MQTT nhúng + backend + ESP32 giả + thời tiết giả
# mở http://127.0.0.1:8080
```
Chu kỳ thời tiết giả mặc định 4 phút: 2:30 nắng, 0:30 "sắp mưa", 1:00 đang mưa. Xem `go run ./cmd/demo -h`
(`-rain-cycle 0` để luôn nắng, `-rain-offset 3m10s` để bắt đầu ngay giữa pha mưa, `-live-weather` để dùng Open-Meteo thật, `-token x` để thử token).
ESP32 giả có cả cảm biến mưa: tấm chỉ ướt khi thời tiết giả đang **mưa thật** (pha "đang mưa"), còn pha "sắp mưa" và nút Giả lập mưa thì tấm vẫn khô, y như trên board thật.

## Chạy thật
```bash
cp .env.example .env       # điền MQTT_PASSWORD, tọa độ, ...
go run ./cmd/server        # cần Mosquitto đang chạy (../deploy)
go run ./cmd/simulator     # (tuỳ chọn) ESP32 giả kết nối vào Mosquitto thật, khi chưa có board
```

## Cấu trúc
| Gói | Vai trò |
|---|---|
| `cmd/server` | Chương trình chính |
| `cmd/demo`, `cmd/simulator` | Demo một tiến trình / thiết bị giả (dev và demo) |
| `internal/service` | Lõi: telemetry → trạng thái, online/offline, lệnh, đẩy thời tiết xuống thiết bị, sự kiện |
| `internal/api` | REST, CORS, token, phục vụ front-end |
| `internal/hub` | WebSocket broadcast |
| `internal/mqttx` | Client Paho: tự kết nối lại, tự subscribe lại; bản `Fake` cho test |
| `internal/weather` | Client Open-Meteo, poller có backoff, bản `Scripted` cho demo |
| `internal/store` | SQLite (thuần Go, không cần cgo) |
| `internal/devicesim` | Mô phỏng hành vi firmware trên MQTT, gồm cảm biến mưa (hai ngưỡng có độ trễ, ưu tiên nguồn `sim` > `sensor` > `api` > `local`) |
| `internal/embedbroker` | Broker MQTT nhúng (chỉ dùng cho demo/test) |
| `internal/app` | Ghép các phần trên lại; chứa test end-to-end |

## Test
```bash
go test -race ./...          # có test end-to-end, khoảng 10 giây
go test -short ./...         # bỏ qua end-to-end
```
Test end-to-end chạy broker MQTT thật (nhúng), hai client Paho, thiết bị giả, HTTP và WebSocket, rồi kiểm tra từng
response có khớp `docs/openapi.yaml` không (`internal/api/contract_test.go`, `internal/app/e2e_test.go`).

## Quyết định thiết kế đáng biết
- **`online` = có telemetry mới (< `DEVICE_TIMEOUT`) và chưa nhận Last Will `offline`.** Không tin riêng topic `status`, vì gói retained có thể cũ.
- **Lệnh không retained**, thiết bị offline thì trả **409** thay vì nhét lệnh vào hàng đợi để nó tự chạy vào lúc bất ngờ.
- **Thời tiết gửi kèm `age_s`** (tuổi dữ liệu) thay vì retained, để ESP32 không cần đồng hồ thật mà vẫn biết dữ liệu cũ đến đâu.
- **`POST /api/command` bắt buộc `Content-Type: application/json` và kiểm tra `Origin`.** Trình duyệt gửi được request `text/plain`
  sang máy khác trong LAN mà không cần preflight; nếu cho qua, trang web lạ có thể điều khiển giàn. CORS không chặn được loại request này.
- Handler MQTT không được chặn (không `Publish` rồi chờ bên trong handler), vì Paho xử lý tin đến trên một goroutine duy nhất.

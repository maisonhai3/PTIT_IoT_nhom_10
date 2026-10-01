# Front-end

Giao diện web tiếng Việt: xem trạng thái giàn, điều khiển giàn, xem thời tiết và số liệu cảm biến, biểu đồ lịch sử, nhật ký.
Viết bằng ES modules thuần, **không cần build và không có thư viện khi chạy**: sửa file rồi tải lại trang là thấy.

## Chạy
Backend Go tự phục vụ thư mục này, nên chỉ cần chạy backend:
```bash
cd ../backend && go run ./cmd/demo     # thiết bị giả + thời tiết giả, không cần phần cứng
# mở http://127.0.0.1:8080
```
Các tùy chọn của `demo` giúp xem các trạng thái khó gặp:
| Lệnh | Để làm gì |
|---|---|
| `go run ./cmd/demo -device=false` | Thiết bị offline: dải cảnh báo, nút bị khóa |
| `go run ./cmd/demo -no-weather` | Không có thời tiết: thẻ thời tiết trống, sau 15 giây thiết bị vào chế độ dự phòng |
| `go run ./cmd/demo -token abc` | Máy chủ đòi mã truy cập: hộp thoại nhập mã |
| `go run ./cmd/demo -rain-cycle 0` | Luôn nắng (mặc định chu kỳ 4 phút: nắng, sắp mưa, mưa) |
| `go run ./cmd/demo -rain-offset 3m10s` | Bắt đầu ngay giữa pha mưa: tấm cảm biến mưa giả "ướt", nguồn mưa là **Cảm biến mưa**, ô "Cảm biến mưa" báo **Ướt** |

### Dev front-end bằng server riêng (Vite, live-server, ...)
Mở trang kèm `?api=` trỏ tới backend (được nhớ trong trình duyệt), và cho backend biết origin của bạn:
```bash
CORS_ORIGINS=http://localhost:5173 go run ./cmd/demo      # hoặc đặt CORS_ORIGINS trong backend/.env
# mở http://localhost:5173/?api=http://127.0.0.1:8080
```
Không dùng `CORS_ORIGINS=*`: khi đó trang web bất kỳ mà người dùng trong LAN mở đều điều khiển được giàn.
Mã truy cập (nếu có) được lưu riêng cho từng địa chỉ API, không bao giờ gửi sang địa chỉ khác.

## Cấu trúc
```
index.html            khung trang (không có script/style nội tuyến, để CSP của backend áp dụng được)
css/style.css         toàn bộ style; màu là biến CSS, có bản sáng và tối
js/main.js            điểm vào: nối store, API, WebSocket, các thẻ
js/api.js             client REST + WebSocket (tự kết nối lại), ước lượng lệch đồng hồ từ header Date
js/store.js           store pub/sub + hàm thuần áp tin nhắn WebSocket vào state, gộp nhật ký từ REST và WebSocket
js/format.js          nhãn và định dạng tiếng Việt (thuần logic)
js/chart-math.js      toán cho biểu đồ: thang đo, vạch chia, ngắt đường khi mất dữ liệu (thuần logic)
js/dom.js, icons.js   tạo DOM an toàn (không innerHTML), biểu tượng SVG
js/ui/*.js            các thẻ: awning, weather, sensors, history, events, status, toast
test/                 test cho phần thuần logic (node --test)
e2e/smoke.mjs         kiểm thử trình duyệt thật chạy trên `cmd/demo`
```
Contract với backend nằm ở [`../docs/openapi.yaml`](../docs/openapi.yaml). Đổi contract thì báo người làm backend.

## Kiểm thử
```bash
npm test                 # 42 test cho định dạng, biểu đồ, store, API client; không cần cài gì
npm i && npx playwright install chromium
npm run e2e              # chạy demo thật + trình duyệt: điều khiển, mưa giả lập, cảm biến mưa, biểu đồ, mất máy chủ, thiết bị offline, token
npm run e2e:axe          # thêm kiểm tra trợ năng (axe-core) ở chế độ sáng và tối
SHOTS=/tmp/anh npm run e2e   # lưu ảnh chụp màn hình các trạng thái
```

## Quyết định thiết kế
- **Dữ liệu từ máy chủ và thiết bị luôn đi qua `textContent`/text node** (`js/dom.js`), không dùng `innerHTML`, nên chuỗi lạ (ví dụ lỗi thời tiết) không thể chèn mã vào trang.
- **Ô "Cảm biến mưa" hiện kết luận Khô/Ướt do firmware đưa ra** (đã qua hai ngưỡng có độ trễ), kèm số đo thô `rain_level/4095` để người lắp mạch tự chỉnh ngưỡng; giao diện không tự suy lại từ số đo.
  Thiết bị không có cảm biến (hoặc firmware cũ chưa gửi hai trường này) thì `rain_level`/`rain_wet` là `null` và ô hiện "—".
- **Nhật ký gộp, không ghi đè**: sự kiện đến theo hai đường (REST lúc tải/nối lại và WebSocket realtime). Bản chụp REST có thể được tính từ trước một sự kiện WebSocket vừa đẩy tới,
  nên ghi đè thẳng làm mất sự kiện đó vĩnh viễn (lỗi này từng làm test trình duyệt thỉnh thoảng đỏ). `mergeEvents` gộp theo (thời điểm, loại, chi tiết), bỏ trùng, mới nhất trước.
- **Nút bấm chờ thiết bị phản hồi**: sau khi máy chủ nhận lệnh, nút vẫn quay cho tới khi telemetry cho thấy lệnh có hiệu lực (hoặc 6 giây), vì "máy chủ đã nhận" chưa có nghĩa "giàn đã chạy".
- **"x phút trước" dùng giờ máy chủ** (ước lượng từ header `Date`), vì đồng hồ điện thoại/laptop trong LAN hay lệch.
- **Biểu đồ**: hai panel (nhiệt độ, độ ẩm) dùng chung trục thời gian thay vì hai trục Y; vùng nền xanh lục là lúc giàn đang thu; đường kẻ ngắt khi thiết bị offline hoặc DHT11 đọc lỗi thay vì nối ngang qua chỗ mất dữ liệu.
  Có tooltip (chuột/chạm/phím mũi tên), tóm tắt bằng chữ, và bảng dữ liệu tương đương. Màu lấy từ bảng màu đã kiểm chứng cho người mù màu.
- **Trợ năng**: tương phản đạt chuẩn ở cả hai chế độ (axe-core không báo lỗi), mọi thao tác dùng được bằng bàn phím, trạng thái giàn nằm trong vùng `aria-live`, tôn trọng `prefers-reduced-motion`.

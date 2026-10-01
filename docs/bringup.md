# Bật board lần đầu: từng bước, có kết quả mong đợi

Phần mềm đã được kiểm thử trên máy tính nhưng **chưa từng chạy trên board thật**. Làm lần lượt các bước dưới đây; mỗi bước nói rõ bạn phải thấy gì.
Dừng ở bước đầu tiên không đúng và xem mục "Nếu không đúng". Tháo dây trước khi cắm/rút USB.

## 0. Chuẩn bị
- Nối dây theo [`wiring.md`](wiring.md) (hoặc trang tương tác [`wiring-guide.html`](wiring-guide.html)). **Chưa nối** tải nào ngoài LED qua điện trở 220 Ω.
- Máy chạy broker + backend đã chạy, và `make ps` báo cả hai đang chạy. Làm đúng thứ tự trong README, mục "Chạy thật":
  `cd deploy && ./mosquitto/gen-passwd.sh`, rồi `cp ../backend/.env.example ../backend/.env` (điền `MQTT_PASSWORD`, toạ độ), rồi `docker compose up -d --build` (hoặc `make up` từ thư mục gốc).
  Lệnh `docker compose` chỉ chạy được trong `deploy/`, nơi có `docker-compose.yml`; ở thư mục khác sẽ báo `no configuration file provided: not found`.
  Phải có file mật khẩu **trước** khi chạy compose, nếu không Docker tạo ra một thư mục tên `passwd` và broker không khởi động được.
- Cổng 1883 (broker) phải mở trên máy này cho mạng nhà: Windows hỏi cho phép thì chọn mạng Private, và mạng WiFi đang là Private chứ không phải Public.
  Cổng 8080 (web) chỉ cần mở nếu bạn muốn xem từ điện thoại.
- `firmware/include/secrets.h` đã điền: SSID (WiFi 2,4 GHz), mật khẩu, `MQTT_HOST` = IP LAN của máy chạy broker, `MQTT_USER = esp32-awning01`, mật khẩu khớp `gen-passwd.sh`.
- Lần đầu nên nạp bản demo để thấy phản ứng nhanh: `pio run -e esp32dev-demo -t upload` (xác nhận mưa 3 giây thay vì 30 giây, khô 20 giây thay vì 15 phút).
  Chạy thật thì dùng `pio run -t upload`.

## 1. Nạp và xem serial
`pio device monitor` (115200). Bạn phải thấy, theo thứ tự:
```
[boot] awning firmware, reset reason: poweron
[boot] limit closed=0 open=0, sim travel=4000 ms
[net] WiFi connecting
[net] WiFi up, ip=192.168.x.x rssi=-60
[net] MQTT connecting to <IP broker>:1883
[net] MQTT connected
[ctl] light_raw=... light=... T=.. H=.. state=OPEN mode=AUTO rain=no src=none age=-1s ...   (mỗi giây)
```
**Nếu không đúng:** xem bảng "Sự cố thường gặp" trong [`firmware/README.md`](../firmware/README.md). Hai lỗi hay gặp nhất là WiFi 5 GHz và
WiFi trường có cách ly thiết bị (AP isolation): thử hotspot điện thoại.

## 2. Backend thấy thiết bị
Đi từng chặng, chặng nào không đúng thì dừng ở đó (ESP32 thử nối lại broker với khoảng chờ tăng dần từ 1 đến tối đa 30 giây; bấm **EN** trên board để thử lại ngay):
1. **Gói tin tới broker:** `make logs` (hoặc `docker compose logs mosquitto` trong `deploy/`) có dòng `New client connected from <IP ESP32> as esp32-awning01`.
   Không có dòng nào: gói tin chưa tới broker (serial `MQTT connect failed, rc=-2`): sai `MQTT_HOST`, tường lửa chặn cổng 1883, hoặc WiFi có cách ly thiết bị.
   Có dòng `Client esp32-awning01 disconnected, not authorised`: sai mật khẩu (serial `rc=5`), xem bảng "Sự cố thường gặp" trong `firmware/README.md`.
2. **Backend nối broker:** `make logs` có dòng `mqtt connected` của backend. Lệnh `curl http://localhost:8080/healthz` trả `"mqtt_connected":true`
   và, sau vài giây khi ESP32 gửi telemetry, `"device_online":true`.
3. **Trình duyệt:** mở `http://<IP máy>:8080`: huy hiệu **Thiết bị: online**, ô "Cảm biến" có số liệu, trạng thái **Giàn đang mở**.

Hoặc dòng lệnh: `mosquitto_sub -h <IP> -u backend -P <mật khẩu> -t 'pkg/awning01/#' -v` phải in `telemetry` mỗi 5 giây và `status online`.

## 3. Màn hình OLED
Hiện `DA MO` cỡ lớn, dưới là `T:..C H:..% L:..`, chế độ, mưa, `WiFi:OK MQTT:OK`.
Nếu trống: serial có dòng `OLED not found at 0x3C`. Kiểm tra SDA = GPIO21, SCL = GPIO22, nguồn 3,3 V. Firmware vẫn chạy được khi không có OLED.

## 4. Cảm biến
- **DHT11**: `T=` và `H=` trong serial là số hợp lý (không phải `--`). Thổi hơi lên cảm biến, độ ẩm phải tăng trong vài giây.
- **Quang trở** (module 3 chân DO, GND, VCC: chỉ có ngõ số): vặn biến trở xanh trên module cho tới khi đèn DO đổi trạng thái đúng lúc bạn che cảm biến.
  `light_raw` trong serial chỉ nhảy giữa hai mức (gần 0 và gần 4095). Che tối mà `light_raw` **tăng** thì đặt `LDR_INVERT 1` trong `firmware/include/config.h`
  và nạp lại; sau đó `light` phải về gần 0 khi che tối. Chi tiết: `docs/wiring.md`, mục "Hiệu chỉnh quang trở".

## 5. Relay và công tắc hành trình (chưa có motor nên dùng LED)
1. Trên web bấm **Thu giàn**: relay CH1 phải kêu "tách", LED đỏ sáng khoảng 4 giây (bản demo; 8 giây bản thật), rồi tắt, web báo **Giàn đã thu**.
2. Bấm **Mở giàn**: relay CH2 kêu, LED xanh sáng, rồi tắt.
3. Trong lúc LED đỏ đang sáng, bấm nút "công tắc hành trình đã thu" (GPIO32): LED phải tắt **ngay** và trạng thái thành **Đã thu**.
4. Lúc đứng yên (chưa bấm gì) cả hai LED **tắt**, và hai LED **không bao giờ** cùng sáng, kể cả khi bạn bấm Thu rồi Mở liên tục.
   Cả hai sáng sẵn và chỉ tắt khi relay hút: dây LED đang ở đầu **NC**. Rút USB, chuyển mỗi dây sang đầu vít ngoài còn lại của kênh (bên kia chân giữa COM).

**Nếu relay hoạt động ngược** (hút khi lẽ ra nhả, kêu ngay lúc cắm điện): module của bạn kích mức cao, đặt `RELAY_ACTIVE_LOW 0` và nạp lại.
Phân biệt với lỗi dây NC ở mục 4: nhìn hai đèn nhỏ trên chính board relay lúc đứng yên. Chúng **tắt** mà LED của bạn sáng thì là dây ở NC; chúng **sáng** thì mới là kích ngược.

## 6. Luật tự động
1. Bấm **Tự động**. Bật công tắc **Giả lập mưa** trên web (hoặc giữ nút GPIO25 ba giây): buzzer bíp ba tiếng, giàn tự thu ngay.
2. Tắt giả lập mưa: bản demo tự mở lại sau khoảng 20 giây khô ráo (bản thật: 15 phút). Việc này cần backend lấy được thời tiết
   (thẻ "Thời tiết hiện tại" có số liệu); nếu máy chủ không ra được Internet thì thiết bị vào chế độ dự phòng và **giữ nguyên** thay vì tự mở.
3. Tắt backend khoảng 1 phút (`docker compose stop backend`): board vẫn chạy (nút tay vẫn thu/mở được, luật tự động vẫn hoạt động theo bản tin thời tiết cuối);
   bật lại thì web và board tự hồi phục.
4. Rút nguồn ESP32: sau khoảng 20 giây web báo **Thiết bị: offline** và các nút điều khiển bị khóa.

## 7. Trước khi nối motor hoặc tải thật
- Chỉ dùng tải **một chiều điện áp thấp** qua relay, nguồn ngoài riêng, nối chung GND với ESP32, diode chống dội song song motor. **Không** nối điện 220 V.
- Thay `SIM_TRAVEL_MS` bằng `0` để chỉ tin công tắc hành trình thật, và lắp hai công tắc hành trình vào GPIO32/GPIO33 (nối xuống GND).
  Nếu không tới được công tắc trong 30 giây thì giàn báo `ERROR`, tắt relay và buzzer kêu dài: đây là tính năng an toàn, hãy thử nó (giữ dây công tắc rời ra) trước khi để giàn chạy một mình.
- Đặt `API_TOKEN` trong `backend/.env` nếu mạng LAN có người khác.

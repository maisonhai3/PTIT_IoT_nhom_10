# Sơ đồ nối dây (ESP32 DEVKIT V1, 30 chân)

Nguồn đối chiếu: tài liệu của kit (`LontenTechnology/ESP32_Basic_Starter_Kit_LTARK_8`, file *ESP32 Basic Starter Kit.pdf*).
Tài liệu xác nhận: board DEVKIT V1 (CP2102), relay 2 kênh **kích mức thấp** (chân IN xuống dưới ~2 V thì relay hút, kit dùng GPIO 26 và 27),
DHT11 dạng module ở GPIO 4, OLED **SSD1306 128x64** I2C địa chỉ **0x3C** (SDA = GPIO 21, SCL = GPIO 22).

Nguyên tắc chọn chân: tránh GPIO 0/2/12/15 (strapping); GPIO 34–39 chỉ là input và **không có pull-up nội**;
ADC2 (GPIO 0, 2, 4, 12–15, 25–27) **không đọc được khi bật WiFi**, nên cảm biến analog phải đặt trên ADC1 (GPIO 32–39).
(Ví dụ Project 3 của kit đọc biến trở ở GPIO 4 = ADC2, cách đó sẽ không chạy được cùng WiFi.)

| Linh kiện | Chân linh kiện | ESP32 | Ghi chú |
|---|---|---|---|
| DHT11 (module 3 chân) | DATA | GPIO4 | VCC 3.3V, GND. Module đã có trở kéo lên |
| Module quang trở | AO | GPIO34 (ADC1) | VCC 3.3V. Cực tính tuỳ module, xem mục "Hiệu chỉnh quang trở" |
| OLED 0.96" SSD1306 | SDA / SCL | GPIO21 / GPIO22 | VCC 3.3V, địa chỉ 0x3C |
| Relay CH1 (**thu** giàn) | IN1 | GPIO26 | VCC relay → **5V/VIN**, GND chung. Kích mức thấp (LOW = hút) |
| Relay CH2 (**mở** giàn) | IN2 | GPIO27 | Như trên |
| Công tắc hành trình "đã thu" | nút nhấn | GPIO32 | Một chân nối GND, dùng `INPUT_PULLUP` (bấm = LOW), không cần điện trở |
| Công tắc hành trình "đã mở" | nút nhấn | GPIO33 | Như trên |
| Nút điều khiển tay | nút nhấn | GPIO25 | `INPUT_PULLUP`; bấm ngắn = đảo mở/thu, giữ 3 giây = bật/tắt giả lập mưa |
| Buzzer hoạt động | + | GPIO23 | HIGH = kêu. Chân còn lại nối GND |
| LED đỏ ("đang thu") | | tải của relay CH1 | Xem sơ đồ tải bên dưới |
| LED xanh ("đang mở") | | tải của relay CH2 | Xem sơ đồ tải bên dưới |

## Tải giả lập bằng LED (khi chưa có motor)
Mỗi kênh relay đóng cắt một LED, nên nghe được tiếng relay và thấy đèn:

```
3.3V ──► COM (relay)      NO (relay) ──► điện trở 220Ω ──► LED (anode → cathode) ──► GND
```
Kênh 1 dùng LED đỏ, kênh 2 dùng LED xanh lục. Về sau thay LED bằng motor DC + nguồn ngoài thì **không phải sửa firmware**.

## Hiệu chỉnh quang trở
Tài liệu kit không có module quang trở nên chưa biết ngõ AO tăng hay giảm khi trời sáng (mỗi hãng một kiểu).
Firmware quy ước `light` **cao = sáng** (0..4095). Cách hiệu chỉnh:

1. Nạp firmware, mở serial monitor (115200). Firmware in `light_raw` mỗi giây.
2. Che module bằng tay rồi chiếu đèn pin vào, quan sát số thay đổi thế nào.
3. Nếu che tối mà số **tăng** thì đặt `LDR_INVERT = true` trong `firmware/include/config.h`.
4. Đặt `LOCAL_DARK_BELOW` cao hơn giá trị "che tối" một chút (mặc định 800).

## An toàn
- Chỉ dùng tải **DC điện áp thấp** qua relay. **Không nối điện 220V** khi chưa đủ hiểu biết/thiết bị bảo vệ.
- Nếu sau này dùng motor DC: nguồn ngoài riêng, nối GND chung với ESP32, thêm diode flyback song song motor.
- Không bao giờ bật đồng thời CH1 và CH2 (đảo chiều motor cùng lúc gây ngắn mạch). Firmware có interlock: tắt kênh này, chờ >= 200 ms, mới bật kênh kia.
- Cắm/rút dây khi board đã ngắt USB.

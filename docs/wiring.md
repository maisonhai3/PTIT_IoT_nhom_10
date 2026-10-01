# Sơ đồ nối dây (ESP32 DEVKIT V1, 30 chân)

**Sơ đồ hình toàn mạch (30 dây, chia 7 giai đoạn):** [`wiring-diagram.svg`](wiring-diagram.svg) (bản PNG: [`wiring-diagram.png`](wiring-diagram.png)).
Các chân GPIO trong hình đã được đối chiếu với `firmware/include/config.h`. Thứ tự chân của module quang trở (DO, GND, VCC) trong hình là giả định (tài liệu kit không ghi).
Thứ tự cực vít của relay (NC, COM, NO từ trái sang phải khi đặt board như hình) **đã kiểm trên board thật của nhóm**: bản vẽ đầu tiên đặt NO ở bên trái nên hai LED sáng sẵn lúc đứng yên.
Luôn đọc nhãn in trên module.

![Sơ đồ đi dây toàn mạch](wiring-diagram.svg)

Nguồn đối chiếu: tài liệu của kit (`LontenTechnology/ESP32_Basic_Starter_Kit_LTARK_8`, file *ESP32 Basic Starter Kit.pdf*).
Tài liệu xác nhận: board DEVKIT V1 (CP2102), relay 2 kênh **kích mức thấp** (chân IN xuống dưới ~2 V thì relay hút, kit dùng GPIO 26 và 27),
DHT11 dạng module ở GPIO 4, OLED **SSD1306 128x64** I2C địa chỉ **0x3C** (SDA = GPIO 21, SCL = GPIO 22).

Nguyên tắc chọn chân: tránh GPIO 0/2/12/15 (strapping); GPIO 34–39 chỉ là input và **không có pull-up nội**;
ADC2 (GPIO 0, 2, 4, 12–15, 25–27) **không đọc được khi bật WiFi**, nên cảm biến analog phải đặt trên ADC1 (GPIO 32–39).
(Ví dụ Project 3 của kit đọc biến trở ở GPIO 4 = ADC2, cách đó sẽ không chạy được cùng WiFi.)

| Linh kiện | Chân linh kiện | ESP32 | Ghi chú |
|---|---|---|---|
| DHT11 (module 3 chân) | DATA | GPIO4 | VCC 3.3V, GND. Module đã có trở kéo lên |
| Module quang trở (3 chân: DO, GND, VCC) | **DO** | GPIO34 (ADC1) | VCC 3.3V. Module của kit **không có chân AO**, chỉ có ngõ số DO (0 V hoặc 3,3 V). Ngưỡng sáng/tối chỉnh bằng biến trở xanh trên module, xem mục "Hiệu chỉnh quang trở" |
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

Dùng đầu **NO** (hở khi relay nghỉ), không dùng NC. **Lúc đứng yên cả hai LED phải tắt**; chỉ LED của kênh đang chạy mới sáng.
Cả hai sáng sẵn và chỉ tắt khi relay hút nghĩa là dây LED đang ở NC: chuyển sang đầu ngoài còn lại của kênh (bên kia chân giữa COM).

Vì sao NO: khi ESP32 mất điện, treo hoặc đang khởi động thì relay nhả, và tải phải **không có điện**. Với motor hai chiều, nối NC làm lúc nghỉ cả hai chiều cùng có điện;
interlock trong firmware không cứu được vì nó chỉ chạy khi phần mềm đang chạy.

## Hiệu chỉnh quang trở
Module quang trở của kit (LM393, 3 chân: DO, GND, VCC) **chỉ có ngõ số DO**, không có ngõ tương tự AO. DO chỉ ở hai mức, gần 0 V hoặc 3,3 V, tuỳ ánh sáng
đang mạnh hơn hay yếu hơn ngưỡng đặt bằng **biến trở xanh** trên module. Vì vậy `light_raw` trong serial chỉ nhận hai giá trị (gần 0 và gần 4095), không đổi từ từ.
Firmware quy ước `light` **cao = sáng** (0..4095). Cách hiệu chỉnh:

1. Cấp nguồn. Trên module có hai đèn nhỏ: đèn nguồn và đèn DO (báo mức đầu ra).
2. Vặn biến trở xanh bằng tua vít nhỏ cho tới khi đèn DO đổi trạng thái đúng lúc bạn che cảm biến bằng tay (hoặc tắt đèn phòng).
3. Nạp firmware, mở serial monitor (115200), xem `light_raw` khi sáng và khi che.
4. Nếu che tối mà `light_raw` **tăng** lên gần 4095 (thường gặp: DO lên mức cao khi tối) thì đặt `LDR_INVERT 1` trong `firmware/include/config.h`, nạp lại.
   Khi đó `light` về gần 0 lúc tối. Che tối mà vẫn gần 0 thì ngưỡng chưa đúng: vặn tiếp biến trở.
5. `LOCAL_DARK_BELOW` (mặc định 800) nằm giữa hai mức nên giữ nguyên.

Muốn đo ánh sáng liên tục thì cần module có chân AO (loại 4 chân) hoặc quang trở rời có điện trở phân áp. Nối ngõ analog đó vào cùng GPIO34, không phải sửa code.

## An toàn
- Chỉ dùng tải **DC điện áp thấp** qua relay. **Không nối điện 220V** khi chưa đủ hiểu biết/thiết bị bảo vệ.
- Nếu sau này dùng motor DC: nguồn ngoài riêng, nối GND chung với ESP32, thêm diode flyback song song motor.
- Không bao giờ bật đồng thời CH1 và CH2 (đảo chiều motor cùng lúc gây ngắn mạch). Firmware có interlock: tắt kênh này, chờ >= 200 ms, mới bật kênh kia.
- Cắm/rút dây khi board đã ngắt USB.

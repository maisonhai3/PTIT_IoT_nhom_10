# Sơ đồ nối dây (ESP32 DEVKIT V1, 30 chân)

**Sơ đồ hình toàn mạch (33 dây, chia 8 giai đoạn):** [`wiring-diagram.svg`](wiring-diagram.svg) (bản PNG: [`wiring-diagram.png`](wiring-diagram.png)).
**Trang tương tác** (xem từng giai đoạn, tick từng dây, có bước kiểm sau mỗi giai đoạn): tải [`wiring-guide.html`](wiring-guide.html) về hoặc clone repo rồi mở bằng trình duyệt (GitHub chỉ hiện mã nguồn của file HTML).
Các chân GPIO trong hình đã được đối chiếu với `firmware/include/config.h`. Thứ tự chân của module quang trở (DO, GND, VCC) trong hình là giả định (tài liệu kit không ghi).
Module cảm biến mưa (giai đoạn 8) không thuộc kit mà nhóm mua thêm; thứ tự chân AO, DO, GND, VCC trong hình lấy từ ảnh module của nhóm và nhãn in trên module, hãy đối chiếu với module của bạn.
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
| Cảm biến mưa YL-83 (module 4 chân: AO, DO, GND, VCC, kèm tấm cảm biến) | **AO** | GPIO35 (ADC1) | VCC → **3V3 (không phải 5V)**, GND. **DO không nối.** Xem mục "Cảm biến mưa" |
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

## Cảm biến mưa (YL-83)
Module mua thêm, gồm **tấm cảm biến** (hai mảng đồng đan xen, nước nhỏ lên tấm thì nối hai mảng với nhau) và **mạch so sánh LM393** có 4 chân: `AO`, `DO`, `GND`, `VCC`.
Tấm nối với mạch bằng hai dây có sẵn. Ta chỉ cần nối **3 dây** từ mạch vào ESP32:

| Chân module | Nối vào | Ghi chú |
|---|---|---|
| `VCC` | ray **3V3** của breadboard (nối từ chân 3V3 của ESP32) | **Không phải 5V**, xem bên dưới |
| `GND` | ray GND của breadboard | |
| `AO` | **GPIO35** (D35) | ngõ tương tự; GPIO35 thuộc ADC1 nên đọc được cả khi bật WiFi |
| `DO` | để trống | ngõ số theo ngưỡng của biến trở xanh trên module; firmware tự so ngưỡng trên AO nên không dùng |

**Vì sao 3V3 mà không phải 5V:** khi tấm khô, chân AO có điện áp bằng VCC. Cấp 5V thì chân GPIO35 của ESP32 phải chịu 5 V, trong khi nó chỉ chịu được 3,3 V.

Thử nhanh khi mới cấp điện (nhóm đã làm trên module của mình): tấm khô thì chỉ đèn nguồn sáng; nhỏ nước lên tấm thì đèn thứ hai (đèn DO) sáng thêm; lau khô thì đèn đó tắt.
Đèn DO phụ thuộc biến trở xanh và không ảnh hưởng tới firmware.

Firmware đọc AO (16 mẫu, mỗi 0,5 giây) và **đảo** thành `rain_level` 0..4095, **cao = ướt** (khô ≈ 0). Hai ngưỡng có độ trễ trong `firmware/include/config.h`:
ướt khi `rain_level` ≥ `RAIN_WET_ABOVE` (400), khô lại khi ≤ `RAIN_DRY_BELOW` (200), ở giữa thì giữ kết luận cũ. Ướt liên tục 5 giây thì thu giàn.

**Hiệu chỉnh ngưỡng** (hai số 400 và 200 là ước đoán, mỗi module khác một chút):
1. Mở serial monitor (115200) hoặc xem ô "Cảm biến mưa" trên web: dòng `[ctl]` có `rain_level=… wet=…`.
2. Ghi lại `rain_level` khi **tấm khô**, khi nhỏ **vài giọt** nước, và khi **ướt đẫm**.
3. Đặt `RAIN_WET_ABOVE` cỡ một nửa mức "vài giọt" nhưng cao hơn rõ rệt mức khô, và `RAIN_DRY_BELOW` cỡ một nửa `RAIN_WET_ABOVE`. Khoảng hở giữa hai ngưỡng phải lớn hơn độ nhiễu của số đọc.
4. Nếu `rain_level` **giảm** khi ướt (ngược chiều) thì đặt `RAIN_INVERT 1`.
5. Nếu tấm khô mà `wet=1`, hoặc nhỏ nước mà `rain_level` vẫn ≈ 0: xem bảng "Sự cố thường gặp" trong `firmware/README.md`.

**Lắp đặt:** đặt tấm ngoài trời ở chỗ nước mưa rơi vào được, **nghiêng khoảng 20–30°** để nước chảy đi chứ không đọng (đọng thì tấm ướt mãi sau khi tạnh).
Tấm không chống nước tốt và mạch LM393 thì không chống nước hoàn toàn: giữ mạch ở chỗ khô, che mưa. Sương hoặc hơi ẩm đọng lên tấm cũng làm tấm ướt: đó là hành vi đúng (đồ phơi cũng bị ướt),
nhưng nếu sáng nào giàn cũng thu vì sương thì nâng hai ngưỡng lên.

**Chống ăn mòn (tuỳ chọn):** hai mảng đồng bị ăn mòn dần khi vừa ướt vừa có điện. Muốn giảm hao mòn, nối `VCC` của module vào một chân GPIO còn trống (đề xuất **GPIO18**) thay vì ray 3V3,
rồi đổi `#define RAIN_PWR_PIN -1` thành `18` trong `config.h`. Khi đó firmware chỉ cấp điện khoảng 20 ms trong mỗi 500 ms (chừng 5% thời gian); module chỉ ăn vài mA nên một chân GPIO cấp được.
Không đổi `RAIN_PWR_PIN` mà vẫn nối VCC vào GPIO18 thì module không bao giờ có điện.

## An toàn
- Chỉ dùng tải **DC điện áp thấp** qua relay. **Không nối điện 220V** khi chưa đủ hiểu biết/thiết bị bảo vệ.
- Nếu sau này dùng motor DC: nguồn ngoài riêng, nối GND chung với ESP32, thêm diode flyback song song motor.
- Không bao giờ bật đồng thời CH1 và CH2 (đảo chiều motor cùng lúc gây ngắn mạch). Firmware có interlock: tắt kênh này, chờ >= 200 ms, mới bật kênh kia.
- Cắm/rút dây khi board đã ngắt USB.

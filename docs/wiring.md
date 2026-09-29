# Sơ đồ nối dây (ESP32 DevKit)

Nguyên tắc chọn chân: tránh GPIO 0/2/12/15 (strapping); GPIO 34–39 chỉ input; ADC2 không dùng được khi bật WiFi nên analog đặt trên ADC1 (GPIO 32–39).

| Linh kiện | Chân linh kiện | ESP32 | Ghi chú |
|---|---|---|---|
| DHT11 | DATA | GPIO4 | VCC 3.3V, GND. Module 3 chân đã có trở kéo lên |
| Quang trở (module) | AO | GPIO34 (ADC1) | VCC 3.3V. Dùng ngõ analog |
| OLED 0.96" (I2C) | SDA / SCL | GPIO21 / GPIO22 | VCC 3.3V, địa chỉ thường 0x3C |
| Relay CH1 (**thu** giàn) | IN1 | GPIO26 | VCC relay -> **5V/VIN**, GND chung. Kiểm tra module kích mức thấp hay cao |
| Relay CH2 (**mở** giàn) | IN2 | GPIO27 | Như trên |
| Công tắc hành trình "đã thu" | nút nhấn | GPIO32 | Một chân nối GND, dùng `INPUT_PULLUP` (bấm = LOW) |
| Công tắc hành trình "đã mở" | nút nhấn | GPIO33 | Như trên |
| Nút điều khiển tay | nút nhấn | GPIO25 | `INPUT_PULLUP`; bấm ngắn = đảo mở/thu, giữ 3s = giả lập mưa |
| Buzzer chủ động | + | GPIO23 | Qua trở 220R nếu cần |
| LED đỏ ("đang thu") | anode | tải của relay CH1 | Qua trở 220R. Demo: relay đóng cắt LED, sau này thay bằng motor DC |
| LED xanh ("đang mở") | anode | tải của relay CH2 | Qua trở 220R |

## An toàn
- Chỉ dùng tải **DC điện áp thấp** qua relay. **Không nối điện 220V** khi chưa đủ hiểu biết/thiết bị bảo vệ.
- Nếu sau này dùng motor DC: nguồn ngoài riêng, nối GND chung với ESP32, thêm diode flyback song song motor.
- Không bao giờ bật đồng thời CH1 và CH2 (đảo chiều motor cùng lúc gây ngắn mạch). Firmware phải interlock: tắt kênh này, chờ >= 200 ms, mới bật kênh kia.

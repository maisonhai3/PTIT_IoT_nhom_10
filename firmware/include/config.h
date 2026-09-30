// Toàn bộ chân, ngưỡng, thời gian và cờ cấu hình nằm ở đây.
// Chỉ dùng macro (không include header Arduino) để test trên máy tính đọc được file này.
// Giá trị lấy từ docs/wiring.md và docs/mqtt-topics.md: đổi ở đây thì phải đổi cả docs.
#pragma once

// ---- Chân (docs/wiring.md) ----------------------------------------------------
#define PIN_DHT 4            // DHT11 DATA
#define PIN_LDR 34           // AO của module quang trở (ADC1, chỉ input)
#define PIN_I2C_SDA 21       // OLED SSD1306
#define PIN_I2C_SCL 22
#define PIN_RELAY_CLOSE 26   // relay CH1: THU giàn
#define PIN_RELAY_OPEN 27    // relay CH2: MỞ giàn
#define PIN_LIMIT_CLOSED 32  // công tắc hành trình "đã thu", INPUT_PULLUP, bấm = LOW
#define PIN_LIMIT_OPEN 33    // công tắc hành trình "đã mở"
#define PIN_BTN_MANUAL 25    // nút tay: bấm ngắn = đảo mở/thu, giữ 3 giây = giả lập mưa
#define PIN_BUZZER 23        // buzzer chủ động, HIGH = kêu

#define OLED_I2C_ADDR 0x3C
#define OLED_WIDTH 128
#define OLED_HEIGHT 64

// ---- Phân cực -----------------------------------------------------------------
// Relay của kit kích mức thấp: LOW = hút. Đặt 0 nếu dùng module kích mức cao.
#define RELAY_ACTIVE_LOW 1

// Firmware quy ước light: cao = sáng (0..4095). Nếu module quang trở của bạn cho AO
// GIẢM khi sáng (che tối mà số tăng) thì đặt 1. Cách hiệu chỉnh: README.md, mục "Quang trở".
#define LDR_INVERT 0

// ---- Ngưỡng fail-safe khi thời tiết từ backend đã cũ ---------------------------
#define LOCAL_DARK_BELOW 800    // light < ngưỡng này = trời tối
#define LOCAL_HUMIDITY_HIGH 85  // độ ẩm >= ngưỡng này (%RH) = ẩm cao

// ---- Thời gian (ms) -----------------------------------------------------------
#ifdef DEMO_FAST_TIMERS
// Bản demo: rút ngắn để thấy hết chu trình trong vài chục giây. KHÔNG dùng để chạy thật.
#define RAIN_CONFIRM_MS 3000UL
#define DRY_CONFIRM_MS 20000UL
#define MANUAL_TIMEOUT_MS 60000UL
#define SIM_TRAVEL_MS 4000UL
#else
#define RAIN_CONFIRM_MS 30000UL      // mưa liên tục bấy nhiêu mới thu (sim mưa: thu ngay)
#define DRY_CONFIRM_MS 900000UL      // khô liên tục 15 phút mới mở lại
#define MANUAL_TIMEOUT_MS 600000UL   // MANUAL tự về AUTO sau 10 phút
// Chưa có motor: coi như hành trình kết thúc sau bấy nhiêu ms (công tắc hành trình vẫn
// có tác dụng sớm hơn). 0 = chỉ tin công tắc hành trình.
#define SIM_TRAVEL_MS 8000UL
#endif

#define TRAVEL_TIMEOUT_MS 30000UL         // không tới công tắc trong 30 giây => ERROR
#define RELAY_DEAD_TIME_MS 200UL          // tắt kênh này, đợi 200 ms mới bật kênh kia
#define WEATHER_STALE_MS 1800000UL        // thời tiết cũ hơn 30 phút => fail-safe
#define FIRST_WEATHER_GRACE_MS 120000UL   // chờ bản tin đầu tiên tối đa 2 phút sau khi khởi động

// ---- Vòng điều khiển & cảm biến -----------------------------------------------
#define CONTROL_TICK_MS 20UL          // chu kỳ vòng điều khiển (core 1)
#define WDT_TIMEOUT_S 5               // vòng điều khiển treo quá lâu => reset (relay về OFF)
#define BUTTON_DEBOUNCE_MS 30UL
#define BUTTON_LONG_PRESS_MS 3000UL
#define DHT_PERIOD_MS 2500UL          // DHT11 tối đa ~0.5 Hz
#define DHT_MAX_FAILURES 3            // lỗi liên tiếp bấy nhiêu lần thì coi là mất số liệu
#define LDR_SAMPLES 16                // số mẫu ADC lấy trung bình mỗi lần đọc
#define LDR_PERIOD_MS 100UL
#define OLED_PERIOD_MS 500UL          // 2 Hz
#define SERIAL_LOG_PERIOD_MS 1000UL   // in light_raw để hiệu chỉnh quang trở

// ---- Buzzer -------------------------------------------------------------------
#define BEEP_ON_MS 100UL              // "tít tít tít" khi bắt đầu tự thu giàn
#define BEEP_OFF_MS 100UL
#define ALARM_ON_MS 1000UL            // báo lỗi: kêu dài lặp lại tới khi thoát ERROR
#define ALARM_OFF_MS 1000UL

// ---- MQTT (docs/mqtt-topics.md) -----------------------------------------------
#define MQTT_TOPIC_PREFIX "pkg/awning01/"
#define MQTT_CLIENT_ID "esp32-awning01"
#define MQTT_KEEPALIVE_S 15
// PubSubClient chờ CONNACK bằng vòng lặp bận (không nhường CPU) tới MQTT_SOCKET_TIMEOUT_S giây.
// Phải nhỏ hơn WDT_TIMEOUT_S: task idle của core 0 cũng được watchdog giám sát.
#define MQTT_SOCKET_TIMEOUT_S 2
#define MQTT_BUFFER_BYTES 512
#define TELEMETRY_PERIOD_MS 5000UL
#define TELEMETRY_MIN_GAP_MS 500UL    // gộp các sự kiện Changed dồn dập
#define WIFI_RETRY_MIN_MS 15000UL     // lùi dần khi WiFi mất
#define WIFI_RETRY_MAX_MS 60000UL
#define MQTT_RETRY_MIN_MS 1000UL
#define MQTT_RETRY_MAX_MS 30000UL

// ---- Kiểm tra lúc biên dịch ---------------------------------------------------
#if SIM_TRAVEL_MS > 0 && SIM_TRAVEL_MS >= TRAVEL_TIMEOUT_MS
#error "SIM_TRAVEL_MS phai nho hon TRAVEL_TIMEOUT_MS, neu khong giang luon bi ERROR"
#endif
#if MQTT_SOCKET_TIMEOUT_S >= WDT_TIMEOUT_S
#error "MQTT_SOCKET_TIMEOUT_S phai nho hon WDT_TIMEOUT_S"
#endif

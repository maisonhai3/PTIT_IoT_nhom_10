// Toàn bộ chân, ngưỡng, thời gian và cờ cấu hình nằm ở đây.
// Chỉ dùng macro (không include header Arduino) để test trên máy tính đọc được file này.
// Giá trị lấy từ docs/wiring.md và docs/mqtt-topics.md: đổi ở đây thì phải đổi cả docs.
#pragma once

// ---- Chân (docs/wiring.md) ----------------------------------------------------
#define PIN_DHT 4            // DHT11 DATA
#define PIN_LDR 34           // DO (module 3 chân của kit) hoặc AO (module 4 chân) của quang trở; ADC1, chỉ input
#define PIN_I2C_SDA 21       // OLED SSD1306
#define PIN_I2C_SCL 22
#define PIN_RELAY_CLOSE 26   // relay CH1: THU giàn
#define PIN_RELAY_OPEN 27    // relay CH2: MỞ giàn
#define PIN_LIMIT_CLOSED 32  // công tắc hành trình "đã thu", INPUT_PULLUP, bấm = LOW
#define PIN_LIMIT_OPEN 33    // công tắc hành trình "đã mở"
#define PIN_BTN_MANUAL 25    // nút tay: bấm ngắn = đảo mở/thu, giữ 3 giây = giả lập mưa
#define PIN_BUZZER 23        // buzzer chủ động, HIGH = kêu
#define PIN_RAIN_AO 35       // AO của cảm biến mưa YL-83 (LM393); ADC1 (đọc được khi bật WiFi), chỉ input

#define OLED_I2C_ADDR 0x3C
#define OLED_WIDTH 128
#define OLED_HEIGHT 64

// ---- Phân cực -----------------------------------------------------------------
// Relay của kit kích mức thấp: LOW = hút. Đặt 0 nếu dùng module kích mức cao.
#define RELAY_ACTIVE_LOW 1

// Firmware quy ước light: cao = sáng (0..4095). Nếu che tối mà light_raw TĂNG thì đặt 1
// (module DO của kit thường cần 1). Cách hiệu chỉnh: docs/wiring.md, mục "Hiệu chỉnh quang trở".
#define LDR_INVERT 0

// ---- Cảm biến mưa YL-83 (docs/wiring.md, mục "Cảm biến mưa") ---------------------
#ifndef RAIN_SENSOR_ENABLED
#define RAIN_SENSOR_ENABLED 1  // 0 = chưa gắn cảm biến mưa: firmware bỏ qua nó, telemetry gửi rain_level = null
#endif
#define RAIN_INVERT 0          // 0: AO giảm khi ướt (YL-83 thông thường). 1: AO tăng khi ướt
// -1 = VCC của module nối thẳng ray 3V3. Muốn tấm đỡ bị ăn mòn thì nối VCC vào một chân GPIO còn trống
// (đề xuất 18) và đặt số chân ở đây: firmware chỉ cấp điện trong lúc đo. Module chỉ ăn vài mA.
#ifndef RAIN_PWR_PIN
#define RAIN_PWR_PIN -1
#endif
// rain_level chuẩn hoá 0..4095, cao = ướt (gần 0 khi khô). Ướt từ RAIN_WET_ABOVE trở lên, khô lại từ
// RAIN_DRY_BELOW trở xuống, ở giữa giữ kết luận cũ. Hiệu chỉnh: xem rain_level trong serial và trên web.
#define RAIN_WET_ABOVE 400
#define RAIN_DRY_BELOW 200

// ---- Ngưỡng fail-safe khi thời tiết từ backend đã cũ ---------------------------
#define LOCAL_DARK_BELOW 800    // light < ngưỡng này = trời tối
#define LOCAL_HUMIDITY_HIGH 85  // độ ẩm >= ngưỡng này (%RH) = ẩm cao

// ---- Thời gian (ms) -----------------------------------------------------------
#ifdef DEMO_FAST_TIMERS
// Bản demo: rút ngắn để thấy hết chu trình trong vài chục giây. KHÔNG dùng để chạy thật.
#define RAIN_CONFIRM_MS 3000UL
#define SENSOR_RAIN_CONFIRM_MS 2000UL
#define DRY_CONFIRM_MS 20000UL
#define MANUAL_TIMEOUT_MS 60000UL
#define SIM_TRAVEL_MS 4000UL
#else
#define RAIN_CONFIRM_MS 30000UL      // mưa liên tục bấy nhiêu mới thu (sim mưa: thu ngay)
#define SENSOR_RAIN_CONFIRM_MS 5000UL  // tấm cảm biến ướt liên tục bấy nhiêu mới thu (lọc giọt bắn)
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
#define RAIN_SAMPLES 16               // số mẫu ADC lấy trung bình mỗi lần đọc cảm biến mưa
#define RAIN_PERIOD_MS 500UL
#define RAIN_SETTLE_MS 20UL           // chờ module ổn định sau khi cấp điện (chỉ khi RAIN_PWR_PIN >= 0)
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
#if RAIN_DRY_BELOW >= RAIN_WET_ABOVE
#error "RAIN_DRY_BELOW phai nho hon RAIN_WET_ABOVE (hai nguong cua cam bien mua co do tre)"
#endif
#if MQTT_SOCKET_TIMEOUT_S >= WDT_TIMEOUT_S
#error "MQTT_SOCKET_TIMEOUT_S phai nho hon WDT_TIMEOUT_S"
#endif

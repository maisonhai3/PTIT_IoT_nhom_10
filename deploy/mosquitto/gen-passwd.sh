#!/usr/bin/env bash
# Tạo deploy/mosquitto/passwd cho 2 tài khoản MQTT: esp32-awning01 (thiết bị) và backend.
# Dùng mosquitto_passwd nếu máy đã cài, không thì chạy nó trong Docker.
# Cách dùng: ESP32_PASS=... BACKEND_PASS=... ./gen-passwd.sh   (không đặt biến thì sẽ hỏi)
set -euo pipefail
cd "$(dirname "$0")"

read_pass() { # $1=tên biến, $2=nhãn
  local v="${!1:-}"
  if [ -z "$v" ]; then
    read -r -s -p "Mật khẩu cho $2: " v
    echo >&2
  fi
  [ -n "$v" ] || { echo "Mật khẩu không được để trống ($2)" >&2; exit 1; }
  printf '%s' "$v"
}

ESP32_PASS="$(read_pass ESP32_PASS esp32-awning01)"
BACKEND_PASS="$(read_pass BACKEND_PASS backend)"
export ESP32_PASS BACKEND_PASS # truyền qua biến môi trường, không nội suy vào chuỗi lệnh (mật khẩu có ' hay $ vẫn an toàn)

rm -f passwd
touch passwd
if command -v mosquitto_passwd >/dev/null 2>&1; then
  mosquitto_passwd -b passwd esp32-awning01 "$ESP32_PASS"
  mosquitto_passwd -b passwd backend "$BACKEND_PASS"
else
  # Cùng phiên bản với image trong docker-compose.yml.
  # MSYS_NO_PATHCONV: Git Bash trên Windows không được tự đổi "/work" thành đường dẫn Windows (biến này bị bỏ qua ở nơi khác).
  MSYS_NO_PATHCONV=1 docker run --rm -e ESP32_PASS -e BACKEND_PASS -v "$PWD:/work" -w /work eclipse-mosquitto:2.1.2-alpine sh -c '
    mosquitto_passwd -b passwd esp32-awning01 "$ESP32_PASS" &&
    mosquitto_passwd -b passwd backend "$BACKEND_PASS"'
fi
# Container Mosquitto chạy bằng user khác nên cần đọc được file; file chỉ chứa mật khẩu đã băm.
chmod 644 passwd
echo "Đã tạo $(pwd)/passwd. Nhớ dùng cùng mật khẩu trong backend/.env (MQTT_PASSWORD) và firmware/include/secrets.h."

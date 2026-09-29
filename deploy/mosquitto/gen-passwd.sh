#!/usr/bin/env bash
# Tạo deploy/mosquitto/passwd cho 2 user. Cần Docker.
# Dùng: ESP32_PASS=... BACKEND_PASS=... ./gen-passwd.sh   (không đặt biến thì sẽ hỏi)
set -euo pipefail
cd "$(dirname "$0")"

read_pass() { # $1=tên biến, $2=nhãn
  local v="${!1:-}"
  if [ -z "$v" ]; then
    read -r -s -p "Password cho $2: " v; echo >&2
  fi
  printf '%s' "$v"
}

ESP32_PASS="$(read_pass ESP32_PASS esp32-awning01)"
BACKEND_PASS="$(read_pass BACKEND_PASS backend)"

rm -f passwd
touch passwd
docker run --rm -v "$PWD:/work" -w /work eclipse-mosquitto:2 sh -c "
  mosquitto_passwd -b passwd esp32-awning01 '$ESP32_PASS' &&
  mosquitto_passwd -b passwd backend '$BACKEND_PASS'
"
chmod 644 passwd
echo "Đã tạo $(pwd)/passwd"

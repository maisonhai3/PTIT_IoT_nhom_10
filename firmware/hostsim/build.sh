#!/usr/bin/env bash
# Biên dịch CHÍNH các file nguồn firmware (src/, lib/) cho Linux, chạy trên các stub trong stubs/.
# Kết quả: ../.pio/hostsim/fwsim_demo (DEMO_FAST_TIMERS) và fwsim_prod (thời gian thật).
# Cần chạy `pio run` hoặc `pio test -e native` một lần trước để .pio/libdeps có PubSubClient và ArduinoJson.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
FW="$(cd "$HERE/.." && pwd)"
OUT="$FW/.pio/hostsim"
mkdir -p "$OUT"

first_dir() { for d in "$@"; do [ -d "$d" ] && { echo "$d"; return; }; done; return 1; }
PSC_SRC="$(first_dir "$FW"/.pio/libdeps/*/PubSubClient/src)" || { echo "Thiếu PubSubClient: chạy 'pio run' trước" >&2; exit 1; }
JSON_SRC="$(first_dir "$FW"/.pio/libdeps/*/ArduinoJson/src)" || { echo "Thiếu ArduinoJson: chạy 'pio test -e native' trước" >&2; exit 1; }

# Trên ESP32 unsigned long là 32 bit; ép host giống vậy để timer của PubSubClient cũng tràn như trên board.
PSC="$OUT/PubSubClient"
rm -rf "$PSC"; mkdir -p "$PSC"
cp "$PSC_SRC"/* "$PSC"/
sed -i 's/unsigned long/uint32_t/g' "$PSC/PubSubClient.h" "$PSC/PubSubClient.cpp"

build() { # $1 = hậu tố, còn lại = cờ biên dịch thêm
  local suffix="$1"; shift
  g++ -std=gnu++11 -O1 -g -pthread -Wall -Wextra -Wno-unused-parameter -Wno-sign-compare "$@" \
    -I"$HERE/stubs" -I"$FW/include" -I"$FW/lib/awning_core" -I"$FW/lib/awning_msg" -I"$PSC" -I"$JSON_SRC" \
    "$FW"/src/*.cpp "$FW"/lib/awning_core/*.cpp "$FW"/lib/awning_msg/*.cpp "$PSC/PubSubClient.cpp" \
    "$HERE/sim.cpp" "$HERE/hostmain.cpp" -o "$OUT/fwsim_$suffix"
  echo "built $OUT/fwsim_$suffix"
}
build demo -DDEMO_FAST_TIMERS=1
build prod

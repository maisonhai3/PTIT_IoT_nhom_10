// Thẻ "Cảm biến tại giàn": số liệu do ESP32 gửi lên.
import { h, setText, clear } from '../dom.js';
import { icon, wifiIcon } from '../icons.js';
import { fmtNumber, fmtCountdown, lightLabel, lightPercent, rainSourceLabel, timeAgo, wifiBars } from '../format.js';

function uptimeText(sec) {
  if (sec === undefined || sec === null) return '—';
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m} phút`;
  const hrs = Math.floor(m / 60);
  return hrs < 48 ? `${hrs} giờ ${m % 60} phút` : `${Math.floor(hrs / 24)} ngày ${hrs % 24} giờ`;
}

export function mountSensors(root, { store, serverNow }) {
  const body = h('div', { class: 'tiles' });
  const status = h('span', { class: 'chip', 'data-tone': 'muted' });
  root.append(h('div', { class: 'card-head' }, h('h2', { id: 'sensors-h' }, 'Cảm biến tại giàn'), status), body);

  let lastSeen = null;

  function updateStatus() {
    if (lastSeen) setText(status, `Cập nhật ${timeAgo(serverNow() - Date.parse(lastSeen))}`);
  }

  function tile(label, iconName, value, unit, sub, extra, { text = false } = {}) {
    return h('div', { class: 'tile' },
      h('div', { class: 'tile-label' }, icon(iconName), label),
      h('div', { class: `tile-value${text ? ' is-text' : ''}` }, value, unit ? h('small', {}, unit) : null),
      sub ? h('div', { class: 'tile-sub' }, sub) : null,
      extra ?? null);
  }

  function render(st) {
    const d = st.device;
    const t = d?.telemetry;
    root.dataset.stale = d && !d.online ? 'true' : 'false';
    lastSeen = d?.last_seen ?? null;
    clear(body);
    if (!t) {
      setText(status, '—');
      body.append(h('p', { class: 'empty' }, d ? 'Chưa nhận được số liệu nào từ thiết bị.' : 'Đang tải…'));
      return;
    }
    updateStatus();
    const light = t.light;
    body.append(
      tile('Nhiệt độ', 'thermo', t.temp === null ? '—' : fmtNumber(t.temp), t.temp === null ? '' : '°C', t.temp === null ? 'DHT11 đọc lỗi' : 'Cảm biến DHT11'),
      tile('Độ ẩm', 'drop', t.humidity === null ? '—' : fmtNumber(t.humidity, 0), t.humidity === null ? '' : '%', t.humidity === null ? 'DHT11 đọc lỗi' : 'Cảm biến DHT11'),
      tile('Ánh sáng', 'sun', lightLabel(light), '', `${lightPercent(light)}% (${light}/4095)`,
        h('div', { class: 'meter', 'aria-hidden': 'true' }, h('div', { class: 'meter-fill', 'data-p': Math.round(lightPercent(light) / 5) * 5 })), { text: true }),
      tile('Mưa (theo thiết bị)', 'rain', t.rain ? 'Có mưa' : 'Không mưa', '',
        [t.rain_source !== 'none' ? `Nguồn: ${rainSourceLabel(t.rain_source)}. ` : '',
          t.weather_age_s < 0 ? 'Chưa nhận thời tiết từ máy chủ.' : `Thời tiết thiết bị nhận: ${timeAgo(t.weather_age_s * 1000)}.`].join(''), null, { text: true }),
      tile('Kết nối thiết bị', 'clock', t.rssi === undefined ? '—' : `${t.rssi}`, t.rssi === undefined ? '' : 'dBm',
        `Đã chạy ${uptimeText(t.uptime_s)}`, t.rssi === undefined ? null : h('div', { class: 'tile-sub' }, wifiIcon(wifiBars(t.rssi)), 'Tín hiệu WiFi')),
    );
    if (t.mode === 'MANUAL' && t.manual_left_s > 0) {
      body.append(tile('Thủ công', 'auto', fmtCountdown(t.manual_left_s), '', 'Tự về Tự động khi hết giờ'));
    }
  }

  store.subscribe(render);
  setInterval(updateStatus, 15000);
}

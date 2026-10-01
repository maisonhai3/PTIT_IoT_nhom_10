// Định dạng và nhãn tiếng Việt. Thuần logic, không đụng DOM nên test được bằng `node --test`.

const LOCALE = 'vi-VN';

export function fmtNumber(n, digits = 1) {
  if (n === null || n === undefined || Number.isNaN(n)) return '—';
  return new Intl.NumberFormat(LOCALE, { minimumFractionDigits: 0, maximumFractionDigits: digits }).format(n);
}

export const fmtTemp = (n) => (n === null || n === undefined ? '—' : `${fmtNumber(n)} °C`);
export const fmtPercent = (n) => (n === null || n === undefined ? '—' : `${fmtNumber(n, 0)}%`);

/** "14:05" hoặc "14:05:09". `timeZone` chỉ dùng cho test. */
export function fmtTime(input, { seconds = false, timeZone } = {}) {
  const d = new Date(input);
  if (Number.isNaN(d.getTime())) return '—';
  return new Intl.DateTimeFormat(LOCALE, {
    hour: '2-digit', minute: '2-digit', second: seconds ? '2-digit' : undefined, hourCycle: 'h23', timeZone,
  }).format(d);
}

/** "30/9 14:05" */
export function fmtDateTime(input, { timeZone } = {}) {
  const d = new Date(input);
  if (Number.isNaN(d.getTime())) return '—';
  const day = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'numeric', timeZone }).format(d);
  return `${day} ${fmtTime(d, { timeZone })}`;
}

/** Khoảng thời gian đã trôi qua, bằng mili giây, thành "3 phút trước". Số âm (lệch đồng hồ) coi như vừa xong. */
export function timeAgo(deltaMs) {
  const s = Math.floor(deltaMs / 1000);
  if (!Number.isFinite(s) || s < 5) return 'vừa xong';
  if (s < 60) return `${s} giây trước`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} phút trước`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} giờ trước`;
  return `${Math.floor(h / 24)} ngày trước`;
}

/** 572 -> "9:32" */
export function fmtCountdown(totalSeconds) {
  const s = Math.max(0, Math.floor(totalSeconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

// ---- nhãn ----

export const STATE_TITLE = {
  OPEN: 'Giàn đang mở',
  CLOSING: 'Đang thu giàn…',
  CLOSED: 'Giàn đã thu',
  OPENING: 'Đang mở giàn…',
  ERROR: 'Giàn bị lỗi',
};
export const STATE_SHORT = { OPEN: 'Mở', CLOSING: 'Đang thu', CLOSED: 'Đã thu', OPENING: 'Đang mở', ERROR: 'Lỗi' };
export const MODE_LABEL = { AUTO: 'Tự động', MANUAL: 'Thủ công' };
export const RAIN_SOURCE_LABEL = {
  none: 'Không mưa', api: 'Open-Meteo', sim: 'Giả lập', sensor: 'Cảm biến mưa', local: 'Độ ẩm và ánh sáng',
};

export const stateTitle = (s) => STATE_TITLE[s] ?? 'Không rõ';
export const stateShort = (s) => STATE_SHORT[s] ?? '—';
export const modeLabel = (m) => MODE_LABEL[m] ?? '—';
export const rainSourceLabel = (r) => RAIN_SOURCE_LABEL[r] ?? '—';

/** Câu giải thích vì sao giàn đang ở trạng thái đó / thiết bị đang coi trời thế nào. */
export function rainReason(t) {
  if (!t) return '';
  switch (t.rain_source) {
    case 'api': return 'Có mưa hoặc sắp mưa theo Open-Meteo';
    case 'sim': return 'Đang giả lập mưa';
    case 'sensor': return 'Cảm biến mưa báo tấm đang ướt';
    case 'local': return 'Không có dữ liệu thời tiết mới; độ ẩm cao và trời tối nên coi là mưa';
    default: return t.fail_safe ? 'Không có dữ liệu thời tiết mới; chưa thấy dấu hiệu mưa tại chỗ' : 'Không mưa';
  }
}

/** Giá trị ADC 12-bit 0..4095 thành phần trăm 0..100 (ngoài khoảng thì chặn lại). */
export const adcPercent = (v) => Math.max(0, Math.min(100, Math.round(((v ?? 0) / 4095) * 100)));

/** Ánh sáng 0..4095 (cao = sáng) thành nhãn và phần trăm. */
export function lightLabel(light) {
  if (light === null || light === undefined) return '—';
  if (light < 800) return 'Tối';
  if (light < 2000) return 'Âm u';
  if (light < 3200) return 'Sáng';
  return 'Rất sáng';
}
export const lightPercent = adcPercent;

/**
 * Cảm biến mưa (YL-83) trong telemetry: `{ wet, level }` hoặc null nếu thiết bị không có cảm biến / chưa đọc lần nào
 * (firmware cũ không gửi hai trường này nên cũng ra null). `level` 0..4095, cao = ướt; `wet` là kết luận của firmware
 * sau hai ngưỡng có độ trễ, giao diện không tự suy lại từ `level`.
 */
export function rainPlate(t) {
  const level = Number.isFinite(t?.rain_level) ? t.rain_level : null;
  const wet = typeof t?.rain_wet === 'boolean' ? t.rain_wet : null;
  return level === null && wet === null ? null : { wet, level };
}

/** Những cảm biến thiết bị dựa vào khi mất dữ liệu thời tiết, để nói trong dải cảnh báo. */
export const fallbackSensorsText = (t) => (rainPlate(t) ? 'cảm biến mưa, độ ẩm và ánh sáng' : 'độ ẩm và ánh sáng');

/** RSSI (dBm) thành 0..4 vạch. */
export function wifiBars(rssi) {
  if (rssi === null || rssi === undefined) return 0;
  if (rssi >= -55) return 4;
  if (rssi >= -65) return 3;
  if (rssi >= -75) return 2;
  if (rssi >= -85) return 1;
  return 0;
}

export const COMMAND_LABEL = {
  open: 'Mở giàn', close: 'Thu giàn', auto: 'Tự động', simulate_rain: 'Bật giả lập mưa', clear_rain: 'Tắt giả lập mưa',
};

/** Nhật ký sự kiện thành câu tiếng Việt. `detail` của weather_error là dữ liệu không tin cậy: chỉ dùng textContent. */
export function eventText(e) {
  switch (e.kind) {
    case 'state': return `Giàn: ${stateShort(e.detail).toLowerCase()}`;
    case 'mode': return `Chế độ: ${modeLabel(e.detail).toLowerCase()}`;
    case 'online': return e.detail === 'true' ? 'Thiết bị online' : 'Thiết bị mất kết nối';
    case 'command': return `Lệnh từ web: ${(COMMAND_LABEL[e.detail] ?? e.detail).toLowerCase()}`;
    case 'rain': return e.detail === 'none' ? 'Hết mưa' : `Có mưa (${rainSourceLabel(e.detail)})`;
    case 'weather_error': return `Không lấy được thời tiết: ${e.detail}`;
    default: return `${e.kind}: ${e.detail}`;
  }
}

/** Mã WMO của Open-Meteo thành tên biểu tượng trong icons.js. */
export function weatherIconName(code) {
  if (code === 0 || code === 1) return 'sun';
  if (code === 2) return 'partly';
  if (code === 3) return 'cloud';
  if (code === 45 || code === 48) return 'fog';
  if (code >= 51 && code <= 57) return 'drizzle';
  if (code >= 61 && code <= 67) return 'rain';
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return 'snow';
  if (code >= 80 && code <= 82) return 'rain';
  if (code >= 95 && code <= 99) return 'storm';
  return 'cloud';
}

/** Thông báo lỗi thân thiện cho người dùng theo mã trạng thái của API. */
export function commandErrorText(status) {
  switch (status) {
    case 400: return 'Máy chủ không hiểu lệnh này.';
    case 401: return 'Cần nhập mã truy cập để điều khiển giàn.';
    case 403: return 'Trang này không được phép điều khiển giàn.';
    case 409: return 'Thiết bị đang offline nên không nhận được lệnh.';
    case 503: return 'Máy chủ mất kết nối tới MQTT broker.';
    case 0: return 'Không kết nối được tới máy chủ.';
    default: return 'Gửi lệnh thất bại. Thử lại sau.';
  }
}

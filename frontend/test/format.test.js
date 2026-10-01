import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import * as f from '../js/format.js';

const HCM = 'Asia/Ho_Chi_Minh';

test('số và đơn vị theo kiểu Việt Nam', () => {
  assert.equal(f.fmtNumber(29.5), '29,5');
  assert.equal(f.fmtNumber(30), '30');
  assert.equal(f.fmtNumber(29.54), '29,5');
  assert.equal(f.fmtNumber(null), '—');
  assert.equal(f.fmtNumber(undefined), '—');
  assert.equal(f.fmtNumber(NaN), '—');
  assert.equal(f.fmtTemp(29.5), '29,5 °C');
  assert.equal(f.fmtTemp(null), '—');
  assert.equal(f.fmtPercent(71.4), '71%');
  assert.equal(f.fmtPercent(null), '—');
});

test('giờ và ngày', () => {
  assert.equal(f.fmtTime('2026-09-30T07:05:09Z', { timeZone: HCM }), '14:05');
  assert.equal(f.fmtTime('2026-09-30T07:05:09Z', { timeZone: HCM, seconds: true }), '14:05:09');
  assert.equal(f.fmtTime('2026-09-29T17:00:00Z', { timeZone: HCM }), '00:00', 'nửa đêm phải là 00:00, không phải 24:00');
  assert.equal(f.fmtDateTime('2026-09-30T07:05:09Z', { timeZone: HCM }), '30/9 14:05');
  assert.equal(f.fmtTime('không phải ngày'), '—');
  assert.equal(f.fmtDateTime('x'), '—');
});

test('timeAgo', () => {
  const s = 1000;
  assert.equal(f.timeAgo(0), 'vừa xong');
  assert.equal(f.timeAgo(4 * s), 'vừa xong');
  assert.equal(f.timeAgo(-30 * s), 'vừa xong', 'đồng hồ lệch không được ra số âm');
  assert.equal(f.timeAgo(5 * s), '5 giây trước');
  assert.equal(f.timeAgo(59 * s), '59 giây trước');
  assert.equal(f.timeAgo(60 * s), '1 phút trước');
  assert.equal(f.timeAgo(59 * 60 * s), '59 phút trước');
  assert.equal(f.timeAgo(3600 * s), '1 giờ trước');
  assert.equal(f.timeAgo(23 * 3600 * s), '23 giờ trước');
  assert.equal(f.timeAgo(48 * 3600 * s), '2 ngày trước');
  assert.equal(f.timeAgo(NaN), 'vừa xong');
});

test('đếm ngược', () => {
  assert.equal(f.fmtCountdown(572), '9:32');
  assert.equal(f.fmtCountdown(5), '0:05');
  assert.equal(f.fmtCountdown(600), '10:00');
  assert.equal(f.fmtCountdown(-3), '0:00');
  assert.equal(f.fmtCountdown(59.9), '0:59');
});

test('nhãn trạng thái đủ cho mọi giá trị của contract', () => {
  for (const s of ['OPEN', 'CLOSING', 'CLOSED', 'OPENING', 'ERROR']) {
    assert.notEqual(f.stateTitle(s), 'Không rõ', s);
    assert.notEqual(f.stateShort(s), '—', s);
  }
  for (const m of ['AUTO', 'MANUAL']) assert.notEqual(f.modeLabel(m), '—');
  for (const r of ['none', 'api', 'sim', 'sensor', 'local']) assert.notEqual(f.rainSourceLabel(r), '—');
  assert.equal(f.stateTitle('???'), 'Không rõ');
});

test('mọi giá trị rain_source trong docs/openapi.yaml đều có nhãn và lý do (thêm giá trị vào contract mà quên giao diện thì test này đỏ)', () => {
  const yaml = readFileSync(new URL('../../docs/openapi.yaml', import.meta.url), 'utf8');
  const m = yaml.match(/\n    RainSource:\n(?:      .*\n)*?      enum: \[([^\]]+)\]/);
  assert.ok(m, 'không tìm thấy enum RainSource trong docs/openapi.yaml');
  const values = m[1].split(',').map((x) => x.trim());
  assert.deepEqual(Object.keys(f.RAIN_SOURCE_LABEL).sort(), [...values].sort(), 'nhãn và contract lệch nhau');
  const reasons = new Set();
  for (const v of values) {
    assert.notEqual(f.rainSourceLabel(v), '—', v);
    reasons.add(f.rainReason({ rain_source: v, fail_safe: false }));
  }
  assert.equal(reasons.size, values.length, 'mỗi nguồn phải có câu giải thích riêng');
  assert.equal(new Set(values.map(f.rainSourceLabel)).size, values.length, 'hai nguồn không được trùng nhãn');
});

test('rainReason giải thích đúng nguồn', () => {
  assert.equal(f.rainReason(null), '');
  assert.match(f.rainReason({ rain_source: 'api' }), /Open-Meteo/);
  assert.match(f.rainReason({ rain_source: 'sim' }), /giả lập/);
  assert.match(f.rainReason({ rain_source: 'sensor' }), /cảm biến mưa.*ướt/i);
  assert.match(f.rainReason({ rain_source: 'local' }), /độ ẩm cao/);
  assert.equal(f.rainReason({ rain_source: 'none', fail_safe: false }), 'Không mưa');
  assert.match(f.rainReason({ rain_source: 'none', fail_safe: true }), /không có dữ liệu thời tiết mới/i);
  assert.match(f.rainReason({ rain_source: 'local' }), /không có dữ liệu thời tiết mới/i);
});

test('ánh sáng và WiFi', () => {
  assert.equal(f.lightLabel(0), 'Tối');
  assert.equal(f.lightLabel(799), 'Tối');
  assert.equal(f.lightLabel(800), 'Âm u');
  assert.equal(f.lightLabel(2000), 'Sáng');
  assert.equal(f.lightLabel(3200), 'Rất sáng');
  assert.equal(f.lightLabel(null), '—');
  assert.equal(f.lightPercent(4095), 100);
  assert.equal(f.lightPercent(0), 0);
  assert.equal(f.lightPercent(99999), 100);
  assert.equal(f.lightPercent(null), 0);
  assert.equal(f.adcPercent(2048), 50);
  assert.equal(f.adcPercent(-10), 0);
  assert.deepEqual([-40, -55, -56, -65, -75, -85, -86, null].map(f.wifiBars), [4, 4, 3, 3, 2, 1, 0, 0]);
});

test('cảm biến mưa trong telemetry', () => {
  assert.deepEqual(f.rainPlate({ rain_level: 30, rain_wet: false }), { wet: false, level: 30 });
  assert.deepEqual(f.rainPlate({ rain_level: 2600, rain_wet: true }), { wet: true, level: 2600 });
  assert.deepEqual(f.rainPlate({ rain_level: 0, rain_wet: false }), { wet: false, level: 0 }, 'mức 0 là số đo hợp lệ, không phải "không có"');
  assert.equal(f.rainPlate({ rain_level: null, rain_wet: null }), null, 'thiết bị không có cảm biến mưa');
  assert.equal(f.rainPlate({}), null, 'firmware cũ không gửi hai trường này');
  assert.equal(f.rainPlate(null), null);
  assert.deepEqual(f.rainPlate({ rain_level: 120, rain_wet: null }), { wet: null, level: 120 }, 'thiếu một nửa thì vẫn hiện nửa còn lại');
  assert.deepEqual(f.rainPlate({ rain_level: 'x', rain_wet: true }), { wet: true, level: null }, 'dữ liệu lạ không làm hỏng giao diện');
  assert.match(f.fallbackSensorsText({ rain_level: 30, rain_wet: false }), /cảm biến mưa/);
  assert.equal(f.fallbackSensorsText({ rain_level: null, rain_wet: null }), 'độ ẩm và ánh sáng');
});

test('nhật ký sự kiện thành câu', () => {
  assert.equal(f.eventText({ kind: 'state', detail: 'CLOSING' }), 'Giàn: đang thu');
  assert.equal(f.eventText({ kind: 'mode', detail: 'MANUAL' }), 'Chế độ: thủ công');
  assert.equal(f.eventText({ kind: 'online', detail: 'true' }), 'Thiết bị online');
  assert.equal(f.eventText({ kind: 'online', detail: 'false' }), 'Thiết bị mất kết nối');
  assert.equal(f.eventText({ kind: 'command', detail: 'close' }), 'Lệnh từ web: thu giàn');
  assert.equal(f.eventText({ kind: 'rain', detail: 'none' }), 'Hết mưa');
  assert.equal(f.eventText({ kind: 'rain', detail: 'sim' }), 'Có mưa (Giả lập)');
  assert.equal(f.eventText({ kind: 'rain', detail: 'sensor' }), 'Có mưa (Cảm biến mưa)');
  assert.equal(f.eventText({ kind: 'rain', detail: 'local' }), 'Có mưa (Độ ẩm và ánh sáng)');
  assert.equal(f.eventText({ kind: 'weather_error', detail: 'boom' }), 'Không lấy được thời tiết: boom');
  assert.equal(f.eventText({ kind: 'lạ', detail: 'x' }), 'lạ: x', 'kind lạ vẫn hiển thị được thay vì làm hỏng giao diện');
});

test('biểu tượng thời tiết theo mã WMO', () => {
  const cases = { 0: 'sun', 1: 'sun', 2: 'partly', 3: 'cloud', 45: 'fog', 48: 'fog', 51: 'drizzle', 57: 'drizzle', 61: 'rain', 67: 'rain',
    71: 'snow', 77: 'snow', 80: 'rain', 82: 'rain', 85: 'snow', 95: 'storm', 99: 'storm', 12345: 'cloud' };
  for (const [code, icon] of Object.entries(cases)) assert.equal(f.weatherIconName(Number(code)), icon, `mã ${code}`);
});

test('thông báo lỗi lệnh', () => {
  for (const s of [0, 400, 401, 403, 409, 503, 500]) assert.ok(f.commandErrorText(s).length > 10, String(s));
  assert.match(f.commandErrorText(409), /offline/);
  assert.match(f.commandErrorText(401), /mã truy cập/);
});

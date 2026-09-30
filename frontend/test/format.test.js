import test from 'node:test';
import assert from 'node:assert/strict';
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
  for (const r of ['none', 'api', 'sim', 'local']) assert.notEqual(f.rainSourceLabel(r), '—');
  assert.equal(f.stateTitle('???'), 'Không rõ');
});

test('rainReason giải thích đúng nguồn', () => {
  assert.equal(f.rainReason(null), '');
  assert.match(f.rainReason({ rain_source: 'api' }), /Open-Meteo/);
  assert.match(f.rainReason({ rain_source: 'sim' }), /giả lập/);
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
  assert.deepEqual([-40, -55, -56, -65, -75, -85, -86, null].map(f.wifiBars), [4, 4, 3, 3, 2, 1, 0, 0]);
});

test('nhật ký sự kiện thành câu', () => {
  assert.equal(f.eventText({ kind: 'state', detail: 'CLOSING' }), 'Giàn: đang thu');
  assert.equal(f.eventText({ kind: 'mode', detail: 'MANUAL' }), 'Chế độ: thủ công');
  assert.equal(f.eventText({ kind: 'online', detail: 'true' }), 'Thiết bị online');
  assert.equal(f.eventText({ kind: 'online', detail: 'false' }), 'Thiết bị mất kết nối');
  assert.equal(f.eventText({ kind: 'command', detail: 'close' }), 'Lệnh từ web: thu giàn');
  assert.equal(f.eventText({ kind: 'rain', detail: 'none' }), 'Hết mưa');
  assert.equal(f.eventText({ kind: 'rain', detail: 'sim' }), 'Có mưa (Giả lập)');
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

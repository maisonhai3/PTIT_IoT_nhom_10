import test from 'node:test';
import assert from 'node:assert/strict';
import * as m from '../js/chart-math.js';

const MIN = 60000;

test('linearScale', () => {
  const s = m.linearScale([0, 10], [100, 0]);
  assert.equal(s(0), 100);
  assert.equal(s(10), 0);
  assert.equal(s(2.5), 75);
  assert.equal(m.linearScale([5, 5], [0, 100])(5), 50, 'miền suy biến không được chia cho 0');
});

test('niceStep chọn 1, 2, 5 × 10^n', () => {
  assert.equal(m.niceStep(10, 5), 2);
  assert.equal(m.niceStep(8.2, 4), 2);
  assert.equal(m.niceStep(100, 4), 20);
  assert.equal(m.niceStep(0.9, 3), 0.2);
  assert.equal(m.niceStep(1000, 4), 200);
});

test('niceDomain: vạch đẹp, phủ hết dữ liệu', () => {
  const d = m.niceDomain([25.2, 33.4, 29], { minSpan: 4, ticks: 4 });
  assert.equal(d.min, 24);
  assert.equal(d.max, 34);
  assert.deepEqual(d.ticks, [24, 26, 28, 30, 32, 34]);
  assert.ok(d.min <= 25.2 && d.max >= 33.4);
});

test('niceDomain: minSpan không phóng đại nhiễu, clamp giữ trong giới hạn', () => {
  const flat = m.niceDomain([29.9, 30.1], { minSpan: 4, ticks: 4 });
  assert.ok(flat.max - flat.min >= 4, 'dữ liệu gần như phẳng vẫn phải có miền ít nhất 4 đơn vị');
  const hum = m.niceDomain([95, 99], { minSpan: 20, ticks: 4, clamp: [0, 100] });
  assert.equal(hum.max, 100);
  assert.ok(hum.ticks.every((t) => t <= 100 && t >= hum.min));
  assert.deepEqual(m.niceDomain([], {}).ticks, [0, 1]);
  assert.deepEqual(m.niceDomain([null, undefined, NaN], {}).ticks, [0, 1]);
});

test('timeTicks căn theo giờ địa phương và không vượt quá maxTicks', () => {
  const UTC7 = -420; // getTimezoneOffset của Việt Nam
  const t0 = Date.parse('2026-09-30T05:07:00Z'); // 12:07 giờ VN
  const t1 = t0 + 6 * 60 * MIN;
  const ticks = m.timeTicks(t0, t1, 6, UTC7);
  assert.ok(ticks.length >= 1 && ticks.length <= 6, `được ${ticks.length} vạch`);
  assert.equal(new Date(ticks[0]).getUTCHours(), 6, 'vạch đầu là 13:00 giờ VN = 06:00 UTC');
  assert.equal(new Date(ticks[0]).getUTCMinutes(), 0);
  for (const t of ticks) assert.ok(t >= t0 && t <= t1);
  for (let i = 1; i < ticks.length; i++) assert.equal(ticks[i] - ticks[i - 1], ticks[1] - ticks[0], 'đều nhau');

  const hour = m.timeTicks(t0, t0 + 60 * MIN, 6, UTC7);
  assert.equal(hour[1] - hour[0], 10 * MIN, '1 giờ chia mỗi 10 phút');
  const week = m.timeTicks(t0, t0 + 7 * 24 * 60 * MIN, 6, UTC7);
  assert.ok(week.length <= 6, `7 ngày phải có tối đa 6 vạch, được ${week.length}`);
  assert.equal(week[1] - week[0], 2880 * MIN, '7 ngày chia mỗi 2 ngày');
});

test('segments ngắt tại null và tại khoảng trống lớn', () => {
  const pts = [
    { t: 0, v: 1 }, { t: 1 * MIN, v: 2 }, { t: 2 * MIN, v: null }, { t: 3 * MIN, v: 4 }, { t: 4 * MIN, v: 5 },
    { t: 30 * MIN, v: 6 }, { t: 31 * MIN, v: 7 },
  ];
  const segs = m.segments(pts, (p) => p.v);
  assert.deepEqual(segs.map((s) => s.map((p) => p.v)), [[1, 2], [4, 5], [6, 7]]);
  assert.deepEqual(m.segments([], (p) => p.v), []);
  assert.deepEqual(m.segments([{ t: 0, v: null }], (p) => p.v), []);
});

test('segments: ngưỡng khoảng trống tỉ lệ với mật độ dữ liệu (mẫu thưa vẫn liền nét)', () => {
  // 7 ngày bị lấy mẫu thưa: mỗi điểm cách nhau 11 phút là bình thường, không phải mất dữ liệu.
  const pts = Array.from({ length: 50 }, (_, i) => ({ t: i * 11 * MIN, v: i }));
  assert.equal(m.segments(pts, (p) => p.v).length, 1);
  pts.push({ t: 49 * 11 * MIN + 3 * 60 * MIN, v: 99 }); // rồi im 3 giờ
  assert.equal(m.segments(pts, (p) => p.v).length, 2);
});

test('linePath làm tròn và bắt đầu bằng M', () => {
  const x = (t) => t / 1000;
  const y = (v) => 100 - v;
  assert.equal(m.linePath([{ t: 0, v: 1.234 }, { t: 1000, v: 2.5 }, { t: 2000, v: 3 }], x, y), 'M0 98.8 L1 97.5 L2 97');
});

test('nearestIndex', () => {
  const t = [0, 10, 20, 30];
  assert.equal(m.nearestIndex(t, -100), 0);
  assert.equal(m.nearestIndex(t, 4), 0);
  assert.equal(m.nearestIndex(t, 6), 1);
  assert.equal(m.nearestIndex(t, 21), 2);
  assert.equal(m.nearestIndex(t, 999), 3);
  assert.equal(m.nearestIndex([5], 100), 0);
  assert.equal(m.nearestIndex([], 1), -1);
});

test('closedSpans', () => {
  const p = (t, state) => ({ t, state });
  const pts = [p(0, 'OPEN'), p(10, 'CLOSING'), p(20, 'CLOSED'), p(30, 'OPENING'), p(40, 'OPEN'), p(50, 'CLOSED')];
  assert.deepEqual(m.closedSpans(pts, 100), [{ start: 10, end: 30 }, { start: 50, end: 100 }]);
  assert.deepEqual(m.closedSpans([p(0, 'OPEN')], 100), []);
  assert.deepEqual(m.closedSpans([], 100), []);
  assert.deepEqual(m.closedSpans([p(5, 'CLOSED')], undefined), [{ start: 5, end: 5 }]);
});

test('summarize', () => {
  assert.deepEqual(m.summarize([3, null, 1, 5, undefined]), { min: 1, max: 5, last: 5, count: 3 });
  assert.equal(m.summarize([null]), null);
});

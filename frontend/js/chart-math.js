// Toán học cho biểu đồ lịch sử: thang đo, vạch chia đẹp, ngắt đường khi mất dữ liệu, tìm điểm gần nhất.
// Không đụng DOM nên test được bằng `node --test`.

/** Thang tuyến tính: hàm ánh xạ [d0, d1] -> [r0, r1]. Miền suy biến trả về giữa khoảng. */
export function linearScale([d0, d1], [r0, r1]) {
  if (d1 === d0) return () => (r0 + r1) / 2;
  const k = (r1 - r0) / (d1 - d0);
  return (v) => r0 + (v - d0) * k;
}

/** Bước chia "đẹp" (1, 2, 5 × 10^n) sao cho số vạch xấp xỉ `target`. */
export function niceStep(span, target = 4) {
  const raw = span / Math.max(1, target);
  const pow = 10 ** Math.floor(Math.log10(raw));
  const f = raw / pow;
  const nice = f < 1.5 ? 1 : f < 3 ? 2 : f < 7 ? 5 : 10;
  return nice * pow;
}

/**
 * Miền y đẹp cho một dãy giá trị. `minSpan` giữ cho nhiễu cảm biến không bị phóng đại thành dao động lớn;
 * `clamp` giới hạn cứng (vd độ ẩm 0..100).
 */
export function niceDomain(values, { minSpan = 1, ticks = 4, clamp } = {}) {
  const v = values.filter((x) => x !== null && x !== undefined && Number.isFinite(x));
  if (v.length === 0) return { min: 0, max: 1, ticks: [0, 1] };
  let lo = Math.min(...v);
  let hi = Math.max(...v);
  if (hi - lo < minSpan) {
    const mid = (hi + lo) / 2;
    lo = mid - minSpan / 2;
    hi = mid + minSpan / 2;
  }
  const step = niceStep(hi - lo, ticks);
  let min = Math.floor(lo / step) * step;
  let max = Math.ceil(hi / step) * step;
  if (clamp) {
    min = Math.max(clamp[0], min);
    max = Math.min(clamp[1], max);
  }
  const out = [];
  for (let t = Math.ceil(min / step) * step; t <= max + step * 1e-9; t += step) out.push(round(t, 6));
  return { min, max, ticks: out };
}

const round = (n, d = 2) => Math.round(n * 10 ** d) / 10 ** d;

const STEPS_MS = [1, 2, 5, 10, 15, 30, 60, 120, 180, 360, 720, 1440, 2880].map((m) => m * 60000);

/**
 * Vạch chia thời gian căn theo giờ địa phương. `tzOffsetMin` là chênh lệch của getTimezoneOffset() (phút, dương = phía tây UTC).
 * Trả về các mốc mili giây nằm trong [t0, t1], không nhiều hơn `maxTicks`.
 */
export function timeTicks(t0, t1, maxTicks = 6, tzOffsetMin = new Date(t0).getTimezoneOffset()) {
  const span = t1 - t0;
  const step = STEPS_MS.find((s) => span / s <= maxTicks) ?? STEPS_MS[STEPS_MS.length - 1];
  const off = tzOffsetMin * 60000; // local = utc - off
  const first = Math.ceil((t0 - off) / step) * step + off;
  const out = [];
  for (let t = first; t <= t1; t += step) out.push(t);
  return out;
}

/**
 * Chia dãy điểm thành các đoạn liên tục: ngắt tại giá trị null (cảm biến lỗi) và tại khoảng trống lớn (thiết bị offline),
 * để đường kẻ không nối ngang qua chỗ mất dữ liệu. `gapFactor` × khoảng cách trung vị, tối thiểu `minGapMs`.
 */
export function segments(points, valueOf, { gapFactor = 4, minGapMs = 5 * 60000 } = {}) {
  const deltas = [];
  for (let i = 1; i < points.length; i++) deltas.push(points[i].t - points[i - 1].t);
  deltas.sort((a, b) => a - b);
  const median = deltas.length ? deltas[Math.floor(deltas.length / 2)] : 0;
  const maxGap = Math.max(minGapMs, gapFactor * median);

  const out = [];
  let cur = [];
  let prevT = null;
  for (const p of points) {
    const v = valueOf(p);
    const gap = prevT !== null && p.t - prevT > maxGap;
    if (v === null || v === undefined || !Number.isFinite(v) || gap) {
      if (cur.length) out.push(cur);
      cur = [];
    }
    if (v !== null && v !== undefined && Number.isFinite(v)) cur.push({ t: p.t, v });
    prevT = p.t;
  }
  if (cur.length) out.push(cur);
  return out;
}

/** "M x y L x y ..." với tọa độ làm tròn 1 chữ số. */
export function linePath(seg, x, y) {
  return seg.map((p, i) => `${i ? 'L' : 'M'}${round(x(p.t), 1)} ${round(y(p.v), 1)}`).join(' ');
}

/** Chỉ số điểm có thời gian gần `t` nhất (tìm nhị phân). `times` tăng dần. */
export function nearestIndex(times, t) {
  if (times.length === 0) return -1;
  let lo = 0;
  let hi = times.length - 1;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (times[mid] < t) lo = mid;
    else hi = mid;
  }
  return Math.abs(times[lo] - t) <= Math.abs(times[hi] - t) ? lo : hi;
}

/** Các khoảng thời gian giàn không ở trạng thái mở (đang thu, đã thu) — để tô nền trên biểu đồ. */
export function closedSpans(points, endT) {
  const spans = [];
  let start = null;
  const isClosed = (s) => s === 'CLOSED' || s === 'CLOSING';
  points.forEach((p, i) => {
    const closed = isClosed(p.state);
    if (closed && start === null) start = p.t;
    if (!closed && start !== null) {
      spans.push({ start, end: p.t });
      start = null;
    }
    if (i === points.length - 1 && start !== null) spans.push({ start, end: Math.max(endT ?? p.t, p.t) });
  });
  return spans;
}

/** Tóm tắt bằng chữ cho trình đọc màn hình. */
export function summarize(values) {
  const v = values.filter((x) => x !== null && x !== undefined && Number.isFinite(x));
  if (!v.length) return null;
  return { min: Math.min(...v), max: Math.max(...v), last: v[v.length - 1], count: v.length };
}

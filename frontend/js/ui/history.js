// Thẻ "Lịch sử": nhiệt độ và độ ẩm theo thời gian, hai panel dùng chung trục thời gian (không dùng hai trục Y),
// vùng nền cho lúc giàn đang thu, tooltip + phím mũi tên, và bảng dữ liệu tương đương.
import { h, s, setText, clear } from '../dom.js';
import { fmtDateTime, fmtNumber, fmtTime, lightLabel, modeLabel, rainSourceLabel, stateShort } from '../format.js';
import { closedSpans, linePath, linearScale, nearestIndex, niceDomain, segments, summarize, timeTicks } from '../chart-math.js';

export const RANGES = [
  { hours: 1, label: '1 giờ' },
  { hours: 6, label: '6 giờ' },
  { hours: 24, label: '24 giờ' },
  { hours: 168, label: '7 ngày' },
];

const M = { left: 46, right: 58, top: 6, bottom: 26 };
const TITLE_H = 20;
const PANEL_H = 104;
const GAP = 18;
const HEIGHT = M.top + 2 * (TITLE_H + PANEL_H) + GAP + M.bottom;

const PANELS = [
  { key: 'temp', title: 'Nhiệt độ (°C)', value: (p) => p.temp, dom: { minSpan: 4, ticks: 2 }, fmt: (v) => `${fmtNumber(v)}°` },
  { key: 'hum', title: 'Độ ẩm (%)', value: (p) => p.humidity, dom: { minSpan: 20, ticks: 2, clamp: [0, 100] }, fmt: (v) => `${fmtNumber(v, 0)}%` },
];

const toPoints = (history) => history.map((p) => ({
  t: Date.parse(p.ts), temp: p.temp, humidity: p.humidity, light: p.light, state: p.state, mode: p.mode, rain: p.rain, rain_source: p.rain_source,
}));

export function mountHistory(root, { store, serverNow, setRange }) {
  const rangeButtons = RANGES.map((r) => h('button', { class: 'btn', type: 'button', 'aria-pressed': 'false', 'data-hours': r.hours, onclick: () => setRange(r.hours) }, r.label));
  const tableToggle = h('button', { class: 'btn', type: 'button', 'aria-expanded': 'false', 'aria-controls': 'history-table' }, 'Xem dạng bảng');
  const legend = h('div', { class: 'legend', 'aria-label': 'Chú thích' },
    h('span', { class: 'key k-temp' }, h('i'), 'Nhiệt độ'),
    h('span', { class: 'key k-hum' }, h('i'), 'Độ ẩm'),
    h('span', { class: 'key' }, h('i', { class: 'wash' }), 'Giàn đang thu / đã thu'));
  const svg = s('svg', {
    class: 'chart', role: 'group', tabindex: '0', focusable: 'true',
    'aria-label': 'Biểu đồ lịch sử nhiệt độ và độ ẩm. Dùng phím mũi tên trái phải để xem từng điểm dữ liệu.',
  });
  const tooltip = h('div', { class: 'tooltip', hidden: true });
  const box = h('div', { class: 'chart-box' }, svg, tooltip);
  const summary = h('p', { class: 'chart-summary' });
  const tableWrap = h('div', { class: 'table-wrap', id: 'history-table', role: 'region', tabindex: '0', 'aria-label': 'Bảng dữ liệu lịch sử', hidden: true });

  root.append(
    h('div', { class: 'card-head' }, h('h2', { id: 'history-h' }, 'Lịch sử nhiệt độ và độ ẩm')),
    h('div', { class: 'filters', role: 'group', 'aria-label': 'Khoảng thời gian' }, rangeButtons, h('span', { class: 'spacer' }), tableToggle),
    legend, box, summary, tableWrap,
  );

  let pts = [];
  let width = 0;
  let geom = null; // { x, ys[], plotL, plotR } cho tooltip
  let cross = null;
  let dots = [];
  let kb = -1; // chỉ số điểm đang chọn bằng bàn phím

  // ---- vẽ ----
  function draw() {
    const st = store.get();
    const hours = st.historyRange;
    pts = toPoints(st.history);
    box.classList.toggle('is-loading', Boolean(st.historyLoading));
    rangeButtons.forEach((b) => b.setAttribute('aria-pressed', String(Number(b.dataset.hours) === hours)));

    width = Math.max(300, Math.floor(box.clientWidth) || 640);
    svg.setAttribute('viewBox', `0 0 ${width} ${HEIGHT}`);
    svg.setAttribute('width', width);
    svg.setAttribute('height', HEIGHT);
    clear(svg);
    geom = null;
    hideTip();

    if (pts.length === 0) {
      svg.append(s('text', { class: 'nodata', x: width / 2, y: HEIGHT / 2, 'text-anchor': 'middle' },
        st.historyLoading ? 'Đang tải…' : 'Chưa có dữ liệu trong khoảng này. Biểu đồ sẽ hiện khi thiết bị gửi số liệu.'));
      setText(summary, '');
      return;
    }

    const t1 = Math.max(serverNow(), pts[pts.length - 1].t);
    const t0 = t1 - hours * 3600 * 1000;
    const plotL = M.left;
    const plotR = width - M.right;
    const x = linearScale([t0, t1], [plotL, plotR]);
    const spans = closedSpans(pts, t1);
    const ys = [];

    PANELS.forEach((panel, k) => {
      const top = M.top + k * (TITLE_H + PANEL_H + GAP);
      const plotT = top + TITLE_H;
      const plotB = plotT + PANEL_H;
      const dom = niceDomain(pts.map(panel.value), panel.dom);
      const y = linearScale([dom.min, dom.max], [plotB, plotT]);
      ys.push(y);

      svg.append(s('text', { class: 'panel-title', x: plotL, y: top + 13 }, panel.title));
      for (const tv of dom.ticks) {
        svg.append(
          s('line', { class: 'gridline', x1: plotL, x2: plotR, y1: y(tv), y2: y(tv) }),
          s('text', { class: 'tick', x: plotL - 8, y: y(tv) + 4, 'text-anchor': 'end' }, fmtNumber(tv, tv % 1 ? 1 : 0)));
      }
      for (const sp of spans) {
        const a = Math.max(plotL, x(sp.start));
        const b = Math.min(plotR, x(Math.max(sp.end, sp.start)));
        if (b - a >= 1) svg.append(s('rect', { class: 'wash', x: a, y: plotT, width: b - a, height: PANEL_H }));
      }
      svg.append(s('line', { class: 'axisline', x1: plotL, x2: plotR, y1: plotB, y2: plotB }));

      const segs = segments(pts, panel.value);
      for (const seg of segs) {
        if (seg.length === 1) svg.append(s('circle', { class: `end-dot dot-${panel.key}`, cx: x(seg[0].t), cy: y(seg[0].v), r: 3 }));
        else svg.append(s('path', { class: `line line-${panel.key}`, d: linePath(seg, x, y) }));
      }
      const lastSeg = segs[segs.length - 1];
      if (lastSeg) {
        const last = lastSeg[lastSeg.length - 1];
        svg.append(
          s('circle', { class: `end-dot dot-${panel.key}`, cx: x(last.t), cy: y(last.v), r: 4 }),
          s('text', { class: 'end-label', x: Math.min(x(last.t) + 9, width - M.right + 8), y: y(last.v) + 4 }, panel.fmt(last.v)));
      }
    });

    // Trục thời gian
    const bottom = M.top + 2 * (TITLE_H + PANEL_H) + GAP;
    const maxTicks = width < 480 ? 4 : 6;
    for (const tk of timeTicks(t0, t1, maxTicks)) {
      const px = x(tk);
      if (px < plotL || px > plotR) continue;
      const label = hours >= 72
        ? new Intl.DateTimeFormat('vi-VN', { day: 'numeric', month: 'numeric' }).format(tk)
        : fmtTime(tk);
      svg.append(
        s('line', { class: 'axisline', x1: px, x2: px, y1: bottom, y2: bottom + 4 }),
        s('text', { class: 'tick', x: px, y: bottom + 18, 'text-anchor': 'middle' }, label));
    }

    // Lớp tương tác
    cross = s('line', { class: 'crosshair', y1: M.top + TITLE_H, y2: bottom, visibility: 'hidden' });
    dots = PANELS.map((p) => s('circle', { class: `end-dot dot-${p.key}`, r: 4, visibility: 'hidden' }));
    const overlay = s('rect', { x: plotL, y: M.top, width: plotR - plotL, height: bottom - M.top, fill: 'transparent', 'pointer-events': 'all' });
    svg.append(cross, ...dots, overlay);
    const times = pts.map((p) => p.t);
    overlay.addEventListener('pointermove', (e) => {
      const r = svg.getBoundingClientRect();
      const px = ((e.clientX - r.left) / r.width) * width;
      kb = nearestIndex(times, t0 + ((px - plotL) / (plotR - plotL)) * (t1 - t0));
      showAt(kb);
    });
    overlay.addEventListener('pointerleave', hideTip);
    geom = { x, ys, plotL, plotR };

    // Tóm tắt bằng chữ (cho trình đọc màn hình và để đọc nhanh)
    const tempS = summarize(pts.map((p) => p.temp));
    const humS = summarize(pts.map((p) => p.humidity));
    const range = RANGES.find((r) => r.hours === hours)?.label ?? `${hours} giờ`;
    const parts = [];
    if (tempS) parts.push(`nhiệt độ từ ${fmtNumber(tempS.min)} đến ${fmtNumber(tempS.max)} °C (mới nhất ${fmtNumber(tempS.last)} °C)`);
    if (humS) parts.push(`độ ẩm từ ${fmtNumber(humS.min, 0)}% đến ${fmtNumber(humS.max, 0)}% (mới nhất ${fmtNumber(humS.last, 0)}%)`);
    parts.push(spans.length ? `giàn đã thu ${spans.length} lần` : 'giàn chưa thu lần nào');
    setText(summary, `Trong ${range} qua: ${parts.join('; ')}.`);

    if (!tableWrap.hidden) drawTable();
  }

  // ---- tooltip ----
  function row(cls, label, value) {
    return h('div', { class: 'tt-row' }, h('span', { class: `tt-key ${cls ?? ''}` }, cls ? h('i') : null, label), h('span', { class: 'tt-val' }, value));
  }

  function showAt(i) {
    if (!geom || i < 0 || i >= pts.length) return;
    const p = pts[i];
    const cx = geom.x(p.t);
    cross.setAttribute('x1', cx);
    cross.setAttribute('x2', cx);
    cross.setAttribute('visibility', 'visible');
    PANELS.forEach((panel, k) => {
      const v = panel.value(p);
      if (v === null || v === undefined) dots[k].setAttribute('visibility', 'hidden');
      else {
        dots[k].setAttribute('cx', cx);
        dots[k].setAttribute('cy', geom.ys[k](v));
        dots[k].setAttribute('visibility', 'visible');
      }
    });
    clear(tooltip);
    tooltip.append(
      h('div', { class: 'tt-time' }, fmtDateTime(p.t)),
      row('k-temp', 'Nhiệt độ', p.temp === null ? '—' : `${fmtNumber(p.temp)} °C`),
      row('k-hum', 'Độ ẩm', p.humidity === null ? '—' : `${fmtNumber(p.humidity, 0)}%`),
      row(null, 'Ánh sáng', lightLabel(p.light)),
      row(null, 'Giàn', `${stateShort(p.state)} · ${modeLabel(p.mode).toLowerCase()}`),
      row(null, 'Mưa', p.rain ? rainSourceLabel(p.rain_source) : 'Không'));
    tooltip.hidden = false;
    const tw = tooltip.offsetWidth;
    tooltip.style.left = `${cx + 14 + tw > width ? Math.max(0, cx - 14 - tw) : cx + 14}px`;
    tooltip.style.top = `${M.top + TITLE_H}px`;
  }

  function hideTip() {
    tooltip.hidden = true;
    cross?.setAttribute('visibility', 'hidden');
    dots.forEach((d) => d.setAttribute('visibility', 'hidden'));
  }

  svg.addEventListener('keydown', (e) => {
    if (!pts.length) return;
    const last = pts.length - 1;
    if (e.key === 'ArrowLeft') kb = kb < 0 ? last : Math.max(0, kb - 1);
    else if (e.key === 'ArrowRight') kb = kb < 0 ? last : Math.min(last, kb + 1);
    else if (e.key === 'Home') kb = 0;
    else if (e.key === 'End') kb = last;
    else if (e.key === 'Escape') { hideTip(); return; }
    else return;
    e.preventDefault();
    showAt(kb);
  });
  svg.addEventListener('focus', () => { if (pts.length) { if (kb < 0) kb = pts.length - 1; showAt(kb); } });
  svg.addEventListener('blur', hideTip);

  // ---- bảng dữ liệu tương đương ----
  function drawTable() {
    clear(tableWrap);
    const head = ['Thời gian', 'Nhiệt độ (°C)', 'Độ ẩm (%)', 'Ánh sáng', 'Giàn', 'Chế độ', 'Mưa'];
    const rows = [...pts].reverse().map((p) => h('tr', {},
      h('td', {}, fmtDateTime(p.t)),
      h('td', {}, p.temp === null ? '—' : fmtNumber(p.temp)),
      h('td', {}, p.humidity === null ? '—' : fmtNumber(p.humidity, 0)),
      h('td', {}, `${lightLabel(p.light)} (${p.light})`),
      h('td', {}, stateShort(p.state)),
      h('td', {}, modeLabel(p.mode)),
      h('td', {}, p.rain ? rainSourceLabel(p.rain_source) : 'Không')));
    tableWrap.append(h('table', { class: 'data' },
      h('caption', { class: 'sr-only' }, 'Dữ liệu lịch sử, mới nhất ở trên'),
      h('thead', {}, h('tr', {}, head.map((c) => h('th', { scope: 'col' }, c)))),
      h('tbody', {}, rows)));
  }

  tableToggle.addEventListener('click', () => {
    const open = tableWrap.hidden;
    tableWrap.hidden = !open;
    tableToggle.setAttribute('aria-expanded', String(open));
    setText(tableToggle, open ? 'Ẩn bảng' : 'Xem dạng bảng');
    if (open) drawTable();
  });

  // ---- cập nhật ----
  let sig = '';
  store.subscribe((st) => {
    const next = `${st.historyRange}|${st.history.length}|${st.history.at(-1)?.ts ?? ''}|${st.historyLoading ? 1 : 0}`;
    if (next === sig) return;
    sig = next;
    draw();
  });
  let raf = 0;
  new ResizeObserver(() => {
    if (Math.abs(Math.floor(box.clientWidth) - width) < 1 || raf) return;
    raf = requestAnimationFrame(() => { raf = 0; draw(); });
  }).observe(box);
  setInterval(draw, 60000); // trượt cửa sổ thời gian
}

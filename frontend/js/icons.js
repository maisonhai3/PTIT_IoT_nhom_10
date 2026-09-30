// Biểu tượng nét vẽ 24x24 (kiểu Feather). Tự vẽ bằng SVG để không phụ thuộc phông emoji của từng hệ điều hành.
import { s } from './dom.js';

const PATHS = {
  sun: ['circle:12,12,4', 'M12 2v2', 'M12 20v2', 'M2 12h2', 'M20 12h2', 'M4.9 4.9l1.4 1.4', 'M17.7 17.7l1.4 1.4', 'M4.9 19.1l1.4-1.4', 'M17.7 6.3l1.4-1.4'],
  partly: ['circle:8,8,3', 'M8 2v1.5', 'M2 8h1.5', 'M3.8 3.8l1 1', 'M12.2 3.8l-1 1', 'M17.5 21H9a4 4 0 0 1-.6-7.96A5.5 5.5 0 0 1 19 14.5a3.25 3.25 0 0 1-1.5 6.5z'],
  cloud: ['M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z'],
  fog: ['M5 9h14', 'M3 13h18', 'M6 17h12', 'M9 21h6'],
  drizzle: ['M8 19v2', 'M8 13v2', 'M16 19v2', 'M16 13v2', 'M12 21v2', 'M12 15v2', 'M20 16.58A5 5 0 0 0 18 7h-1.26A8 8 0 1 0 4 15.25'],
  rain: ['M16 13v8', 'M8 13v8', 'M12 15v8', 'M20 16.58A5 5 0 0 0 18 7h-1.26A8 8 0 1 0 4 15.25'],
  snow: ['M20 17.58A5 5 0 0 0 18 8h-1.26A8 8 0 1 0 4 16.25', 'M8 16h.01', 'M8 20h.01', 'M12 18h.01', 'M12 22h.01', 'M16 16h.01', 'M16 20h.01'],
  storm: ['M19 16.9A5 5 0 0 0 18 7h-1.26a8 8 0 1 0-11.62 9', 'M13 11l-4 6h6l-4 6'],
  thermo: ['M14 14.76V3.5a2.5 2.5 0 0 0-5 0v11.26a4.5 4.5 0 1 0 5 0z'],
  drop: ['M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z'],
  warn: ['M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z', 'M12 9v4', 'M12 17h.01'],
  offline: ['circle:12,12,10', 'M15 9l-6 6', 'M9 9l6 6'],
  check: ['M20 6L9 17l-5-5'],
  open: ['M5 12h14', 'M13 6l6 6-6 6'],
  close: ['M19 12H5', 'M11 6l-6 6 6 6'],
  auto: ['M23 4v6h-6', 'M1 20v-6h6', 'M3.51 9a9 9 0 0 1 14.85-3.36L23 10', 'M1 14l4.64 4.36A9 9 0 0 0 20.49 15'],
  clock: ['circle:12,12,10', 'M12 6v6l4 2'],
  cmd: ['M4 17l6-6-6-6', 'M12 19h8'],
  info: ['circle:12,12,10', 'M12 16v-4', 'M12 8h.01'],
  refresh: ['M23 4v6h-6', 'M20.49 15a9 9 0 1 1-2.12-9.36L23 10'],
};

/** icon('rain', { size: 24, class: 'wx-icon' }) -> <svg> */
export function icon(name, { size, class: cls } = {}) {
  const parts = PATHS[name] ?? PATHS.info;
  const svg = s('svg', {
    viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 1.75, 'stroke-linecap': 'round', 'stroke-linejoin': 'round',
    'aria-hidden': 'true', focusable: 'false', width: size, height: size, class: cls,
  });
  for (const p of parts) {
    if (p.startsWith('circle:')) {
      const [cx, cy, r] = p.slice(7).split(',');
      svg.append(s('circle', { cx, cy, r }));
    } else {
      svg.append(s('path', { d: p }));
    }
  }
  return svg;
}

/** Biểu tượng sóng WiFi 4 vạch; các vạch chưa "sáng" mờ đi. */
export function wifiIcon(bars) {
  const svg = s('svg', { viewBox: '0 0 22 16', class: 'wifi', 'aria-hidden': 'true', focusable: 'false' });
  [0, 1, 2, 3].forEach((i) => {
    const height = 4 + i * 4;
    svg.append(s('rect', { x: i * 6, y: 16 - height, width: 4, height, rx: 1, class: i < bars ? '' : 'off' }));
  });
  return svg;
}

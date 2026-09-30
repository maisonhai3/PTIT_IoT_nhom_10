// Thẻ "Giàn phơi": minh họa, trạng thái, điều khiển.
import { h, s, setText } from '../dom.js';
import { icon } from '../icons.js';
import { stateTitle, modeLabel, rainReason, fmtCountdown } from '../format.js';

const RAY_ANGLES = [0, 45, 90, 135, 180, 225, 270, 315];

function buildScene() {
  const rays = RAY_ANGLES.map((a) => s('line', { class: 'sun-ray', x1: 0, y1: -18, x2: 0, y2: -23, transform: `rotate(${a})` }));
  const drops = [];
  for (let i = 0; i < 12; i++) {
    const x = 186 + i * 10;
    const y = 60 + ((i * 37) % 70);
    drops.push(s('line', { class: 'drop', x1: x, y1: y, x2: x - 3, y2: y + 9 }));
  }
  // Giàn nằm trong nhóm .rack (CSS trượt ngang); nhóm con hạ xuống 14 đơn vị để cách xa mặt trời và mái.
  const rack = s('g', { class: 'rack' }, s('g', { transform: 'translate(0 14)' },
    s('line', { class: 'rack-line', x1: 56, y1: 60, x2: 56, y2: 146 }),
    s('line', { class: 'rack-line', x1: 152, y1: 60, x2: 152, y2: 146 }),
    s('line', { class: 'rack-line', x1: 50, y1: 60, x2: 158, y2: 60 }),
    s('path', { class: 'cloth-a', d: 'M64 64L74 61Q80 68 86 61L96 64L102 74L95 79L91 75V102H69V75L65 79L58 74Z' }),
    s('rect', { class: 'cloth-b', x: 108, y: 62, width: 16, height: 46, rx: 2 }),
    s('path', { class: 'cloth-c', d: 'M130 62h20v12l-3 24h-6l-1-14-1 14h-6l-3-24z' })));

  return s('svg', { class: 'scene', viewBox: '0 0 320 190', 'aria-hidden': 'true', focusable: 'false' },
    s('rect', { class: 'sky', x: 0, y: 0, width: 320, height: 160 }),
    s('rect', { class: 'ground', x: 0, y: 160, width: 320, height: 30 }),
    s('g', { class: 'sun', transform: 'translate(282 34)' }, s('circle', { class: 'sun-fill', r: 13 }), ...rays),
    s('g', { class: 'cloud', transform: 'translate(226 30)' },
      s('circle', { class: 'cloud-fill', cx: 14, cy: 14, r: 13 }),
      s('circle', { class: 'cloud-fill', cx: 34, cy: 9, r: 17 }),
      s('circle', { class: 'cloud-fill', cx: 54, cy: 15, r: 12 }),
      s('rect', { class: 'cloud-fill', x: 2, y: 14, width: 64, height: 14, rx: 7 })),
    s('g', { class: 'rain' }, ...drops),
    s('rect', { class: 'wall', x: 0, y: 20, width: 44, height: 140 }),
    s('rect', { class: 'shelter', x: 44, y: 26, width: 124, height: 134 }),
    s('rect', { class: 'roof', x: 0, y: 14, width: 168, height: 12, rx: 3 }),
    s('line', { class: 'post', x1: 164, y1: 26, x2: 164, y2: 160 }),
    rack,
    s('g', { class: 'err', transform: 'translate(186 100)' },
      s('circle', { class: 'err-badge', r: 14 }),
      s('line', { class: 'err-mark', x1: 0, y1: -6, x2: 0, y2: 2 }),
      s('line', { class: 'err-mark', x1: 0, y1: 7, x2: 0, y2: 7.5 })));
}

const CONTROLS = [
  { action: 'open', label: 'Mở giàn', icon: 'open' },
  { action: 'close', label: 'Thu giàn', icon: 'close' },
  { action: 'auto', label: 'Tự động', icon: 'auto' },
];

/**
 * @param {HTMLElement} root
 * @param {{store: object, sendCommand: (a: string) => Promise<void>, serverNow: () => number}} ctx
 */
export function mountAwning(root, { store, sendCommand }) {
  const scene = buildScene();
  const title = h('p', { class: 'state-title', 'aria-live': 'polite' });
  const reasonText = h('span');
  const manualLeft = h('span');
  const reason = h('p', { class: 'state-reason' }, reasonText, manualLeft);
  const modeChip = h('span', { class: 'chip', 'data-tone': 'muted' });
  const hint = h('p', { class: 'hint' });

  const buttons = {};
  const controls = h('div', { class: 'controls', role: 'group', 'aria-label': 'Điều khiển giàn' });
  for (const c of CONTROLS) {
    const b = h('button', { class: 'btn', type: 'button', 'aria-pressed': 'false', 'data-action': c.action, onclick: () => sendCommand(c.action) },
      icon(c.icon), h('span', {}, c.label));
    buttons[c.action] = b;
    controls.append(b);
  }

  const simBtn = h('button', { type: 'button', role: 'switch', 'aria-checked': 'false', 'aria-labelledby': 'sim-label', id: 'sim-switch' });
  const simLabel = h('span', { id: 'sim-label' }, 'Giả lập mưa');
  const simToggle = h('label', { class: 'switch', for: 'sim-switch' }, simBtn, simLabel);
  simBtn.addEventListener('click', () => sendCommand(simBtn.getAttribute('aria-checked') === 'true' ? 'clear_rain' : 'simulate_rain'));

  root.append(
    h('div', { class: 'card-head' }, h('h2', { id: 'awning-h' }, 'Giàn phơi'), modeChip),
    h('div', { class: 'scene-wrap' }, scene),
    h('div', { class: 'state-block' }, title, reason),
    controls,
    h('div', { class: 'controls-foot' }, simToggle, hint),
  );

  let lastTelemetry = null;
  let lastReceivedAt = 0;

  function updateCountdown() {
    const t = lastTelemetry;
    if (t && t.mode === 'MANUAL' && t.manual_left_s > 0) {
      const left = t.manual_left_s - (performance.now() - lastReceivedAt) / 1000;
      setText(manualLeft, ` · tự về Tự động sau ${fmtCountdown(left)}`);
    } else {
      setText(manualLeft, '');
    }
  }

  function render(st) {
    const d = st.device;
    const t = d?.telemetry ?? null;
    const online = Boolean(d?.online);
    lastTelemetry = t;
    lastReceivedAt = st.deviceReceivedAt ?? performance.now();

    const name = t?.state;
    scene.classList.toggle('is-open', name === 'OPEN' || name === 'OPENING' || !t);
    scene.classList.toggle('is-error', name === 'ERROR');
    scene.classList.toggle('is-moving', name === 'OPENING' || name === 'CLOSING');
    const raining = Boolean(t && (t.rain || t.rain_source === 'sim')) || Boolean(st.weather?.is_raining);
    const cloudy = st.weather?.rain_expected_15m || st.weather?.weather_code === 3 || st.weather?.weather_code === 45;
    scene.classList.toggle('wx-rainy', raining);
    scene.classList.toggle('wx-cloudy', !raining && Boolean(cloudy));
    scene.classList.toggle('wx-sunny', !raining && !cloudy);

    setText(title, t ? stateTitle(t.state) : d ? 'Chưa có dữ liệu từ thiết bị' : 'Đang tải…');
    const mode = t ? modeLabel(t.mode) : '';
    const why = t ? rainReason(t) : '';
    setText(reasonText, t ? [mode, why].filter(Boolean).join(' · ') : '');
    setText(modeChip, mode || '—');
    modeChip.dataset.tone = t?.mode === 'MANUAL' ? 'warn' : 'muted';
    updateCountdown();

    const pending = st.pending?.action ?? null;
    const isOpenSide = name === 'OPEN' || name === 'OPENING';
    const isClosedSide = name === 'CLOSED' || name === 'CLOSING';
    const pressed = {
      open: t?.mode === 'MANUAL' && isOpenSide,
      close: t?.mode === 'MANUAL' && isClosedSide,
      auto: t?.mode === 'AUTO',
    };
    for (const [action, b] of Object.entries(buttons)) {
      b.setAttribute('aria-pressed', String(Boolean(pressed[action])));
      b.disabled = !online || pending !== null;
      if (pending === action) b.setAttribute('aria-busy', 'true');
      else b.removeAttribute('aria-busy');
    }
    const simOn = t?.rain_source === 'sim';
    simBtn.setAttribute('aria-checked', String(simOn));
    simBtn.disabled = !online || pending !== null;

    setText(hint, !online
      ? 'Thiết bị đang offline nên chưa gửi được lệnh.'
      : 'Ở chế độ Tự động, giàn tự thu khi có mưa và tự mở lại khi hết mưa.');
  }

  store.subscribe(render);
  setInterval(updateCountdown, 1000);
}

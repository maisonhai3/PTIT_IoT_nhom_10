// Thẻ "Nhật ký": sự kiện gần đây, mới nhất ở trên.
import { h, clear } from '../dom.js';
import { icon } from '../icons.js';
import { eventText, fmtTime } from '../format.js';

const ICONS = { state: 'auto', mode: 'auto', command: 'cmd', rain: 'rain', weather_error: 'warn' };

function iconFor(e) {
  if (e.kind === 'online') return e.detail === 'true' ? 'check' : 'offline';
  return ICONS[e.kind] ?? 'info';
}

function tone(e) {
  if (e.kind === 'online' && e.detail === 'false') return 'warn';
  if (e.kind === 'state' && e.detail === 'ERROR') return 'bad';
  if (e.kind === 'weather_error') return 'warn';
  return '';
}

export function mountEvents(root, { store }) {
  const list = h('ul', { class: 'events' });
  // Vùng cuộn phải focus được thì người dùng bàn phím mới cuộn xem hết nhật ký.
  const scroller = h('div', { class: 'events-scroll', role: 'region', tabindex: '0', 'aria-label': 'Danh sách sự kiện' }, list);
  root.append(h('div', { class: 'card-head' }, h('h2', { id: 'events-h' }, 'Nhật ký hoạt động')), scroller);
  let shown = null;

  store.subscribe((st) => {
    if (st.events === shown) return;
    shown = st.events;
    clear(list);
    if (st.events.length === 0) {
      list.append(h('li', { class: 'empty' }, 'Chưa có sự kiện nào.'));
      return;
    }
    for (const e of st.events) {
      list.append(h('li', {},
        h('span', { class: 'ev-time' }, fmtTime(e.ts, { seconds: true })),
        h('span', { class: 'ev-icon', 'data-tone': tone(e) }, icon(iconFor(e))),
        h('span', { class: 'ev-text' }, eventText(e))));
    }
  });
}

// Thẻ "Thời tiết": điều kiện hiện tại, dự báo ngắn, độ tươi của dữ liệu.
import { h, setText, clear } from '../dom.js';
import { icon } from '../icons.js';
import { fmtNumber, fmtTime, timeAgo, weatherIconName } from '../format.js';

const roundTo10 = (p) => Math.max(0, Math.min(100, Math.round((p ?? 0) / 10) * 10));

export function mountWeather(root, { store, serverNow }) {
  const body = h('div');
  const location = h('span', { class: 'chip', 'data-tone': 'muted' });
  root.append(h('div', { class: 'card-head' }, h('h2', { id: 'weather-h' }, 'Thời tiết hiện tại'), location), body);

  let ageEl = null;
  let fetchedAt = null;
  let shownKey = null;

  function updateAge() {
    if (!ageEl || !fetchedAt) return;
    setText(ageEl, `Cập nhật ${fmtTime(fetchedAt)} (${timeAgo(serverNow() - Date.parse(fetchedAt))})`);
  }

  function yesNo(v) {
    return h('span', { class: 'v' }, v ? 'Có' : 'Không');
  }

  function render(st) {
    const w = st.weather;
    const key = w ? w.fetched_at : st.loaded ? 'none' : 'loading';
    if (key === shownKey) return;
    shownKey = key;
    clear(body);
    ageEl = null;
    fetchedAt = null;

    if (!w) {
      setText(location, '—');
      body.append(h('p', { class: 'empty' }, st.loaded
        ? 'Máy chủ chưa lấy được thời tiết từ Open-Meteo. Kiểm tra kết nối Internet của máy chủ.'
        : 'Đang tải…'));
      return;
    }
    setText(location, w.location || '—');
    fetchedAt = w.fetched_at;
    const iconName = weatherIconName(w.weather_code);
    ageEl = h('p', { class: 'wx-meta' });

    body.append(
      h('div', { class: 'wx-main' },
        icon(iconName, { class: `wx-icon${iconName === 'sun' || iconName === 'partly' ? ' is-sun' : ''}` }),
        h('div', {},
          h('div', { class: 'wx-temp' }, `${fmtNumber(w.temperature_c)}°`),
          h('div', { class: 'wx-desc' }, w.description))),
      ageEl,
      h('ul', { class: 'kv' },
        h('li', {}, h('span', { class: 'k' }, 'Độ ẩm'), h('span', { class: 'v' }, `${fmtNumber(w.humidity, 0)}%`)),
        h('li', {}, h('span', { class: 'k' }, 'Lượng mưa'), h('span', { class: 'v' }, `${fmtNumber(w.precipitation_mm)} mm`)),
        h('li', {}, h('span', { class: 'k' }, 'Đang mưa'), yesNo(w.is_raining)),
        h('li', {}, h('span', { class: 'k' }, 'Sắp mưa (15 phút tới)'), yesNo(w.rain_expected_15m))),
      w.forecast.length
        ? h('div', { class: 'forecast', role: 'list', 'aria-label': 'Dự báo theo giờ' },
          w.forecast.map((f) => h('div', { class: 'fc', role: 'listitem' },
            h('div', { class: 'fc-time' }, fmtTime(f.time)),
            h('div', { class: 'fc-bar', 'aria-hidden': 'true' }, h('div', { class: 'fc-fill', 'data-p': roundTo10(f.precipitation_probability) })),
            h('div', { class: 'fc-val' }, f.precipitation_probability === null ? '—' : `${f.precipitation_probability}%`),
            h('div', { class: 'fc-mm' }, `${fmtNumber(f.precipitation_mm)} mm`))))
        : null,
    );
    updateAge();
  }

  store.subscribe(render);
  setInterval(updateAge, 15000);
}

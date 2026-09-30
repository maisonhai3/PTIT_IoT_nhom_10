// Huy hiệu trạng thái trên đầu trang và các dải cảnh báo.
import { h, clear } from '../dom.js';
import { icon } from '../icons.js';

function setChip(el, tone, iconName, text) {
  clear(el);
  el.dataset.tone = tone;
  el.append(icon(iconName), text);
}

function banner(tone, iconName, ...content) {
  return h('div', { class: 'banner', 'data-tone': tone }, icon(iconName), h('div', {}, ...content));
}

export function mountStatus({ store }) {
  const serverPill = document.getElementById('pill-server');
  const devicePill = document.getElementById('pill-device');
  const banners = document.getElementById('banners');
  let key = null;

  function render(st) {
    // Huy hiệu
    if (st.ws === 'open') setChip(serverPill, 'good', 'check', 'Máy chủ: đã kết nối');
    else if (st.ws === 'closed') setChip(serverPill, 'bad', 'offline', 'Máy chủ: mất kết nối');
    else setChip(serverPill, 'muted', 'refresh', 'Máy chủ: đang kết nối…');

    const d = st.device;
    if (!d) setChip(devicePill, 'muted', 'clock', 'Thiết bị: chưa rõ');
    else if (d.online) setChip(devicePill, 'good', 'check', 'Thiết bị: online');
    else setChip(devicePill, 'warn', 'warn', 'Thiết bị: offline');

    // Dải cảnh báo: chỉ dựng lại khi nội dung thật sự đổi để không làm vùng aria-live đọc lặp.
    const t = d?.telemetry;
    const items = [];
    if (st.ws === 'closed') items.push(['muted', 'offline', 'Mất kết nối tới máy chủ. ', 'Đang thử kết nối lại, số liệu bên dưới có thể đã cũ.']);
    if (d && !d.online) {
      items.push(['warn', 'warn', 'Thiết bị (ESP32) đang offline. ', 'Số liệu bên dưới là lần nhận cuối và chưa thể gửi lệnh.']);
    }
    if (t?.fail_safe && d?.online) {
      items.push(['warn', 'warn', 'Thiết bị không có dữ liệu thời tiết mới ', '(chưa nhận được hoặc đã cũ hơn 30 phút). Nó đang dựa vào cảm biến tại chỗ (độ ẩm và ánh sáng) để quyết định.']);
    }
    if (t?.state === 'ERROR') {
      items.push(['bad', 'warn', 'Giàn báo lỗi: ', 'không chạm công tắc hành trình trong thời gian cho phép. Kiểm tra cơ cấu rồi bấm Mở hoặc Thu để thử lại.']);
    }
    const nextKey = JSON.stringify(items);
    if (nextKey === key) return;
    key = nextKey;
    clear(banners);
    for (const [tone, ic, strong, rest] of items) {
      banners.append(banner(tone, ic, h('strong', {}, strong), rest));
    }
  }

  store.subscribe(render);
}

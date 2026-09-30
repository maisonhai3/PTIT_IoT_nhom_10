// Thông báo nhanh ở cuối màn hình (vùng aria-live nên trình đọc màn hình cũng nghe được).
import { h } from '../dom.js';

const MAX = 3;

export function toast(text, tone = 'info', ms = 5000) {
  const box = document.getElementById('toasts');
  if (!box) return;
  const el = h('div', { class: 'toast', 'data-tone': tone }, text);
  box.append(el);
  while (box.children.length > MAX) box.firstElementChild.remove();
  setTimeout(() => el.remove(), ms);
}

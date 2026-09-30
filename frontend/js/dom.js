// Tạo phần tử DOM an toàn: chuỗi luôn thành text node (không bao giờ là HTML), nên dữ liệu từ máy chủ hay thiết bị
// không thể chèn mã vào trang.

const SVG_NS = 'http://www.w3.org/2000/svg';

function apply(el, attrs) {
  for (const [key, value] of Object.entries(attrs ?? {})) {
    if (value === null || value === undefined || value === false) continue;
    if (key.startsWith('on') && typeof value === 'function') el.addEventListener(key.slice(2).toLowerCase(), value);
    else if (key === 'text') el.textContent = value;
    else el.setAttribute(key, value === true ? '' : String(value));
  }
}

function append(el, children) {
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
}

export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  apply(el, attrs);
  append(el, children);
  return el;
}

export function s(tag, attrs, ...children) {
  const el = document.createElementNS(SVG_NS, tag);
  apply(el, attrs);
  append(el, children);
  return el;
}

/** Đổi textContent chỉ khi khác, để vùng aria-live không đọc lại cùng một nội dung. */
export function setText(el, text) {
  if (el.textContent !== text) el.textContent = text;
}

export function clear(el) {
  el.replaceChildren();
}

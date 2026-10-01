// Store tối giản (pub/sub) và các hàm thuần để áp dụng tin nhắn WebSocket vào state.

export function createStore(initial) {
  let state = initial;
  const subs = new Set();
  return {
    get: () => state,
    set(patch) {
      state = { ...state, ...(typeof patch === 'function' ? patch(state) : patch) };
      subs.forEach((fn) => fn(state));
    },
    subscribe(fn) {
      subs.add(fn);
      fn(state);
      return () => subs.delete(fn);
    },
  };
}

export const MAX_EVENTS = 50;

const eventKey = (e) => `${e?.ts}|${e?.kind}|${e?.detail}`;
const eventTime = (e) => Date.parse(e?.ts) || 0;

/**
 * Gộp hai danh sách sự kiện thành nhật ký: bỏ trùng, mới nhất trước, tối đa MAX_EVENTS.
 * Có hai đường đưa sự kiện vào trang (REST và WebSocket) và không đường nào được ghi đè đường kia: bản chụp REST có thể
 * được tính từ trước một sự kiện mà WebSocket vừa đẩy tới, và WebSocket cũng có thể đẩy một sự kiện mà bản chụp đã
 * chứa. Sự kiện không có id nên định danh là (ts, kind, detail). Khi hai sự kiện cùng mili giây, `incoming` nằm trên.
 */
export function mergeEvents(incoming, existing) {
  const seen = new Set();
  const all = [];
  for (const e of [...incoming, ...existing]) {
    const key = eventKey(e);
    if (!seen.has(key)) {
      seen.add(key);
      all.push(e);
    }
  }
  return all.sort((a, b) => eventTime(b) - eventTime(a)).slice(0, MAX_EVENTS);
}

/** Áp một tin nhắn `{type, data}` từ WebSocket vào state; trả về phần cần cập nhật (hoặc {} nếu bỏ qua). */
export function applyMessage(state, msg, receivedAt) {
  if (!msg || typeof msg !== 'object') return {};
  switch (msg.type) {
    case 'state':
      return { device: msg.data, deviceReceivedAt: receivedAt };
    case 'weather':
      return { weather: msg.data };
    case 'event':
      return { events: mergeEvents([msg.data], state.events) };
    default:
      return {};
  }
}

/**
 * Lệnh đã "có hiệu lực" chưa, xét theo telemetry mới nhất? Dùng để tắt trạng thái chờ của nút bấm.
 * open/close chỉ cần thiết bị bắt đầu chạy đúng hướng (không phải chờ chạy xong).
 */
export function commandTookEffect(action, t) {
  if (!t) return false;
  switch (action) {
    case 'open': return t.mode === 'MANUAL' && (t.state === 'OPENING' || t.state === 'OPEN');
    case 'close': return t.mode === 'MANUAL' && (t.state === 'CLOSING' || t.state === 'CLOSED');
    case 'auto': return t.mode === 'AUTO';
    case 'simulate_rain': return t.rain_source === 'sim';
    case 'clear_rain': return t.rain_source !== 'sim';
    default: return false;
  }
}

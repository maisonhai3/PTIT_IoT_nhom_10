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

/** Áp một tin nhắn `{type, data}` từ WebSocket vào state; trả về phần cần cập nhật (hoặc {} nếu bỏ qua). */
export function applyMessage(state, msg, receivedAt) {
  if (!msg || typeof msg !== 'object') return {};
  switch (msg.type) {
    case 'state':
      return { device: msg.data, deviceReceivedAt: receivedAt };
    case 'weather':
      return { weather: msg.data };
    case 'event':
      return { events: [msg.data, ...state.events].slice(0, MAX_EVENTS) };
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

// Client REST + WebSocket. Không phụ thuộc DOM; fetch/WebSocket/timer được tiêm vào để test.

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status; // 0 = không kết nối được
  }
}

/**
 * @param {object} o
 * @param {string} [o.base]      Gốc của API ('' = cùng origin). Có thể là http://192.168.1.10:8080 khi dev front-end riêng.
 * @param {() => string} [o.getToken]
 */
export function createApi({ base = '', fetchFn = (...a) => fetch(...a), getToken = () => '', timeoutMs = 8000, now = () => Date.now() } = {}) {
  let clockOffsetMs = 0; // (giờ máy chủ) - (giờ trình duyệt), ước lượng từ header Date

  async function request(path, { method = 'GET', body } = {}) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeoutMs);
    const headers = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
    let res;
    try {
      res = await fetchFn(base + path, {
        method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal: ctrl.signal, cache: 'no-store',
      });
    } catch (err) {
      throw new ApiError(0, err?.name === 'AbortError' ? 'timeout' : 'network');
    } finally {
      clearTimeout(timer);
    }
    const date = res.headers?.get?.('Date');
    if (date) {
      const server = Date.parse(date);
      if (Number.isFinite(server)) clockOffsetMs = server - now();
    }
    let data = null;
    try {
      data = await res.json();
    } catch { /* thân rỗng hoặc không phải JSON */ }
    if (!res.ok) throw new ApiError(res.status, data?.error ?? `HTTP ${res.status}`);
    return data;
  }

  return {
    getState: () => request('/api/state'),
    /** null nếu máy chủ chưa lấy được thời tiết lần nào (503). */
    async getWeather() {
      try {
        return await request('/api/weather');
      } catch (err) {
        if (err instanceof ApiError && err.status === 503) return null;
        throw err;
      }
    },
    getHistory: (hours) => request(`/api/history?hours=${encodeURIComponent(hours)}`),
    getEvents: (limit = 50) => request(`/api/events?limit=${encodeURIComponent(limit)}`),
    sendCommand: (action) => request('/api/command', { method: 'POST', body: { action } }),
    /** Giờ máy chủ hiện tại (ms), để tính "x phút trước" đúng dù đồng hồ trình duyệt lệch. */
    serverNow: () => now() + clockOffsetMs,
    clockOffsetMs: () => clockOffsetMs,
  };
}

/** http://host:8080 -> ws://host:8080/ws ; '' -> cùng origin. */
export function wsUrl(base, loc = globalThis.location) {
  if (base) return `${base.replace(/^http/, 'ws').replace(/\/$/, '')}/ws`;
  return `${loc.protocol === 'https:' ? 'wss:' : 'ws:'}//${loc.host}/ws`;
}

/**
 * WebSocket tự kết nối lại (backoff 1s, 2s, 4s ... tối đa 15s, có jitter).
 * onStatus nhận 'connecting' | 'open' | 'closed'.
 */
export function connectEvents({
  url, onMessage, onStatus,
  WebSocketImpl = globalThis.WebSocket, setTimeoutFn = (f, ms) => setTimeout(f, ms), clearTimeoutFn = (t) => clearTimeout(t),
  random = Math.random,
}) {
  let ws = null;
  let attempt = 0;
  let stopped = false;
  let timer = null;

  function open() {
    if (stopped) return;
    onStatus('connecting');
    ws = new WebSocketImpl(url);
    ws.onopen = () => {
      attempt = 0;
      onStatus('open');
    };
    ws.onmessage = (e) => {
      let msg;
      try { msg = JSON.parse(e.data); } catch { return; }
      onMessage(msg);
    };
    ws.onerror = () => {}; // onclose luôn đến sau và lo việc kết nối lại
    ws.onclose = () => {
      onStatus('closed');
      if (stopped) return;
      const delay = Math.min(15000, 1000 * 2 ** attempt) * (0.75 + random() * 0.5);
      attempt += 1;
      timer = setTimeoutFn(open, delay);
    };
  }
  open();
  return {
    close() {
      stopped = true;
      if (timer !== null) clearTimeoutFn(timer);
      ws?.close();
    },
  };
}

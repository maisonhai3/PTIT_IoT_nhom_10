import test from 'node:test';
import assert from 'node:assert/strict';
import { createApi, ApiError, wsUrl, connectEvents } from '../js/api.js';

function fakeFetch(handler) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, init });
    const r = await handler(url, init);
    if (r instanceof Error) throw r;
    const headers = new Map(Object.entries(r.headers ?? {}));
    return {
      ok: r.status >= 200 && r.status < 300,
      status: r.status,
      headers: { get: (k) => headers.get(k) ?? null },
      json: async () => { if (r.body === undefined) throw new Error('no body'); return r.body; },
    };
  };
  fn.calls = calls;
  return fn;
}

test('GET đúng URL, không gửi Content-Type/Authorization khi không cần', async () => {
  const fetchFn = fakeFetch(() => ({ status: 200, body: { online: true } }));
  const api = createApi({ fetchFn });
  assert.deepEqual(await api.getState(), { online: true });
  const { url, init } = fetchFn.calls[0];
  assert.equal(url, '/api/state');
  assert.equal(init.method, 'GET');
  assert.deepEqual(init.headers, {});
  assert.equal(init.cache, 'no-store');
});

test('base tuyệt đối và tham số truy vấn được mã hóa', async () => {
  const fetchFn = fakeFetch(() => ({ status: 200, body: [] }));
  const api = createApi({ base: 'http://192.168.1.10:8080', fetchFn });
  await api.getHistory(6);
  await api.getEvents(20);
  assert.equal(fetchFn.calls[0].url, 'http://192.168.1.10:8080/api/history?hours=6');
  assert.equal(fetchFn.calls[1].url, 'http://192.168.1.10:8080/api/events?limit=20');
});

test('sendCommand: JSON, Content-Type và Bearer token', async () => {
  const fetchFn = fakeFetch(() => ({ status: 202, body: { status: 'sent' } }));
  const api = createApi({ fetchFn, getToken: () => 's3cret' });
  assert.deepEqual(await api.sendCommand('close'), { status: 'sent' });
  const { url, init } = fetchFn.calls[0];
  assert.equal(url, '/api/command');
  assert.equal(init.method, 'POST');
  assert.equal(init.headers['Content-Type'], 'application/json');
  assert.equal(init.headers.Authorization, 'Bearer s3cret');
  assert.equal(init.body, '{"action":"close"}');
});

test('lỗi HTTP thành ApiError có status và thông điệp của máy chủ', async () => {
  const api = createApi({ fetchFn: fakeFetch(() => ({ status: 409, body: { error: 'device is offline' } })) });
  await assert.rejects(api.sendCommand('open'), (e) => e instanceof ApiError && e.status === 409 && e.message === 'device is offline');
  const noBody = createApi({ fetchFn: fakeFetch(() => ({ status: 502 })) });
  await assert.rejects(noBody.getState(), (e) => e.status === 502 && e.message === 'HTTP 502');
});

test('lỗi mạng và quá hạn thành ApiError status 0', async () => {
  const down = createApi({ fetchFn: fakeFetch(() => new TypeError('fetch failed')) });
  await assert.rejects(down.getState(), (e) => e instanceof ApiError && e.status === 0 && e.message === 'network');

  const hang = createApi({
    timeoutMs: 20,
    fetchFn: (url, init) => new Promise((_, reject) => init.signal.addEventListener('abort', () => reject(Object.assign(new Error('x'), { name: 'AbortError' })))),
  });
  await assert.rejects(hang.getState(), (e) => e.status === 0 && e.message === 'timeout');
});

test('getWeather: 503 nghĩa là chưa có dữ liệu, không phải lỗi', async () => {
  const none = createApi({ fetchFn: fakeFetch(() => ({ status: 503, body: { error: 'weather not available yet' } })) });
  assert.equal(await none.getWeather(), null);
  const boom = createApi({ fetchFn: fakeFetch(() => ({ status: 500, body: { error: 'x' } })) });
  await assert.rejects(boom.getWeather(), (e) => e.status === 500);
});

test('ước lượng lệch đồng hồ từ header Date', async () => {
  const clientNow = Date.parse('2026-09-30T12:00:00Z');
  const api = createApi({
    now: () => clientNow,
    fetchFn: fakeFetch(() => ({ status: 200, body: {}, headers: { Date: 'Wed, 30 Sep 2026 12:10:00 GMT' } })),
  });
  assert.equal(api.clockOffsetMs(), 0);
  await api.getState();
  assert.equal(api.clockOffsetMs(), 10 * 60 * 1000);
  assert.equal(api.serverNow(), clientNow + 10 * 60 * 1000);
});

test('wsUrl', () => {
  assert.equal(wsUrl('', { protocol: 'http:', host: 'pi.local:8080' }), 'ws://pi.local:8080/ws');
  assert.equal(wsUrl('', { protocol: 'https:', host: 'x.test' }), 'wss://x.test/ws');
  assert.equal(wsUrl('http://192.168.1.10:8080/', {}), 'ws://192.168.1.10:8080/ws');
  assert.equal(wsUrl('https://a.test', {}), 'wss://a.test/ws');
});

class FakeSocket {
  static instances = [];
  constructor(url) { this.url = url; this.closed = false; FakeSocket.instances.push(this); }
  close() { this.closed = true; }
}

function wsHarness() {
  FakeSocket.instances = [];
  const timers = [];
  const statuses = [];
  const messages = [];
  const h = connectEvents({
    url: 'ws://x/ws',
    onMessage: (m) => messages.push(m),
    onStatus: (s) => statuses.push(s),
    WebSocketImpl: FakeSocket,
    setTimeoutFn: (fn, ms) => { timers.push({ fn, ms }); return timers.length; },
    clearTimeoutFn: (id) => { timers[id - 1].cleared = true; },
    random: () => 0.5, // jitter = 1.0
  });
  return { h, timers, statuses, messages, sock: () => FakeSocket.instances.at(-1) };
}

test('WebSocket: kết nối, nhận tin, bỏ qua tin hỏng', () => {
  const t = wsHarness();
  assert.deepEqual(t.statuses, ['connecting']);
  t.sock().onopen();
  t.sock().onmessage({ data: '{"type":"state","data":1}' });
  t.sock().onmessage({ data: 'không phải json' });
  assert.deepEqual(t.statuses, ['connecting', 'open']);
  assert.deepEqual(t.messages, [{ type: 'state', data: 1 }]);
});

test('WebSocket: tự kết nối lại với backoff 1s,2s,4s… tối đa 15s và reset sau khi thành công', () => {
  const t = wsHarness();
  const delays = [];
  for (let i = 0; i < 7; i++) {
    t.sock().onclose();
    delays.push(t.timers.at(-1).ms);
    t.timers.at(-1).fn(); // hẹn giờ kích hoạt: mở socket mới
  }
  assert.deepEqual(delays, [1000, 2000, 4000, 8000, 15000, 15000, 15000]);
  t.sock().onopen(); // thành công thì reset
  t.sock().onclose();
  assert.equal(t.timers.at(-1).ms, 1000);
  assert.equal(FakeSocket.instances.length, 8);
});

test('WebSocket: close() dừng hẳn, hủy hẹn giờ và không kết nối lại', () => {
  const t = wsHarness();
  t.sock().onclose();
  assert.equal(t.timers.length, 1);
  t.h.close();
  assert.equal(t.timers[0].cleared, true);
  t.timers[0].fn(); // dù hẹn giờ vẫn chạy, cũng không được mở socket mới
  assert.equal(FakeSocket.instances.length, 1);
  const before = t.statuses.length;
  t.sock().onclose();
  assert.equal(t.timers.length, 1, 'không hẹn thêm sau khi đã close()');
  assert.ok(t.statuses.length >= before);
});

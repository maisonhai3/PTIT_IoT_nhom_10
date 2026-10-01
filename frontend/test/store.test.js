import test from 'node:test';
import assert from 'node:assert/strict';
import { createStore, applyMessage, commandTookEffect, mergeEvents, MAX_EVENTS } from '../js/store.js';

// Sự kiện thật luôn có ts/kind/detail; ts cách nhau theo giây để dễ đọc.
const ev = (sec, kind = 'state', detail = 'OPEN') => ({ ts: `2026-10-01T10:${String(Math.floor(sec / 60)).padStart(2, '0')}:${String(sec % 60).padStart(2, '0')}.000Z`, kind, detail });

test('store: subscribe gọi ngay với state hiện tại, set gộp patch, unsubscribe dừng gọi', () => {
  const s = createStore({ a: 1, b: 1 });
  const seen = [];
  const off = s.subscribe((st) => seen.push({ ...st }));
  s.set({ b: 2 });
  s.set((st) => ({ a: st.a + 10 }));
  assert.deepEqual(seen, [{ a: 1, b: 1 }, { a: 1, b: 2 }, { a: 11, b: 2 }]);
  off();
  s.set({ a: 0 });
  assert.equal(seen.length, 3);
  assert.deepEqual(s.get(), { a: 0, b: 2 });
});

test('applyMessage', () => {
  const base = { events: [ev(1, 'old')] };
  assert.deepEqual(applyMessage(base, { type: 'state', data: { online: true } }, 123), { device: { online: true }, deviceReceivedAt: 123 });
  assert.deepEqual(applyMessage(base, { type: 'weather', data: { t: 1 } }, 1), { weather: { t: 1 } });
  assert.deepEqual(applyMessage(base, { type: 'event', data: ev(2, 'new') }, 1).events.map((e) => e.kind), ['new', 'old']);
  for (const junk of [null, undefined, 5, 'x', {}, { type: 'lạ' }]) assert.deepEqual(applyMessage(base, junk, 1), {});
});

test('applyMessage: nhật ký bị giới hạn', () => {
  let st = { events: [] };
  for (let i = 0; i < MAX_EVENTS + 20; i++) st = { events: applyMessage(st, { type: 'event', data: { ...ev(i), n: i } }, 0).events };
  assert.equal(st.events.length, MAX_EVENTS);
  assert.equal(st.events[0].n, MAX_EVENTS + 19, 'mới nhất ở đầu');
});

test('applyMessage: sự kiện đến hai lần (bản chụp REST đã chứa nó, rồi WebSocket đẩy nó) chỉ hiện một lần', () => {
  const st = { events: [ev(5, 'rain', 'sensor'), ev(1, 'online', 'true')] };
  assert.deepEqual(applyMessage(st, { type: 'event', data: ev(5, 'rain', 'sensor') }, 0).events, st.events);
});

test('mergeEvents: bản chụp REST cũ hơn không được xoá sự kiện mới đã đến qua WebSocket', () => {
  // Tình huống thật: trang vừa nối WebSocket nên gọi REST; trong lúc chờ, thiết bị báo "có mưa" và WebSocket đẩy nó tới.
  // REST trả về bản chụp tính từ trước sự kiện đó. Ghi đè thẳng thì sự kiện biến mất và không bao giờ quay lại.
  const pushedByWebSocket = [ev(5, 'rain', 'sensor'), ev(1, 'online', 'true')];
  const olderSnapshot = [ev(1, 'online', 'true')];
  assert.deepEqual(mergeEvents(olderSnapshot, pushedByWebSocket).map((e) => e.kind), ['rain', 'online']);
});

test('mergeEvents: bỏ trùng, mới nhất trước, cùng mili giây thì bản đến sau nằm trên', () => {
  const merged = mergeEvents([ev(9, 'state', 'CLOSED'), ev(3, 'rain', 'api')], [ev(9, 'state', 'CLOSED'), ev(6, 'state', 'CLOSING'), ev(3, 'rain', 'api')]);
  assert.deepEqual(merged.map((e) => `${e.kind}:${e.detail}`), ['state:CLOSED', 'state:CLOSING', 'rain:api']);
  const sameMs = mergeEvents([ev(4, 'state', 'CLOSING')], [ev(4, 'rain', 'sensor')]);
  assert.deepEqual(sameMs.map((e) => e.kind), ['state', 'rain'], 'danh sách đến sau (incoming) đứng trước khi trùng thời điểm');
  assert.deepEqual(mergeEvents([], []), []);
});

test('mergeEvents: giữ MAX_EVENTS cái mới nhất và không vỡ khi ts hỏng', () => {
  const many = Array.from({ length: MAX_EVENTS + 10 }, (_, i) => ev(i, 'state', `S${i}`));
  const merged = mergeEvents(many.slice(0, 30), many.slice(20));
  assert.equal(merged.length, MAX_EVENTS);
  assert.equal(merged[0].detail, `S${MAX_EVENTS + 9}`);
  assert.doesNotThrow(() => mergeEvents([{ ts: 'không phải ngày', kind: 'x', detail: 'y' }], [ev(1)]));
});

test('commandTookEffect', () => {
  const t = (state, mode, rain_source = 'none') => ({ state, mode, rain_source });
  assert.equal(commandTookEffect('open', null), false);
  assert.equal(commandTookEffect('open', t('OPENING', 'MANUAL')), true);
  assert.equal(commandTookEffect('open', t('OPEN', 'MANUAL')), true);
  assert.equal(commandTookEffect('open', t('OPEN', 'AUTO')), false, 'đang mở nhưng vẫn AUTO: lệnh chưa có hiệu lực');
  assert.equal(commandTookEffect('open', t('CLOSING', 'MANUAL')), false);
  assert.equal(commandTookEffect('close', t('CLOSING', 'MANUAL')), true);
  assert.equal(commandTookEffect('close', t('CLOSED', 'MANUAL')), true);
  assert.equal(commandTookEffect('close', t('CLOSED', 'AUTO')), false);
  assert.equal(commandTookEffect('auto', t('OPEN', 'AUTO')), true);
  assert.equal(commandTookEffect('auto', t('OPEN', 'MANUAL')), false);
  assert.equal(commandTookEffect('simulate_rain', t('OPEN', 'AUTO', 'sim')), true);
  assert.equal(commandTookEffect('simulate_rain', t('OPEN', 'AUTO', 'api')), false);
  assert.equal(commandTookEffect('clear_rain', t('OPEN', 'AUTO', 'none')), true);
  assert.equal(commandTookEffect('clear_rain', t('OPEN', 'AUTO', 'sim')), false);
  assert.equal(commandTookEffect('bay', t('OPEN', 'AUTO')), false);
});

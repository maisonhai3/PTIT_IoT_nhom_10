import test from 'node:test';
import assert from 'node:assert/strict';
import { createStore, applyMessage, commandTookEffect, MAX_EVENTS } from '../js/store.js';

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
  const base = { events: [{ kind: 'old' }] };
  assert.deepEqual(applyMessage(base, { type: 'state', data: { online: true } }, 123), { device: { online: true }, deviceReceivedAt: 123 });
  assert.deepEqual(applyMessage(base, { type: 'weather', data: { t: 1 } }, 1), { weather: { t: 1 } });
  assert.deepEqual(applyMessage(base, { type: 'event', data: { kind: 'new' } }, 1).events.map((e) => e.kind), ['new', 'old']);
  for (const junk of [null, undefined, 5, 'x', {}, { type: 'lạ' }]) assert.deepEqual(applyMessage(base, junk, 1), {});
});

test('applyMessage: nhật ký bị giới hạn', () => {
  let st = { events: [] };
  for (let i = 0; i < MAX_EVENTS + 20; i++) st = { events: applyMessage(st, { type: 'event', data: { n: i } }, 0).events };
  assert.equal(st.events.length, MAX_EVENTS);
  assert.equal(st.events[0].n, MAX_EVENTS + 19, 'mới nhất ở đầu');
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

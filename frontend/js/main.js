// Điểm vào: nối store, API, WebSocket và các thẻ giao diện.
import { createStore, applyMessage, commandTookEffect } from './store.js';
import { createApi, ApiError, wsUrl, connectEvents } from './api.js';
import { commandErrorText } from './format.js';
import { mountAwning } from './ui/awning.js';
import { mountWeather } from './ui/weather.js';
import { mountSensors } from './ui/sensors.js';
import { mountHistory, RANGES } from './ui/history.js';
import { mountEvents } from './ui/events.js';
import { mountStatus } from './ui/status.js';
import { toast } from './ui/toast.js';

// localStorage có thể ném lỗi (chế độ riêng tư, bị chặn): luôn bọc và cho phép chạy không cần nó.
const ls = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch { /* bỏ qua */ } },
};

// ---- cấu hình: gốc API (mặc định cùng origin; ?api=http://host:8080 khi dev front-end trên máy khác) ----
const params = new URLSearchParams(location.search);
if (params.has('api')) ls.set('awning.api', params.get('api'));
let apiBase = (ls.get('awning.api') ?? '').trim().replace(/\/$/, '');
if (apiBase && !/^https?:\/\//.test(apiBase)) apiBase = '';
// Token gắn với từng gốc API để không bao giờ gửi mã của máy chủ này sang máy chủ khác.
const tokenKey = `awning.token@${apiBase || location.origin}`;

const api = createApi({ base: apiBase, getToken: () => ls.get(tokenKey) ?? '' });
const RANGE_KEY = 'awning.range';
const savedRange = Number(ls.get(RANGE_KEY));

const store = createStore({
  ws: 'connecting',
  loaded: false,
  device: null,
  deviceReceivedAt: 0,
  weather: null,
  events: [],
  history: [],
  historyRange: RANGES.some((r) => r.hours === savedRange) ? savedRange : 6,
  historyLoading: false,
  pending: null,
});

// ---- tải dữ liệu ----
async function loadState() {
  store.set({ device: await api.getState(), deviceReceivedAt: performance.now() });
}
async function loadWeather() {
  store.set({ weather: await api.getWeather() });
}
async function loadEvents() {
  store.set({ events: await api.getEvents(50) });
}
async function loadHistory() {
  const hours = store.get().historyRange;
  store.set({ historyLoading: true });
  try {
    const history = await api.getHistory(hours);
    if (store.get().historyRange === hours) store.set({ history, historyLoading: false }); // bỏ kết quả của khoảng đã đổi
  } catch {
    store.set({ historyLoading: false });
  }
}
async function loadAll() {
  await Promise.allSettled([loadState(), loadWeather(), loadEvents(), loadHistory()]);
  store.set({ loaded: true });
}

function setRange(hours) {
  ls.set(RANGE_KEY, String(hours));
  store.set({ historyRange: hours });
  loadHistory();
}

// ---- lệnh điều khiển ----
let seq = 0;

function askToken() {
  const dlg = document.getElementById('token-dialog');
  const form = document.getElementById('token-form');
  const input = document.getElementById('token-input');
  const cancel = document.getElementById('token-cancel');
  return new Promise((resolve) => {
    input.value = '';
    const done = (value) => {
      form.removeEventListener('submit', onSubmit);
      cancel.removeEventListener('click', onCancel);
      dlg.removeEventListener('close', onCancel);
      if (dlg.open) dlg.close();
      resolve(value);
    };
    const onSubmit = (e) => { e.preventDefault(); done(input.value.trim()); };
    const onCancel = () => done('');
    form.addEventListener('submit', onSubmit);
    cancel.addEventListener('click', onCancel);
    dlg.addEventListener('close', onCancel);
    dlg.showModal();
    input.focus();
  });
}

async function sendCommand(action, retried = false) {
  if (store.get().pending) return;
  const id = ++seq;
  store.set({ pending: { id, action } });
  try {
    await api.sendCommand(action);
  } catch (err) {
    store.set({ pending: null });
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 401 && !retried) {
      const token = await askToken();
      if (token) {
        ls.set(tokenKey, token);
        return sendCommand(action, true);
      }
      return undefined;
    }
    toast(commandErrorText(status), 'bad');
    return undefined;
  }
  // Máy chủ đã nhận lệnh; nút chỉ mở lại khi thiết bị phản hồi (hoặc sau 6 giây).
  if (commandTookEffect(action, store.get().device?.telemetry)) {
    store.set({ pending: null });
  } else {
    setTimeout(() => {
      if (store.get().pending?.id === id) {
        store.set({ pending: null });
        toast('Thiết bị chưa phản hồi lệnh. Kiểm tra kết nối rồi thử lại.', 'warn');
      }
    }, 6000);
  }
  return undefined;
}

// Khi telemetry cho thấy lệnh đã có hiệu lực thì bỏ trạng thái chờ.
store.subscribe((st) => {
  if (st.pending && commandTookEffect(st.pending.action, st.device?.telemetry)) store.set({ pending: null });
});

// ---- giao diện ----
const ctx = { store, serverNow: api.serverNow, sendCommand, setRange };
mountStatus(ctx);
mountAwning(document.getElementById('awning'), ctx);
mountWeather(document.getElementById('weather'), ctx);
mountSensors(document.getElementById('sensors'), ctx);
mountHistory(document.getElementById('history'), ctx);
mountEvents(document.getElementById('events'), ctx);

// Chủ đề sáng/tối: mặc định theo hệ điều hành, có thể ép bằng nút.
const THEME_LABEL = { auto: 'Tự động', light: 'Sáng', dark: 'Tối' };
const themeBtn = document.getElementById('theme-toggle');
let theme = ls.get('awning.theme') ?? 'auto';
if (!(theme in THEME_LABEL)) theme = 'auto';
function applyTheme() {
  if (theme === 'auto') delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
  themeBtn.textContent = `Giao diện: ${THEME_LABEL[theme]}`;
}
themeBtn.addEventListener('click', () => {
  theme = { auto: 'light', light: 'dark', dark: 'auto' }[theme];
  ls.set('awning.theme', theme);
  applyTheme();
});
applyTheme();

// ---- realtime ----
connectEvents({
  url: wsUrl(apiBase),
  onStatus: (ws) => {
    const before = store.get().ws;
    store.set({ ws });
    if (ws === 'open' && before !== 'open') loadAll(); // đồng bộ lại nhật ký và lịch sử sau mỗi lần nối
  },
  onMessage: (msg) => {
    const st = store.get();
    const patch = applyMessage(st, msg, performance.now());
    if (msg?.type === 'state' && st.device && st.device.online !== msg.data.online) {
      toast(msg.data.online ? 'Thiết bị đã online trở lại.' : 'Thiết bị mất kết nối.', msg.data.online ? 'info' : 'warn');
    }
    if (Object.keys(patch).length) store.set(patch);
  },
});

// Khi WebSocket chưa thông thì hỏi lại trạng thái định kỳ, để giao diện không đứng hình.
setInterval(() => {
  if (store.get().ws !== 'open') loadState().catch(() => {});
}, 5000);
setInterval(() => { if (document.visibilityState === 'visible') loadHistory(); }, 60000);
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') loadAll();
});

loadAll();

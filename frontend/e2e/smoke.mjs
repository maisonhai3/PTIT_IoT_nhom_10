// Kiểm thử end-to-end bằng trình duyệt thật: chạy `backend/cmd/demo` (broker MQTT nhúng + thiết bị giả) rồi điều
// khiển giao diện như người dùng. Cần Go và Playwright:
//   cd frontend && npm i -D playwright && npx playwright install chromium && node e2e/smoke.mjs
// Biến môi trường: DEMO_BIN (dùng file demo đã build sẵn), SHOTS (thư mục lưu ảnh chụp màn hình), AXE=1 (kiểm tra trợ năng nếu có axe-core).
import { createRequire } from 'node:module';
import { spawn, execFileSync } from 'node:child_process';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const here = path.dirname(fileURLToPath(import.meta.url));
const backendDir = path.resolve(here, '../../backend');
const shots = process.env.SHOTS || '';
if (shots) fs.mkdirSync(shots, { recursive: true });

// ---------- tiện ích ----------
const results = [];
async function step(name, fn) {
  const t0 = Date.now();
  try {
    await fn();
    results.push({ name, ok: true });
    console.log(`  ok   ${name} (${Date.now() - t0} ms)`);
  } catch (err) {
    results.push({ name, ok: false, err });
    console.log(`  FAIL ${name}\n       ${String(err.message ?? err).split('\n').join('\n       ')}`);
  }
}
function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}
function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on('error', reject);
  });
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function buildDemo() {
  if (process.env.DEMO_BIN) return process.env.DEMO_BIN;
  const bin = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'awning-e2e-')), 'demo');
  execFileSync('go', ['build', '-o', bin, './cmd/demo'], { cwd: backendDir, stdio: 'inherit' });
  return bin;
}

async function startDemo(bin, args = [], ports) {
  const http = ports?.http ?? (await freePort());
  const mqtt = ports?.mqtt ?? (await freePort());
  const proc = spawn(bin, ['-http', `127.0.0.1:${http}`, '-mqtt', `127.0.0.1:${mqtt}`, ...args], { cwd: backendDir, stdio: ['ignore', 'pipe', 'pipe'] });
  let log = '';
  proc.stdout.on('data', (d) => { log += d; });
  proc.stderr.on('data', (d) => { log += d; });
  const base = `http://127.0.0.1:${http}`;
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(`${base}/healthz`)).ok) return { proc, base, http, mqtt, log: () => log };
    } catch { /* chưa lên */ }
    await sleep(100);
  }
  proc.kill();
  throw new Error(`demo không khởi động được:\n${log}`);
}
function stop(demo) {
  return new Promise((resolve) => {
    if (demo.proc.exitCode !== null) return resolve();
    demo.proc.once('exit', resolve);
    demo.proc.kill('SIGTERM');
    setTimeout(() => demo.proc.kill('SIGKILL'), 3000);
  });
}

const text = (page, sel) => page.locator(sel).first().innerText();
// Ô "Cảm biến mưa" trong thẻ cảm biến (khác ô "Mưa (theo thiết bị)", nơi tên cảm biến chỉ nằm trong dòng phụ).
const PLATE_TILE = '#sensors .tile:has(.tile-label:text-is("Cảm biến mưa"))';
const plateValue = (page) => text(page, `${PLATE_TILE} .tile-value`);
const plateSub = (page) => text(page, `${PLATE_TILE} .tile-sub`);
const stateTitle = (page) => text(page, '#awning .state-title');
const waitTitle = (page, expected, timeout = 20000) => page.waitForFunction(
  (want) => document.querySelector('#awning .state-title')?.textContent === want, expected, { timeout });

function collectProblems(page) {
  const problems = [];
  page.on('console', (m) => { if (['error', 'warning'].includes(m.type())) problems.push(`[console.${m.type()}] ${m.text()}`); });
  page.on('pageerror', (e) => problems.push(`[pageerror] ${e.message}`));
  return problems;
}

// ---------- kịch bản ----------
const bin = buildDemo();
const browser = await chromium.launch();
const newPage = async (viewport = { width: 1180, height: 900 }) => {
  // CSP của backend chặn script nội tuyến; chỉ khi chạy axe-core mới cần bỏ qua để chèn được thư viện kiểm tra.
  const ctx = await browser.newContext({ viewport, locale: 'vi-VN', timezoneId: 'Asia/Ho_Chi_Minh', bypassCSP: Boolean(process.env.AXE) });
  const page = await ctx.newPage();
  return { ctx, page, problems: collectProblems(page) };
};

console.log('\nKịch bản 1: thiết bị online, điều khiển, mưa giả lập, biểu đồ');
let demo = await startDemo(bin);
{
  const { ctx, page, problems } = await newPage();
  await page.goto(demo.base);
  await step('trang tải xong, máy chủ và thiết bị đều báo online', async () => {
    await page.waitForSelector('#pill-server:has-text("đã kết nối")', { timeout: 15000 });
    await page.waitForSelector('#pill-device:has-text("online")', { timeout: 15000 });
    assert((await page.title()) === 'Giàn phơi thông minh', 'tiêu đề trang sai');
    assert((await stateTitle(page)) === 'Giàn đang mở', `trạng thái ban đầu: ${await stateTitle(page)}`);
    assert((await text(page, '#weather .wx-temp')).includes('°'), 'thiếu nhiệt độ thời tiết');
    assert((await page.locator('#sensors .tile').count()) >= 6, 'thiếu ô cảm biến');
  });

  await step('ô Cảm biến mưa: tấm khô, có số đo thô để chỉnh ngưỡng', async () => {
    await page.waitForSelector(PLATE_TILE, { timeout: 5000 });
    assert((await plateValue(page)) === 'Khô', `giá trị: ${await plateValue(page)}`);
    const m = /Giá trị đo (\d+)\/4095/.exec(await plateSub(page));
    assert(m && Number(m[1]) < 200, `dòng phụ: ${await plateSub(page)}`);
  });

  await step('Thu giàn: đang thu rồi đã thu, chế độ Thủ công, có đếm ngược', async () => {
    await page.click('button[data-action="close"]');
    await waitTitle(page, 'Đang thu giàn…', 8000);
    await waitTitle(page, 'Giàn đã thu');
    assert((await page.getAttribute('button[data-action="close"]', 'aria-pressed')) === 'true', 'nút Thu giàn không ở trạng thái nhấn');
    assert((await text(page, '#awning .card-head .chip')) === 'Thủ công', 'chip chế độ không phải Thủ công');
    assert(/tự về Tự động sau \d+:\d\d/.test(await text(page, '#awning .state-reason')), 'không có đếm ngược thủ công');
    await page.waitForFunction(() => document.querySelector('button[data-action="close"]')?.disabled === false, null, { timeout: 8000 });
    if (shots) await page.screenshot({ path: path.join(shots, 'closed.png') });
  });

  await step('Mở giàn rồi Tự động', async () => {
    await page.click('button[data-action="open"]');
    await waitTitle(page, 'Giàn đang mở');
    await page.click('button[data-action="auto"]');
    await page.waitForFunction(() => document.querySelector('button[data-action="auto"]')?.getAttribute('aria-pressed') === 'true', null, { timeout: 8000 });
    assert((await text(page, '#awning .card-head .chip')) === 'Tự động', 'chip chế độ không phải Tự động');
  });

  await step('giả lập mưa: giàn tự thu, nêu đúng lý do; tắt giả lập thì hết mưa', async () => {
    await page.click('#sim-switch');
    await waitTitle(page, 'Giàn đã thu');
    assert((await text(page, '#awning .state-reason')).includes('giả lập mưa'), `lý do: ${await text(page, '#awning .state-reason')}`);
    assert((await page.getAttribute('#sim-switch', 'aria-checked')) === 'true', 'công tắc giả lập không bật');
    assert((await plateValue(page)) === 'Khô', 'giả lập mưa chỉ là cờ trong firmware, tấm cảm biến vẫn phải khô');
    if (shots) await page.screenshot({ path: path.join(shots, 'sim-rain.png') });
    await page.waitForFunction(() => document.querySelector('#sim-switch')?.disabled === false, null, { timeout: 8000 });
    await page.click('#sim-switch');
    await page.waitForFunction(() => document.querySelector('#sim-switch')?.getAttribute('aria-checked') === 'false', null, { timeout: 8000 });
  });

  await step('nhật ký ghi lại lệnh, trạng thái và mưa (cập nhật realtime)', async () => {
    const items = (await page.locator('#events .events li').allInnerTexts()).join('\n');
    for (const want of ['Lệnh từ web: thu giàn', 'Giàn: đã thu', 'Có mưa (Giả lập)', 'Hết mưa']) assert(items.includes(want), `nhật ký thiếu "${want}":\n${items}`);
  });

  await step('biểu đồ: có đường dữ liệu, tooltip khi rê chuột, phím mũi tên, bảng dữ liệu', async () => {
    await page.click('#history button[data-hours="1"]');
    await page.waitForFunction(() => document.querySelectorAll('#history .chart .line, #history .chart .end-dot').length >= 2, null, { timeout: 15000 });
    const box = await page.locator('#history .chart').boundingBox();
    await page.mouse.move(box.x + box.width - 80, box.y + 90);
    await page.waitForSelector('#history .tooltip:not([hidden])', { timeout: 3000 });
    const tip = await text(page, '#history .tooltip');
    for (const want of ['Nhiệt độ', 'Độ ẩm', 'Giàn']) assert(tip.includes(want), `tooltip thiếu "${want}": ${tip}`);
    await page.mouse.move(5, 5);
    await page.waitForSelector('#history .tooltip[hidden]', { state: 'attached', timeout: 3000 });
    await page.focus('#history .chart');
    await page.keyboard.press('ArrowLeft');
    await page.waitForSelector('#history .tooltip:not([hidden])', { timeout: 3000 });
    await page.keyboard.press('Escape');
    await page.waitForSelector('#history .tooltip[hidden]', { state: 'attached', timeout: 3000 });
    assert((await text(page, '#history .chart-summary')).startsWith('Trong 1 giờ qua'), 'thiếu tóm tắt bằng chữ');
    await page.click('#history button[aria-controls="history-table"]');
    await page.waitForSelector('#history-table table tbody tr', { timeout: 3000 });
    assert((await page.locator('#history-table thead th').count()) === 7, 'bảng dữ liệu thiếu cột');
    if (shots) {
      await page.locator('#history').screenshot({ path: path.join(shots, 'history.png') });
    }
  });

  await step('đổi giao diện sáng/tối bằng nút', async () => {
    const btn = page.locator('#theme-toggle');
    await btn.click();
    assert((await page.getAttribute('html', 'data-theme')) === 'light', 'chưa vào chế độ sáng');
    await btn.click();
    assert((await page.getAttribute('html', 'data-theme')) === 'dark', 'chưa vào chế độ tối');
    if (shots) await page.screenshot({ path: path.join(shots, 'dark.png'), fullPage: true });
    await btn.click();
    assert((await page.getAttribute('html', 'data-theme')) === null, 'chưa về chế độ tự động');
  });

  await step('không có lỗi hay cảnh báo trong console', async () => {
    assert(problems.length === 0, problems.join('\n'));
  });

  if (process.env.AXE) {
    await step('trợ năng (axe-core): không có vi phạm ở chế độ sáng và tối', async () => {
      const axePath = require.resolve('axe-core/axe.min.js');
      const all = [];
      for (const scheme of ['light', 'dark']) {
        await page.emulateMedia({ colorScheme: scheme });
        await page.addScriptTag({ path: axePath });
        const violations = await page.evaluate(async () => (await window.axe.run(document, { resultTypes: ['violations'] })).violations
          .map((v) => `${v.id} (${v.impact}): ${v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')}`));
        all.push(...violations.map((v) => `[${scheme}] ${v}`));
      }
      assert(all.length === 0, all.join('\n'));
    });
  }

  await step('di động 390px: không tràn ngang', async () => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.waitForTimeout(500);
    assert(!(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)), 'có thanh cuộn ngang');
    if (shots) await page.screenshot({ path: path.join(shots, 'mobile.png'), fullPage: true });
  });
  await ctx.close();

  console.log('\nKịch bản 2: mất máy chủ rồi tự hồi phục');
  const second = await newPage();
  await second.page.goto(demo.base);
  await second.page.waitForSelector('#pill-device:has-text("online")', { timeout: 15000 });
  const ports = { http: demo.http, mqtt: demo.mqtt };
  await step('tắt máy chủ: hiện dải cảnh báo mất kết nối', async () => {
    await stop(demo);
    await second.page.waitForSelector('#pill-server:has-text("mất kết nối")', { timeout: 15000 });
    await second.page.waitForSelector('#banners .banner:has-text("Mất kết nối tới máy chủ")', { timeout: 5000 });
  });
  await step('bật lại máy chủ: giao diện tự kết nối lại, không cần tải lại trang', async () => {
    demo = await startDemo(bin, [], ports);
    await second.page.waitForSelector('#pill-server:has-text("đã kết nối")', { timeout: 30000 });
    await second.page.waitForSelector('#pill-device:has-text("online")', { timeout: 20000 });
    await second.page.waitForFunction(() => document.querySelectorAll('#banners .banner').length === 0, null, { timeout: 5000 });
  });
  await second.ctx.close();
}
await stop(demo);

console.log('\nKịch bản 3: thiết bị offline');
demo = await startDemo(bin, ['-device=false']);
{
  const { ctx, page, problems } = await newPage();
  await page.goto(demo.base);
  await step('hiện dải cảnh báo, khóa các nút điều khiển', async () => {
    await page.waitForSelector('#pill-device:has-text("offline")', { timeout: 15000 });
    await page.waitForSelector('#banners .banner:has-text("đang offline")', { timeout: 5000 });
    for (const a of ['open', 'close', 'auto']) assert(await page.locator(`button[data-action="${a}"]`).isDisabled(), `nút ${a} chưa bị khóa`);
    assert(await page.locator('#sim-switch').isDisabled(), 'công tắc giả lập chưa bị khóa');
    assert((await text(page, '#awning .state-block')).includes('Chưa có dữ liệu từ thiết bị'), 'không báo chưa có dữ liệu');
    if (shots) await page.screenshot({ path: path.join(shots, 'offline.png'), fullPage: true });
  });
  await step('không có lỗi console khi thiết bị offline', async () => { assert(problems.length === 0, problems.join('\n')); });
  await ctx.close();
}
await stop(demo);

console.log('\nKịch bản 4: không có dữ liệu thời tiết -> thiết bị vào chế độ dự phòng');
demo = await startDemo(bin, ['-no-weather']);
{
  const { ctx, page } = await newPage();
  await page.goto(demo.base);
  await step('thẻ thời tiết báo chưa có dữ liệu; sau ít giây có dải cảnh báo dự phòng', async () => {
    await page.waitForSelector('#pill-device:has-text("online")', { timeout: 15000 });
    await page.waitForSelector('#weather .empty:has-text("chưa lấy được thời tiết")', { timeout: 10000 });
    await page.waitForSelector('#banners .banner:has-text("không có dữ liệu thời tiết mới")', { timeout: 40000 });
    await page.waitForSelector('#events .events li:has-text("Không lấy được thời tiết")', { timeout: 10000 });
    if (shots) await page.screenshot({ path: path.join(shots, 'no-weather.png'), fullPage: true });
  });
  await ctx.close();
}
await stop(demo);

console.log('\nKịch bản 5: máy chủ yêu cầu mã truy cập');
demo = await startDemo(bin, ['-token', 'secret']);
{
  const { ctx, page } = await newPage();
  await page.goto(demo.base);
  await page.waitForSelector('#pill-device:has-text("online")', { timeout: 15000 });
  await step('không có mã: hộp thoại xuất hiện; mã sai: báo lỗi; mã đúng: lệnh chạy', async () => {
    await page.click('button[data-action="close"]');
    await page.waitForSelector('#token-dialog[open]', { timeout: 5000 });
    await page.fill('#token-input', 'sai-roi');
    await page.click('#token-form button[type="submit"]');
    await page.waitForSelector('#toasts .toast:has-text("mã truy cập")', { timeout: 5000 });
    assert((await stateTitle(page)) === 'Giàn đang mở', 'lệnh bị từ chối mà giàn vẫn chạy');
    await page.click('button[data-action="close"]');
    await page.waitForSelector('#token-dialog[open]', { timeout: 5000 });
    await page.fill('#token-input', 'secret');
    await page.click('#token-form button[type="submit"]');
    await waitTitle(page, 'Giàn đã thu');
  });
  await step('mã đã lưu: lần sau không hỏi lại', async () => {
    await page.waitForFunction(() => document.querySelector('button[data-action="open"]')?.disabled === false, null, { timeout: 8000 });
    await page.click('button[data-action="open"]');
    await waitTitle(page, 'Giàn đang mở');
    assert(!(await page.locator('#token-dialog[open]').count()), 'hộp thoại hỏi lại mã');
  });
  await ctx.close();
}
await stop(demo);
console.log('\nKịch bản 6: mưa thật làm ướt cảm biến mưa');
demo = await startDemo(bin, ['-rain-cycle', '10m', '-rain-offset', '8m']); // bắt đầu giữa pha "đang mưa"
{
  const { ctx, page, problems } = await newPage();
  await page.goto(demo.base);
  await step('tấm ướt: giàn tự thu, lý do và ô cảm biến nêu đúng, nhật ký ghi nguồn cảm biến', async () => {
    await page.waitForSelector('#pill-device:has-text("online")', { timeout: 15000 });
    await waitTitle(page, 'Giàn đã thu', 30000);
    assert((await text(page, '#awning .state-reason')).includes('Cảm biến mưa báo tấm đang ướt'), `lý do: ${await text(page, '#awning .state-reason')}`);
    await page.waitForSelector(`${PLATE_TILE} .tile-value:text-is("Ướt")`, { timeout: 5000 }); // `:text-is` chỉ có trong bộ chọn của Playwright
    const m = /Giá trị đo (\d+)\/4095/.exec(await plateSub(page));
    assert(m && Number(m[1]) >= 400, `dòng phụ: ${await plateSub(page)}`);
    assert((await text(page, '#sensors')).includes('Nguồn: Cảm biến mưa'), 'ô "Mưa (theo thiết bị)" không nêu nguồn là cảm biến mưa');
    await page.waitForFunction((want) => document.querySelector('#events')?.innerText.includes(want), 'Có mưa (Cảm biến mưa)', { timeout: 5000 })
      .catch(async (err) => { throw new Error(`${err.message}\nnhật ký đang hiện:\n${await text(page, '#events')}`); });
    if (shots) await page.screenshot({ path: path.join(shots, 'plate-wet.png'), fullPage: true });
  });
  await step('không có lỗi console', async () => { assert(problems.length === 0, problems.join('\n')); });
  await ctx.close();
}
await stop(demo);
await browser.close();

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} bước đạt${failed.length ? `, ${failed.length} lỗi` : ''}`);
process.exit(failed.length ? 1 : 0);

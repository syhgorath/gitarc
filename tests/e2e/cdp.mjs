// Küçük, sıfır bağımlılıklı Chrome DevTools Protocol sürücüsü (yalnızca geliştirme/test için; uygulamaya GÖMÜLMEZ).
// Node'un yerleşik WebSocket'ini kullanır: npm paketi yok. Chrome bulunamazsa launch() `null` döndürür.
import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export function findChrome() {
  const fromEnv = process.env.CHROME_BIN;
  if (fromEnv && existsSync(fromEnv)) return fromEnv;
  const fixed = [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
  ];
  for (const p of fixed) if (existsSync(p)) return p;
  for (const name of ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser', 'chrome']) {
    const r = spawnSync('which', [name], { encoding: 'utf8' });
    if (r.status === 0 && r.stdout.trim()) return r.stdout.trim();
  }
  return null;
}

export async function launch({ width = 1500, height = 950 } = {}) {
  const bin = findChrome();
  if (!bin) return null;
  const profile = mkdtempSync(join(tmpdir(), 'gitarc-e2e-'));
  const proc = spawn(bin, [
    '--headless=new', '--remote-debugging-port=0', `--user-data-dir=${profile}`, `--window-size=${width},${height}`,
    '--no-first-run', '--no-default-browser-check', '--disable-gpu', '--no-sandbox', '--disable-dev-shm-usage',
    // Geçici profil için macOS Anahtar Zinciri'ne ("Chrome Safe Storage") hiç dokunma: aksi halde "anahtar zinciri bulunamadı"
    // iletişim kutusu çıkar ve ilk açılış saniyelerce takılır. Test profili şifre/çerez saklamaz, gerçek bir anahtara gerek yok.
    '--use-mock-keychain', '--password-store=basic',
    'about:blank',
  ], { stdio: 'ignore' });

  let port = 0;
  for (let i = 0; i < 100 && !port; i++) {
    await new Promise((r) => setTimeout(r, 100));
    try { port = Number(readFileSync(join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0]); } catch { /* henüz yok */ }
  }
  if (!port) { proc.kill(); return null; }

  let targets = [];
  for (let i = 0; i < 50; i++) {
    try { targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json(); if (targets.some((t) => t.type === 'page')) break; } catch { /* bekle */ }
    await new Promise((r) => setTimeout(r, 100));
  }
  const page = targets.find((t) => t.type === 'page');
  if (!page) { proc.kill(); return null; }

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
  let id = 0;
  const pending = new Map();
  const errors = [];
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data);
    if (d.id && pending.has(d.id)) { pending.get(d.id)(d); pending.delete(d.id); return; }
    if (d.method === 'Runtime.exceptionThrown') errors.push(d.params.exceptionDetails.exception?.description || d.params.exceptionDetails.text);
    if (d.method === 'Runtime.consoleAPICalled' && d.params.type === 'error') errors.push(d.params.args.map((a) => a.value ?? a.description).join(' '));
    if (d.method === 'Log.entryAdded' && d.params.entry.level === 'error') errors.push(`${d.params.entry.text} ${d.params.entry.url || ''}`.trim());
  };
  const send = (method, params = {}) => new Promise((res) => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params })); });
  await send('Page.enable'); await send('Runtime.enable'); await send('Log.enable');
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false });

  const api = {
    errors,
    sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
    async goto(url) { const t0 = Date.now(); errors.length = 0; await send('Page.navigate', { url }); await api.sleep(1200); if (process.env.E2E_TRACE) console.log(`  goto ${((Date.now() - t0) / 1000).toFixed(1)}s`); },
    async eval(expr) {
      const r = await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true });
      if (r.result?.exceptionDetails) throw new Error(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text);
      return r.result?.result?.value;
    },
    // GERÇEK fare tıklaması (pointerdown/up dahil): e.click() işaretçi olaylarını üretmez ve kaydırma/yakalama hatalarını kaçırır.
    async realClick(selector) {
      const pt = await api.eval(`(() => { const e = document.querySelector(${JSON.stringify(selector)}); if (!e) return null; const r = e.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
      if (!pt) throw new Error(`bulunamadı: ${selector}`);
      await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: pt.x, y: pt.y });
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: pt.x, y: pt.y, button: 'left', clickCount: 1 });
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: pt.x, y: pt.y, button: 'left', clickCount: 1 });
    },
    // koşul doğru olana kadar bekler (en fazla timeout ms)
    async waitFor(expr, timeout = 15000) {
      const t0 = Date.now();
      while (Date.now() - t0 < timeout) {
        try { if (await api.eval(expr)) return true; } catch { /* sayfa henüz hazır değil */ }
        await api.sleep(200);
      }
      return false;
    },
    close() { try { ws.close(); } catch { /* */ } proc.kill(); try { rmSync(profile, { recursive: true, force: true }); } catch { /* */ } },
  };
  return api;
}

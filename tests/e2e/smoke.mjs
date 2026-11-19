// Uçtan uca duman testi (yalnızca geliştirme için): gerçek bir Chrome'da ana pencereleri TIKLAYIP açılıp açılmadığına bakar.
// Birim testleri fonksiyonları/sahte DOM'u dener; "Diff penceresi hiç açılmıyordu" gibi sessiz bozulmaları ancak gerçek bir
// tıklama yakalar. Kullanım: node tests/e2e/smoke.mjs <proje-yok-url> <demo-url> <canlı-url>. Çıkış: 0 geçti, 1 kaldı, 77 atlandı.
import { launch } from './cdp.mjs';

const [noneUrl, simUrl, liveUrl] = process.argv.slice(2);
if (!noneUrl || !simUrl || !liveUrl) { console.error('kullanım: smoke.mjs <none> <sim> <live>'); process.exit(2); }

const t00 = Date.now();
const b = await launch();
console.log(`Chrome açıldı (${((Date.now() - t00) / 1000).toFixed(1)}s)`);
if (!b) { console.log('SKIP: Chrome bulunamadı (CHROME_BIN ile verebilirsiniz)'); process.exit(77); }

const failures = [];
const visible = (sel) => `(()=>{const e=document.querySelector(${JSON.stringify(sel)}); if(!e) return false; const r=e.getBoundingClientRect(); const c=getComputedStyle(e); return !e.hidden && c.display!=='none' && c.visibility!=='hidden' && r.width>0 && r.height>0;})()`;
const hidden = (sel) => `(()=>{const e=document.querySelector(${JSON.stringify(sel)}); return !e || e.hidden || getComputedStyle(e).display==='none';})()`;
const click = (sel) => b.eval(`(()=>{const e=document.querySelector(${JSON.stringify(sel)}); if(!e) throw new Error('bulunamadı: ${sel}'); e.click(); return true;})()`);

async function step(name, fn) {
  const t0 = Date.now();
  const took = () => `${((Date.now() - t0) / 1000).toFixed(1)}s`;
  try { await fn(); console.log(`  ok   ${name} (${took()})`); } catch (e) { failures.push(name); console.log(`  FAIL ${name} (${took()}): ${e.message}`); }
}
const expect = async (cond, what, timeout = 12000) => { if (!(await b.waitFor(cond, timeout))) throw new Error(what); };
const noConsoleErrors = () => {
  const real = b.errors.filter((e) => !/favicon/i.test(e));
  if (real.length) throw new Error(`konsol hatası: ${real.slice(0, 2).join(' | ')}`);
};

try {
  console.log('Proje yok (depo bulunamadı):');
  await b.goto(noneUrl);
  await step('karşılama paneli görünür, demo yok', async () => {
    await expect(`document.body.classList.contains('no-project')`, 'gövdede no-project sınıfı yok');
    await expect(visible('#no-project-card'), 'karşılama paneli görünmüyor');
    await expect(hidden('#demo-banner'), 'demo şeridi görünmemeli');
  });
  await step('"Proje Ekle" penceresi açılır', async () => {
    await click('#btn-no-project-add');
    await expect(visible('#add-project-modal'), 'Proje Ekle penceresi açılmadı');
  });
  await step('konsolda hata yok', async () => noConsoleErrors());

  console.log('Demo (-repo sim):');
  await b.goto(simUrl);
  await step('DEMO şeridi görünür', async () => {
    await expect(`document.body.classList.contains('demo-mode')`, 'gövdede demo-mode sınıfı yok');
    await expect(visible('#demo-banner'), 'DEMO şeridi görünmüyor');
  });
  await step('tuvalde dallar çizilir', async () => expect(`document.querySelectorAll('#rail-svg .fo-capsule').length > 2`, 'kapsül çizilmedi'));
  await step('Kokpit Turu açılır, ilerler, kapanır', async () => {
    await click('#btn-start-tour');
    await expect(visible('#tour-card'), 'tur kartı açılmadı');
    await click('#tour-next-btn');
    await expect(`document.getElementById('tour-step-badge').textContent.includes('2')`, 'tur 2. adıma geçmedi');
    await click('#tour-close-btn');
    await expect(hidden('#tour-card'), 'tur kapanmadı');
  });
  await step('MCP penceresi açılır ve kapanır', async () => {
    await click('#btn-mcp-connect');
    await expect(visible('#mcp-connect-modal'), 'MCP penceresi açılmadı');
    await click('#mcp-close');
    await expect(hidden('#mcp-connect-modal'), 'MCP penceresi kapanmadı');
  });
  await step('Çakışma Stüdyosu radar kartından açılır ve kapanır', async () => {
    await expect(`[...document.querySelectorAll('.radar-card button')].some((x) => x.textContent.includes('Çakışma Stüdyosu'))`, 'radarda çakışma kartı yok');
    await b.eval(`[...document.querySelectorAll('.radar-card button')].find((x) => x.textContent.includes('Çakışma Stüdyosu')).click()`);
    await expect(visible('#conflict-studio-modal'), 'stüdyo açılmadı');
    await click('#studio-close');
    await expect(hidden('#conflict-studio-modal'), 'stüdyo kapanmadı');
  });
  await step('konsolda hata yok', async () => noConsoleErrors());

  console.log('Canlı depo (birleşmiş dal):');
  await b.goto(liveUrl);
  await step('gerçek depo canlı kipte açılır, demo şeridi yok', async () => {
    await expect(`(async()=>{const m=await import('/modules/state.js'); return m.state.snapshot?.repo?.mode==='live';})()`, 'canlı kipte açılmadı');
    await expect(hidden('#demo-banner'), 'canlı depoda demo şeridi görünmemeli');
  });
  await step('birleşmiş dalı seçince "Değişen Dosyalar" satırları yüklenir', async () => {
    await b.eval(`(async()=>{const m=await import('/modules/state.js'); const app=await import('/app.js'); const t=m.state.snapshot.branches.find((x)=>x.state==='merged'); app.selectBranch(t.id);})()`);
    await expect(`document.querySelectorAll('.merged-file-row').length > 0`, 'dosya satırı yüklenmedi');
  });
  await step('dosya satırına tıklayınca Diff penceresi AÇILIR ve +/- satırları görünür', async () => {
    await click('.merged-file-row');
    await expect(visible('#diff-viewer-modal'), 'Diff penceresi açılmadı');
    await expect(`document.querySelectorAll('#diff-viewer-container .diff-file-card').length > 0 && document.querySelectorAll('#diff-viewer-container .diff-add').length > 0`, 'diff içeriği (eklenen satırlar) yok');
  });
  await step('Diff penceresi kapanır', async () => {
    await click('#diff-modal-close');
    await expect(hidden('#diff-viewer-modal'), 'Diff penceresi kapanmadı');
  });
  await step('"Diff Gör" düğmesi de pencereyi açar', async () => {
    await click('#act-branch-diff, #studio-act-diff');
    await expect(visible('#diff-viewer-modal'), '"Diff Gör" pencereyi açmadı');
    await click('#diff-modal-close');
  });
  await step('konsolda hata yok', async () => noConsoleErrors());
} finally {
  b.close();
}

console.log(`\nToplam ${((Date.now() - t00) / 1000).toFixed(1)}s`);
if (failures.length) { console.log(`${failures.length} adım KALDI: ${failures.join(' · ')}`); process.exit(1); }
console.log('Hepsi geçti.');
process.exit(0); // Chrome süreci kapanırken beklemeyelim

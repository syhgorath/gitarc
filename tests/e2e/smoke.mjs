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
  // Proje bağlama adımı BİLEREK 409 üretir (başka projeyi gösteren sekme taklidi); başka hiçbir 409'a izin yok.
  const real = b.errors.filter((e) => !/favicon/i.test(e) && !/status of 409.*\/api\/branch\/delete/.test(e));
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
    await expect(`document.getElementById('mcp-connect-modal').textContent.includes('gitarc_protection_apply') && document.getElementById('mcp-connect-modal').textContent.includes('22 Araç')`, 'MCP penceresi koruma araçlarını/16 araç sayısını göstermiyor');
    await click('#mcp-close');
    await expect(hidden('#mcp-connect-modal'), 'MCP penceresi kapanmadı');
  });
  await step('Merge kuyruğu planı penceresi: PR\'lar listelenir, plan hesaplanır, kapanır', async () => {
    await click('#btn-queue');
    await expect(visible('#queue-modal'), 'Kuyruk penceresi açılmadı');
    await expect(`document.querySelectorAll('#queue-candidates .queue-row').length > 0`, 'kuyruğa girecek PR satırları görünmüyor');
    await expect(`document.querySelectorAll('#queue-result .queue-step').length > 0 && !!document.querySelector('#queue-result .queue-summary')`, 'plan adımları/özet görünmüyor');
    // Eski ayrı "Dalı güncelle" düğmesi kalktı: güncelleme yalnızca kuyruğun adımı olarak yapılır
    await expect(`document.querySelectorAll('#queue-result .queue-update-btn').length === 0`, 'ayrı "Dalı güncelle" düğmesi kalkmış olmalı');
    // 3a: kuyruğun sıradaki dalı (çakışan #421): engel gösterilir, uygulama düğmesi YOK
    await expect(`document.querySelectorAll('#queue-result .queue-step-btn').length === 1`, 'yalnızca sıradaki (1.) adımda "Sıradaki adımı planla" düğmesi olmalı');
    await click('#queue-result .queue-step-btn');
    await expect(`!!document.querySelector('.queue-stepplan') && document.querySelector('.queue-stepplan').textContent.includes('Kuyruk burada durur') && !document.querySelector('.queue-step-confirm') && !!document.querySelector('.queue-step-recheck')`, 'engelli adım planı gösterilmedi ya da uygulama düğmesi göründü');
    // #421'i kuyruktan çıkar, #422'nin CI'ını yeşile çevir (demo): sıradaki adım "güncelle", onaylanınca "birleştir"
    await b.eval(`(async () => { const m = await import('/modules/api.js'); await m.apiCall('/api/branch/fix-ci', 'POST', { branchId: 'br-open-422' }); })()`);
    await b.eval(`[...document.querySelectorAll('#queue-candidates .queue-row')].find((r) => r.textContent.includes('#421')).querySelector('input[type=checkbox]').click()`);
    await expect(`!!document.querySelector('#queue-result .queue-step-btn') && document.querySelector('#queue-result .queue-step').textContent.includes('#422')`, '#422 kuyruğun başına geçmedi');
    await click('#queue-result .queue-step-btn');
    await expect(`!!document.querySelector('.queue-stepplan .queue-step-confirm') && document.querySelector('.queue-stepplan').textContent.includes('dalı güncelle')`, 'güncelleme adımı planı/onay düğmesi görünmedi');
    await click('.queue-stepplan .queue-step-confirm');
    await expect(`!document.querySelector('.queue-stepplan') && !!document.querySelector('#queue-result .queue-step-btn')`, 'güncellemeden sonra kuyruk yeniden çizilmedi');
    await click('#queue-result .queue-step-btn');
    await expect(`!!document.querySelector('.queue-stepplan .queue-step-confirm') && document.querySelector('.queue-stepplan').textContent.includes('birleştir')`, 'güncellemeden sonra birleştirme adımı planlanmadı');
    await click('.queue-stepplan .queue-step-confirm');
    await expect(`![...document.querySelectorAll('#queue-candidates .queue-row')].some((r) => r.textContent.includes('#422')) && !document.querySelector('#queue-result').textContent.includes('#422 ')`, 'birleşen #422 kuyruktan kalkmadı');
    // Pencere ekrandan taşmaz: gövde kendi içinde kayar, kapat düğmesi hep erişilebilir
    await expect(`(() => { const c = document.querySelector('#queue-modal .modal-card'); const bd = document.querySelector('#queue-modal .modal-body'); return c.getBoundingClientRect().height <= window.innerHeight - 16 && getComputedStyle(bd).overflowY === 'auto'; })()`, 'kuyruk penceresi ekrandan taşıyor ya da gövdesi kaydırılamıyor');
    await click('#queue-close');
    await expect(hidden('#queue-modal'), 'Kuyruk penceresi kapanmadı');
  });
  await step('Merge sonrası main CI rozeti: birleşen PR\'ın merge düğümünde çıkar, tıklayınca kart açılır', async () => {
    await expect(`!!document.querySelector('#svg-canvas .svg-mainci-badge, svg .svg-mainci-badge')`, 'birleşme sonrası merge düğümünde main CI rozeti yok');
    await b.realClick('.svg-mainci-badge'); // GERÇEK fare: tuval kaydırma işleyicisi tıklamayı yutmamalı
    await expect(`!!document.querySelector('.mainci-card') && document.querySelector('.mainci-card').textContent.includes('main CI')`, 'rozete tıklayınca main CI kartı açılmadı');
  });
  await step('Çalışan kuyruk koşusu: PR kartında "kuyrukta/işleniyor" rozeti çıkar, durdurunca kalkar', async () => {
    const refresh = `(async () => { const api = await import('/modules/api.js'); const app = await import('/app.js'); app.applySnapshot(await api.apiCall('/api/snapshot')); return true; })()`;
    // #424'ün CI'ı sürüyor: koşu başlar ve bekler (rozet görünür kalır)
    await b.eval(`(async () => { const api = await import('/modules/api.js'); const plan = await api.apiCall('/api/queue/run/plan', 'POST', { branchIds: ['br-open-424'] }); await api.apiCall('/api/queue/run/start', 'POST', { branchIds: ['br-open-424'], confirm: plan.fingerprint }); })()`);
    await b.eval(refresh);
    await expect(`!!document.querySelector('.cap-queue-tag.current') && document.querySelector('.cap-queue-tag.current').textContent.includes('İşleniyor')`, 'çalışan koşudaki PR kartında "İşleniyor" rozeti yok');
    await expect(`!!document.querySelector('.svg-queue-target .svg-queue-target-path') && !!document.querySelector('.svg-queue-landing') && document.querySelector('.svg-queue-target-path').getAttribute('marker-end') === 'url(#queue-arrow)'`, 'işlenen PR için sarı kesik hedef çizgisi/hayalet düğüm yok');
    await b.eval(`(async () => { const api = await import('/modules/api.js'); await api.apiCall('/api/queue/run/stop', 'POST', {}); })()`);
    await b.eval(refresh);
    await expect(`!document.querySelector('.cap-queue-tag')`, 'durdurulan koşudan sonra rozet kalkmalı');
    await expect(`!document.querySelector('.svg-queue-target')`, 'durdurulan koşudan sonra hedef çizgisi kalkmalı');
  });
  await step('Koşu sürerken kuyruk penceresi planı yeniden HESAPLAMAZ; seçim kilitli; durdurunca plan bir kez yenilenir', async () => {
    const planCalls = `performance.getEntriesByType('resource').filter((e) => e.name.includes('/api/queue/plan')).length`;
    // #424'ün CI'ı sürüyor: koşu başlar ve bekler
    await b.eval(`(async () => { const api = await import('/modules/api.js'); const plan = await api.apiCall('/api/queue/run/plan', 'POST', { branchIds: ['br-open-424'] }); await api.apiCall('/api/queue/run/start', 'POST', { branchIds: ['br-open-424'], confirm: plan.fingerprint }); })()`);
    const before = await b.eval(planCalls);
    await click('#btn-queue');
    await expect(visible('#queue-modal'), 'Kuyruk penceresi açılmadı');
    await expect(`!!document.querySelector('.queue-run-note') && !!document.querySelector('.queue-run-stop')`, 'koşu sürerken plan yerine not + Durdur görünmeli');
    await expect(`[...document.querySelectorAll('#queue-candidates input[type=checkbox]')].every((c) => c.disabled)`, 'koşu sürerken PR seçimi kilitli olmalı');
    await b.sleep(800);
    const during = await b.eval(planCalls);
    if (during !== before) throw new Error(`koşu sürerken plan yeniden hesaplandı (${during - before} istek)`);
    await click('.queue-run-stop');
    await expect(`!!document.querySelector('#queue-result .queue-summary') && !document.querySelector('.queue-run-note')`, 'durdurunca plan yeniden gösterilmedi');
    await expect(`[...document.querySelectorAll('#queue-candidates input[type=checkbox]')].every((c) => !c.disabled)`, 'durdurunca PR seçimi açılmalı');
    await click('#queue-close');
    await expect(hidden('#queue-modal'), 'Kuyruk penceresi kapanmadı');
  });
  await step('Kuyruğu çalıştır (3b): plan → onay → sunucu sırayla birleştirir → durum "Koşu bitti"', async () => {
    await b.eval(`(async () => { const m = await import('/modules/api.js'); await m.apiCall('/api/branch/fix-ci', 'POST', { branchId: 'br-open-424' }); })()`);
    await click('#btn-queue');
    await expect(visible('#queue-modal'), 'Kuyruk penceresi açılmadı');
    await expect(`!!document.querySelector('#queue-candidates .queue-row')`, 'kuyruk satırları görünmüyor');
    // çakışan #421'i kuyruktan çıkar: koşu yalnızca sorunsuz öneki içerir (ilk sorunda durur)
    await b.eval(`[...document.querySelectorAll('#queue-candidates .queue-row')].find((r) => r.textContent.includes('#421'))?.querySelector('input[type=checkbox]').click()`);
    await expect(`!!document.querySelector('.queue-run-btn') && !document.querySelector('.queue-run-btn').disabled`, '"Kuyruğu çalıştır" düğmesi görünmedi');
    await click('.queue-run-btn');
    await expect(`!!document.querySelector('#queue-run .queue-run-confirm') && document.querySelector('#queue-run').textContent.includes('sırayla işlenecek')`, 'koşu planı/onay düğmesi görünmedi');
    await click('#queue-run .queue-run-confirm');
    await expect(`document.querySelector('#queue-run') && (document.querySelector('#queue-run').textContent.includes('Koşu bitti') || document.querySelector('#queue-run').textContent.includes('Koşu sürüyor'))`, 'koşu durumu görünmedi');
    await expect(`document.querySelector('#queue-run').textContent.includes('Koşu bitti')`, 'koşu bitmedi ("Koşu bitti" görünmedi)', 20000);
    await expect(`![...document.querySelectorAll('#queue-candidates .queue-row')].some((r) => r.textContent.includes('#424'))`, 'birleşen #424 kuyruktan kalkmadı');
    await click('#queue-close');
    await expect(hidden('#queue-modal'), 'Kuyruk penceresi kapanmadı');
  });
  await step('Koruma penceresi: plan görünür, "Temel" uygulanınca depo korumalı olur, pencere kapanır', async () => {
    await click('#btn-protect-branch');
    await expect(visible('#security-update-modal'), 'Koruma penceresi açılmadı');
    await expect(`document.querySelectorAll('#prot-changes li').length > 0`, 'plan satırları görünmüyor');
    await click('input[name="prot-profile"][value="temel"]');
    // Düğme plan gelene kadar gizlidir: GÖRÜNÜR olmasını beklemek, "Temel" planının geldiğinin kanıtıdır (önceki profilin
    // eski düğmesine tıklayıp yarışa girmeyelim).
    await expect(visible('#btn-enable-protection'), 'Uygula düğmesi görünmedi');
    await expect(`document.getElementById('btn-enable-protection').textContent.includes('Değişikliği Uygula') && !document.getElementById('btn-enable-protection').disabled`, 'Uygula düğmesi açılmadı');
    await click('#btn-enable-protection');
    await expect(`document.getElementById('btn-protect-branch').textContent.includes('Korumalı')`, 'uygulama sonrası düğme "Korumalı" olmadı');
    await expect(hidden('#btn-enable-protection'), 'uygulanacak bir şey kalmadı, Uygula düğmesi gizlenmeliydi');
    await click('#sec-close');
    await expect(hidden('#security-update-modal'), 'Koruma penceresi kapanmadı');
  });
  await step('Proje bağlama: başka projeyi gösteren sekmenin değiştirici isteği 409 ile REDDEDİLİR', async () => {
    // Gerçek tarayıcıda: sunucudaki etkin projeden farklı bir kimlikle gelen istek işlem yapmamalı ve etkin projeyi bildirmeli.
    await expect(`(async()=>{
      const m = await import('/modules/state.js');
      const r = await fetch('/api/branch/delete', { method: 'POST', credentials: 'same-origin',
        headers: { 'X-Requested-With': 'gitarc', 'Content-Type': 'application/json', 'X-GitArc-Repo': 'baska-bir-proje' },
        body: JSON.stringify({ branchId: 'x' }) });
      const j = await r.json();
      return r.status === 409 && j.code === 'repo_mismatch' && j.activeRepoId === m.state.snapshot.repo.id;
    })()`, 'yanlış proje kimliğiyle gelen istek 409 repo_mismatch ile reddedilmedi');
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

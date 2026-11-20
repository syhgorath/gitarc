// README ekran görüntülerini (demo/simülasyon verisiyle) üretir. Kullanım: node tests/e2e/ekran-goruntuleri.mjs <çıktı-klasörü> <sim-url>
// Sonra tam pencere olanları: scripts/ekran-cercevesi.py girdi.png cikti.svg (tarayıcı penceresi çerçevesi). Yalnızca geliştirme için.

import { launch } from './cdp.mjs';
const OUT = process.argv[2], URL = process.argv[3];
const b = await launch({ width: 1500, height: 940 });
const click = (sel) => b.eval(`(()=>{const e=document.querySelector(${JSON.stringify(sel)}); if(!e) throw new Error('yok: ${'${sel}'}'); e.click(); return true;})()`);
const wait = (c, t = 8000) => b.waitFor(c, t);
await b.goto(URL); await b.sleep(1500);
await wait(`document.querySelectorAll('#rail-svg .fo-capsule').length > 2`);
// 1 kokpit
await b.shot(`${OUT}/kokpit.png`);
// 2 üst çubuk + çipler (kırpma)
await b.shot(`${OUT}/ust-cubuk-ve-cipler.png`, { x: 0, y: 0, width: 1500, height: 150 });
// 3 hata radarı (alt çekmece kırpma)
const dock = await b.eval(`(()=>{const r=document.getElementById('bottom-dock').getBoundingClientRect(); return {x:r.x,y:r.y,width:r.width,height:r.height}})()`);
await b.shot(`${OUT}/hata-radari.png`, dock);
// 4 kuyruk
await click('#btn-queue'); await wait(`!!document.querySelector('#queue-candidates .queue-row')`); await b.sleep(800);
await b.shot(`${OUT}/kuyruk.png`); await click('#queue-close'); await b.sleep(300);
// 5 koruma
await click('#btn-protect-branch'); await wait(`document.querySelectorAll('#prot-now .prot-now-row').length > 3`); await b.sleep(600);
await b.shot(`${OUT}/koruma.png`); await click('#sec-close'); await b.sleep(300);
// 6 mcp
await click('#btn-mcp-connect'); await b.sleep(500);
await b.shot(`${OUT}/mcp.png`); await click('#mcp-close'); await b.sleep(300);
// 7 çakışma stüdyosu
await b.eval(`[...document.querySelectorAll('.radar-card button')].find((x) => x.textContent.includes('Çakışma Stüdyosu'))?.click()`);
await wait(`!document.getElementById('conflict-studio-modal').hidden`); await b.sleep(900);
await b.shot(`${OUT}/cakisma-studyosu.png`); await click('#studio-close'); await b.sleep(300);
// 8 birleşmiş PR detayı
await b.eval(`(async()=>{const m=await import('/modules/state.js'); const app=await import('/app.js'); const t=m.state.snapshot.branches.find((x)=>x.state==='merged'); app.selectBranch(t.id);})()`);
await wait(`document.querySelectorAll('.merged-file-row').length > 0`); await b.sleep(600);
await b.shot(`${OUT}/birlesmis-pr-detay.png`);
await b.eval(`(async()=>{const app=await import('/app.js'); app.selectBranch(null);})()`).catch(()=>{});
// 9 tur
await click('#btn-start-tour'); await b.sleep(700);
await b.shot(`${OUT}/kokpit-turu.png`); await click('#tour-close-btn'); await b.sleep(300);
// 10 açık tema
await click('#btn-theme'); await b.sleep(700);
await b.shot(`${OUT}/acik-tema.png`);
b.close();
console.log('tamam');

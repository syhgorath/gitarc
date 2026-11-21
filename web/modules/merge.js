// web/modules/merge.js - "Merge" niyeti: bir PR'ı birleştirmek için hangi yol? (app.js'ten çıkarıldı; davranış aynı)
//
//  1. Dal main'in gerisinde (korumada "dal güncel olmalı") → "Güncelle & Birleştir": 1 PR'lık kuyruk koşusu planı açılır.
//  2. CI sürüyor ya da zorunlu CI henüz belirmedi → "CI Bitince Merge Et": çevrimiçi SUNUCU koşusu (yenilemede kaybolmaz; ikinci
//     tıklama durdurur); çevrimdışı yedekte bu sekmenin bekçisi (state.pendingAutoMerge).
//  3. Aksi halde doğrudan birleştir. Sunucu "gerisinde kaldı" ya da "koruma kuralı engelliyor" derse çıkmaz sokak yerine güvenli ön adım.
//
// deps: { applySnapshot(snap), rerender() } — app.js'ten gelir (bu modül app.js'i içe aktarmaz: döngüsel bağımlılık yok).
import { state } from './state.js';
import { apiCall } from './api.js';
import { showToast, runWithBtnBusy } from './utils.js';
import { needsUpdateBeforeMerge, queueRunBadge, ciPendingForMerge } from './syncsteps.js';
import { openQueueModal, startSingleMergeRun, stopMergeRun } from './queue.js';

const findBranch = (branchId) => (state.snapshot?.branches || []).find((x) => x.id === branchId);

/** Çevrimiçi "CI bitince merge et": koşu varsa durdur, yoksa 1 PR'lık sunucu koşusu başlat. */
async function toggleServerRun(branchId, b, { applySnapshot }) {
  if (queueRunBadge(state.snapshot, b)) {
    try {
      await stopMergeRun();
      applySnapshot(await apiCall('/api/snapshot'));
      showToast('Otomatik merge koşusu durduruldu', 'info');
    } catch (err) { showToast(err.message, 'danger'); }
    return;
  }
  try {
    const res = await startSingleMergeRun(branchId);
    if (res.started) {
      showToast(`⏳ CI devam ediyor: Bitince '${b?.name}' otomatik birleştirilecek (sunucu koşusu)`, 'warn');
      applySnapshot(await apiCall('/api/snapshot'));
    } else {
      showToast(`Otomatik merge başlatılamadı: ${res.reason}`, 'danger');
    }
  } catch (err) { showToast(err.message, 'danger'); }
}

/** Çevrimdışı yedek: bu sekmenin bekçisi (pendingAutoMerge) aç/kapat. */
function toggleTabWatcher(branchId, b, { rerender }) {
  if (state.pendingAutoMerge[branchId]) {
    delete state.pendingAutoMerge[branchId];
    rerender();
    showToast('Otomatik merge kuyruğu iptal edildi', 'info');
    return;
  }
  state.pendingAutoMerge[branchId] = {
    branchId,
    branchName: b?.name,
    strategy: state.mergeStrategy,
    queuedAt: new Date().toLocaleTimeString(),
  };
  rerender();
  showToast(`⏳ CI devam ediyor: Bitince '${b?.name}' otomatik birleştirilecek!`, 'warn');
}

/** Doğrudan birleştirme ve reddedilirse güvenli ön adım. */
async function mergeNow(branchId, b, btnEl, { applySnapshot }) {
  await runWithBtnBusy(btnEl, '⚡ Birleştiriliyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.mergeBranch(branchId) : state.snapshot)
        : await apiCall('/api/branch/merge', 'POST', { branchId, strategy: state.mergeStrategy });
      applySnapshot(snap);
      showToast(`✓ Dal birleştirildi (${state.mergeStrategy.toUpperCase()}) ve parabol kapatıldı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
      // Sunucu "dal main'in gerisinde" diyorsa (kural arayüzce bilinmiyordu: ruleset/okunamadı) çıkmaz sokakta bırakma: aynı planı aç.
      if (/gerisinde kaldı/.test(err.message || '')) openQueueModal(applySnapshot, { only: branchId, autoPlan: true });
      // Koruma kuralı (bitmemiş zorunlu CI) engelledi: çıkmaz sokak yerine güvenli ön adım — CI bitince birleştiren 1 PR'lık koşu.
      else if (!state.offlineMode && /koruma kuralı engelliyor/.test(err.message || '')) {
        try {
          const res = await startSingleMergeRun(branchId);
          if (res.started) {
            showToast(`⏳ '${b?.name}' için kontroller bitince otomatik birleştirilecek (sunucu koşusu)`, 'warn');
            applySnapshot(await apiCall('/api/snapshot'));
          }
        } catch { /* koşu başlatılamadı: yukarıdaki açıklayıcı hata zaten gösterildi */ }
      }
    }
  });
}

/** Giriş noktası: kart/inspector "Merge" düğmesi. */
export async function mergeBranch(branchId, btnEl, deps) {
  const b = findBranch(branchId);
  if (needsUpdateBeforeMerge(b, state.snapshot)) {
    openQueueModal(deps.applySnapshot, { only: branchId, autoPlan: true });
    return;
  }
  if (ciPendingForMerge(b, state.snapshot)) { // CI sürüyor ya da (zorunlu kontrol varken) henüz belirmedi
    if (state.offlineMode) toggleTabWatcher(branchId, b, deps);
    else await toggleServerRun(branchId, b, deps);
    return;
  }
  await mergeNow(branchId, b, btnEl, deps);
}

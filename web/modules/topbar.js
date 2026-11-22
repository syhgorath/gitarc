// web/modules/topbar.js - Üst çubuk (mod rozeti, depo seçici, temizle/koruma/sürüm düğmeleri) ve sağ üstteki
// "main ↔ origin/main" senkron rozeti. Metin ve durum kararları saf fonksiyonlardadır (Node'da test edilir);
// `renderTopbar` yalnızca bunları DOM'a yazar.
import { state, el } from './state.js';
import { esc } from './utils.js';
import { summarizeSync, protectionView, mainCIView, loadingHintView } from './syncsteps.js';

export function modeLabel(repo, offline) {
  if (offline) return 'Offline Simülasyon';
  if (repo.mode === 'none') return 'Proje yok';
  return repo.mode === 'live' ? 'Canlı Git · Aktif' : 'Simülasyon · Etkileşimli';
}

// Kokpitin kipi: "proje yok" (karşılama paneli) ve "demo" (sahte veri şeridi). Demo yalnızca bilerek istenince
// (`gitarc -repo sim`) ya da sunucusuz açılışta görünür; gerçek bir depoyla karıştırılmasın diye belirgin işaretlenir.
export function modeView(repo, offline = false) {
  const mode = repo?.mode || 'live';
  return {
    noProject: mode === 'none' && !offline,
    demo: mode === 'simulation' || (offline && mode !== 'live'),
  };
}

// Güncelleme durumu satırı (koruma penceresindeki "Sürüm & güncelleme" kartı). `res`: /api/system/update-check yanıtı (yoksa anlık
// görüntüdeki son bilinen değerler). Denetim hiç yapılmadıysa "güncel" DENMEZ: "henüz denetlenmedi".
export function updateStatusText(repo, res = null) {
  const cur = res?.currentVersion || repo?.appVersion || '';
  if (res?.checkError) return `Denetlenemedi (ağ/erişim): ${res.checkError}`;
  const latest = res?.latestVersion || repo?.latestVersion || '';
  const avail = res ? Boolean(res.updateAvailable) : Boolean(repo?.updateAvailable);
  if (avail) return `⬆ Yeni sürüm mevcut: v${latest} (şu an v${cur})`;
  if (latest) return `✓ Güncel (v${cur})`;
  return `v${cur} · Henüz denetlenmedi: "Yeni sürüm var mı?" ile denetleyin`;
}

export function versionView(repo) {
  const text = `v${repo.appVersion || '0.10.0'}${repo.teamMode ? ' · TEAM' : ''}${repo.updateAvailable ? ' ⬆ Güncelle' : ''}`;
  return { text, className: `btn mono ${repo.updateAvailable ? 'warn' : 'ghost'}` };
}

// Temizlenebilir (silinmiş/birleşmiş kalıntı) dal sayısı.
/** Saf: üst çubuktaki "Şu an: <dal>" çipi. Dal adı canlı depoda sunucudan (repo.currentBranch), demo/yedek motorda HEAD dalından gelir.
 *  Ana dalda değilsen (ya da kaydedilmemiş dosya varsa) dikkat çeker; dal bilinmiyorsa (detached HEAD / proje yok) çip gizlenir. */
export function headChipModel(repo, branches = []) {
  const name = repo?.currentBranch || (branches || []).find((b) => b.isHead)?.name || '';
  if (!name || repo?.mode === 'none') return { show: false, text: '', title: '', tone: 'neutral' };
  const def = repo?.defaultBranch || 'main';
  const dirty = Number(repo?.dirtyCount) || 0;
  const off = name !== def;
  const text = `🌿 ${name}${dirty > 0 ? ` · ${dirty} kaydedilmemiş` : ''}`;
  const title = off
    ? `Çalışma klasörü şu an "${name}" dalında (${def} değil)${dirty > 0 ? `; ${dirty} dosya kaydedilmemiş` : ''}.`
    : `Çalışma klasörü ana dalda (${def})${dirty > 0 ? `; ${dirty} dosya kaydedilmemiş` : ''}.`;
  return { show: true, text, title, tone: off ? 'off-default' : 'on-default' };
}

export function cleanableCount(branches) {
  return (branches || []).filter((b) => b.prunable).length;
}

// Senkron rozetinin metni, sağlık durumu ve ipucu (DOM yok).
export function syncBadgeModel(repo) {
  const ahead = repo.ahead || 0;
  const behind = repo.behind || 0;
  const remoteName = repo.remoteName || 'origin';
  const defBranch = repo.defaultBranch || 'main';
  const isSynced = ahead === 0 && behind === 0;
  const text = isSynced
    ? `${defBranch} ↔ ${remoteName}/${defBranch} (Synced)`
    : `${defBranch} ↔ ${remoteName}/${defBranch}  ↑${ahead} · ↓${behind}`;
  // "Synced" yalnızca yerel origin/main referansına bakar; hangi verinin eski veya eksik olduğunu
  // veri toplama adımları (2. satır) gösterir.
  const sync = summarizeSync(repo.syncSteps);
  const healthy = isSynced && (sync.items.length === 0 || sync.allOk);
  const stepsHint = sync.items.length ? `\n\nVeri toplama:\n${sync.title}` : '';
  const title = isSynced
    ? `${defBranch} ile ${remoteName}/${defBranch} eşitlendi · Eşitlemek için tıklayın (git fetch & fast-forward)${stepsHint}`
    : `Fark var (↑${ahead} / ↓${behind}) · Şimdi senkronize etmek için tıklayın!${stepsHint}`;
  return { text, isSynced, healthy, title, steps: sync.items, skewed: isSynced ? !healthy : true };
}

// Proje seçicideki etiket: onay bekleyen öneri kartı olan projenin önünde 📥, arka planda kuyruk koşusu süren projenin önünde ⏳ rozeti.
export function repoOptionLabel(r) {
  // ⏳: bu projede arka planda bir kuyruk koşusu sürüyor (başka projedeyken de görünsün).
  return `${r?.queueRunning ? '⏳ ' : ''}${r?.pendingProposal ? '📥 ' : ''}${r?.name || ''}`;
}

function renderRepoSelect(snap) {
  if (!el.repoSelect) return;
  const currentVal = snap.repo.id;
  // Değer VE etiket karşılaştırılır: yalnızca rozet değişince de seçici yeniden çizilir.
  const existingOptions = Array.from(el.repoSelect.options).map((o) => `${o.value}|${o.textContent}`);
  const newOptions = (snap.repos || []).map((r) => `${r.id}|${repoOptionLabel(r)}`);
  const needsRebuild = existingOptions.join(',') !== newOptions.join(',');

  if (needsRebuild || el.repoSelect.options.length === 0) {
    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = repoOptionLabel(r);
      if (r.pendingProposal) opt.title = 'Bu projede onay bekleyen bir öneri kartı var';
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });
    if ((snap.repos || []).length === 0) {
      const opt = document.createElement('option');
      opt.value = '';
      opt.textContent = 'Proje yok — Proje Ekle ile başlayın';
      opt.selected = true;
      el.repoSelect.appendChild(opt);
    }
  } else {
    el.repoSelect.value = currentVal;
  }
}

function renderSyncBadge(snap) {
  if (!el.spineSyncBadge) return;
  const m = syncBadgeModel(snap.repo);
  if (el.canvasSyncText) {
    el.canvasSyncText.textContent = m.text;
  } else {
    el.spineSyncBadge.textContent = m.text;
  }
  const stepsEl = document.getElementById('canvas-sync-steps');
  if (stepsEl) {
    stepsEl.hidden = m.steps.length === 0;
    stepsEl.innerHTML = m.steps
      .map((i) => `<span class="sync-step ${i.tone}" title="${esc(i.title)}">${esc(i.text)}</span>`)
      .join('');
  }
  if (el.canvasSyncDot) {
    el.canvasSyncDot.className = `sync-dot ${m.healthy ? 'synced' : 'pending'}`;
  }
  if (m.isSynced) {
    el.spineSyncBadge.classList.toggle('skewed', m.skewed);
  } else {
    el.spineSyncBadge.classList.add('skewed');
  }
  el.spineSyncBadge.title = m.title;
}

export function renderTopbar(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap) return;

  const hint = loadingHintView(snap.repo);
  if (el.loadingHint) {
    el.loadingHint.hidden = !hint;
    if (hint) {
      el.loadingHint.textContent = hint.text;
      el.loadingHint.setAttribute('title', hint.title);
    }
  }

  const view = modeView(snap.repo, state.offlineMode);
  document.body.classList.toggle('no-project', view.noProject);
  document.body.classList.toggle('demo-mode', view.demo);

  if (el.modeBadge) el.modeBadge.textContent = `${snap.repo.name} · ${modeLabel(snap.repo, state.offlineMode)}`;
  if (el.engineStatus) el.engineStatus.textContent = snap.repo.mode === 'live' ? '● Canlı Repo Bağlı' : (snap.repo.mode === 'none' ? '● Proje yok' : '● Simülasyon Hazır');

  renderRepoSelect(snap);
  const head = headChipModel(snap.repo, snap.branches);
  if (el.headChip) {
    el.headChip.hidden = !head.show;
    el.headChip.textContent = head.text;
    el.headChip.title = head.title;
    el.headChip.className = `head-chip ${head.tone}`;
  }

  if (el.btnRemoveProject) el.btnRemoveProject.hidden = snap.repo.mode !== 'live';
  const cleanable = cleanableCount(snap.branches);
  if (el.pruneCount) el.pruneCount.textContent = String(cleanable);

  const btnPrune = document.getElementById('btn-prune-gone');
  if (btnPrune) btnPrune.disabled = cleanable === 0 && snap.repo.mode !== 'live';
  const btnSync = document.getElementById('btn-sync-spine');
  if (btnSync) btnSync.disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;

  const mainCI = mainCIView(snap.repo);
  if (el.btnMainCI) {
    el.btnMainCI.hidden = !mainCI;
    if (mainCI) {
      el.btnMainCI.textContent = mainCI.text;
      el.btnMainCI.className = `btn ${mainCI.btnClass}`;
      el.btnMainCI.title = mainCI.title;
      el.btnMainCI.dataset.url = mainCI.url;
    }
  }

  const prot = protectionView(snap.repo);
  if (el.btnProtectBranch) {
    el.btnProtectBranch.textContent = prot.button;
    el.btnProtectBranch.className = `btn ${prot.btnClass}`;
    el.btnProtectBranch.title = snap.repo.protectionSummary || 'GitHub main dal koruma ve güvenlik durumu';
  }

  const ver = versionView(snap.repo);
  if (el.btnUpdateCheck) {
    el.btnUpdateCheck.textContent = ver.text;
    el.btnUpdateCheck.className = ver.className;
  }

  if (el.perfCacheBadge) el.perfCacheBadge.textContent = `⚡ ${snap.repo.cacheHits || 0} SHA hit`;

  renderSyncBadge(snap);
}

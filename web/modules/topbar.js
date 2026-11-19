// web/modules/topbar.js - Üst çubuk (mod rozeti, depo seçici, temizle/koruma/sürüm düğmeleri) ve sağ üstteki
// "main ↔ origin/main" senkron rozeti. Metin ve durum kararları saf fonksiyonlardadır (Node'da test edilir);
// `renderTopbar` yalnızca bunları DOM'a yazar.
import { state, el } from './state.js';
import { esc } from './utils.js';
import { summarizeSync, protectionView } from './syncsteps.js';

export function modeLabel(repo, offline) {
  if (offline) return 'Offline Simülasyon';
  return repo.mode === 'live' ? 'Canlı Git · Aktif' : 'Simülasyon · Etkileşimli';
}

export function versionView(repo) {
  const text = `v${repo.appVersion || '0.9.0'}${repo.teamMode ? ' · TEAM' : ''}${repo.updateAvailable ? ' ⬆ Güncelle' : ''}`;
  return { text, className: `btn mono ${repo.updateAvailable ? 'warn' : 'ghost'}` };
}

// Temizlenebilir (silinmiş/birleşmiş kalıntı) dal sayısı.
export function cleanableCount(branches) {
  return (branches || []).filter((b) => !b.longLived && (b.originGone || b.state === 'gone' || (b.state === 'merged' && b.hasLocal))).length;
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

function renderRepoSelect(snap) {
  if (!el.repoSelect) return;
  const currentVal = snap.repo.id;
  const existingOptions = Array.from(el.repoSelect.options).map((o) => o.value);
  const newOptions = (snap.repos || []).map((r) => r.id);
  const needsRebuild = existingOptions.join(',') !== newOptions.join(',');

  if (needsRebuild || el.repoSelect.options.length === 0) {
    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = r.name;
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });
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

  if (el.modeBadge) el.modeBadge.textContent = `${snap.repo.name} · ${modeLabel(snap.repo, state.offlineMode)}`;
  if (el.engineStatus) el.engineStatus.textContent = snap.repo.mode === 'live' ? '● Canlı Repo Bağlı' : '● Simülasyon Hazır';

  renderRepoSelect(snap);

  if (el.btnRemoveProject) el.btnRemoveProject.hidden = snap.repo.mode !== 'live';
  const cleanable = cleanableCount(snap.branches);
  if (el.pruneCount) el.pruneCount.textContent = String(cleanable);

  const btnPrune = document.getElementById('btn-prune-gone');
  if (btnPrune) btnPrune.disabled = cleanable === 0 && snap.repo.mode !== 'live';
  const btnSync = document.getElementById('btn-sync-spine');
  if (btnSync) btnSync.disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;

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

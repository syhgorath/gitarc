// web/modules/dock.js - Bottom Dock, Hata Radarı and live logging panel
import { state, el } from './state.js';
import { esc, truncate, showToast } from './utils.js';
import { apiCall } from './api.js';

export function getMergedLogs() {
  const serverLogs = state.snapshot?.logs || [];
  const seenMsgs = new Set(serverLogs.map((l) => `${l.level}:${l.message}`));
  const extra = state.clientLogs.filter((l) => !seenMsgs.has(`${l.level}:${l.message}`));
  return [...extra, ...serverLogs].slice(0, 60);
}

export function renderLogBadge(onRenderModal = null) {
  const logs = getMergedLogs();
  if (el.logsModal && !el.logsModal.hidden && onRenderModal) {
    onRenderModal();
  }
}

export function recordClientLog(level, action, message, detail = '', onRenderModal = null) {
  const now = new Date();
  const ts = now.toTimeString().slice(0, 8);
  state.clientLogs.unshift({
    id: `cli-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
    timestamp: ts,
    level,
    action,
    message: String(message || ''),
    detail: String(detail || ''),
  });
  if (state.clientLogs.length > 40) {
    state.clientLogs.length = 40;
  }
  renderLogBadge(onRenderModal);
}

// "Sistem Logları" sekmesindeki sayı: toplam günlük sayısı; en az bir hata varsa kırmızı ve ipucunda hata sayısı.
export function logBadgeModel(logs) {
  const list = logs || [];
  const errors = list.filter((l) => l.level === 'error').length;
  return {
    count: list.length,
    errors,
    tone: errors > 0 ? 'danger' : '',
    title: errors > 0 ? `${errors} hata içeriyor` : 'Hata yok',
  };
}

// "Son İşlemler": günlüğün BAŞARILI (level "ok") satırları, yani kullanıcının/ajanın yaptığı ve tamamlanan işlemler
// ("PR #16 birleştirildi", "dal güncellendi", "koşu bitti"). Hata satırları Hata Radarı'nda/Sistem Loglarında kalır, bilgi
// satırları yalnızca Sistem Loglarında; sürüm kontrolü gibi kendiliğinden yapılan işlemler "işlem" sayılmaz. Veri tek yerde
// (snapshot.logs); ayrı bir depolama yok.
export const AUTOMATIC_ACTIONS = new Set(['Sürüm Kontrolü']);

export function recentActionsModel(logs, limit = 30) {
  return (logs || [])
    .filter((l) => l && l.level === 'ok' && !AUTOMATIC_ACTIONS.has(l.action))
    .slice(0, limit);
}

export function renderDockStreams() {
  const logs = getMergedLogs();
  const actionLogs = recentActionsModel(logs);

  if (el.dockLogCount) {
    const badge = logBadgeModel(logs);
    el.dockLogCount.textContent = String(badge.count);
    el.dockLogCount.classList.toggle('danger', badge.errors > 0);
    el.dockLogCount.setAttribute('title', badge.title);
  }
  if (el.dockActionCount) el.dockActionCount.textContent = actionLogs.length;

  if (el.dockLogsStream) {
    el.dockLogsStream.innerHTML = '';
    if (logs.length === 0) {
      el.dockLogsStream.innerHTML = '<div class="empty-detail">Henüz sistem veya Git logu yok.</div>';
    } else {
      logs.slice(0, 50).forEach((l) => {
        const item = document.createElement('div');
        item.className = `dock-log-item ${l.level || 'info'}`;
        item.innerHTML = `
          <span class="dock-log-time">${esc(l.timestamp || '')}</span>
          <span class="dock-log-act">${esc(l.action || '')}</span>
          <span class="dock-log-msg">${esc(l.message || '')}</span>
          ${l.detail ? `<span class="dock-log-detail">${esc(truncate(l.detail, 60))}</span>` : ''}
        `;
        el.dockLogsStream.appendChild(item);
      });
    }
  }

  if (el.dockActionsStream) {
    el.dockActionsStream.innerHTML = '';
    if (actionLogs.length === 0) {
      el.dockActionsStream.innerHTML = '<div class="empty-detail">Henüz tamamlanmış bir işlem yok. Başarılı işlemler (birleştirme, dal güncelleme, kuyruk koşusu…) burada görünür; hatalar Hata Radarı ve Sistem Loglarındadır.</div>';
    } else {
      actionLogs.forEach((a) => {
        const item = document.createElement('div');
        item.className = 'dock-log-item ok';
        item.innerHTML = `
          <span class="dock-log-time">${esc(a.timestamp || '')}</span>
          <span class="dock-log-act">${esc(a.action || '')}</span>
          <span class="dock-log-msg">${esc(a.branch ? a.branch + ' · ' : '')}${esc(a.message || '')}</span>
          ${a.detail ? `<span class="dock-log-detail">${esc(truncate(a.detail, 60))}</span>` : ''}
        `;
        el.dockActionsStream.appendChild(item);
      });
    }
  }
}

// Hata Radarı: İŞLEM HATALARI. Günlükteki hata (`level: error`) satırları, kullanıcının bir şey yapması gereken işlemlerin
// (merge, push, dal güncelleme, kuyruk adımı/koşusu, koruma, onay akışı…) başarısızlığıdır; günlükte kaybolmasın, radara
// kart olarak düşsün. Kendiliğinden yapılan ağ okumaları (anlık görüntü fetch'i, sürüm kontrolü) "işlem hatası" değildir:
// onlar zaten "Synced" rozetinde/SyncSteps'te görünür.
export const NON_ACTION_ERRORS = new Set(['Snapshot', 'Sürüm Kontrolü']);

/** Aynı (eylem, dal, mesaj) hatası tekrar ederse TEK kart + sayaç. dismissed[anahtar] === son günlük kimliği ise kapatılmıştır;
 *  hata YENİDEN olursa (yeni günlük kimliği) kart yeniden çıkar. Günlük en yeni önce gelir. */
export function actionFailureCards(logs, dismissed = {}, limit = 8) {
  const groups = new Map();
  (logs || []).forEach((l) => {
    if (!l || l.level !== 'error' || NON_ACTION_ERRORS.has(l.action)) return;
    const key = [l.action || '', l.branch || '', l.message || ''].join('\u0001');
    const g = groups.get(key);
    if (g) { g.count += 1; return; }
    const isQueue = /^Kuyruk/.test(l.action || '');
    groups.set(key, {
      key, count: 1, latestId: l.id, time: l.timestamp || '', action: l.action || 'İşlem', branch: l.branch || '',
      message: l.message || '', detail: l.detail || '', isQueue,
      severity: isQueue ? 'critical' : 'warning',
      title: isQueue && /Koşu/.test(l.action || '') ? 'Kuyruk koşusu durdu' : `${l.action || 'İşlem'} başarısız`,
    });
  });
  return [...groups.values()].filter((g) => { const d = dismissed[g.key]; return d === undefined || d !== g.latestId; }).slice(0, limit);
}

function buildFailureCard(c, callbacks) {
  const { onSelectBranch, onOpenQueue } = callbacks;
  const card = document.createElement('div');
  card.className = `radar-card ${c.severity} action-failure`;
  const badgeClass = c.severity === 'critical' ? 'danger' : 'warn';
  const badgeLabel = c.severity === 'critical' ? '🚨 KRİTİK' : '⚠️ UYARI';
  card.innerHTML = `
      <div class="rc-header">
        <div class="rc-title-group">
          <span class="rc-badge ${badgeClass}">${badgeLabel} · İşlem Hatası</span>
          ${c.branch ? `<span class="rc-branch-badge">${esc(c.branch)}</span>` : ''}
          <strong class="rc-title">${esc(c.title)}</strong>
          ${c.count > 1 ? `<span class="rc-count" title="Aynı hata ${c.count} kez oldu">×${c.count}</span>` : ''}
        </div>
        <div class="rc-actions">
          ${c.isQueue && onOpenQueue ? '<button type="button" class="rc-action-btn" data-act="open-queue">🚦 Kuyruk Penceresini Aç</button>' : ''}
          <button type="button" class="rc-action-btn rc-dismiss" data-act="dismiss" aria-label="Kartı kapat" title="Kapat (aynı hata yeniden olursa tekrar görünür)">✕</button>
        </div>
      </div>
      <div class="rc-body">
        <div class="rc-detail-text">${esc(c.message)}</div>
        ${c.detail ? `<div class="rc-ci-snippet">${esc(truncate(c.detail, 240))}</div>` : ''}
        <div class="rc-detail-text rc-time">${esc(c.time)} · ${esc(c.action)}</div>
      </div>`;
  card.querySelector('[data-act="open-queue"]')?.addEventListener('click', (ev) => { ev.stopPropagation(); onOpenQueue(); });
  card.querySelector('[data-act="dismiss"]')?.addEventListener('click', (ev) => {
    ev.stopPropagation();
    state.radarDismissed[c.key] = c.latestId;
    renderRadar(callbacks);
  });
  card.addEventListener('click', () => {
    const b = (state.snapshot?.branches || []).find((x) => x.name === c.branch);
    if (b && onSelectBranch) onSelectBranch(b.id);
  });
  return card;
}

export function renderRadar(callbacks = {}) {
  const { onSelectBranch, onOpenConflictStudio, onPruneGone } = callbacks;
  const diags = state.snapshot?.diagnostics || [];
  const failures = actionFailureCards(state.snapshot?.logs, state.radarDismissed);
  if (el.dockRadarCount) el.dockRadarCount.textContent = diags.length + failures.length;
  if (!el.radarList) return;
  el.radarList.innerHTML = '';
  failures.forEach((c) => el.radarList.appendChild(buildFailureCard(c, callbacks)));

  if (diags.length === 0 && failures.length === 0) {
    const clean = document.createElement('div');
    clean.className = 'empty-detail';
    clean.textContent = 'Radar temiz — çakışma, Cross-PR, CI veya işlem hatası yok.';
    el.radarList.appendChild(clean);
    renderDockStreams();
    return;
  }
  if (diags.length === 0) { renderDockStreams(); return; }

  const branches = state.snapshot?.branches || [];

  diags.forEach((d) => {
    const b = branches.find((x) => x.id === d.branchId || x.name === d.branchName);
    const card = document.createElement('div');
    const sev = d.severity || 'warning';
    card.className = `radar-card ${sev}`;

    let badgeClass = 'warn';
    let badgeLabel = '⚠️ UYARI';
    if (sev === 'critical') {
      badgeClass = 'danger';
      badgeLabel = '🚨 KRİTİK';
    } else if (sev === 'info') {
      badgeClass = 'sky';
      badgeLabel = 'ℹ️ BİLGİ';
    }

    let kindLabel = '';
    if (d.kind === 'ci_failure') kindLabel = 'CI Test Hatası';
    else if (d.kind === 'conflict') kindLabel = 'Merge Çakışması';
    else if (d.kind === 'cross_pr') kindLabel = 'Cross-PR Çarpışması';
    else if (d.kind === 'prune_candidate') kindLabel = 'Silinmiş/Kalıntı Dal';
    else if (d.kind === 'main_ci_failure') kindLabel = 'Omurga Testleri Kırık';
    else if (d.kind === 'stale_branch') kindLabel = 'Eskimiş Dal';

    const files = d.files || b?.conflict?.files || [];
    let filesHtml = '';
    if (files.length > 0) {
      filesHtml = files.map((f) => `<span class="rc-file-tag">${esc(f)}</span>`).join('');
    }

    let ciSnippet = '';
    if (b?.pr?.ci?.failureSummary) {
      ciSnippet = b.pr.ci.failureSummary;
    } else if (b?.pr?.ci?.rawLog && (b?.pr?.ci?.state === 'FAILURE' || b?.pr?.ci?.status === 'failure')) {
      ciSnippet = b.pr.ci.rawLog.split('\n').filter(Boolean).slice(0, 3).join('\n');
    }

    let actBtnText = '🎯 Odaklan & İncele';
    let actAction = 'select';
    if (d.kind === 'conflict') {
      actBtnText = '⚡ Çakışma Stüdyosu';
      actAction = 'studio';
    } else if (d.kind === 'ci_failure') {
      actBtnText = '🔍 CI Hatasını Gör';
      actAction = 'select';
    } else if (d.kind === 'prune_candidate') {
      actBtnText = '🧹 Dalı Temizle';
      actAction = 'prune';
    } else if (d.kind === 'main_ci_failure') {
      actBtnText = '🔗 Çalıştırmayı Aç';
      actAction = 'open-url';
    }

    const suggestedCmd = b?.suggestedCmd || d.actionHint || '';

    card.innerHTML = `
      <div class="rc-header">
        <div class="rc-title-group">
          <span class="rc-badge ${badgeClass}">${badgeLabel}${kindLabel ? ' · ' + kindLabel : ''}</span>
          <span class="rc-branch-badge">${esc(d.branchName || 'Genel')}</span>
          <strong class="rc-title">${esc(d.title)}</strong>
        </div>
        <div class="rc-actions">
          <button type="button" class="rc-action-btn" data-act="${actAction}" data-branch="${esc(d.branchId || '')}" data-url="${esc(d.url || '')}">${actBtnText}</button>
        </div>
      </div>
      <div class="rc-body">
        <div class="rc-detail-text">${esc(d.detail)}</div>
        ${filesHtml ? `<div class="rc-files-row"><span class="rc-files-label">Etkilenen Dosyalar:</span> ${filesHtml}</div>` : ''}
        ${ciSnippet ? `<div class="rc-ci-snippet">${esc(ciSnippet)}</div>` : ''}
        ${suggestedCmd ? `
          <div class="rc-cmd-row">
            <code class="rc-cmd-code">${esc(suggestedCmd)}</code>
            <button type="button" class="rc-action-btn copy-cmd-btn" data-cmd="${esc(suggestedCmd)}" title="Komutu panoya kopyala">📋</button>
          </div>
        ` : ''}
      </div>
    `;

    card.querySelector('button[data-act]')?.addEventListener('click', (ev) => {
      ev.stopPropagation();
      const act = ev.currentTarget.dataset.act;
      const brId = ev.currentTarget.dataset.branch;
      if (act === 'open-url') {
        const url = ev.currentTarget.dataset.url || '';
        if (url.startsWith('https://')) window.open(url, '_blank', 'noopener');
        else showToast('Çalıştırma bağlantısı bulunamadı', 'warn');
      } else if (act === 'studio' && brId && onOpenConflictStudio) {
        onOpenConflictStudio(brId);
      } else if (act === 'prune' && onPruneGone) {
        onPruneGone();
      } else if (brId && onSelectBranch) {
        onSelectBranch(brId);
      }
    });

    card.querySelector('.copy-cmd-btn')?.addEventListener('click', async (ev) => {
      ev.stopPropagation();
      const cmd = ev.currentTarget.dataset.cmd;
      if (!cmd) return;
      try {
        await navigator.clipboard.writeText(cmd);
        showToast('Komut panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi', 'warn');
      }
    });

    card.addEventListener('click', () => {
      if (d.branchId && onSelectBranch) onSelectBranch(d.branchId);
    });

    el.radarList.appendChild(card);
  });

  renderDockStreams();
}

export function setupBottomDock(onApplySnapshot = null) {
  const tabs = [
    { tab: el.dockTabRadar, pane: el.dockPaneRadar },
    { tab: el.dockTabLogs, pane: el.dockPaneLogs },
    { tab: el.dockTabActions, pane: el.dockPaneActions },
  ];

  tabs.forEach(({ tab, pane }) => {
    tab?.addEventListener('click', () => {
      tabs.forEach((t) => {
        t.tab?.classList.remove('active');
        t.tab?.setAttribute('aria-selected', 'false');
        t.pane?.classList.add('hidden');
        t.pane?.classList.remove('active');
      });
      tab.classList.add('active');
      tab.setAttribute('aria-selected', 'true');
      pane?.classList.remove('hidden');
      pane?.classList.add('active');
    });
  });

  el.dockToggleBtn?.addEventListener('click', () => {
    const isCollapsed = el.dock?.classList.toggle('dock-collapsed');
    if (el.dockToggleBtn) {
      el.dockToggleBtn.textContent = isCollapsed ? '▲' : '▼';
    }
  });

  el.dockCopyBtn?.addEventListener('click', async () => {
    const logs = state.snapshot?.logs || [];
    const client = state.clientLogs || [];
    const all = [...logs, ...client];
    if (all.length === 0) {
      showToast('Kopyalanacak log kaydı bulunmuyor', 'info');
      return;
    }
    const text = all.map((l) => `[${l.timestamp || ''}] [${(l.level || 'info').toUpperCase()}] ${l.action || ''}: ${l.message || ''}${l.detail ? ` (${l.detail})` : ''}`).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      showToast(`✓ ${all.length} adet log panoya kopyalandı`, 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  el.dockClearBtn?.addEventListener('click', async () => {
    state.clientLogs = [];
    if (!state.offlineMode) {
      try {
        const snap = await apiCall('/api/logs/clear', 'POST', {});
        if (onApplySnapshot) onApplySnapshot(snap);
      } catch (err) {
        showToast(err.message, 'danger');
      }
    }
    renderLogBadge();
    renderDockStreams();
    showToast('Loglar temizlendi', 'info');
  });
}

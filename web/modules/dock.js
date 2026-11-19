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

export function renderDockStreams() {
  const logs = getMergedLogs();
  const actionLogs = state.snapshot?.actionLogs || [];

  if (el.dockLogCount) el.dockLogCount.textContent = logs.length;
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
      el.dockActionsStream.innerHTML = '<div class="empty-detail">Henüz kaydedilmiş işlem yok.</div>';
    } else {
      actionLogs.slice(0, 50).forEach((a) => {
        const item = document.createElement('div');
        item.className = `dock-log-item ${a.level === 'error' ? 'danger' : (a.level === 'warn' ? 'warn' : 'ok')}`;
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

export function renderRadar(callbacks = {}) {
  const { onSelectBranch, onOpenConflictStudio, onPruneGone } = callbacks;
  const diags = state.snapshot?.diagnostics || [];
  if (el.dockRadarCount) el.dockRadarCount.textContent = diags.length;
  if (!el.radarList) return;
  el.radarList.innerHTML = '';

  if (diags.length === 0) {
    const clean = document.createElement('div');
    clean.className = 'empty-detail';
    clean.textContent = 'Radar temiz — çakışma, Cross-PR veya CI hatası yok.';
    el.radarList.appendChild(clean);
    renderDockStreams();
    return;
  }

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

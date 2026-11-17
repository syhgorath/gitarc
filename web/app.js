(() => {
  'use strict';

  const SVG_NS = 'http://www.w3.org/2000/svg';
  const XHTML_NS = 'http://www.w3.org/1999/xhtml';
  const MAIN_Y = 300;
  const SPINE_START_X = 140;
  const SPINE_STEP_X = 156;
  const LANE_BASE_OFFSET = 100;
  const LANE_STEP_Y = 98;
  const FO_W = 222;
  const FO_H = 114;
  const MERGED_FO_W = 188;
  const MERGED_FO_H = 34;
  const OPEN_CAPSULE_MIN_GAP_X = 238;
  const REPO_STORAGE_KEY = 'gitarc_repo_id';

  const state = {
    snapshot: null,
    selectedBranchId: null,
    selectedNodeSha: null,
    filter: 'all', // 'all' | 'mine' | 'issues' | 'ci_fail' | 'lower' | 'gone'
    pathLens: '',
    showMergedArches: true,
    mergeStrategy: 'merge',
    offlineMode: false,
    lastSig: '',
    zoom: 1,
    panX: 0,
    panY: 0,
    panning: null,
    drag: null,
    localRunOutput: {},
    pendingAutoMerge: {},
    autoMergingInFlight: {},
    draftPrEdits: {},
    studio: null,
    clientLogs: [],
  };

  const el = {
    svg: document.getElementById('rail-svg'),
    viewport: document.getElementById('svg-viewport'),
    repoSelect: document.getElementById('repo-select'),
    modeBadge: document.getElementById('mode-badge'),
    kpiPills: document.getElementById('kpi-pills'),
    radarList: document.getElementById('radar-list'),
    radarCount: document.getElementById('radar-count'),
    dock: document.getElementById('bottom-dock'),
    dockTabRadar: document.getElementById('dock-tab-radar'),
    dockTabLogs: document.getElementById('dock-tab-logs'),
    dockTabActions: document.getElementById('dock-tab-actions'),
    dockRadarCount: document.getElementById('dock-radar-count'),
    dockLogCount: document.getElementById('dock-log-count'),
    dockActionCount: document.getElementById('dock-action-count'),
    dockToggleBtn: document.getElementById('dock-toggle-btn'),
    dockClearBtn: document.getElementById('dock-clear-btn'),
    dockPaneRadar: document.getElementById('dock-pane-radar'),
    dockPaneLogs: document.getElementById('dock-pane-logs'),
    dockPaneActions: document.getElementById('dock-pane-actions'),
    dockLogsStream: document.getElementById('dock-logs-stream'),
    dockActionsStream: document.getElementById('dock-actions-stream'),
    detail: document.getElementById('inspector-detail'),
    pruneCount: document.getElementById('prune-count'),
    dragHint: document.getElementById('drag-hint'),
    spineSyncBadge: document.getElementById('spine-sync-badge'),
    engineStatus: document.getElementById('engine-status'),
    zoomReset: document.getElementById('zoom-reset'),
    toggleMerged: document.getElementById('toggle-merged-arches'),
    modal: document.getElementById('create-branch-modal'),
    newBranchForm: document.getElementById('new-branch-form'),
    newBranchName: document.getElementById('new-branch-name'),
    newBranchBase: document.getElementById('new-branch-base'),
    toastStack: document.getElementById('toast-stack'),
    conflictStudioModal: document.getElementById('conflict-studio-modal'),
    studioBranchBadge: document.getElementById('studio-branch-badge'),
    studioFileTabs: document.getElementById('studio-file-tabs'),
    studioGuideBox: document.getElementById('studio-guide-box'),
    studioGuideText: document.getElementById('studio-guide-text'),
    studioStrategyBar: document.getElementById('studio-strategy-bar'),
    studioLineBadge: document.getElementById('studio-line-badge'),
    studioModeBadge: document.getElementById('studio-mode-badge'),
    studioLeftConflict: document.getElementById('studio-left-conflict'),
    studioRightPreview: document.getElementById('studio-right-preview'),
    studioErrorBanner: document.getElementById('studio-error-banner'),
    studioErrorText: document.getElementById('studio-error-text'),
    studioRunVet: document.getElementById('studio-run-vet'),
    studioApplyBtn: document.getElementById('studio-apply'),
    mcpModal: document.getElementById('mcp-connect-modal'),
    mcpConfigSnippet: document.getElementById('mcp-config-snippet'),
    btnLogsModal: document.getElementById('btn-logs-modal'),
    logCount: document.getElementById('log-count'),
    logsModal: document.getElementById('logs-modal'),
    logsList: document.getElementById('logs-list'),
    btnProtectBranch: document.getElementById('btn-protect-branch'),
    btnUpdateCheck: document.getElementById('btn-update-check'),
    lensPathInput: document.getElementById('lens-path-input'),
    perfCacheBadge: document.getElementById('perf-cache-badge'),
    securityUpdateModal: document.getElementById('security-update-modal'),
    secProtBadge: document.getElementById('sec-prot-badge'),
    secProtSummary: document.getElementById('sec-prot-summary'),
    btnEnableProtection: document.getElementById('btn-enable-protection'),
    secTeamChip: document.getElementById('sec-team-chip'),
    secUpdateStatus: document.getElementById('sec-update-status'),
    secUpdateCmd: document.getElementById('sec-update-cmd'),
    btnAddProject: document.getElementById('btn-add-project'),
    btnRemoveProject: document.getElementById('btn-remove-project'),
    addProjectModal: document.getElementById('add-project-modal'),
    addProjectForm: document.getElementById('add-project-form'),
    projectPathInput: document.getElementById('project-path-input'),
    projectNameInput: document.getElementById('project-name-input'),
    addProjClose: document.getElementById('add-proj-close'),
    addProjCancel: document.getElementById('add-proj-cancel'),
    btnStartTour: document.getElementById('btn-start-tour'),
    tourBackdrop: document.getElementById('tour-backdrop'),
    tourCard: document.getElementById('tour-card'),
    tourStepBadge: document.getElementById('tour-step-badge'),
    tourStepTitle: document.getElementById('tour-step-title'),
    tourStepBody: document.getElementById('tour-step-body'),
    tourPrevBtn: document.getElementById('tour-prev-btn'),
    tourNextBtn: document.getElementById('tour-next-btn'),
    tourCloseBtn: document.getElementById('tour-close-btn'),
    tourDots: document.getElementById('tour-dots'),
  };

  // Fallback simulation engine for standalone file:// opening without Go server (defined in fallback.js)
  const LocalFallbackEngine = window.LocalFallbackEngine;

  async function apiCall(path, method = 'GET', body = null) {
    if (state.offlineMode || window.location.protocol === 'file:') {
      throw new Error('offline');
    }
    const opts = {
      method,
      headers: {
        'X-Requested-With': 'gitarc',
      },
      credentials: 'same-origin',
    };
    if (body !== null) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const resp = await fetch(path, opts);
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return data;
  }

  function getMergedLogs() {
    const serverLogs = state.snapshot?.logs || [];
    const seenMsgs = new Set(serverLogs.map((l) => `${l.level}:${l.message}`));
    const extra = state.clientLogs.filter((l) => !seenMsgs.has(`${l.level}:${l.message}`));
    return [...extra, ...serverLogs].slice(0, 60);
  }

  function renderLogBadge() {
    const logs = getMergedLogs();
    const errCount = logs.filter((l) => l.level === 'error').length;
    el.logCount.textContent = String(errCount > 0 ? errCount : logs.length);
    el.btnLogsModal.classList.toggle('danger', errCount > 0);
    if (!el.logsModal.hidden) {
      renderLogsModal();
    }
  }

  function recordClientLog(level, action, message, detail = '') {
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
    renderLogBadge();
  }

  function showToast(msg, tone = 'info') {
    if (tone === 'danger') {
      recordClientLog('error', 'Arayüz / API Hatası', msg);
    }
    const t = document.createElement('div');
    t.className = `toast ${tone}`;
    t.title = 'Metni panoya kopyalamak için tıklayın';
    t.textContent = msg;
    t.addEventListener('click', () => {
      navigator.clipboard?.writeText(msg).catch(() => {});
      t.classList.add('fade-out');
      setTimeout(() => t.remove(), 180);
    });
    el.toastStack.appendChild(t);
    const timeoutMs = tone === 'danger' ? 7500 : 3200;
    setTimeout(() => {
      t.classList.add('fade-out');
      setTimeout(() => t.remove(), 260);
    }, timeoutMs);
  }

  async function runWithBtnBusy(btnEl, busyText, fn) {
    if (!btnEl) return await fn();
    const origText = btnEl.innerHTML;
    btnEl.disabled = true;
    btnEl.classList.add('btn-busy');
    btnEl.textContent = busyText || '⏳ İşleniyor...';

    const statusEl = el.engineStatus;
    const origStatus = statusEl ? statusEl.textContent : '';
    if (statusEl) {
      statusEl.textContent = '● Git İşlemi Yürütülüyor...';
      statusEl.classList.add('running-pulse');
    }

    try {
      return await fn();
    } finally {
      btnEl.disabled = false;
      btnEl.classList.remove('btn-busy');
      btnEl.innerHTML = origText;
      if (statusEl) {
        statusEl.textContent = origStatus || '● Hazır';
        statusEl.classList.remove('running-pulse');
      }
    }
  }

  function svgEl(tag, attrs = {}, text = null) {
    const node = document.createElementNS(SVG_NS, tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (v !== undefined && v !== null) {
        node.setAttribute(k, String(v));
      }
    }
    if (text !== null) {
      node.textContent = text;
    }
    return node;
  }

  function esc(str) {
    return String(str ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function truncate(str, max) {
    if (!str) return '';
    return str.length > max ? str.slice(0, max - 1) + '…' : str;
  }

  function snapshotSignature(snap) {
    if (!snap) return '';
    const r = snap.repo || {};
    const lastNode = snap.nodes?.[snap.nodes.length - 1]?.shortSha || '';
    const bSig = (snap.branches || []).map((b) => [
      b.id, b.state, b.hemisphere, b.isHead ? 1 : 0, b.hasLocal ? 1 : 0, b.aheadMain, b.behindMain,
      b.uncommittedCount, b.lastCommitMsg, b.draftPrTitle || '', b.draftPrBody || '',
      (b.changedFiles || []).length, b.originGone ? 1 : 0,
      b.pr?.title || '', b.pr?.body || '', b.pr?.ci?.state || '', b.pr?.ci?.duration || '',
      b.conflict?.hasConflict ? 1 : 0,
    ].join('|')).join(';');
    const logSig = `${snap.logs?.length || 0}:${snap.logs?.[0]?.id || ''}`;
    return `${r.id}:${r.ahead}:${r.behind}:${r.dirtyCount}:${snap.nodes?.length || 0}:${lastNode}:${bSig}:${logSig}`;
  }

  async function bootstrap() {
    setupBottomDock();
    try {
      let snap = await apiCall('/api/snapshot');
      const savedId = localStorage.getItem(REPO_STORAGE_KEY);
      const isSavedSimulation = savedId && savedId.startsWith('sim-');

      // Eğer sunucu canlı bir git deposuyla açıldıysa (mode === 'live'),
      // localStorage'daki eski simülasyon (sim-*) sunucuyu zorla ezmemeli!
      if (savedId && savedId !== snap.repo.id && (snap.repos || []).some((r) => r.id === savedId)) {
        if (!(snap.repo.mode === 'live' && isSavedSimulation)) {
          try {
            snap = await apiCall('/api/repo/switch', 'POST', { repoId: savedId });
          } catch {
            // Keep initial snapshot if switch fails
          }
        }
      }
      localStorage.setItem(REPO_STORAGE_KEY, snap.repo.id);
      applySnapshot(snap);
    } catch {
      state.offlineMode = true;
      const snap = LocalFallbackEngine.snapshot();
      applySnapshot(snap);
      showToast('Sunucusuz tarayıcı simülasyonu aktif (Offline Mode)', 'info');
    }
  }

  // 4-second silent live polling when user is idle (not dragging or typing)
  setInterval(async () => {
    if (state.offlineMode || state.drag || state.panning || !el.modal.hidden || !el.conflictStudioModal.hidden || !el.logsModal.hidden || !el.mcpModal.hidden || !el.securityUpdateModal.hidden) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    try {
      const nextSnap = await apiCall('/api/snapshot');
      const sig = snapshotSignature(nextSnap);
      if (sig && sig !== state.lastSig) {
        applySnapshot(nextSnap);
      }
    } catch {
      // Ignore transient polling errors
    }
  }, 4000);

  function applySnapshot(snap) {
    state.snapshot = snap;
    state.lastSig = snapshotSignature(snap);

    const queuedIds = Object.keys(state.pendingAutoMerge || {});
    queuedIds.forEach((branchId) => {
      const queued = state.pendingAutoMerge[branchId];
      if (!queued) return;
      const target = (snap.branches || []).find((x) => x.id === branchId || (queued.branchName && x.name === queued.branchName));
      if (!target || target.state === 'merged') {
        delete state.pendingAutoMerge[branchId];
        return;
      }
      const ci = target.pr?.ci?.state;
      if (ci === 'FAILURE') {
        delete state.pendingAutoMerge[branchId];
        showToast(`❌ ${target.name} dalında CI başarısız oldu; otomatik birleştirme iptal edildi`, 'danger');
        return;
      }
      if (target.conflict?.hasConflict && !target.conflict.resolved) {
        delete state.pendingAutoMerge[branchId];
        showToast(`⚠ ${target.name} dalında çakışma tespit edildi; otomatik birleştirme durduruldu`, 'danger');
        return;
      }
      if (ci === 'SUCCESS' && !state.autoMergingInFlight[branchId]) {
        state.autoMergingInFlight[branchId] = true;
        delete state.pendingAutoMerge[branchId];
        executeMergeNow(target.id, queued.strategy || 'merge', true).finally(() => {
          delete state.autoMergingInFlight[branchId];
        });
      }
    });

    if (!state.selectedBranchId && snap.branches?.length) {
      const priority = snap.branches.find((b) => b.pr?.ci?.state === 'FAILURE')
        || snap.branches.find((b) => b.conflict?.hasConflict)
        || snap.branches.find((b) => b.hemisphere === 'lower')
        || snap.branches.find((b) => b.state === 'open')
        || snap.branches[0];
      state.selectedBranchId = priority.id;
    } else if (state.selectedBranchId && !snap.branches.some((b) => b.id === state.selectedBranchId)) {
      state.selectedBranchId = snap.branches[0]?.id || null;
    }

    renderAll();
  }

  function renderAll() {
    renderTopbar();
    renderKPIs();
    renderRadar();
    renderSVG();
    renderInspector();
    populateBaseCommitOptions();
  }

  function renderTopbar() {
    const snap = state.snapshot;
    if (!snap) return;

    const liveLabel = state.offlineMode ? 'Offline Simülasyon' : (snap.repo.mode === 'live' ? 'Canlı Git · Aktif' : 'Simülasyon · Etkileşimli');
    el.modeBadge.textContent = `${snap.repo.name} · ${liveLabel}`;
    el.engineStatus.textContent = snap.repo.mode === 'live' ? '● Canlı Repo Bağlı' : '● Simülasyon Hazır';

    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = r.name;
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });

    el.btnRemoveProject.hidden = snap.repo.mode !== 'live';
    const cleanableCount = (snap.branches || []).filter((b) => b.originGone || b.state === 'gone' || (b.state === 'merged' && b.hasLocal)).length;
    el.pruneCount.textContent = String(cleanableCount);
    document.getElementById('btn-prune-gone').disabled = cleanableCount === 0 && snap.repo.mode !== 'live';
    document.getElementById('btn-new-branch').disabled = false;
    document.getElementById('btn-sync-spine').disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;
    document.getElementById('btn-reset-sim').hidden = snap.repo.mode === 'live';
    renderLogBadge();

    const isProtected = Boolean(snap.repo.branchProtected);
    el.btnProtectBranch.textContent = isProtected ? '🛡️ main Korumalı' : '🔓 Korumasız';
    el.btnProtectBranch.className = `btn ${isProtected ? 'ok' : 'warn'}`;

    const verTxt = `v${snap.repo.appVersion || '0.7.0'}`;
    const teamTag = snap.repo.teamMode ? ' · TEAM' : '';
    const updTag = snap.repo.updateAvailable ? ' ⬆ Güncelle' : '';
    el.btnUpdateCheck.textContent = verTxt + teamTag + updTag;
    el.btnUpdateCheck.className = `btn mono ${snap.repo.updateAvailable ? 'warn' : 'ghost'}`;

    el.perfCacheBadge.textContent = `⚡ ${snap.repo.cacheHits || 0} SHA hit`;

    if (!el.securityUpdateModal.hidden) {
      renderSecurityUpdateModal();
    }

    const ahead = snap.repo.ahead || 0;
    const behind = snap.repo.behind || 0;
    const remoteName = snap.repo.remoteName || 'origin';
    const defBranch = snap.repo.defaultBranch || 'main';
    if (ahead === 0 && behind === 0) {
      el.spineSyncBadge.textContent = `${defBranch} ↔ ${remoteName}/${defBranch} (Synced)`;
      el.spineSyncBadge.classList.remove('skewed');
    } else {
      el.spineSyncBadge.textContent = `${defBranch} ↔ ${remoteName}/${defBranch}  ↑${ahead} · ↓${behind}`;
      el.spineSyncBadge.classList.add('skewed');
    }
  }

  function isMyBranch(b) {
    const me = (state.snapshot?.repo?.currentUser || '').trim().toLowerCase();
    if (!me) return Boolean(b.isHead || b.hasLocal);
    const bAuth = (b.author || '').trim().toLowerCase();
    const prAuth = (b.pr?.author || '').trim().toLowerCase();
    return bAuth === me || prAuth === me || Boolean(b.isHead);
  }

  function isMergedOrGone(b) {
    return Boolean(b.state === 'merged' || b.state === 'gone' || b.originGone);
  }

  function isSecretFile(f, snap = state.snapshot) {
    if (!f) return false;
    const patterns = snap?.repo?.secretPatterns || [
      'img/logoyeni.jpg',
      'referans/**',
      'referans/*',
      'referans',
      '.mcp.json',
      '.env*',
      '*.env',
      '*.pem',
      '*.key',
      'id_*',
      'id_rsa*',
      'id_ed25519*',
    ];
    const clean = f.replace(/\\/g, '/').replace(/^\.\//, '').toLowerCase();
    const base = clean.split('/').pop();

    for (const p of patterns) {
      const pClean = p.replace(/\\/g, '/').replace(/^\.\//, '').toLowerCase();
      if (pClean.endsWith('/**') || pClean.endsWith('/*')) {
        const prefix = pClean.replace(/\/\*\*?$/, '');
        if (clean === prefix || clean.startsWith(prefix + '/')) return true;
      } else if (pClean.endsWith('/')) {
        if (clean.startsWith(pClean)) return true;
      } else if (clean === pClean) {
        return true;
      }
      if (!pClean.includes('/')) {
        if (pClean.includes('*')) {
          const reg = new RegExp('^' + pClean.replace(/\./g, '\\.').replace(/\*/g, '.*') + '$');
          if (reg.test(base)) return true;
        } else if (base === pClean) {
          return true;
        }
      }
      if (pClean.includes('*')) {
        const reg = new RegExp('^' + pClean.replace(/\./g, '\\.').replace(/\*/g, '.*') + '$');
        if (reg.test(clean)) return true;
      }
    }
    return false;
  }

  function isOpenPR(b) {
    return Boolean(b.pr && b.pr.number > 0 && b.state === 'open' && b.pr.reviewState !== 'MERGED');
  }

  function isLocalOpen(b) {
    return Boolean(b.hasLocal && b.state === 'open' && !isMergedOrGone(b));
  }

  function renderKPIs() {
    const branches = state.snapshot?.branches || [];
    const localBranches = branches.filter((b) => b.hasLocal && !b.id.startsWith('gh-merged-') && b.id !== 'local-wip-working-tree');
    const localCount = localBranches.length;
    const mergedPRCount = branches.filter((b) => b.id.startsWith('gh-merged-') || (b.state === 'merged' && !b.hasLocal)).length;
    const dirtyCount = state.snapshot?.repo?.dirtyCount || branches.find((b) => b.isHead)?.uncommittedCount || 0;

    const upperCount = branches.filter(isOpenPR).length;
    const lowerCount = branches.filter(isLocalOpen).length;
    const mineCount = branches.filter((b) => b.state === 'open' && !isMergedOrGone(b) && isMyBranch(b)).length;
    const conflictCount = branches.filter((b) => b.state === 'open' && !isMergedOrGone(b) && ((b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length > 0))).length;
    const failCount = branches.filter((b) => b.state === 'open' && b.pr?.ci?.state === 'FAILURE').length;
    const goneCount = branches.filter(isMergedOrGone).length;

    const allTitle = `Tümü (${branches.length} düğüm):\n• ${localCount} yerel dal (klasik 'git branch' sayısı)\n• ${mergedPRCount} GitHub birleşmiş PR parabolü\n• ${branches.length - localCount - mergedPRCount} diğer uzak/sanal dal.\n\nNot: Klasik 'git branch' komutu yalnızca ${localCount} yerel dalı listeler; GitArc topolojideki birleşmiş PR kemerlerini de gösterir.`;

    const defs = [
      { key: 'all', label: 'Tümü', val: branches.length, tone: '', title: allTitle },
      { key: 'mine', label: '👤 Benim Dallarım (@me)', val: mineCount, tone: mineCount ? 'accent' : '', title: 'Giriş yapan kullanıcıya ait açık dallar' },
      { key: 'lower', label: 'Alt Ray (Local)', val: lowerCount, tone: lowerCount ? 'purple' : '', title: 'PR açılmamış açık yerel dallar' },
      { key: 'dirty', label: 'Değişiklik (Dirty)', val: dirtyCount, tone: dirtyCount ? 'warn' : '', title: `Çalışma ağacında ${dirtyCount} adet kaydedilmemiş (staged/indekslenmiş veya untracked/takipsiz) dosya var.` },
      { key: 'issues', label: 'Çakışma / Cross-PR', val: conflictCount, tone: conflictCount ? 'warn' : '', title: 'Çakışan veya aynı dosyaları değiştiren açık dallar' },
      { key: 'ci_fail', label: 'CI Hatası', val: failCount, tone: failCount ? 'danger' : '', title: 'CI testleri başarısız olan dallar' },
      { key: 'gone', label: 'Birleşmiş / Gone', val: goneCount, tone: goneCount ? 'ok' : '', title: 'Main ile birleşmiş veya uzaktaki dalı silinmiş temizleme adayları' },
    ];

    el.kpiPills.innerHTML = '';
    defs.forEach((d) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `kpi-pill ${d.tone} ${state.filter === d.key ? 'active' : ''}`;
      if (d.title) btn.title = d.title;
      btn.innerHTML = `<span>${esc(d.label)}</span><strong>${d.val}</strong>`;
      btn.addEventListener('click', () => {
        state.filter = d.key;
        renderKPIs();
        renderSVG();
      });
      el.kpiPills.appendChild(btn);
    });

    const prBadge = document.createElement('span');
    prBadge.className = `kpi-pill ${upperCount ? 'accent' : ''}`;
    prBadge.title = "GitHub üzerinde açık olan Pull Request'ler";
    prBadge.innerHTML = `<span>Üst Ray (PR)</span><strong>${upperCount}</strong>`;
    el.kpiPills.appendChild(prBadge);

    if (el.dragHint) {
      el.dragHint.innerHTML = `💡 <strong>Tümü Sayacı (${branches.length}):</strong> ${localCount} yerel dal + ${mergedPRCount} birleşmiş PR parabolüdür (<code>git branch</code> yalnızca ${localCount} yerel dalı listeler).`;
    }
  }

  function matchesFilter(b) {
    const q = (state.pathLens || '').trim().toLowerCase();
    if (q) {
      const nameHit = (b.name || '').toLowerCase().includes(q);
      const fileHit = (b.changedFiles || []).some((f) => f.toLowerCase().includes(q));
      if (!nameHit && !fileHit) return false;
    }
    switch (state.filter) {
      case 'mine':
        return Boolean(b.state === 'open' && !isMergedOrGone(b) && isMyBranch(b));
      case 'lower':
        return isLocalOpen(b);
      case 'dirty':
        return Boolean(b.isHead && ((b.uncommittedCount || 0) > 0 || (state.snapshot?.repo?.dirtyCount || 0) > 0));
      case 'issues':
        return Boolean(b.state === 'open' && !isMergedOrGone(b) && ((b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length)));
      case 'ci_fail':
        return Boolean(b.state === 'open' && b.pr?.ci?.state === 'FAILURE');
      case 'gone':
        return isMergedOrGone(b);
      default:
        return true;
    }
  }

  function renderRadar() {
    const diags = state.snapshot?.diagnostics || [];
    if (el.dockRadarCount) el.dockRadarCount.textContent = diags.length;
    if (el.radarCount) el.radarCount.textContent = `${diags.length} uyarı`;
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

      // Files list
      const files = d.files || b?.conflict?.files || [];
      let filesHtml = '';
      if (files.length > 0) {
        filesHtml = files.map((f) => `<span class="rc-file-tag">${esc(f)}</span>`).join('');
      }

      // CI error snippet if any
      let ciSnippet = '';
      if (b?.pr?.ci?.failureSummary) {
        ciSnippet = b.pr.ci.failureSummary;
      } else if (b?.pr?.ci?.rawLog && (b?.pr?.ci?.state === 'FAILURE' || b?.pr?.ci?.status === 'failure')) {
        ciSnippet = b.pr.ci.rawLog.split('\n').filter(Boolean).slice(0, 3).join('\n');
      }

      // Action button text and handler
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
      }

      // Suggested CLI command
      const suggestedCmd = b?.suggestedCmd || d.actionHint || '';

      card.innerHTML = `
        <div class="rc-header">
          <div class="rc-title-group">
            <span class="rc-badge ${badgeClass}">${badgeLabel}${kindLabel ? ' · ' + kindLabel : ''}</span>
            <span class="rc-branch-badge">${esc(d.branchName || 'Genel')}</span>
            <strong class="rc-title">${esc(d.title)}</strong>
          </div>
          <div class="rc-actions">
            <button type="button" class="rc-action-btn" data-act="${actAction}" data-branch="${esc(d.branchId || '')}">${actBtnText}</button>
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
        if (act === 'studio' && brId) {
          openConflictStudio(brId);
        } else if (act === 'prune') {
          handlePruneGone();
        } else if (brId) {
          selectBranch(brId);
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
        if (d.branchId) selectBranch(d.branchId);
      });

      el.radarList.appendChild(card);
    });

    renderDockStreams();
  }

  function renderDockStreams() {
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

  function setupBottomDock() {
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

    el.dockClearBtn?.addEventListener('click', async () => {
      state.clientLogs = [];
      if (!state.offlineMode) {
        try {
          const snap = await apiCall('/api/logs/clear', 'POST', {});
          applySnapshot(snap);
        } catch (err) {
          showToast(err.message, 'danger');
        }
      }
      renderLogBadge();
      renderDockStreams();
      showToast('Loglar temizlendi', 'info');
    });
  }

  function xOfSpine(idx) {
    return SPINE_START_X + idx * SPINE_STEP_X;
  }

  function yOfLane(hemisphere, lane, isMerged = false) {
    const l = Math.max(1, lane || 1);
    const offset = isMerged ? (84 + (l - 1) * 56) : (LANE_BASE_OFFSET + (l - 1) * LANE_STEP_Y);
    return hemisphere === 'lower' ? MAIN_Y + offset : MAIN_Y - offset;
  }

  function layoutOpenApexPositions(branches) {
    const map = new Map();
    const lastMergedEndX = branches
      .filter((b) => b.state === 'merged')
      .reduce((acc, b) => Math.max(acc, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex))), 0);

    ['upper', 'lower'].forEach((hemi) => {
      const openList = branches
        .filter((b) => b.state !== 'merged' && b.hemisphere === hemi)
        .sort((a, b) => a.startSpineIndex - b.startSpineIndex || a.endSpineIndex - b.endSpineIndex || a.name.localeCompare(b.name));

      let prevX = -Infinity;
      openList.forEach((b) => {
        const startX = xOfSpine(b.startSpineIndex);
        const baseX = Math.max(startX + 150, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex)) - 10);
        // Keep open upper capsules clear of merged arch crests when nearby
        const minClearX = hemi === 'upper' && lastMergedEndX > 0 ? Math.max(baseX, lastMergedEndX + 20) : baseX;
        const apexX = prevX === -Infinity ? minClearX : Math.max(minClearX, prevX + OPEN_CAPSULE_MIN_GAP_X);
        const apexY = hemi === 'lower' ? MAIN_Y + LANE_BASE_OFFSET : MAIN_Y - LANE_BASE_OFFSET;
        prevX = apexX;
        map.set(b.id, { apexX, apexY });
      });
    });
    return map;
  }

  function computeBranchGeometry(b, tipXOverride = null, tipYOverride = null, apexLayout = null) {
    const startX = xOfSpine(b.startSpineIndex);
    const isMerged = b.state === 'merged';
    const laneY = yOfLane(b.hemisphere, b.lane, isMerged);

    if (isMerged && tipXOverride === null) {
      const endX = xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex));
      const apexX = (startX + endX) / 2;
      const apexY = laneY;
      const cp1X = startX + (apexX - startX) * 0.55;
      const cp2X = apexX + (endX - apexX) * 0.45;
      const openD = `M ${startX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
      const closeD = `M ${apexX} ${apexY} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y}`;
      const areaD = `${openD} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y} Z`;
      return { startX, endX, apexX, apexY, openD, closeD, areaD };
    }

    const laid = apexLayout?.get(b.id);
    const defaultApexX = laid ? laid.apexX : Math.max(startX + 150, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex)) - 10);
    const defaultApexY = laid ? laid.apexY : laneY;
    const apexX = tipXOverride !== null ? tipXOverride : defaultApexX;
    const apexY = tipYOverride !== null ? tipYOverride : defaultApexY;
    const cp1X = startX + (apexX - startX) * 0.55;
    const openD = `M ${startX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
    return { startX, endX: null, apexX, apexY, openD, closeD: null, areaD: null };
  }

  function branchToneClass(b) {
    if (b.pr?.ci?.state === 'FAILURE') return 'ci-fail';
    if (b.conflict?.hasConflict && !b.conflict.resolved) return 'conflict';
    if (b.pr?.ci?.state === 'RUNNING') return 'ci-running';
    if (b.state === 'merged') return 'merged';
    if (b.originGone || b.state === 'gone') return 'gone';
    return b.hemisphere === 'lower' ? 'lower-open' : 'upper-open';
  }

  function branchStrokeColor(b) {
    if (b.pr?.ci?.state === 'FAILURE') return '#f85149';
    if (b.conflict?.hasConflict && !b.conflict.resolved) return '#f97316';
    if (b.pr?.ci?.state === 'RUNNING') return '#d29922';
    if (b.state === 'merged') return '#3fb950';
    if (b.originGone || b.state === 'gone') return '#d29922';
    return b.hemisphere === 'lower' ? '#a371f7' : '#58a6ff';
  }

  function renderSVG() {
    const snap = state.snapshot;
    if (!snap) return;

    const nodes = snap.nodes || [];
    const branches = snap.branches || [];
    const visibleBranches = branches.filter((b) => state.showMergedArches || b.state !== 'merged');
    const apexLayout = layoutOpenApexPositions(visibleBranches);
    state.apexLayout = apexLayout;

    const maxSpineIdx = Math.max(nodes.length - 1, ...visibleBranches.map((b) => b.endSpineIndex || 0), 6);
    const maxMergedUpperLane = Math.max(1, ...visibleBranches.filter((b) => b.hemisphere === 'upper' && b.state === 'merged').map((b) => b.lane || 1));
    const maxApexX = Math.max(0, ...Array.from(apexLayout.values()).map((v) => v.apexX));

    const minY = Math.min(0, MAIN_Y - LANE_BASE_OFFSET - (maxMergedUpperLane - 1) * LANE_STEP_Y - 130);
    const maxY = Math.max(680, MAIN_Y + LANE_BASE_OFFSET + 220);
    const sceneW = Math.max(1280, SPINE_START_X + (maxSpineIdx + 2) * SPINE_STEP_X + 140, maxApexX + 200);
    const sceneH = maxY - minY;

    const viewW = sceneW / state.zoom;
    const viewH = sceneH / state.zoom;
    el.svg.setAttribute('viewBox', `${state.panX} ${minY + state.panY} ${viewW} ${viewH}`);
    el.zoomReset.textContent = `${Math.round(state.zoom * 100)}%`;

    el.svg.innerHTML = '';

    // 1. SVG <defs> (glow filters + main gradient from parabolic.html)
    const defs = svgEl('defs');
    defs.innerHTML = `
      <filter id="glow-green" x="-20%" y="-20%" width="140%" height="140%">
        <feGaussianBlur stdDeviation="4" result="blur"/>
        <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
      </filter>
      <filter id="glow-red" x="-30%" y="-30%" width="160%" height="160%">
        <feGaussianBlur stdDeviation="4.5" result="blur"/>
        <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
      </filter>
      <linearGradient id="main-rail-grad" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" stop-color="#238636" stop-opacity="0.45"/>
        <stop offset="20%" stop-color="#3fb950" stop-opacity="0.95"/>
        <stop offset="85%" stop-color="#3fb950" stop-opacity="1"/>
        <stop offset="100%" stop-color="#58a6ff" stop-opacity="0.45"/>
      </linearGradient>
    `;
    el.svg.appendChild(defs);

    // 2. Background horizontal dashed guide rails (Upper Remote + Lower Local)
    el.svg.appendChild(svgEl('line', { x1: 40, y1: MAIN_Y - LANE_BASE_OFFSET, x2: sceneW - 40, y2: MAIN_Y - LANE_BASE_OFFSET, class: 'svg-guide-line' }));
    el.svg.appendChild(svgEl('line', { x1: 40, y1: MAIN_Y + LANE_BASE_OFFSET, x2: sceneW - 40, y2: MAIN_Y + LANE_BASE_OFFSET, class: 'svg-guide-line' }));

    // 3. Parabolic Curves Layer (Merged arches behind, open half-parabolas in front)
    const arcLayer = svgEl('g', { id: 'arc-layer' });
    const capsuleLayer = svgEl('g', { id: 'capsule-layer' });

    const ordered = [...visibleBranches].sort((a, b) => (a.state === 'merged' ? 0 : 1) - (b.state === 'merged' ? 0 : 1));
    ordered.forEach((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout);
      const visible = matchesFilter(b);
      const isSelected = state.selectedBranchId === b.id;
      const tone = branchToneClass(b);
      const strokeColor = branchStrokeColor(b);

      if (b.state === 'merged' && geom.areaD) {
        arcLayer.appendChild(svgEl('path', {
          d: geom.areaD,
          fill: strokeColor,
          class: `svg-arc-area ${isSelected ? 'selected' : ''} ${visible ? '' : 'dimmed'}`,
        }));
      }

      const openCls = ['svg-arc-path', tone];
      if (isSelected) openCls.push('selected');
      if (!visible) openCls.push('dimmed');
      const openAttrs = { d: geom.openD, class: openCls.join(' ') };
      if (b.pr?.ci?.state === 'FAILURE') openAttrs.filter = 'url(#glow-red)';
      const openPath = svgEl('path', openAttrs);
      openPath.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      arcLayer.appendChild(openPath);

      if (b.state === 'merged' && geom.closeD) {
        const closePath = svgEl('path', { d: geom.closeD, class: openCls.join(' ') });
        closePath.addEventListener('click', (ev) => {
          ev.stopPropagation();
          selectBranch(b.id);
        });
        arcLayer.appendChild(closePath);

        if (b.hasLocal || b.originGone) {
          const mergeX = geom.endX || xOfSpine(b.endSpineIndex || b.startSpineIndex || 0);
          const remY = MAIN_Y + 56 + (((b.endSpineIndex || 0) % 2) * 30);
          const remX = mergeX + 18;
          const remPath = svgEl('path', {
            d: `M ${mergeX} ${MAIN_Y} C ${mergeX} ${MAIN_Y + 30}, ${remX - 16} ${remY}, ${remX} ${remY}`,
            class: `svg-local-remnant-path ${isSelected ? 'selected' : ''} ${visible ? '' : 'dimmed'}`,
          });
          remPath.addEventListener('click', (ev) => {
            ev.stopPropagation();
            selectBranch(b.id);
          });
          arcLayer.appendChild(remPath);

          const remFo = svgEl('foreignObject', {
            x: remX,
            y: remY - 12,
            width: 150,
            height: 26,
            class: `fo-capsule ${visible ? '' : 'dimmed'}`,
            'data-remnant-branch-id': b.id,
          });
          const remPill = document.createElementNS(XHTML_NS, 'div');
          remPill.className = `cap-local-remnant ${isSelected ? 'selected' : ''}`;
          remPill.innerHTML = `
            <span class="remnant-txt" title="${esc(b.name)}">💻 ${b.isHead ? 'HEAD·' : ''}${esc(truncate(b.name, 13))}</span>
            <button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>
          `;
          remPill.querySelector('button[data-act="delete"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
          remPill.querySelector('button[data-act="delete"]')?.addEventListener('click', (ev) => {
            ev.stopPropagation();
            handleDeleteBranch(b.id);
          });
          remPill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
          remPill.addEventListener('click', (ev) => {
            ev.stopPropagation();
            selectBranch(b.id);
          });
          remFo.appendChild(remPill);
          capsuleLayer.appendChild(remFo);
        }
      } else {
        // Open half-parabola dashed tail extending past the apex
        arcLayer.appendChild(svgEl('line', {
          x1: geom.apexX,
          y1: geom.apexY,
          x2: geom.apexX + 95,
          y2: geom.apexY,
          class: `svg-open-tail ${tone} ${visible ? '' : 'dimmed'}`,
        }));
      }

      // Apex commit dot
      const dot = svgEl('circle', {
        cx: geom.apexX,
        cy: geom.apexY,
        r: isSelected ? 6 : 4.5,
        class: `svg-apex-dot ${tone}`,
      });
      dot.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      arcLayer.appendChild(dot);

      // Apex Capsule Card via <foreignObject>
      const fo = buildCapsuleForeignObject(b, geom, isSelected, visible, tone);
      capsuleLayer.appendChild(fo);
    });

    el.svg.appendChild(arcLayer);

    // 4. Central Main Spine Drop Zone + Glowing Gradient Line
    const spineEndX = Math.max(xOfSpine(maxSpineIdx) + 140, 1140);
    el.svg.appendChild(svgEl('rect', {
      id: 'svg-main-drop-rect',
      x: 50,
      y: MAIN_Y - 24,
      width: spineEndX - 40,
      height: 48,
      rx: 24,
      class: 'svg-main-drop-rect',
    }));

    el.svg.appendChild(svgEl('line', {
      x1: 72,
      y1: MAIN_Y,
      x2: spineEndX - 15,
      y2: MAIN_Y,
      stroke: 'url(#main-rail-grad)',
      filter: 'url(#glow-green)',
      class: 'svg-spine-line',
    }));

    const defBranch = snap.repo.defaultBranch || 'main';
    const mainTagW = Math.max(58, defBranch.length * 8 + 22);
    el.svg.appendChild(svgEl('rect', { x: 22, y: MAIN_Y - 13, width: mainTagW, height: 26, rx: 8, class: 'svg-main-tag-bg' }));
    el.svg.appendChild(svgEl('text', { x: 22 + mainTagW / 2, y: MAIN_Y + 4, class: 'svg-main-tag-txt' }, defBranch));

    // 5. Commit Nodes on the Central Spine
    const nodeLayer = svgEl('g', { id: 'node-layer' });
    nodes.forEach((n, idx) => {
      const cx = xOfSpine(n.spineIndex);
      const isLatest = idx === nodes.length - 1;
      const cls = ['svg-node-circle'];
      if (n.isMerge) cls.push('merge-node');
      if (n.isHead || isLatest) cls.push('head-node');

      const circle = svgEl('circle', {
        cx,
        cy: MAIN_Y,
        r: isLatest ? 8.5 : (n.isMerge ? 7 : 5.5),
        class: cls.join(' '),
      });
      circle.addEventListener('click', (ev) => {
        ev.stopPropagation();
        state.selectedNodeSha = n.shortSha;
        state.selectedBranchId = null;
        renderInspector();
      });
      nodeLayer.appendChild(circle);

      nodeLayer.appendChild(svgEl('text', {
        x: cx,
        y: MAIN_Y - 14,
        class: `svg-node-sha ${isLatest ? 'latest' : ''}`,
      }, n.shortSha));

      nodeLayer.appendChild(svgEl('text', {
        x: cx,
        y: MAIN_Y + 23,
        class: 'svg-node-label',
      }, truncate(n.subject, 19)));

      if (n.tag) {
        const tw = n.tag.length * 6.8 + 14;
        nodeLayer.appendChild(svgEl('rect', { x: cx - tw / 2, y: MAIN_Y - 40, width: tw, height: 17, rx: 5, class: 'svg-node-tag-bg' }));
        nodeLayer.appendChild(svgEl('text', { x: cx, y: MAIN_Y - 28, class: 'svg-node-tag-txt' }, n.tag));
      }
    });

    el.svg.appendChild(nodeLayer);
    el.svg.appendChild(capsuleLayer);
    el.svg.appendChild(svgEl('path', { id: 'drag-preview-path', class: 'svg-drag-preview', d: '' }));
  }

  function capsulePositionForApex(b, apexX, apexY) {
    if (b.state === 'merged') {
      return {
        x: apexX - MERGED_FO_W / 2,
        y: apexY - MERGED_FO_H - 6,
      };
    }
    const x = apexX - FO_W / 2;
    const y = b.hemisphere === 'lower' ? apexY + 14 : apexY - FO_H + 12;
    return { x, y };
  }

  function buildCapsuleForeignObject(b, geom, isSelected, visible, tone) {
    const snap = state.snapshot;
    const isMerged = b.state === 'merged';
    const pos = capsulePositionForApex(b, geom.apexX, geom.apexY);
    const fo = svgEl('foreignObject', {
      x: pos.x,
      y: pos.y,
      width: isMerged ? MERGED_FO_W : FO_W,
      height: isMerged ? MERGED_FO_H : FO_H,
      class: `fo-capsule ${visible ? '' : 'dimmed'}`,
      'data-branch-id': b.id,
    });

    if (isMerged) {
      const pill = document.createElementNS(XHTML_NS, 'div');
      const hasLocalLeftover = Boolean(b.originGone || b.hasLocal);
      pill.className = `cap-merged-pill ${isSelected ? 'selected' : ''} ${hasLocalLeftover ? 'gone' : ''}`;
      const prTxt = b.pr ? `#${b.pr.number} ` : '';
      const localTagHtml = hasLocalLeftover
        ? `<span class="cap-merged-local-tag" title="Yerelde dal kopyası duruyor">${b.isHead ? 'HEAD·Local' : '💻 Local'}</span>`
        : '';
      const delBtnHtml = hasLocalLeftover
        ? '<button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>'
        : '';
      pill.innerHTML = `
        <span class="cap-merged-txt">✓ ${esc(prTxt + truncate(b.name, hasLocalLeftover ? 11 : 19))}</span>
        ${localTagHtml}
        ${delBtnHtml}
      `;
      pill.querySelector('button[data-act="delete"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      pill.querySelector('button[data-act="delete"]')?.addEventListener('click', (ev) => {
        ev.stopPropagation();
        handleDeleteBranch(b.id);
      });
      pill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      pill.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      fo.appendChild(pill);
      return fo;
    }

    const isLower = b.hemisphere === 'lower';
    const isWorkingTreeWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-') || b.name === 'wip/working-tree';
    const isFailed = b.pr?.ci?.state === 'FAILURE';
    const isRunning = b.pr?.ci?.state === 'RUNNING';
    const isQueued = Boolean(state.pendingAutoMerge[b.id]);
    const hasConflict = Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
    const isRemoteOnly = !b.hasLocal && b.hasRemote && b.state === 'open';
    const delTitle = isRemoteOnly ? 'PR Kapat &amp; Uzak Dalı Sil' : 'Dalı Sil';

    let badgeTone = 'sky';
    let badgePulse = false;
    let badgeText = b.pr ? `PR #${b.pr.number} · ${b.pr.reviewState}` : 'Remote Ray';
    if (isFailed) {
      badgeTone = 'danger';
      badgeText = `PR #${b.pr.number} · CI Failed`;
    } else if (hasConflict) {
      badgeTone = 'warn';
      badgeText = b.pr ? `PR #${b.pr.number} · Çakışma` : 'Merge Çakışması';
    } else if (isRunning) {
      badgeTone = 'warn';
      badgePulse = true;
      badgeText = `PR #${b.pr.number} · CI Çalışıyor`;
    } else if (b.originGone || b.state === 'gone') {
      badgeTone = 'warn';
      badgeText = 'origin: gone';
    } else if (isLower) {
      badgeTone = 'purple';
      const isFreshLocal = b.aheadMain === 0 && (b.uncommittedCount === 0 || !b.isHead);
      badgeText = isWorkingTreeWip
        ? 'Yerel Taslak · Uncommitted'
        : (isFreshLocal ? '🌱 Yeni Yerel Dal' : 'Yarım Parabol · Local');
    }

    const isFreshLocal = isLower && !isWorkingTreeWip && b.aheadMain === 0;
    const subText = isWorkingTreeWip
      ? `${b.uncommittedCount || 0} dosya bekliyor · İncelemek için tıklayın`
      : (isFreshLocal
        ? 'main omurgasında taze dal · İlk commitinizi bekliyor'
        : (b.lastCommitMsg ? b.lastCommitMsg : (isLower ? `↑${b.aheadMain || 0} yerel · ${b.uncommittedCount || 0} dirty` : `↑${b.aheadMain || 0} ↓${b.behindMain || 0}`)));

    const card = document.createElementNS(XHTML_NS, 'div');
    card.className = `cap-card ${tone} ${isSelected ? 'selected' : ''}`;

    let alertsHtml = '';
    if (isRemoteOnly) {
      alertsHtml += '<span class="cap-alert-pill cross">☁ Sadece Origin</span>';
    }
    if (hasConflict) {
      alertsHtml += `<span class="cap-alert-pill conflict">${b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lock' : '⚠ Conflict'}</span>`;
    }
    if (b.crossPr?.length) {
      alertsHtml += `<span class="cap-alert-pill cross">⇄ Cross-PR ${b.crossPr.map((c) => '#' + (c.otherPrNumber || '?')).join(',')}</span>`;
    }

    const checkoutTitle = isRemoteOnly ? 'Yerelde İndir &amp; Geç (git switch --track)' : 'Bu dala geç (git switch)';
    const checkoutBtn = (!b.isHead && !isWorkingTreeWip)
      ? `<button type="button" class="cap-btn-ico" data-act="checkout" title="${checkoutTitle}">🎯</button>`
      : '';

    let actionRowHtml = '';
    if (isFailed) {
      actionRowHtml = `
        <button type="button" class="cap-btn danger" data-act="inspect">🔍 Hatayı Gör</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else if (hasConflict) {
      actionRowHtml = `
        <button type="button" class="cap-btn amber" data-act="resolve">⚡ Çakışma Stüdyosu</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else if (isLower) {
      if (isWorkingTreeWip) {
        if (snap.pendingProposal) {
          actionRowHtml = `<button type="button" class="cap-btn sky running-pulse" data-act="inspect">✨ AI Önerisini İncele &amp; Onayla</button>`;
        } else {
          actionRowHtml = `<button type="button" class="cap-btn sky" data-act="inspect">🚀 Commit &amp; PR Stüdyosu</button>`;
        }
      } else {
        const hasOpenPR = Boolean(b.pr && b.pr.number > 0 && b.pr.reviewState !== 'MERGED');
        actionRowHtml = `
          <button type="button" class="cap-btn sky" data-act="promote">🚀 Push &amp; PR</button>
          ${hasOpenPR ? `<button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge</button>` : ''}
          ${checkoutBtn}
          <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
        `;
      }
    } else if (isRunning) {
      actionRowHtml = `
        <button type="button" class="cap-btn amber ${isQueued ? 'running-pulse' : ''}" data-act="merge">${isQueued ? '⏳ CI Bekleniyor (İptal)' : '⏳ CI Bitince Merge Et'}</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else {
      actionRowHtml = `
        <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge (Parabolü Kapat)</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    }

    card.innerHTML = `
      <div class="cap-top-row">
        <span class="cap-badge ${badgeTone} ${badgePulse ? 'running-pulse' : ''}">${esc(badgeText)}</span>
        ${b.isHead ? '<span class="cap-head-pill">HEAD</span>' : ''}
        ${isWorkingTreeWip && snap.pendingProposal ? '<span class="cap-badge-ai running-pulse">✨ AI Önerisi Hazır</span>' : ''}
      </div>
      <div class="cap-name">${isWorkingTreeWip ? '✏️ Kaydedilmemiş Değişiklikler' : esc(b.name)}</div>
      <div class="cap-sub">${isWorkingTreeWip ? (snap.pendingProposal ? '🤖 Claude / AI Commit &amp; PR Önerisi Hazır' : `${b.uncommittedCount || 0} dosya bekliyor · Commit stüdyosu`) : esc(truncate(subText, 34))}${!isWorkingTreeWip && b.uncommittedCount > 0 ? ` · ●${b.uncommittedCount} dirty` : ''}</div>
      ${alertsHtml ? `<div class="cap-alerts">${alertsHtml}</div>` : ''}
      <div class="cap-actions">${actionRowHtml}</div>
    `;

    card.querySelectorAll('button[data-act]').forEach((btn) => {
      btn.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      btn.addEventListener('click', (ev) => {
        ev.stopPropagation();
        const act = btn.getAttribute('data-act');
        if (act === 'inspect') selectBranch(b.id);
        else if (act === 'checkout') handleCheckoutBranch(b.id, btn);
        else if (act === 'merge') handleMergeBranch(b.id, btn);
        else if (act === 'promote') handlePromoteBranch(b.id, undefined, undefined, btn);
        else if (act === 'resolve') openConflictStudio(b.id);
        else if (act === 'delete') handleDeleteBranch(b.id, btn);
      });
    });

    card.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0 || ev.target.closest('button')) return;
      ev.stopPropagation();
      startCapsulePointer(ev, b, fo, card);
    });

    fo.appendChild(card);
    return fo;
  }

  function clientToSvg(clientX, clientY) {
    const pt = el.svg.createSVGPoint();
    pt.x = clientX;
    pt.y = clientY;
    const ctm = el.svg.getScreenCTM();
    if (!ctm) return { x: 0, y: 0 };
    return pt.matrixTransform(ctm.inverse());
  }

  function startCapsulePointer(ev, branch, foEl, cardEl) {
    const startPt = clientToSvg(ev.clientX, ev.clientY);
    const geom = computeBranchGeometry(branch, null, null, state.apexLayout);
    const isVirtualWip = branch.id === 'local-wip-working-tree' || branch.id.startsWith('local-wip-');
    const canDragMerge = branch.state === 'open' && !isVirtualWip && branch.pr?.ci?.state !== 'FAILURE';

    state.drag = {
      branch,
      foEl,
      cardEl,
      startClientX: ev.clientX,
      startClientY: ev.clientY,
      startSvgX: startPt.x,
      startSvgY: startPt.y,
      baseApexX: geom.apexX,
      baseApexY: geom.apexY,
      active: false,
      canDragMerge,
    };
  }

  window.addEventListener('pointermove', (ev) => {
    if (state.panning) {
      const dx = (ev.clientX - state.panning.x) / state.zoom;
      const dy = (ev.clientY - state.panning.y) / state.zoom;
      state.panX = state.panning.panX - dx;
      state.panY = state.panning.panY - dy;
      renderSVG();
      return;
    }
    if (!state.drag) return;

    const dist = Math.hypot(ev.clientX - state.drag.startClientX, ev.clientY - state.drag.startClientY);
    if (!state.drag.active && dist >= 7 && state.drag.canDragMerge) {
      state.drag.active = true;
      state.drag.cardEl.classList.add('dragging');
      document.getElementById('svg-main-drop-rect')?.classList.add('visible');
      el.dragHint.classList.add('active-drop');
    }

    if (state.drag.active) {
      const pt = clientToSvg(ev.clientX, ev.clientY);
      const dx = pt.x - state.drag.startSvgX;
      const dy = pt.y - state.drag.startSvgY;
      const curApexX = state.drag.baseApexX + dx;
      const curApexY = state.drag.baseApexY + dy;

      const pos = capsulePositionForApex(state.drag.branch, curApexX, curApexY);
      state.drag.foEl.setAttribute('x', String(pos.x));
      state.drag.foEl.setAttribute('y', String(pos.y));

      const preview = computeBranchGeometry(state.drag.branch, curApexX, curApexY);
      document.getElementById('drag-preview-path')?.setAttribute('d', preview.openD);

      const overSpine = Math.abs(curApexY - MAIN_Y) < 68;
      state.drag.overSpine = overSpine;
      document.getElementById('svg-main-drop-rect')?.classList.toggle('hot', overSpine);
    }
  });

  window.addEventListener('pointerup', async () => {
    if (state.panning) {
      state.panning = null;
      el.viewport.classList.remove('panning');
    }
    if (!state.drag) return;

    const d = state.drag;
    state.drag = null;

    document.getElementById('svg-main-drop-rect')?.classList.remove('visible', 'hot');
    document.getElementById('drag-preview-path')?.setAttribute('d', '');
    el.dragHint.classList.remove('active-drop');
    d.cardEl.classList.remove('dragging');

    if (!d.active) {
      selectBranch(d.branch.id);
      return;
    }

    if (d.overSpine) {
      await handleMergeBranch(d.branch.id);
    } else {
      renderSVG();
    }
  });

  function selectBranch(branchId) {
    state.selectedBranchId = branchId;
    state.selectedNodeSha = null;
    renderSVG();
    renderInspector();
  }

  function suggestCommitAndPR(b) {
    const files = b.changedFiles || [];
    if (files.length === 0) {
      return {
        commitMsg: 'feat: çalışma dizini güncellemeleri',
        prTitle: 'feat: çalışma dizini güncellemeleri',
        prBody: 'GitArc kokpitinden oluşturuldu.',
      };
    }
    const first = files[0].toLowerCase();
    let prefix = 'feat';
    let summary = 'değişiklikleri kaydet';
    if (first.endsWith('.md') || first.startsWith('docs/')) {
      prefix = 'docs';
      summary = 'belgeleri güncelle';
    } else if (first.includes('gitignore')) {
      prefix = 'chore';
      summary = 'gitignore kurallarını güncelle';
    } else if (first.includes('test')) {
      prefix = 'test';
      summary = 'testleri güncelle';
    } else if (first.endsWith('.css') || first.endsWith('.html')) {
      prefix = 'style';
      summary = 'arayüz stil ve şablonlarını güncelle';
    } else if (first.endsWith('.json') || first.endsWith('.yml') || first.endsWith('.yaml') || first.endsWith('.toml')) {
      prefix = 'chore';
      summary = 'yapılandırma dosyalarını güncelle';
    }
    const baseName = files[0].split('/').pop().replace(/^\./, '');
    const commitMsg = `${prefix}: ${summary} (${baseName})`;
    const prTitle = `${prefix}: ${summary}`;
    const prBody = `### Değişiklikler\n- ${files.slice(0, 5).join('\n- ')}\n\n### Doğrulama\nYerel kontroller yapıldı.`;
    return { commitMsg, prTitle, prBody };
  }

  function suggestUniqueBranchName(arg1, arg2, arg3) {
    let prefix = 'feat';
    let files = [];
    let snap = state.snapshot;
    if (typeof arg1 === 'string') {
      prefix = arg1;
      files = Array.isArray(arg2) ? arg2 : [];
      snap = arg3 || state.snapshot;
    } else if (arg1 && typeof arg1 === 'object') {
      files = arg1.changedFiles || [];
      snap = arg2 || state.snapshot;
    }
    let slug = 'new-feature';
    if (files.length > 0) {
      const first = (files[0] || '').toLowerCase();
      if (first.endsWith('.md') || first.startsWith('docs/')) {
        prefix = 'docs';
      } else if (first.includes('test')) {
        prefix = 'test';
      } else if (first.endsWith('.css') || first.endsWith('.html')) {
        prefix = 'style';
      } else if (first.includes('gitignore') || first.endsWith('.json') || first.endsWith('.lock') || first.endsWith('.yml') || first.endsWith('.yaml')) {
        prefix = 'chore';
      }
      const rawBase = first.split('/').pop().replace(/^\./, '').replace(/\.[^.]+$/, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
      if (rawBase && rawBase !== 'index' && rawBase !== 'main') {
        slug = `${rawBase}-guncel`;
      }
    }
    const candidateBase = `${prefix}/${slug}`.slice(0, 40);
    const existing = new Set((snap?.branches || []).map((x) => (x.name || '').toLowerCase()));
    if (!existing.has(candidateBase)) {
      return candidateBase;
    }
    const shortSha = (snap?.nodes?.[snap.nodes.length - 1]?.shortSha || '').slice(0, 5);
    if (shortSha && !existing.has(`${candidateBase}-${shortSha}`.toLowerCase())) {
      return `${candidateBase}-${shortSha}`;
    }
    let n = 2;
    while (existing.has(`${candidateBase}-${n}`)) {
      n++;
    }
    return `${candidateBase}-${n}`;
  }

  function wireProposalActions() {
    const prop = state.snapshot?.pendingProposal;
    if (!prop) return;

    const bIn = document.getElementById('prop-edit-branch');
    const mIn = document.getElementById('prop-edit-message');
    const tIn = document.getElementById('prop-edit-title');
    const dIn = document.getElementById('prop-edit-body');

    const saveDraft = () => {
      state.proposalDraftEdits = {
        id: prop.id,
        branch: bIn ? bIn.value : prop.branchName,
        message: mIn ? mIn.value : prop.message,
        title: tIn ? tIn.value : prop.prTitle,
        body: dIn ? dIn.value : prop.prBody,
      };
    };

    bIn?.addEventListener('input', saveDraft);
    mIn?.addEventListener('input', saveDraft);
    tIn?.addEventListener('input', saveDraft);
    dIn?.addEventListener('input', saveDraft);

    document.getElementById('act-proposal-dismiss')?.addEventListener('click', async () => {
      try {
        state.proposalDraftEdits = null;
        const res = state.offlineMode
          ? LocalFallbackEngine.dismissProposal()
          : await apiCall('/api/proposal/dismiss', 'POST', {});
        applySnapshot(res);
        showToast('Öneri kartı kapatıldı', 'info');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });

    document.getElementById('act-proposal-approve')?.addEventListener('click', async () => {
      const btn = document.getElementById('act-proposal-approve');

      const finalBranch = (bIn ? bIn.value : prop.branchName).trim();
      const finalMessage = (mIn ? mIn.value : prop.message).trim();
      const finalTitle = (tIn ? tIn.value : prop.prTitle || finalMessage).trim();
      const finalBody = (dIn ? dIn.value : prop.prBody || '').trim();

      if (!finalBranch || !finalMessage) {
        showToast('Dal adı ve commit mesajı boş bırakılamaz', 'warn');
        return;
      }

      // 1. Mesaj ve dosya sayısı eşleşme denetimi (Madde 2 & 3)
      const countMatch = finalMessage.match(/(\d+)\s+dosya/i) || finalMessage.match(/update\s+(\d+)\s+files/i) || finalMessage.match(/(\d+)\s+files/i);
      if (countMatch && parseInt(countMatch[1], 10) !== prop.files.length) {
        const mismatchConfirm = window.confirm(
          `⚠️ UYARI: Commit mesajında belirtilen sayı (${countMatch[1]}) ile commitlenecek gerçek dosya sayısı (${prop.files.length}) uyuşmuyor!\n\nDevam etmek istiyor musunuz?`,
        );
        if (!mismatchConfirm) return;
      }

      // 2. 10+ dosya ya da 1000+ satırda ekstra onay (Madde 5)
      if (prop.isLargeDiff || prop.filesChanged > 10 || prop.totalLines >= 1000) {
        const largeConfirm = window.confirm(
          `🚨 DİKKAT: BÜYÜK DEĞİŞİKLİK TESPİT EDİLDİ!\n\nBu öneri ${prop.filesChanged} adet dosya ve ${prop.totalLines} satır değişiklik içermektedir!\nHalka açık depoya yanlışlıkla büyük veri, gizli anahtarlar veya istemediğiniz dosyaları itmediğinizden kesinlikle emin misiniz?\n\nYine de devam etmek istiyor musunuz?`,
        );
        if (!largeConfirm) return;
      }

      // 3. Çalıştırılacak komutları şeffafça göster
      const firstLine = finalMessage.split('\n')[0].replace(/"/g, '\\"');
      const cmd = `git switch -c ${finalBranch} ${prop.baseBranch || 'origin/main'} && git add -- ${prop.files.join(' ')} && git commit -m "${firstLine}" && git push -u origin ${finalBranch} && gh pr create --head ${finalBranch} --title "${finalTitle || firstLine}"`;
      const confirmed = window.confirm(
        `🤖 CLAUDE ÖNERİSİNİ ONAYLA:\n\nÖnerilen dal: ${finalBranch} (${prop.baseBranch})\nPR Başlığı: ${finalTitle}\nCommitlenecek Dosyalar (${prop.files.length} adet):\n${prop.files.map((f) => ' • ' + f).join('\n')}\n\nÇalıştırılacak Komut:\n${cmd}\n\nBu işlemi onaylıyor musunuz?`,
      );
      if (!confirmed) return;

      state.proposalDraftEdits = null;
      await handleCommitWorkingTree(finalBranch, finalMessage, finalTitle, finalBody, true, btn, prop.files);
      if (!state.offlineMode) {
        try {
          const dismissedSnap = await apiCall('/api/proposal/dismiss', 'POST', {});
          applySnapshot(dismissedSnap);
        } catch (_) {}
      } else {
        LocalFallbackEngine.dismissProposal();
      }
    });
  }

  function renderInspector() {
    const snap = state.snapshot;
    if (!snap) return;

    if (state.selectedNodeSha) {
      const node = snap.nodes.find((n) => n.shortSha === state.selectedNodeSha);
      if (node) {
        el.detail.innerHTML = `
          <div class="det-header">
            <div class="det-title-row">
              <h3 class="det-branch-name">Commit ${esc(node.shortSha)}</h3>
              ${node.tag ? `<span class="chip accent">${esc(node.tag)}</span>` : ''}
            </div>
            <div class="det-badges">
              <span class="chip">${esc(node.author)}</span>
              <span class="chip">${esc(node.relativeTime)}</span>
              ${node.isMerge ? '<span class="chip ok">Merge Node</span>' : ''}
            </div>
          </div>
          <div class="diag-card">
            <h4>Commit Mesajı</h4>
            <div class="layer-line">${esc(node.subject)}</div>
          </div>
          <div class="lock-note">🔒 <strong>main</strong> omurgasındaki commit mesajları tarihçe güvenliği için kilitlidir.</div>
        `;
        return;
      }
    }

    let proposalHtml = '';
    if (snap.pendingProposal) {
      const prop = snap.pendingProposal;
      const edits = (state.proposalDraftEdits && state.proposalDraftEdits.id === prop.id)
        ? state.proposalDraftEdits
        : null;
      const branchVal = edits ? edits.branch : prop.branchName;
      const msgVal = edits ? edits.message : prop.message;
      const titleVal = edits ? edits.title : (prop.prTitle || '');
      const bodyVal = edits ? edits.body : (prop.prBody || '');

      proposalHtml = `
        <div class="diag-card proposal-card">
          <div class="proposal-head">
            <h4>
              <span>🤖 Claude / AI Commit &amp; PR Önerisi</span>
              <span class="chip accent">Kullanıcı Onayı Bekliyor</span>
            </h4>
            <span class="proposal-time">${esc(prop.createdAt || '')}</span>
          </div>
          <div class="wt-note">
            Claude bir commit ve PR önerisi hazırladı. Bu öneri <strong>${esc(prop.baseBranch || 'origin/main')}</strong> tabanlı yeni bir dal açacak, yalnız aşağıdaki açık dosyaları stage edecek ve PR oluşturacaktır. İnceleyip onaylayabilir veya başlık/açıklama alanlarını doğrudan düzenleyebilirsiniz.
          </div>

          <div class="wt-field">
            <label class="wt-label" for="prop-edit-branch">Önerilen Dal Adı <span class="chip ok">${esc(prop.baseBranch || 'origin/main')} tabanlı</span></label>
            <input type="text" class="input-control mono" id="prop-edit-branch" value="${esc(branchVal)}" />
          </div>

          <div class="wt-field">
            <label class="wt-label">Commitlenecek Açık Dosyalar (${prop.files?.length || 0} adet)</label>
            <div class="file-list">
              ${(prop.files || []).map((f) => `<span class="file-tag special">${esc(f)}</span>`).join('')}
            </div>
          </div>

          ${prop.diffStat ? `
            <div class="wt-field">
              <label class="wt-label">Değişiklik Özeti (git diff --stat)</label>
              <pre class="terminal-box">${esc(prop.diffStat)}</pre>
            </div>
          ` : ''}

          <div class="wt-field">
            <label class="wt-label">🛡️ Güvenlik Kalkanı Kontrolü</label>
            <div class="layer-line ok"><strong>${esc(prop.shieldResult || '✓ 0 secret, 0 özel anahtar, 0 .gitignore ihlali (Tüm dosyalar güvenli)')}</strong></div>
          </div>

          <div class="wt-field">
            <label class="wt-label">📊 Dosya Sayısı &amp; Mesaj Doğrulaması</label>
            <div class="layer-line ${prop.countCheckMatch !== false ? 'ok' : 'warn'}">
              <strong>${esc(prop.countCheckMessage || `✓ ${prop.files?.length || 0} dosya seçildi`)}</strong>
            </div>
          </div>

          <div class="wt-field">
            <label class="wt-label" for="prop-edit-message">Commit Mesajı &amp; Co-Authors (Düzenlenebilir)</label>
            <textarea class="textarea-control commit-msg-box" id="prop-edit-message" rows="4">${esc(msgVal)}</textarea>
          </div>

          <div class="wt-field">
            <label class="wt-label" for="prop-edit-title">PR Başlığı (Düzenlenebilir)</label>
            <input type="text" class="input-control" id="prop-edit-title" value="${esc(titleVal)}" placeholder="GitHub PR başlığı belirleyin" />
          </div>

          <div class="wt-field">
            <label class="wt-label" for="prop-edit-body">PR Açıklaması (Düzenlenebilir)</label>
            <textarea class="textarea-control" id="prop-edit-body" rows="4" placeholder="GitHub PR açıklama metni belirleyin">${esc(bodyVal)}</textarea>
          </div>

          <div class="det-actions">
            <button type="button" class="btn ok sm" id="act-proposal-approve">✅ İncele &amp; Onayla (Commit + Push + PR)</button>
            <button type="button" class="btn ghost sm" id="act-proposal-dismiss">❌ Öneriyi Reddet</button>
          </div>
        </div>
      `;
    }

    const b = snap.branches.find((x) => x.id === state.selectedBranchId);
    if (!b) {
      if (proposalHtml) {
        el.detail.innerHTML = `${proposalHtml}<div class="empty-detail">İncelemek için soldaki parabolik grafikten bir dal kapsülü veya düğüm seçin.</div>`;
        wireProposalActions();
        return;
      }
      el.detail.innerHTML = '<div class="empty-detail">İncelemek için soldaki parabolik grafikten bir dal kapsülü veya düğüm seçin.</div>';
      return;
    }

    const isLive = snap.repo.mode === 'live';
    const hasConflict = b.conflict?.hasConflict && !b.conflict.resolved;
    const isOpen = b.state === 'open';
    const isRemoteOnly = !b.hasLocal && b.hasRemote;
    const isVirtualWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-') || b.name === 'wip/working-tree' || (b.name && b.name.includes('Kaydedilmemiş'));
    const ciState = b.pr?.ci?.state;
    const isRunning = ciState === 'RUNNING';
    const isQueued = Boolean(state.pendingAutoMerge[b.id]);
    const canCommitWorkingTree = isVirtualWip || (b.uncommittedCount || 0) > 0 || (b.isHead && (state.snapshot?.repo?.dirtyCount || 0) > 0);
    const canPromote = isOpen && b.hemisphere === 'lower' && !isVirtualWip && !canCommitWorkingTree;
    const hasOpenPR = Boolean(b.pr && b.pr.number > 0 && b.pr.reviewState !== 'MERGED');
    const canMerge = isOpen && hasOpenPR && !hasConflict && !isVirtualWip && ciState !== 'FAILURE';

    const ciChip = ciState === 'FAILURE' ? '<span class="chip danger">CI: FAILURE</span>'
      : ciState === 'RUNNING' ? '<span class="chip warn running-pulse">CI: RUNNING</span>'
        : ciState === 'SUCCESS' ? '<span class="chip ok">CI: PASS</span>' : '';

    const queuedChip = isQueued ? '<span class="chip warn running-pulse">⏳ Auto-Merge Kuyruğunda</span>' : '';

    let html = `
      ${proposalHtml}
      <div class="det-header">
        <div class="det-title-row">
          <h3 class="det-branch-name">${isVirtualWip ? '✏️ Çalışma Alanı (Kaydedilmemiş Değişiklikler)' : esc(b.name)}</h3>
          <span class="chip ${b.state === 'merged' ? 'ok' : (isVirtualWip ? 'purple' : (b.hemisphere === 'lower' ? 'purple' : 'accent'))}">
            ${isVirtualWip ? 'Yerel Taslak · Uncommitted' : (b.state === 'merged' ? 'Tam Parabol' : (b.hemisphere === 'lower' && b.aheadMain === 0 ? '🌱 Yeni Yerel Dal' : b.hemisphere === 'lower' ? 'Alt Ray · Local' : 'Üst Ray · PR'))}
          </span>
        </div>
        <div class="det-badges">
          ${b.pr ? `<span class="chip accent">PR #${b.pr.number}</span>` : ''}
          ${ciChip}
          ${queuedChip}
          ${b.isHead ? '<span class="chip ok">HEAD</span>' : ''}
          ${isRemoteOnly ? '<span class="chip warn">☁ Yerelde Yok (Sadece Origin)</span>' : ''}
          ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
        </div>
        <div class="det-actions">
          ${!b.isHead && isOpen && !isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-checkout" ${snap.repo.teamMode ? 'disabled title="Team Server modunda paylaşılan HEAD kilitlidir"' : ''}>${isRemoteOnly ? '🎯 Yerelde İndir &amp; Geç' : '🎯 Dala Geç'}</button>` : ''}
          ${canPromote && !hasOpenPR ? `<button type="button" class="btn primary sm" id="act-promote">🚀 Push &amp; PR Aç</button>` : ''}
          ${hasOpenPR && !isVirtualWip ? `
            <div class="merge-group">
              <select id="merge-strategy" class="select-inline" aria-label="Merge stratejisi">
                <option value="merge" ${state.mergeStrategy === 'merge' ? 'selected' : ''}>Merge</option>
                <option value="squash" ${state.mergeStrategy === 'squash' ? 'selected' : ''}>Squash</option>
                <option value="rebase" ${state.mergeStrategy === 'rebase' ? 'selected' : ''}>Rebase</option>
                <option value="auto" ${state.mergeStrategy === 'auto' ? 'selected' : ''}>Auto-Merge (Queue)</option>
              </select>
              ${isRunning
                ? `<button type="button" class="btn warn sm ${isQueued ? 'running-pulse' : ''}" id="act-merge">${isQueued ? '⏳ CI Bekleniyor (İptal Et)' : '⏳ CI Bitince Otomatik Merge Et'}</button>`
                : `<button type="button" class="btn ok sm" id="act-merge" ${canMerge ? '' : 'disabled'}>⚡ Birleştir</button>`}
            </div>
          ` : ''}
          ${hasConflict ? `<button type="button" class="btn warn sm" id="act-resolve">⚡ Çakışma Stüdyosunu Aç</button>` : ''}
          ${!isVirtualWip && (b.state !== 'merged' || b.hasLocal || b.originGone || b.hasRemote) ? `<button type="button" class="btn ghost sm" id="act-delete">${isRemoteOnly ? '🗑️ PR Kapat &amp; Uzak Dalı Sil' : '🧹 Sil / Temizle'}</button>` : ''}
        </div>
      </div>
    `;

    const upperSummary = b.pr
      ? `PR #${b.pr.number} · ${b.pr.reviewState}`
      : (isRemoteOnly ? `origin/${truncate(b.name, 16)}` : (b.originGone ? 'origin: gone' : 'PR Yok'));

    const lowerSummary = isVirtualWip
      ? `${b.uncommittedCount || snap.repo?.dirtyCount || 0} dosya bekliyor`
      : (!b.hasLocal ? 'Yerelde Yok' : (b.isHead ? 'HEAD (Aktif)' : `↑${b.aheadMain || 0} / ↓${b.behindMain || 0}`));

    html += `
        <div class="layer-accordion-container">
          <details class="layer-accordion">
            <summary class="layer-summary">
              <span class="layer-summary-title">☁️ Üst Ray · Remote / PR</span>
              <span class="layer-summary-badge">${esc(upperSummary)}</span>
            </summary>
            <div class="layer-scroll-body">
              ${b.pr ? `
                <div class="layer-line">PR #${b.pr.number} · ${esc(b.pr.reviewState)}</div>
                <div class="layer-line">${esc(truncate(b.pr.title, 56))}</div>
                <div class="layer-line">CI: ${esc(b.pr.ci?.workflow || 'ci.yml')} (${esc(b.pr.ci?.duration || '—')})</div>
              ` : `
                <div class="layer-line">${isRemoteOnly ? `Remote aktif: origin/${esc(b.name)}` : (b.originGone ? 'Remote silindi (origin: gone)' : 'Henüz PR / Remote yok')}</div>
                <div class="layer-line">${isRemoteOnly ? 'Yerelde kopyası silinmiş uzak dal' : 'Alt rayda yerel çalışma'}</div>
              `}
            </div>
          </details>

          <details class="layer-accordion">
            <summary class="layer-summary">
              <span class="layer-summary-title">💻 Alt Ray · Local</span>
              <span class="layer-summary-badge">${esc(lowerSummary)}</span>
            </summary>
            <div class="layer-scroll-body">
              ${isVirtualWip ? `
                <div class="layer-line">Konum: main omurgası (Kaydedilmemiş Çalışma)</div>
                <div class="layer-line">Durum: Henüz bir dala veya commite aktarılmamış</div>
                <div class="layer-line">Değişen Dosyalar: ${b.uncommittedCount || state.snapshot?.repo?.dirtyCount || 0} adet dosya</div>
              ` : (!b.hasLocal ? `
                <div class="layer-line">${b.state === 'merged' && !b.originGone ? '✓ Yerel kalıntı dal temizlendi' : `Yerelde dal yok (origin/${esc(b.name)})`}</div>
                <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
                <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
              ` : `
                ${b.state === 'merged' ? '<div class="layer-line">⚠ Birleşmiş yerel kalıntı (tek tıkla temizlenebilir)</div>' : (b.aheadMain === 0 && !isVirtualWip ? '<div class="layer-line">🌱 main omurgasında taze dal (ilk commitinizi bekliyor)</div>' : '')}
                <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
                <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
                <div class="layer-line">Uncommitted (Staged + Untracked): ${b.uncommittedCount || (b.isHead ? state.snapshot?.repo?.dirtyCount : 0) || 0} dosya</div>
              `)}
            </div>
          </details>
        </div>
      `;

    if (b.conflict) {
      const resolved = b.conflict.resolved;
      html += `
        <div class="diag-card ${resolved ? '' : 'conflict'}">
          <h4>
            <span>${resolved ? '✓ Çakışma Çözüldü' : (b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lockfile Çakışması' : '⚠ Merge Çakışması')}</span>
            <span class="chip">${esc(b.conflict.strategy || 'merge-tree')}</span>
          </h4>
          <div class="layer-line">${resolved ? esc(b.conflict.resolvedNote) : `git merge-tree --write-tree: ${esc((b.conflict.files || []).join(', '))}`}</div>
          <pre class="terminal-box">${esc(b.conflict.previewUnified || '')}</pre>
          ${!resolved ? `<div class="det-actions"><button type="button" class="btn warn sm" id="act-resolve-card">⚡ Çakışma Stüdyosunu Aç (Sol: Olan / Sağ: Olacak)</button></div>` : ''}
        </div>
      `;
    }

    if (b.crossPr?.length) {
      html += `
        <div class="diag-card cross">
          <h4><span>⇄ Cross-PR Dosya Çarpışması</span></h4>
          ${b.crossPr.map((c) => `
            <div class="layer-line"><strong>${c.otherPrNumber ? 'PR #' + c.otherPrNumber : ''} (${esc(c.otherBranch)})</strong>: ${esc(c.sharedFiles.join(', '))}</div>
          `).join('')}
        </div>
      `;
    }

    if (b.pr?.ci?.state === 'FAILURE') {
      const extracts = b.pr.ci.extracts || [];
      const localRunOut = state.localRunOutput[b.id] || '';
      html += `
        <div class="diag-card ci">
          <h4>
            <span>Gürültüsüz CI Hata Ayıklayıcı</span>
            <span class="chip danger">${extracts.length} kritik bulgu</span>
          </h4>
          <div class="extract-list">
            ${extracts.map((ex) => `
              <div class="extract-item">
                <div class="extract-head">
                  <span>${esc(ex.kind.toUpperCase())} · ${esc(ex.symbol)}</span>
                  <span>${esc(ex.fileLine || '')}</span>
                </div>
                <div class="extract-msg">${esc(ex.message)}</div>
                ${ex.reproCmd ? `<div class="extract-cmd">$ ${esc(ex.reproCmd)}</div>` : ''}
              </div>
            `).join('')}
          </div>
          <div class="det-actions">
            ${!isLive ? '<button type="button" class="btn emerald sm" id="act-fix-ci">🛠️ Hatayı Düzelt &amp; CI Yeniden Çalıştır</button>' : ''}
            <button type="button" class="btn sm" id="act-copy-log">Logu Kopyala</button>
            <button type="button" class="btn primary sm" id="act-run-local">▶ Yerelde Testi Çalıştır</button>
          </div>
          <pre class="terminal-box" id="ci-raw-log">${esc(localRunOut || b.pr.ci.rawLog || 'Ham log bulunamadı.')}</pre>
        </div>
      `;
    }

    if (canCommitWorkingTree) {
      const isMerged = isMergedOrGone(b) || b.state === 'merged' || b.pr?.reviewState === 'MERGED';
      const candidateFiles = (b.changedFiles || []).filter(Boolean);
      const secretFile = candidateFiles.find((f) => isSecretFile(f, snap));

      let defaultWtBranch = b.name;
      let branchPlaceholder = 'feat/ozellik-adi';
      let defaultCommitMsg = b.lastCommitMsg || '';
      if (/update\s+\d+\s+files/i.test(defaultCommitMsg) || defaultCommitMsg.startsWith('Pushlanmamış yerel çalışma')) {
        defaultCommitMsg = '';
      }
      let defaultPrTitle = b.draftPrTitle || defaultCommitMsg;
      if (/update\s+\d+\s+files/i.test(defaultPrTitle)) {
        defaultPrTitle = '';
      }
      let defaultPrBody = b.draftPrBody || '';

      if (isVirtualWip || isMerged) {
        defaultWtBranch = suggestUniqueBranchName('feat', candidateFiles, snap);
        branchPlaceholder = 'Örn: ' + defaultWtBranch;

        let prefix = 'feat';
        let scope = '';
        const first = (candidateFiles[0] || '').toLowerCase();
        if (first.startsWith('internal/mcp')) { scope = '(mcp)'; prefix = 'feat'; }
        else if (first.startsWith('internal/gitengine')) { scope = '(engine)'; prefix = 'feat'; }
        else if (first.startsWith('web/')) { scope = '(ui)'; prefix = 'feat'; }
        else if (first.endsWith('.md') || first.startsWith('docs/')) { scope = '(docs)'; prefix = 'docs'; }
        else if (first.includes('test')) { scope = '(test)'; prefix = 'test'; }

        const fileSummary = candidateFiles.length === 1
          ? candidateFiles[0]
          : `${candidateFiles.length} dosya (${candidateFiles.slice(0, 2).map((f) => f.split('/').pop()).join(', ')}${candidateFiles.length > 2 ? '...' : ''})`;

        defaultCommitMsg = `${prefix}${scope}: ${fileSummary} güncellemeleri ve iyileştirmeler`;
        defaultPrTitle = `${prefix}${scope}: ${fileSummary} Güncellemeleri`;
        defaultPrBody = `### Değişiklik Özeti\n${candidateFiles.map((f) => `- \`${f}\``).join('\n')}\n\n### Doğrulama\n- Otomatik testler ve kontroller başarıyla tamamlandı.`;
      }
      if (snap.pendingProposal) {
        defaultWtBranch = snap.pendingProposal.branchName || defaultWtBranch;
        defaultCommitMsg = snap.pendingProposal.message || defaultCommitMsg;
        defaultPrTitle = snap.pendingProposal.prTitle || defaultCommitMsg;
        defaultPrBody = snap.pendingProposal.prBody || defaultPrBody;
      }
      const defBranch = snap.repo?.defaultBranch || 'main';
      const wtNote = isMerged
        ? `Bu dal (<strong>${esc(b.name)}</strong>) GitHub'da birleşmiş. Kaydedilmemiş değişiklikleriniz korunarak <strong>origin/${esc(defBranch)}</strong> tabanlı yeni bir dala aktarılacaktır.`
        : (isVirtualWip
          ? `Şu an <strong>${esc(defBranch)}</strong> omurgası üzerindeki kaydedilmemiş açık değişikliklerinizi görüyorsunuz. Terminal açmadan doğrudan yeni bir dal açabilir, commit atabilir ve PR oluşturabilirsiniz.`
          : 'Bu daldaki kaydedilmemiş değişiklikleri konsol kullanmadan yerelde commitleyin veya tek tıkla pushlayıp PR açın.');

      html += `
        <div class="diag-card">
          <h4>
            <span>🚀 Konsolsuz Dal Aç, Commit &amp; PR Oluştur</span>
            <span class="chip accent">${candidateFiles.length || b.uncommittedCount || (b.isHead ? snap.repo?.dirtyCount : 0) || 0} dosya</span>
          </h4>
          <div class="wt-note">${wtNote}</div>
          ${candidateFiles.length > 0 ? `
            <div class="wt-field">
              <label class="wt-label">Commitlenecek Staged / Takipsiz Dosyalar (${candidateFiles.length} adet)</label>
              <div class="file-list">
                ${candidateFiles.map((f) => {
                  const isSec = isSecretFile(f, snap);
                  return `<span class="file-tag ${isSec ? 'special danger' : ''}">${esc(f)}${isSec ? ' 🚨 GİZLİ DOSYA' : ''}</span>`;
                }).join('')}
              </div>
              ${secretFile ? `<div class="lock-note danger">🚨 GÜVENLİK ENGELİ: Staged dosyalar arasında gizli/korumalı dosya (<strong>${esc(secretFile)}</strong>) tespit edildi! Commit ve Push kilitlendi.</div>` : ''}
            </div>
          ` : ''}
          <form id="form-commit-wt" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="wt-branch-name">Özellik Dalı Adı ${isMerged || isVirtualWip ? '<span class="chip warn">Yeni dal adı girin</span>' : ''}</label>
              <input type="text" id="wt-branch-name" class="input-control mono" value="${esc(defaultWtBranch)}" placeholder="${esc(branchPlaceholder)}" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-commit-msg">Commit Mesajı</label>
              <input type="text" id="wt-commit-msg" class="input-control mono" value="${esc(defaultCommitMsg)}" placeholder="feat: yapılan değişikliği açıklayın" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-title">PR Başlığı (Push &amp; PR için)</label>
              <input type="text" id="wt-pr-title" class="input-control mono" value="${esc(defaultPrTitle)}" placeholder="PR başlığı">
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-body">PR Açıklaması</label>
              <textarea id="wt-pr-body" class="textarea-control" placeholder="Neler değişti ve nasıl doğrulandı?">${esc(defaultPrBody)}</textarea>
            </div>
            <div class="det-actions">
              <button type="submit" class="btn emerald sm" id="act-wt-commit-only" ${snap.repo.teamMode || secretFile ? 'disabled' : ''}>💾 Sadece Dal Aç &amp; Commit At</button>
              <button type="button" class="btn primary sm" id="act-wt-commit-push" ${snap.repo.teamMode || secretFile ? 'disabled' : ''}>${b.pr && b.pr.number > 0 ? `🚀 Commit &amp; PR #${b.pr.number}'a Pushla` : '🚀 Commit + Push &amp; PR Oluştur'}</button>
              <button type="button" class="btn warn sm" id="act-wt-discard" ${snap.repo.teamMode ? 'disabled' : ''}>🗑️ Değişiklikleri Geri Al (Discard)</button>
            </div>
          </form>
        </div>
      `;
    }

    if (canPromote) {
      const savedDraft = state.draftPrEdits[b.name];
      const promoteTitle = savedDraft?.title ?? b.draftPrTitle ?? b.lastCommitMsg ?? ('feat: ' + b.name);
      const promoteBody = savedDraft?.body ?? b.draftPrBody ?? '';
      html += `
        <div class="diag-card">
          <h4>
            <span>📝 Push Öncesi PR Başlık &amp; Açıklama</span>
            <span class="chip purple">Local → Remote</span>
          </h4>
          <div class="wt-note">Dalı origin’e pushlamadan önce PR başlığını ve açıklamasını burada hazırlayıp kaydedebilirsiniz.</div>
          <form id="form-promote-pr" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="promote-pr-title">PR Başlığı</label>
              <input type="text" id="promote-pr-title" class="input-control mono" value="${esc(promoteTitle)}" placeholder="Örn: feat(ui): add new feature" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="promote-pr-body">PR Açıklaması</label>
              <textarea id="promote-pr-body" class="textarea-control" placeholder="Neler değişti ve doğrulama adımları...">${esc(promoteBody)}</textarea>
            </div>
            <div class="det-actions">
              <button type="submit" class="btn primary sm" id="act-promote-submit">🚀 Pushla &amp; Bu Bilgilerle PR Aç</button>
            </div>
          </form>
        </div>
      `;
    }

    if (b.changedFiles?.length) {
      html += `
        <div class="diag-card">
          <h4>Değişen Dosyalar (${b.changedFiles.length})</h4>
          <div class="file-list">
            ${b.changedFiles.map((f) => {
              const special = /changelog|components\.lock|go\.mod|go\.sum|version/i.test(f);
              return `<span class="file-tag ${special ? 'special' : ''}">${esc(f)}</span>`;
            }).join('')}
          </div>
        </div>
      `;
    }

    if (b.pr && isOpen) {
      html += `
        <div class="diag-card">
          <h4>✏️ PR #${b.pr.number} Başlık &amp; Açıklama Düzenle</h4>
          <form id="form-edit-pr" class="edit-form">
            <input type="text" id="pr-edit-title" class="input-control mono" value="${esc(b.pr.title)}" placeholder="PR başlığı" required>
            <textarea id="pr-edit-body" class="textarea-control" placeholder="PR açıklaması (opsiyonel)">${esc(b.pr.body || '')}</textarea>
            <div class="det-actions">
              <button type="submit" class="btn primary sm">PR Bilgisini Güncelle</button>
            </div>
          </form>
        </div>
      `;
    }

    if (b.canAmendCommit) {
      html += `
        <div class="diag-card">
          <h4>✏️ Son Commit Mesajını Değiştir (--amend --only)</h4>
          <form id="form-amend-commit" class="edit-form">
            <input type="text" id="commit-amend-msg" class="input-control mono" value="${esc(b.lastCommitMsg || '')}" placeholder="Yeni commit mesajı" required>
            <div class="det-actions">
              <button type="submit" class="btn sm">Commit Mesajını Güncelle</button>
            </div>
          </form>
        </div>
      `;
    } else if (isOpen && !isVirtualWip && !b.isHead) {
      html += `<div class="lock-note">🔒 Commit mesajı yalnızca aktif çalışma dalı (HEAD) iken değiştirilebilir. Önce <strong>🎯 Dala Geç</strong> ile geçiş yapın.</div>`;
    }

    if (b.suggestedCmd) {
      html += `
        <div class="diag-card">
          <h4>
            <span>📋 Tek Tıkla Terminal Komutu</span>
            <button type="button" class="btn sm" id="act-copy-cmd">Kopyala</button>
          </h4>
          <div class="cmd-box"><code class="cmd-code">${esc(b.suggestedCmd)}</code></div>
        </div>
      `;
    }

    el.detail.innerHTML = html;

    wireProposalActions();

    document.getElementById('act-checkout')?.addEventListener('click', (ev) => handleCheckoutBranch(b.id, ev.currentTarget));
    document.getElementById('act-promote')?.addEventListener('click', (ev) => {
      const titleInput = document.getElementById('promote-pr-title');
      if (titleInput) {
        titleInput.scrollIntoView({ behavior: 'smooth', block: 'center' });
        titleInput.focus();
        showToast('Push öncesi PR başlık ve açıklamasını kontrol edip formu onaylayın', 'info');
      } else {
        handlePromoteBranch(b.id, undefined, undefined, ev.currentTarget);
      }
    });
    document.getElementById('form-commit-wt')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const bName = document.getElementById('wt-branch-name').value.trim();
      const existing = (state.snapshot?.branches || []).find((x) => x.name.toLowerCase() === bName.toLowerCase());
      if (existing && (existing.state === 'merged' || existing.originGone)) {
        showToast(`⚠️ '${bName}' dalı daha önce birleşmiş (merged/gone). Lütfen benzersiz bir dal adı belirleyin.`, 'warn');
        return;
      }
      const cMsg = document.getElementById('wt-commit-msg').value;
      const pTitle = document.getElementById('wt-pr-title').value;
      const pBody = document.getElementById('wt-pr-body').value;
      const btn = document.getElementById('act-wt-commit-only');
      handleCommitWorkingTree(bName, cMsg, pTitle, pBody, false, btn);
    });
    document.getElementById('act-wt-commit-push')?.addEventListener('click', (ev) => {
      const form = document.getElementById('form-commit-wt');
      if (form && form.reportValidity && !form.reportValidity()) return;
      const bName = document.getElementById('wt-branch-name').value.trim();
      const existing = (state.snapshot?.branches || []).find((x) => x.name.toLowerCase() === bName.toLowerCase());
      if (existing && (existing.state === 'merged' || existing.originGone)) {
        showToast(`⚠️ '${bName}' dalı daha önce birleşmiş (merged/gone). Lütfen benzersiz bir dal adı belirleyin.`, 'warn');
        return;
      }
      const cMsg = document.getElementById('wt-commit-msg').value;
      const pTitle = document.getElementById('wt-pr-title').value;
      const pBody = document.getElementById('wt-pr-body').value;
      const btn = ev.currentTarget || document.getElementById('act-wt-commit-push');
      handleCommitWorkingTree(bName, cMsg, pTitle, pBody, true, btn);
    });
    document.getElementById('act-wt-discard')?.addEventListener('click', (ev) => handleDiscardWorkingTree(ev.currentTarget));
    document.getElementById('promote-pr-title')?.addEventListener('input', (ev) => {
      state.draftPrEdits[b.name] = {
        title: ev.target.value,
        body: document.getElementById('promote-pr-body')?.value || '',
      };
    });
    document.getElementById('promote-pr-body')?.addEventListener('input', (ev) => {
      state.draftPrEdits[b.name] = {
        title: document.getElementById('promote-pr-title')?.value || '',
        body: ev.target.value,
      };
    });
    document.getElementById('form-promote-pr')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const pTitle = document.getElementById('promote-pr-title').value;
      const pBody = document.getElementById('promote-pr-body').value;
      const btn = document.getElementById('act-promote-submit');
      handlePromoteBranch(b.id, pTitle, pBody, btn);
    });
    document.getElementById('merge-strategy')?.addEventListener('change', (ev) => {
      state.mergeStrategy = ev.target.value;
    });
    document.getElementById('act-merge')?.addEventListener('click', (ev) => handleMergeBranch(b.id, ev.currentTarget));
    document.getElementById('act-resolve')?.addEventListener('click', () => openConflictStudio(b.id));
    document.getElementById('act-resolve-card')?.addEventListener('click', () => openConflictStudio(b.id));
    document.getElementById('act-fix-ci')?.addEventListener('click', () => handleFixCI(b.id));
    document.getElementById('act-delete')?.addEventListener('click', (ev) => handleDeleteBranch(b.id, ev.currentTarget));
    document.getElementById('form-edit-pr')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const title = document.getElementById('pr-edit-title').value;
      const body = document.getElementById('pr-edit-body').value;
      handleEditPR(b.id, title, body);
    });
    document.getElementById('form-amend-commit')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const msg = document.getElementById('commit-amend-msg').value;
      handleAmendCommit(b.id, msg);
    });
    document.getElementById('act-copy-cmd')?.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.suggestedCmd);
        showToast('Terminal komutu panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi', 'warn');
      }
    });
    document.getElementById('act-copy-log')?.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.pr.ci.rawLog || '');
        showToast('CI hata logu panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi; logu seçip kopyalayabilirsiniz', 'warn');
      }
    });
    document.getElementById('act-run-local')?.addEventListener('click', () => {
      const cmd = b.pr.ci.extracts?.[0]?.reproCmd || 'go test -v ./...';
      handleLocalTestRun(b.id, cmd);
    });
  }

  function populateBaseCommitOptions() {
    const nodes = state.snapshot?.nodes || [];
    el.newBranchBase.innerHTML = '';
    [...nodes].reverse().forEach((n) => {
      const opt = document.createElement('option');
      opt.value = n.shortSha;
      opt.textContent = `${n.shortSha} — ${truncate(n.subject, 36)}`;
      el.newBranchBase.appendChild(opt);
    });
  }

  // Action Handlers
  async function handleCheckoutBranch(branchId, btnEl = null) {
    await runWithBtnBusy(btnEl, '⏳ Dala Geçiliyor...', async () => {
      try {
        const snap = state.offlineMode
          ? LocalFallbackEngine.checkoutBranch(branchId)
          : await apiCall('/api/branch/checkout', 'POST', { branchId });
        applySnapshot(snap);
        showToast('Çalışma dalı (HEAD) değiştirildi', 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  async function handleEditPR(branchId, title, body) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.updatePRMeta(branchId, title, body)
        : await apiCall('/api/pr/edit', 'POST', { branchId, title, body });
      applySnapshot(snap);
      showToast('PR başlık ve açıklaması güncellendi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleAmendCommit(branchId, message) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.amendLastCommit(branchId, message)
        : await apiCall('/api/commit/amend', 'POST', { branchId, message });
      applySnapshot(snap);
      showToast('Son commit mesajı (--amend --only) güncellendi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function executeMergeNow(branchId, strategy = 'merge', fromQueue = false, btnEl = null) {
    await runWithBtnBusy(btnEl, '⏳ Birleştiriliyor...', async () => {
      try {
        const snap = state.offlineMode
          ? LocalFallbackEngine.mergeBranch(branchId, strategy)
          : await apiCall('/api/branch/merge', 'POST', { branchId, strategy });
        applySnapshot(snap);
        showToast(
          fromQueue
            ? `✅ CI başarılı! Dal otomatik olarak (${strategy}) main'e birleştirildi.`
            : `Dal merkez main omurgasına (${strategy}) birleşti — Tam parabol tamamlandı!`,
          'ok',
        );
      } catch (err) {
        showToast(err.message, 'warn');
        renderSVG();
      }
    });
  }

  async function handleMergeBranch(branchId, btnEl = null) {
    const b = state.snapshot?.branches?.find((x) => x.id === branchId);
    if (!b) return;
    const strategy = state.mergeStrategy || 'merge';
    const defBranch = state.snapshot?.repo?.defaultBranch || 'main';

    let cmd = '';
    if (b.pr && b.pr.number > 0) {
      const flag = strategy === 'squash' ? '--squash' : strategy === 'rebase' ? '--rebase' : '--merge';
      cmd = `gh pr merge ${b.pr.number} ${flag} --auto`;
    } else {
      const flag = strategy === 'squash' ? '--squash' : '--no-ff';
      cmd = `git checkout ${defBranch} && git merge ${flag} ${b.name}`;
    }

    const confirmed = window.confirm(
      `⚠️ MERGE ONAYI:\n"${b.name}" dalı ${defBranch} omurgasına (${strategy}) birleştirilecek.\n\nÇalıştırılacak Komut:\n${cmd}\n\nBu birleştirme işlemini onaylıyor musunuz?`,
    );
    if (!confirmed) return;

    if (b?.pr?.ci?.state === 'RUNNING') {
      if (state.pendingAutoMerge[branchId]) {
        delete state.pendingAutoMerge[branchId];
        recordClientLog('info', 'Auto-Merge İptal', b.name, 'CI bekleme kuyruğundan çıkarıldı');
        showToast(`${b.name} için otomatik birleştirme kuyruğu iptal edildi`, 'info');
      } else {
        state.pendingAutoMerge[branchId] = { strategy, branchName: b.name };
        recordClientLog('ok', 'Auto-Merge Kuyruğu', b.name, `CI geçince (${strategy}) otomatik birleştirilecek`);
        showToast(`⏳ ${b.name} kuyruğa alındı: CI geçince otomatik merge edilecek!`, 'info');
        if (state.offlineMode) {
          setTimeout(() => {
            if (!state.pendingAutoMerge[branchId]) return;
            try {
              const nextSnap = LocalFallbackEngine.completeRunningCI(branchId);
              applySnapshot(nextSnap);
            } catch {
              // ignore
            }
          }, 2600);
        }
      }
      renderSVG();
      renderInspector();
      return;
    }
    await executeMergeNow(branchId, strategy, false, btnEl);
  }

  async function handleCommitWorkingTree(branchName, commitMsg, prTitle, prBody, pushAndPr, btnEl = null, explicitFiles = null) {
    const cleanBranch = (branchName || '').trim();
    const cleanCommit = (commitMsg || '').trim();
    if (!cleanBranch || !cleanCommit) {
      showToast('Özellik dalı adı ve commit mesajı zorunludur', 'warn');
      return;
    }

    const snap = state.snapshot;
    const curBranch = snap?.branches?.find((x) => x.isHead)?.name || '';
    const defBranch = snap?.repo?.defaultBranch || 'main';

    const b = snap?.branches?.find((x) => x.id === state.selectedBranchId) || snap?.branches?.find((x) => x.isHead);
    const candidateFiles = (explicitFiles && explicitFiles.length > 0)
      ? explicitFiles
      : (b?.changedFiles || []).filter(Boolean);

    // 1. Secret file check
    const secretFile = candidateFiles.find((f) => isSecretFile(f, snap));
    if (secretFile) {
      showToast(`🚨 GÜVENLİK ENGELİ: Gizli dosya tespit edildi: '${secretFile}'. Commit engellendi!`, 'danger');
      return;
    }

    const existingBranch = (snap?.branches || []).find((x) => x.name === cleanBranch);
    if (pushAndPr && existingBranch && isMergedOrGone(existingBranch)) {
      showToast(`'${cleanBranch}' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın üzerine push yapılamaz! Lütfen yeni bir dal adı belirleyin.`, 'warn');
      return;
    }

    const isNewBranch = cleanBranch !== curBranch;
    const switchCmd = isNewBranch ? `git checkout -b ${cleanBranch}` : '';

    // NEVER use git add -A or git add .!
    const filesListStr = candidateFiles.length > 0 ? candidateFiles.join(' ') : '<dosyalar>';
    const addCmd = `git add -- ${filesListStr}`;
    const commitCmd = `git commit -m "${cleanCommit}"`;

    const fileListPreview = candidateFiles.length > 0
      ? `\nCommitlenecek Dosyalar (${candidateFiles.length} adet):\n` + candidateFiles.map((f) => ` • ${f}`).join('\n') + '\n'
      : '';

    if (!pushAndPr) {
      state.draftPrEdits[cleanBranch] = {
        title: (prTitle || '').trim() || cleanCommit,
        body: (prBody || '').trim(),
      };
      const fullCmd = [switchCmd, addCmd, commitCmd].filter(Boolean).join(' && ');
      const confirmed = window.confirm(
        `💾 COMMIT ONAYI:\nÇalışma dizini '${cleanBranch}' dalına kaydedilecektir.${fileListPreview}\nCommit Mesajı: "${cleanCommit}"\n\nÇalıştırılacak Komut:\n${fullCmd}\n\nBu işlemi onaylıyor musunuz?`,
      );
      if (!confirmed) return;
    } else {
      delete state.draftPrEdits[cleanBranch];

      // Fetch diff stat
      let diffStat = null;
      if (!state.offlineMode) {
        try {
          diffStat = await apiCall('/api/working-tree/diff-stat', 'GET');
        } catch (e) {
          console.warn('diff-stat fetch failed', e);
        }
      }

      const pushCmd = `git push -u origin ${cleanBranch}`;
      const prCmd = `gh pr create --head ${cleanBranch} --title "${prTitle || cleanCommit}"`;
      const fullCmd = [switchCmd, addCmd, commitCmd, pushCmd, prCmd].filter(Boolean).join(' && ');

      const statPreview = diffStat?.stat ? `\n📊 Değişiklik Özeti (git diff --stat):\n${diffStat.stat}\n` : '';

      const confirmed = window.confirm(
        `⚠️ PUSH & PR ONAYI:\nÇalışma dizini kaydedilip doğrudan uzak sunucuya pushlanacak ve GitHub PR açılacaktır.${fileListPreview}${statPreview}\nÇalıştırılacak Komut:\n${fullCmd}\n\nBu işlemi onaylıyor musunuz?`,
      );
      if (!confirmed) return;

      // 10'dan fazla dosya ya da 1000+ satır varsa ekstra onay iste (Madde 5)
      const filesCount = diffStat?.filesChanged || candidateFiles.length;
      const totalLines = diffStat?.totalLines || 0;
      if (filesCount > 10 || totalLines >= 1000 || diffStat?.isLargeDiff) {
        const largeConfirmed = window.confirm(
          `🚨 DİKKAT: BÜYÜK DEĞİŞİKLİK TESPİT EDİLDİ!\n\nBu işlem ${filesCount} adet dosya ve ${totalLines > 0 ? totalLines + ' satır ' : ''}değişiklik içermektedir!\nHalka açık (public) depoya yanlışlıkla büyük veri, gizli anahtar veya istemediğiniz dosyaları itmediğinizden kesinlikle emin misiniz?\n\nYine de devam etmek istiyor musunuz?`,
        );
        if (!largeConfirmed) return;
      }
    }

    await runWithBtnBusy(btnEl, pushAndPr ? '⏳ Push & PR Oluşturuluyor...' : '⏳ Commitleniyor...', async () => {
      try {
        const snapResult = state.offlineMode
          ? LocalFallbackEngine.commitWorkingTree(cleanBranch, cleanCommit, prTitle || '', prBody || '', Boolean(pushAndPr), candidateFiles)
          : await apiCall('/api/working-tree/commit', 'POST', {
            branchName: cleanBranch,
            commitMsg: cleanCommit,
            prTitle: (prTitle || '').trim(),
            prBody: (prBody || '').trim(),
            pushAndPr: Boolean(pushAndPr),
            files: candidateFiles,
          });
        const created = (snapResult.branches || []).find((x) => x.name === cleanBranch);
        if (created) {
          state.selectedBranchId = created.id;
        }
        applySnapshot(snapResult);
        showToast(
          pushAndPr
            ? `Dal açıldı, commitlendi, pushlandı ve PR oluşturuldu (${cleanBranch})!`
            : `Değişiklikler '${cleanBranch}' dalına commitlendi!`,
          'ok',
        );
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  async function handleDiscardWorkingTree(btnEl = null) {
    const dirtyCount = state.snapshot?.repo?.dirtyCount || 0;
    const ok = window.confirm(`⚠️ DİKKAT: Kaydedilmemiş ${dirtyCount} adet değişiklik/dosya (staged ve untracked) tamamen geri alınacak ve temizlenecektir.\n\nBu işlem geri alınamaz. Devam etmek istiyor musunuz?`);
    if (!ok) return;

    await runWithBtnBusy(btnEl, '⏳ Sıfırlanıyor...', async () => {
      try {
        let snap;
        if (state.offlineMode) {
          snap = LocalFallbackEngine.discardWorkingTree();
        } else {
          snap = await apiCall('/api/working-tree/discard', 'POST');
        }
        applySnapshot(snap);
        showToast('✓ Kaydedilmemiş değişiklikler temizlendi', 'info');
      } catch (err) {
        showToast(`Hata: ${err.message}`, 'danger');
      }
    });
  }

  function openAddProjectModal() {
    el.addProjectModal.hidden = false;
    el.projectPathInput.value = '';
    el.projectNameInput.value = '';
    el.projectPathInput.focus();
  }

  function closeAddProjectModal() {
    el.addProjectModal.hidden = true;
  }

  async function handleAddProjectSubmit(ev) {
    ev.preventDefault();
    const repoPath = el.projectPathInput.value.trim();
    const repoName = el.projectNameInput.value.trim();
    if (!repoPath) {
      showToast('Lütfen geçerli bir yerel depo yolu girin', 'warn');
      return;
    }
    const btn = document.getElementById('add-proj-submit');
    if (btn) btn.disabled = true;

    try {
      let snap;
      if (state.offlineMode) {
        snap = LocalFallbackEngine.addProject(repoPath, repoName);
      } else {
        snap = await apiCall('/api/repo/add', 'POST', { path: repoPath, name: repoName });
      }
      closeAddProjectModal();
      applySnapshot(snap);
      showToast(`✓ Proje başarıyla eklendi: ${snap.repo.name}`, 'ok');
    } catch (err) {
      showToast(`Proje eklenemedi: ${err.message}`, 'danger');
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  async function handleRemoveCurrentProject() {
    const currentRepo = state.snapshot?.repo;
    if (!currentRepo || String(currentRepo.id).startsWith('sim-')) {
      showToast('Simülasyon deposu listeden kaldırılamaz', 'warn');
      return;
    }
    const ok = window.confirm(`"${currentRepo.name}" projesini GitArc listesinden kaldırmak istiyor musunuz?\n\n(Not: Disk üzerindeki yerel dosyalarınız veya git deponuz ASLA silinmez, yalnızca GitArc proje listesinden çıkarılır.)`);
    if (!ok) return;

    try {
      let snap;
      if (state.offlineMode) {
        snap = LocalFallbackEngine.removeProject(currentRepo.id);
      } else {
        snap = await apiCall('/api/repo/remove', 'POST', { repoId: currentRepo.id });
      }
      applySnapshot(snap);
      showToast(`Proje listeden kaldırıldı`, 'info');
    } catch (err) {
      showToast(`Hata: ${err.message}`, 'danger');
    }
  }

  // =========================================================================
  // INTERACTIVE COCKPIT TOUR (SPOTLIGHT HELPER)
  // =========================================================================
  const TOUR_STEPS = [
    {
      targetId: null,
      targetSelector: '.repo-picker',
      posClass: 'tour-pos-top-left',
      badge: 'Adım 1 / 5',
      title: '📂 Depo Seçimi & ➕ Proje Ekle',
      body: '<p>Buradan çalıştığınız canlı Git depoları veya simülasyon demoları arasında anında geçiş yapabilirsiniz.</p><p><strong>➕ Proje Ekle</strong> butonuna basarak bilgisayarınızdaki herhangi bir yerel Git projesini tek tıkla GitArc kokpitine dahil edebilirsiniz.</p>',
    },
    {
      targetId: null,
      targetSelector: null,
      posClass: 'tour-pos-center',
      badge: 'Adım 2 / 5',
      title: '🚀 Parabolik Ray Tuvali',
      body: '<p>Git yaşam döngünüz burada <strong>parabolik raylar</strong> olarak görselleştirilir:</p><p>• <strong>Merkez Ray:</strong> <code>main</code> omurgası, commitler ve sürüm tag\'leri.<br>• <strong>Üst Ray:</strong> GitHub Remote ve Pull Request durumları.<br>• <strong>Alt Ray:</strong> Henüz pushlanmamış yerel dallarınız ve kaydedilmemiş çalışma dizininiz.</p>',
    },
    {
      targetId: 'bottom-dock',
      targetSelector: '#bottom-dock',
      posClass: 'tour-pos-bottom-dock',
      badge: 'Adım 3 / 5',
      title: '🚨 PocketUI Alt Çekmece (Radar & Loglar)',
      body: '<p><strong>Hata & Uyarı Radarı</strong> ekranın altında tam genişlikte çalışır. CI kırılmaları, merge çakışmaları ve silinmiş uzak dallar etkilenen dosya etiketleriyle burada listelenir.</p><p>Ayrıca <strong>📜 Loglar</strong> ve <strong>⚡ Son İşlemler</strong> sekmeleriyle sistemin attığı tüm adımları canlı takip edebilirsiniz.</p>',
    },
    {
      targetId: 'inspector-detail',
      targetSelector: '.inspector-pane',
      posClass: 'tour-pos-inspector',
      badge: 'Adım 4 / 5',
      title: '🛠️ Dal, Çakışma & Commit Stüdyosu',
      body: '<p>Grafikten bir dal veya commit seçtiğinizde sağ panelde stüdyo açılır:</p><p>• <strong>Tek Tıkla Dala Geçiş (Checkout)</strong> ve PR bilgisi düzenleme.<br>• <strong>Çakışma Stüdyosu:</strong> İzole <code>git worktree</code> üzerinde kayıpsız çakışma çözümü.<br>• <strong>Konsolsuz Commit:</strong> Terminal açmadan yerel değişikliklerinizi commitleyip PR açma.</p>',
    },
    {
      targetId: null,
      targetSelector: '.toolbar-actions',
      posClass: 'tour-pos-toolbar',
      badge: 'Adım 5 / 5',
      title: '🛡️ Güvenlik, Temizlik & MCP AI',
      body: '<p>Üst çubuktaki güçlü yardımcılar:</p><p>• <strong>🧹 Temizle:</strong> GitHub\'da birleşmiş veya silinmiş uzak kalıntı dalları temizler.<br>• <strong>🛡️ Koruma:</strong> main dal koruma durumunu denetler.<br>• <strong>🔌 MCP:</strong> GitArc\'ı Antigravity, Claude Code veya Cursor yapay zekanıza bağlar!</p>',
    },
  ];

  let currentTourStep = 0;
  let activeTourTargetEl = null;

  function renderTourStep(stepIndex) {
    if (stepIndex < 0 || stepIndex >= TOUR_STEPS.length) {
      closeTour();
      return;
    }
    currentTourStep = stepIndex;
    const step = TOUR_STEPS[stepIndex];

    if (activeTourTargetEl) {
      activeTourTargetEl.classList.remove('tour-highlight-target');
      activeTourTargetEl = null;
    }

    const target = step.targetId
      ? document.getElementById(step.targetId)
      : (step.targetSelector ? document.querySelector(step.targetSelector) : null);

    if (target) {
      activeTourTargetEl = target;
      target.classList.add('tour-highlight-target');
      try {
        target.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      } catch (_) {}
    }

    el.tourCard.className = `tour-card ${step.posClass || 'tour-pos-center'}`;
    el.tourStepBadge.textContent = step.badge;
    el.tourStepTitle.textContent = step.title;
    el.tourStepBody.innerHTML = step.body;

    el.tourPrevBtn.disabled = stepIndex === 0;
    el.tourNextBtn.textContent = stepIndex === TOUR_STEPS.length - 1 ? 'Tamamla ✓' : 'Sonraki ▶';

    el.tourDots.innerHTML = '';
    TOUR_STEPS.forEach((_, idx) => {
      const dot = document.createElement('span');
      dot.className = `tour-dot ${idx === stepIndex ? 'active' : ''}`;
      dot.setAttribute('title', `Adım ${idx + 1}`);
      dot.addEventListener('click', () => renderTourStep(idx));
      el.tourDots.appendChild(dot);
    });
  }

  function startTour() {
    el.tourBackdrop.hidden = false;
    el.tourCard.hidden = false;
    renderTourStep(0);
  }

  function closeTour() {
    el.tourBackdrop.hidden = true;
    el.tourCard.hidden = true;
    if (activeTourTargetEl) {
      activeTourTargetEl.classList.remove('tour-highlight-target');
      activeTourTargetEl = null;
    }
  }

  function nextTourStep() {
    if (currentTourStep >= TOUR_STEPS.length - 1) {
      closeTour();
      showToast('🎉 Tebrikler! GitArc kokpit turunu tamamladınız.', 'ok');
    } else {
      renderTourStep(currentTourStep + 1);
    }
  }

  function prevTourStep() {
    if (currentTourStep > 0) {
      renderTourStep(currentTourStep - 1);
    }
  }

  async function handlePromoteBranch(branchId, explicitTitle, explicitBody, btnEl = null) {
    const b = state.snapshot?.branches?.find((x) => x.id === branchId);
    let title = explicitTitle;
    let body = explicitBody;
    if (title === undefined && state.selectedBranchId === branchId) {
      title = document.getElementById('promote-pr-title')?.value ?? state.draftPrEdits[b?.name]?.title ?? b?.draftPrTitle ?? '';
      body = document.getElementById('promote-pr-body')?.value ?? state.draftPrEdits[b?.name]?.body ?? b?.draftPrBody ?? '';
    }
    const bName = b ? b.name : branchId;
    const cmd = `git push -u origin ${bName} && gh pr create --head ${bName} --title "${title || bName}"`;
    const confirmed = window.confirm(
      `⚠️ PUSH & PR AÇMA ONAYI:\n"${bName}" dalı uzak sunucuya pushlanacak ve GitHub PR oluşturulacaktır.\n\nÇalıştırılacak Komut:\n${cmd}\n\nBu işlemi onaylıyor musunuz?`,
    );
    if (!confirmed) return;

    showToast('⏳ Dal pushlanıyor ve GitHub PR oluşturuluyor...', 'info');
    await runWithBtnBusy(btnEl, '⏳ Push & PR Açılıyor...', async () => {
      try {
        const snap = state.offlineMode
          ? LocalFallbackEngine.promoteBranch(branchId, title || '', body || '')
          : await apiCall('/api/branch/promote', 'POST', {
            branchId,
            title: (title || '').trim(),
            body: (body || '').trim(),
          });
        if (b?.name) delete state.draftPrEdits[b.name];
        applySnapshot(snap);
        showToast('Dal origin’e pushlandı ve PR açılarak üst raya taşındı!', 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  const STUDIO_MODE_LABELS = {
    union: '1. Union (Her İkisini Koru)',
    ours: '2. Ours (Main Bazlı)',
    theirs: '3. Theirs (Dal Bazlı)',
    custom: '4. AI / Elle Düzenlendi',
  };

  async function openConflictStudio(branchId) {
    try {
      const payload = state.offlineMode
        ? LocalFallbackEngine.inspectConflict(branchId)
        : await apiCall('/api/conflict/inspect', 'POST', { branchId });
      if (!payload.files || payload.files.length === 0) {
        showToast('Bu dalda incelenecek aktif çakışma dosyası bulunamadı', 'warn');
        return;
      }
      state.studio = {
        payload,
        activeFileIdx: 0,
        strategy: payload.recommended || 'union',
        customByPath: {},
      };
      el.studioRunVet.checked = true;
      el.studioErrorBanner.hidden = true;
      el.studioErrorText.textContent = '';
      renderConflictStudio();
      el.conflictStudioModal.hidden = false;
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  function closeConflictStudio() {
    el.conflictStudioModal.hidden = true;
    state.studio = null;
  }

  function getActiveStudioFile() {
    if (!state.studio) return null;
    const files = state.studio.payload.files || [];
    return files[state.studio.activeFileIdx] || files[0] || null;
  }

  function studioPreviewForStrategy(file, strat, customByPath) {
    if (!file) return '';
    if (strat === 'custom') {
      return customByPath[file.path] !== undefined ? customByPath[file.path] : (file.unionPreview || '');
    }
    if (strat === 'ours') return file.oursPreview || '';
    if (strat === 'theirs') return file.theirsPreview || '';
    return file.unionPreview || '';
  }

  function renderConflictStudio() {
    const st = state.studio;
    if (!st) return;
    const { payload, activeFileIdx, strategy, customByPath } = st;
    const files = payload.files || [];
    const curFile = files[activeFileIdx] || files[0];

    el.studioBranchBadge.textContent = `${payload.branchName} ↔ ${payload.baseRef}`;

    el.studioFileTabs.innerHTML = '';
    files.forEach((f, idx) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `studio-file-tab ${idx === activeFileIdx ? 'active' : ''}`;
      btn.textContent = `📄 ${f.path}`;
      btn.addEventListener('click', () => {
        if (st.strategy === 'custom' && curFile) {
          st.customByPath[curFile.path] = el.studioRightPreview.value;
        }
        st.activeFileIdx = idx;
        renderConflictStudio();
      });
      el.studioFileTabs.appendChild(btn);
    });

    el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
      const stratKey = btn.getAttribute('data-strat');
      btn.classList.toggle('active', stratKey === strategy);
      const badgeEl = btn.querySelector('.strat-badge');
      if (badgeEl && payload.strategies) {
        const adv = payload.strategies.find((s) => s.strategy === stratKey);
        if (adv) {
          badgeEl.textContent = adv.badge;
          badgeEl.className = `strat-badge ${adv.tone || ''}`;
        }
      }
    });

    if (payload.recommendationReason) {
      el.studioGuideText.textContent = payload.recommendationReason;
      el.studioGuideBox.hidden = false;
    } else {
      el.studioGuideBox.hidden = true;
    }

    if (curFile) {
      el.studioLineBadge.textContent = `Satır ${curFile.startLine || 1}-${curFile.endLine || 1}`;
      el.studioLeftConflict.textContent = curFile.conflictHunks || '';
      el.studioRightPreview.value = studioPreviewForStrategy(curFile, strategy, customByPath);
    }
    el.studioModeBadge.textContent = STUDIO_MODE_LABELS[strategy] || strategy;
  }

  async function applyConflictStudio() {
    const st = state.studio;
    if (!st) return;
    const curFile = getActiveStudioFile();
    if (st.strategy === 'custom' && curFile) {
      st.customByPath[curFile.path] = el.studioRightPreview.value;
    }

    const customFiles = st.strategy === 'custom' ? st.customByPath : undefined;
    const runVet = Boolean(el.studioRunVet.checked);

    el.studioErrorBanner.hidden = true;
    el.studioErrorText.textContent = '';
    el.studioApplyBtn.disabled = true;
    el.studioApplyBtn.textContent = 'Uygulanıyor...';
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.resolveConflict(st.payload.branchId, st.strategy)
        : await apiCall('/api/branch/resolve', 'POST', {
          branchId: st.payload.branchId,
          strategy: st.strategy,
          customFiles,
          runVet,
        });
      closeConflictStudio();
      applySnapshot(snap);
      showToast(`Çakışma (${st.strategy}) ile çözüldü ve güncellendi!`, 'ok');
    } catch (err) {
      el.studioErrorText.textContent = err.message || String(err);
      el.studioErrorBanner.hidden = false;
      showToast(err.message, 'danger');
      if (!state.offlineMode) {
        apiCall('/api/snapshot').then(applySnapshot).catch(() => {});
      }
    } finally {
      el.studioApplyBtn.disabled = false;
      el.studioApplyBtn.textContent = '✅ Çözümü Uygula & Pushla';
    }
  }

  function renderLogsModal() {
    const logs = getMergedLogs();
    el.logsList.innerHTML = '';
    if (logs.length === 0) {
      el.logsList.innerHTML = '<div class="empty-detail">Henüz kaydedilmiş hata veya işlem günlüğü yok.</div>';
      return;
    }
    logs.forEach((item) => {
      const card = document.createElement('div');
      const tone = item.level === 'error' ? 'error' : (item.level === 'ok' ? 'ok' : '');
      card.className = `log-item ${tone}`;
      card.innerHTML = `
        <div class="log-item-head">
          <span>${item.level === 'error' ? '❌' : (item.level === 'ok' ? '✅' : 'ℹ️')} ${esc(item.action)}${item.branch ? ` · ${esc(item.branch)}` : ''}</span>
          <span class="log-item-meta">${esc(item.timestamp || '')}</span>
        </div>
        <div class="log-item-msg">${esc(item.message)}</div>
        ${item.detail ? `<pre class="log-item-detail">${esc(item.detail)}</pre>` : ''}
      `;
      el.logsList.appendChild(card);
    });
  }

  function openLogsModal() {
    renderLogsModal();
    el.logsModal.hidden = false;
  }

  function closeLogsModal() {
    el.logsModal.hidden = true;
  }

  function renderSecurityUpdateModal() {
    const r = state.snapshot?.repo || {};
    const isProt = Boolean(r.branchProtected);
    el.secProtBadge.textContent = isProt ? '🛡️ Korumalı' : '🔓 Korumasız';
    el.secProtBadge.className = `chip ${isProt ? 'ok' : 'warn'}`;
    el.secProtSummary.textContent = r.protectionSummary || (isProt ? 'Ana dal korumalı' : 'Doğrudan push/force-push açık');
    el.btnEnableProtection.disabled = isProt;
    el.secTeamChip.textContent = r.teamMode ? '🏢 Team Server (-team)' : '💻 Bireysel Masaüstü';
    el.secTeamChip.className = `chip ${r.teamMode ? 'ok' : 'accent'} mono`;

    const cur = r.appVersion || '0.7.0';
    const lat = r.latestVersion || cur;
    el.secUpdateStatus.textContent = r.updateAvailable
      ? `Yeni sürüm bulundu: v${lat} (Mevcut: v${cur})`
      : `Mevcut sürüm güncel: v${cur}${r.latestVersion ? ` (Uzak etiket: v${lat})` : ''}`;
  }

  function openSecurityUpdateModal() {
    renderSecurityUpdateModal();
    el.securityUpdateModal.hidden = false;
  }

  function closeSecurityUpdateModal() {
    el.securityUpdateModal.hidden = true;
  }

  async function handleProtectBranch() {
    el.btnEnableProtection.disabled = true;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.protectBranch()
        : await apiCall('/api/repo/protect', 'POST', {});
      applySnapshot(snap);
      renderSecurityUpdateModal();
      showToast('Ana dal koruması (Branch Protection) etkinleştirildi!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    } finally {
      if (state.snapshot?.repo) {
        el.btnEnableProtection.disabled = Boolean(state.snapshot.repo.branchProtected);
      }
    }
  }

  async function handleCheckUpdates() {
    try {
      const res = state.offlineMode
        ? LocalFallbackEngine.checkUpdates()
        : await apiCall('/api/system/update-check', 'POST', {});
      if (state.snapshot?.repo) {
        state.snapshot.repo.latestVersion = res.latestVersion;
        state.snapshot.repo.updateAvailable = res.updateAvailable;
      }
      if (res.updateCommand) {
        el.secUpdateCmd.textContent = res.updateCommand;
      }
      renderTopbar();
      showToast(res.updateAvailable ? `Yeni sürüm mevcut: v${res.latestVersion}` : `GitArc güncel (v${res.currentVersion})`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  function buildMCPConfigText() {
    const repoPath = state.snapshot?.repo?.mode === 'live' && state.snapshot?.repo?.path
      ? state.snapshot.repo.path
      : '/Users/ziyayesilbahce/Developer/GitArc';
    const binPath = `${repoPath.replace(/\/$/, '')}/gitarc`;
    return JSON.stringify({
      mcpServers: {
        gitarc: {
          command: binPath,
          args: ['-mcp', '-repo', repoPath],
        },
      },
    }, null, 2);
  }

  function openMCPConnectModal() {
    el.mcpConfigSnippet.textContent = buildMCPConfigText();
    el.mcpModal.hidden = false;
  }

  function closeMCPConnectModal() {
    el.mcpModal.hidden = true;
  }

  async function handleFixCI(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.fixCI(branchId)
        : await apiCall('/api/branch/fix-ci', 'POST', { branchId });
      delete state.localRunOutput[branchId];
      applySnapshot(snap);
      showToast('CI testleri yeşile döndü — PR artık merge edilebilir!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleDeleteBranch(branchId, btnEl = null) {
    const b = state.snapshot?.branches?.find((x) => x.id === branchId);
    const branchName = b ? b.name : branchId;
    const isRemote = b && !b.hasLocal && b.hasRemote;
    const isBoth = b && b.hasLocal && b.hasRemote;
    const defBranch = state.snapshot?.repo?.defaultBranch || 'main';
    const isHead = Boolean(b?.isHead);
    const dirty = (b?.uncommittedCount || 0) > 0 || (isHead && (state.snapshot?.repo?.dirtyCount || 0) > 0);
    const dirtyCount = b?.uncommittedCount || state.snapshot?.repo?.dirtyCount || 0;

    let cmd = '';
    if (isRemote) {
      cmd = `git push origin --delete ${branchName}`;
    } else if (isBoth) {
      if (isHead) {
        cmd = `git switch ${defBranch} && git branch -D ${branchName} && git push origin --delete ${branchName}`;
      } else {
        cmd = `git branch -D ${branchName} && git push origin --delete ${branchName}`;
      }
    } else {
      if (isHead) {
        cmd = `git switch ${defBranch} && git branch -D ${branchName}`;
      } else {
        cmd = `git branch -D ${branchName}`;
      }
    }

    let warningText = '';
    if (isHead && dirty) {
      warningText = `\n\nℹ️ Bu dal şu an aktif (HEAD). GitArc önce "git switch ${defBranch}" ile ana omurgaya geçecek (kaydedilmemiş ${dirtyCount} dosyanız korunacaktır), ardından birleşmiş dalı silecektir.`;
    } else if (isHead) {
      warningText = `\n\nℹ️ Bu dal şu an aktif (HEAD) olduğu için doğrudan silinemez. Komut önce "git switch ${defBranch}" ile ana omurgaya geçecek, ardından dalı silecektir.`;
    } else if (dirty) {
      warningText = `\n\n⚠️ UYARI: Çalışma ağacında ${dirtyCount} adet kaydedilmemiş (staged / takipsiz) dosya bulunmaktadır.`;
    }

    const confirmed = window.confirm(
      `⚠️ DAL SİLME ONAYI:\n"${branchName}" dalı silinecektir.${warningText}\n\nÇalıştırılacak Komut:\n${cmd}\n\nBu silme işlemini onaylıyor musunuz?`,
    );
    if (!confirmed) return;

    await runWithBtnBusy(btnEl, '⏳ Siliniyor...', async () => {
      try {
        const snap = state.offlineMode
          ? LocalFallbackEngine.deleteBranch(branchId)
          : await apiCall('/api/branch/delete', 'POST', { branchId });
        applySnapshot(snap);
        showToast('Dal kaldırıldı', 'warn');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  async function handlePruneGone(btnEl = null) {
    const dirtyCount = state.snapshot?.repo?.dirtyCount || 0;
    let warningText = '';
    if (dirtyCount > 0) {
      warningText = `\n\n🚨 UYARI: Çalışma ağacında ${dirtyCount} adet kaydedilmemiş (staged / takipsiz) dosya bulunmaktadır! Temizlik öncesi değişikliklerinizi commitlemeniz önerilir.`;
    }
    const cmd = "git fetch --prune && git branch -vv | grep ': gone]' | awk '{print $1}' | xargs -r git branch -D";
    const confirmed = window.confirm(
      `⚠️ KALINTI TEMİZLİĞİ ONAYI:\nUzak sunucuda birleşmiş veya silinmiş (gone) tüm yerel dallar temizlenecektir.${warningText}\n\nÇalıştırılacak Komut:\n${cmd}\n\nBu temizlik işlemini onaylıyor musunuz?`,
    );
    if (!confirmed) return;

    await runWithBtnBusy(btnEl, '⏳ Temizleniyor...', async () => {
      try {
        let snap;
        let removed = 0;
        if (state.offlineMode) {
          const res = LocalFallbackEngine.pruneGone();
          snap = res.snapshot;
          removed = res.removed;
        } else {
          const res = await apiCall('/api/branch/prune', 'POST', {});
          snap = res.snapshot;
          removed = res.removed;
        }
        applySnapshot(snap);
        showToast(`${removed} adet silinmiş/birleşmiş dal temizlendi`, 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  async function handleSyncSpine(btnEl = null) {
    await runWithBtnBusy(btnEl, '⏳ Senkronize Ediliyor...', async () => {
      try {
        const snap = state.offlineMode
          ? LocalFallbackEngine.syncSpine()
          : await apiCall('/api/spine/sync', 'POST', {});
        applySnapshot(snap);
        showToast('main ile origin/main senkronize edildi', 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  }

  async function handleLocalTestRun(branchId, cmd) {
    const repoDir = state.snapshot?.repo?.path || '.';
    state.localRunOutput[branchId] = `$ cd ${repoDir} && ${cmd}\nÇalıştırılıyor...\n`;
    renderInspector();
    try {
      if (!state.offlineMode) {
        const res = await apiCall('/api/test/run', 'POST', { cmd });
        state.localRunOutput[branchId] = res.output;
      } else {
        await new Promise((r) => setTimeout(r, 350));
        state.localRunOutput[branchId] = [
          `$ ${cmd}`,
          '=== RUN   TestShellGoldCompare',
          '    compare_test.go:118: golden output mismatch on key \'hardening_pve_apply\': got "true", want "false"',
          '--- FAIL: TestShellGoldCompare (0.39s)',
          'FAIL\tkeylab/internal/shellgold\t0.412s',
        ].join('\n');
      }
      renderInspector();
    } catch (err) {
      state.localRunOutput[branchId] += `\nHata: ${err.message}`;
      renderInspector();
    }
  }

  // Toolbar, Modal & Viewport Events
  el.repoSelect.addEventListener('change', async () => {
    const repoId = el.repoSelect.value;
    if (state.offlineMode) return;
    try {
      const snap = await apiCall('/api/repo/switch', 'POST', { repoId });
      localStorage.setItem(REPO_STORAGE_KEY, repoId);
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      applySnapshot(snap);
      showToast(`Aktif depo: ${snap.repo.name}`, 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  el.toggleMerged.addEventListener('change', () => {
    state.showMergedArches = el.toggleMerged.checked;
    renderSVG();
  });

  el.spineSyncBadge.addEventListener('click', (ev) => handleSyncSpine(ev.currentTarget));

  function openNewBranchModal() {
    el.modal.hidden = false;
    el.newBranchName.focus();
  }

  function closeNewBranchModal() {
    el.modal.hidden = true;
  }

  document.getElementById('btn-new-branch').addEventListener('click', openNewBranchModal);
  document.getElementById('modal-close').addEventListener('click', closeNewBranchModal);
  document.getElementById('modal-cancel').addEventListener('click', closeNewBranchModal);

  el.btnAddProject.addEventListener('click', openAddProjectModal);
  el.btnRemoveProject.addEventListener('click', handleRemoveCurrentProject);
  el.addProjClose.addEventListener('click', closeAddProjectModal);
  el.addProjCancel.addEventListener('click', closeAddProjectModal);
  el.addProjectForm.addEventListener('submit', handleAddProjectSubmit);

  el.btnStartTour?.addEventListener('click', startTour);
  el.tourCloseBtn?.addEventListener('click', closeTour);
  el.tourNextBtn?.addEventListener('click', nextTourStep);
  el.tourPrevBtn?.addEventListener('click', prevTourStep);
  el.tourBackdrop?.addEventListener('click', (ev) => {
    if (ev.target === el.tourBackdrop) {
      closeTour();
    }
  });

  window.addEventListener('keydown', (ev) => {
    if (el.tourBackdrop && !el.tourBackdrop.hidden) {
      if (ev.key === 'Escape') {
        closeTour();
      } else if (ev.key === 'ArrowRight') {
        nextTourStep();
      } else if (ev.key === 'ArrowLeft') {
        prevTourStep();
      }
    }
  });

  el.newBranchForm.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const name = el.newBranchName.value.trim();
    if (!name) return;
    const existing = (state.snapshot?.branches || []).find((b) => b.name.toLowerCase() === name.toLowerCase());
    if (existing) {
      const statusTxt = existing.state === 'merged' ? 'daha önce birleşmiş/kalıntı' : 'zaten açık';
      showToast(`⚠️ '${name}' dalı depoda ${statusTxt}. Lütfen benzersiz bir dal adı belirleyin.`, 'warn');
      return;
    }
    const checkedRadio = document.querySelector('input[name="branch-mode"]:checked');
    const hemisphere = checkedRadio ? checkedRadio.value : 'lower';
    const fromSha = el.newBranchBase.value;
    if (!name) return;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.createBranch(name, hemisphere, fromSha)
        : await apiCall('/api/branch/create', 'POST', { name, hemisphere, fromSha });
      closeNewBranchModal();
      el.newBranchName.value = '';
      const created = snap.branches.find((b) => b.name === name);
      if (created) state.selectedBranchId = created.id;
      applySnapshot(snap);
      showToast(`'${name}' dalı ${hemisphere === 'upper' ? 'üst rayda (PR)' : 'alt rayda (yerel kesikli parabol)'} açıldı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('btn-prune-gone').addEventListener('click', (ev) => handlePruneGone(ev.currentTarget));
  el.btnProtectBranch.addEventListener('click', openSecurityUpdateModal);
  el.btnUpdateCheck.addEventListener('click', openSecurityUpdateModal);
  el.lensPathInput.addEventListener('input', () => {
    state.pathLens = el.lensPathInput.value;
    renderSVG();
  });
  el.btnLogsModal.addEventListener('click', openLogsModal);
  document.getElementById('btn-mcp-connect').addEventListener('click', openMCPConnectModal);
  document.getElementById('btn-sync-spine').addEventListener('click', (ev) => handleSyncSpine(ev.currentTarget));

  document.getElementById('sec-close').addEventListener('click', closeSecurityUpdateModal);
  document.getElementById('sec-close-btn').addEventListener('click', closeSecurityUpdateModal);
  el.btnEnableProtection.addEventListener('click', handleProtectBranch);
  document.getElementById('btn-run-update-check').addEventListener('click', handleCheckUpdates);
  document.getElementById('sec-copy-update-cmd').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(el.secUpdateCmd.textContent || '');
      showToast('Güncelleme komutu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  document.getElementById('studio-close').addEventListener('click', closeConflictStudio);
  document.getElementById('studio-cancel').addEventListener('click', closeConflictStudio);
  document.getElementById('studio-apply').addEventListener('click', applyConflictStudio);
  document.getElementById('studio-copy-error').addEventListener('click', async () => {
    const txt = el.studioErrorText.textContent;
    if (!txt) return;
    try {
      await navigator.clipboard.writeText(txt);
      showToast('Hata metni panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  document.getElementById('logs-close').addEventListener('click', closeLogsModal);
  document.getElementById('logs-close-btn').addEventListener('click', closeLogsModal);
  document.getElementById('logs-copy-all').addEventListener('click', async () => {
    const text = getMergedLogs().map((l) => `[${l.timestamp}] [${l.level.toUpperCase()}] ${l.action}${l.branch ? ' (' + l.branch + ')' : ''}: ${l.message}${l.detail ? '\n  ' + l.detail : ''}`).join('\n\n');
    try {
      await navigator.clipboard.writeText(text || 'Log kaydı yok.');
      showToast('Tüm loglar panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  document.getElementById('logs-clear').addEventListener('click', async () => {
    state.clientLogs = [];
    if (!state.offlineMode) {
      try {
        const snap = await apiCall('/api/logs/clear', 'POST', {});
        applySnapshot(snap);
      } catch (err) {
        showToast(err.message, 'danger');
      }
    }
    renderLogBadge();
    renderLogsModal();
    showToast('Log kayıtları temizlendi', 'info');
  });
  document.getElementById('studio-copy-ai').addEventListener('click', async () => {
    const promptTxt = state.studio?.payload?.aiPrompt;
    if (!promptTxt) return;
    try {
      await navigator.clipboard.writeText(promptTxt);
      showToast('AI / MCP çözüm istemi panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      if (!state.studio) return;
      const curFile = getActiveStudioFile();
      if (state.studio.strategy === 'custom' && curFile) {
        state.studio.customByPath[curFile.path] = el.studioRightPreview.value;
      }
      state.studio.strategy = btn.getAttribute('data-strat') || 'union';
      renderConflictStudio();
      if (state.studio.strategy === 'custom') {
        el.studioRightPreview.focus();
      }
    });
  });
  el.studioRightPreview.addEventListener('input', () => {
    if (!state.studio) return;
    const curFile = getActiveStudioFile();
    if (curFile) {
      state.studio.customByPath[curFile.path] = el.studioRightPreview.value;
    }
    if (state.studio.strategy !== 'custom') {
      state.studio.strategy = 'custom';
      el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
        btn.classList.toggle('active', btn.getAttribute('data-strat') === 'custom');
      });
      el.studioModeBadge.textContent = STUDIO_MODE_LABELS.custom;
    }
  });

  document.getElementById('mcp-close').addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-close-btn').addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-copy-json').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(buildMCPConfigText());
      showToast('MCP konfigürasyonu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  document.getElementById('btn-reset-sim').addEventListener('click', async () => {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.reset()
        : await apiCall('/api/simulation/reset', 'POST', {});
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      state.localRunOutput = {};
      applySnapshot(snap);
      showToast('Kokpit simülasyonu başlangıç durumuna döndürüldü', 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('btn-theme').addEventListener('click', () => {
    const root = document.documentElement;
    const next = root.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
    if (next === 'dark') {
      root.removeAttribute('data-theme');
    } else {
      root.setAttribute('data-theme', 'light');
    }
  });

  // Pan & Zoom
  document.getElementById('zoom-in').addEventListener('click', () => {
    state.zoom = Math.min(2.2, +(state.zoom + 0.15).toFixed(2));
    renderSVG();
  });
  document.getElementById('zoom-out').addEventListener('click', () => {
    state.zoom = Math.max(0.55, +(state.zoom - 0.15).toFixed(2));
    renderSVG();
  });
  document.getElementById('zoom-reset').addEventListener('click', () => {
    state.zoom = 1;
    state.panX = 0;
    state.panY = 0;
    renderSVG();
  });

  el.viewport.addEventListener('wheel', (ev) => {
    ev.preventDefault();
    const delta = ev.deltaY < 0 ? 0.08 : -0.08;
    state.zoom = Math.min(2.2, Math.max(0.55, +(state.zoom + delta).toFixed(2)));
    renderSVG();
  }, { passive: false });

  el.viewport.addEventListener('pointerdown', (ev) => {
    if (ev.target.closest('.fo-capsule') || ev.target.closest('.svg-node-circle')) return;
    state.panning = { x: ev.clientX, y: ev.clientY, panX: state.panX, panY: state.panY };
    el.viewport.classList.add('panning');
  });

  bootstrap();
})();

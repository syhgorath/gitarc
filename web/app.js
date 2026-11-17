// web/app.js - GitArc Modular Main Application Entry Point
import { REPO_STORAGE_KEY } from './modules/constants.js';
import { state, el, initElements } from './modules/state.js';
import { esc, truncate, showToast, runWithBtnBusy, snapshotSignature, suggestUniqueBranchName } from './modules/utils.js';
import { apiCall } from './modules/api.js';
import { setupBottomDock, renderRadar, renderLogBadge, recordClientLog } from './modules/dock.js';
import { renderSVG, renderKPIs, renderTopbar, renderRayPopovers, setupCanvasInteractions } from './modules/canvas.js';
import { renderInspector } from './modules/inspector.js';
import {
  openConflictStudio, closeConflictStudio, applyConflictStudio,
  openAddProjectModal, closeAddProjectModal, handleAddProjectSubmit, handleRemoveCurrentProject,
  startCockpitTour, closeTour, nextTourStep, prevTourStep
} from './modules/modals.js';

// Central Branch Selection
export function selectBranch(branchId) {
  state.selectedBranchId = branchId;
  state.selectedNodeSha = null;
  renderSVG(getActionCallbacks());
  renderInspector(getActionCallbacks());
}

export function selectNode(shortSha) {
  state.selectedNodeSha = shortSha;
  state.selectedBranchId = null;
  renderInspector(getActionCallbacks());
}

// Action Callbacks Bundle passed to Canvas and Inspector
export function getActionCallbacks() {
  return {
    onSelectBranch: selectBranch,
    onSelectNode: selectNode,
    onCheckoutBranch: handleCheckoutBranch,
    onMergeBranch: handleMergeBranch,
    onPromoteBranch: handlePromoteBranch,
    onDeleteBranch: handleDeleteBranch,
    onPruneGone: handlePruneGone,
    onOpenConflictStudio: openConflictStudio,
    onFixCI: handleFixCI,
    onLocalTestRun: handleLocalTestRun,
    onEditPR: handleEditPR,
    onAmendCommit: handleAmendCommit,
    onCommitWorkingTree: handleCommitWorkingTree,
    onDiscardWorkingTree: handleDiscardWorkingTree,
    onApplySnapshot: applySnapshot,
  };
}

// Mutation Action Handlers
async function handleCheckoutBranch(branchId, btnEl = null) {
  await runWithBtnBusy(btnEl, '⏳ Dala Geçiliyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.checkoutBranch(branchId) : state.snapshot)
        : await apiCall('/api/branch/checkout', 'POST', { branchId });
      applySnapshot(snap);
      showToast(`✓ '${branchId}' dalına geçildi (git switch)`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handleMergeBranch(branchId, btnEl = null) {
  const b = (state.snapshot?.branches || []).find((x) => x.id === branchId);
  const isRunning = b?.pr?.ci?.state === 'RUNNING';

  if (isRunning) {
    if (state.pendingAutoMerge[branchId]) {
      delete state.pendingAutoMerge[branchId];
      renderSVG(getActionCallbacks());
      renderInspector(getActionCallbacks());
      showToast('Otomatik merge kuyruğu iptal edildi', 'info');
      return;
    }
    state.pendingAutoMerge[branchId] = {
      branchId,
      branchName: b?.name,
      strategy: state.mergeStrategy,
      queuedAt: new Date().toLocaleTimeString(),
    };
    renderSVG(getActionCallbacks());
    renderInspector(getActionCallbacks());
    showToast(`⏳ CI devam ediyor: Bitince '${b?.name}' otomatik birleştirilecek!`, 'warn');
    return;
  }

  await runWithBtnBusy(btnEl, '⚡ Birleştiriliyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.mergeBranch(branchId) : state.snapshot)
        : await apiCall('/api/branch/merge', 'POST', { branchId, strategy: state.mergeStrategy });
      applySnapshot(snap);
      showToast(`✓ Dal birleştirildi (${state.mergeStrategy.toUpperCase()}) ve parabol kapatıldı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handlePromoteBranch(branchId, title = undefined, body = undefined, btnEl = null) {
  const b = (state.snapshot?.branches || []).find((x) => x.id === branchId);
  const confirmed = window.confirm(`"${b?.name || branchId}" dalını origin'e pushlamak ve GitHub üzerinde Pull Request oluşturmak istiyor musunuz?`);
  if (!confirmed) return;

  showToast('⏳ Dal pushlanıyor ve GitHub PR oluşturuluyor...', 'info');
  await runWithBtnBusy(btnEl, '⏳ Push & PR Açılıyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.promoteBranch(branchId, title || '', body || '') : state.snapshot)
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

async function handleDeleteBranch(branchId, btnEl = null) {
  const b = (state.snapshot?.branches || []).find((x) => x.id === branchId);
  const isRemnant = b?.state === 'merged' || b?.originGone || b?.state === 'gone';
  const promptMsg = isRemnant
    ? `"${b?.name || branchId}" dalı uzakta zaten merge edilmiş/silinmiş bir kalıntıdır. Yerel referansı güvenle silmek istiyor musunuz?`
    : `"${b?.name || branchId}" dalını silmek istediğinize emin misiniz?`;
  if (!window.confirm(promptMsg)) return;

  await runWithBtnBusy(btnEl, '🗑️ Siliniyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.deleteBranch(branchId) : state.snapshot)
        : await apiCall('/api/branch/delete', 'POST', { branchId });
      if (state.selectedBranchId === branchId) state.selectedBranchId = null;
      applySnapshot(snap);
      showToast(isRemnant ? 'Kalıntı dal temizlendi' : 'Dal silindi', 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handlePruneGone() {
  await runWithBtnBusy(document.getElementById('btn-prune-gone'), '🧹 Temizleniyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.pruneGone() : state.snapshot)
        : await apiCall('/api/branch/prune', 'POST', {});
      applySnapshot(snap);
      showToast('Uzakta silinmiş veya birleşmiş yerel dallar temizlendi (git fetch --prune)', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handleCommitWorkingTree(branchName, commitMsg, pushAndPR = false, prTitle = '', prBody = '', btnEl = null) {
  await runWithBtnBusy(btnEl, pushAndPR ? '🚀 Commit + Push & PR...' : '💾 Commit Atılıyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.commitWorkingTree(branchName, commitMsg, pushAndPR, prTitle, prBody) : state.snapshot)
        : await apiCall('/api/working-tree/commit', 'POST', {
          branchName: (branchName || '').trim(),
          commitMsg: (commitMsg || '').trim(),
          pushAndPR,
          prTitle: (prTitle || '').trim(),
          prBody: (prBody || '').trim(),
        });
      applySnapshot(snap);
      showToast(pushAndPR ? `✓ '${branchName}' açıldı, commit atıldı ve PR oluşturuldu!` : `✓ '${branchName}' dalına commit atıldı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handleDiscardWorkingTree() {
  const dirtyCount = state.snapshot?.repo?.dirtyCount || 0;
  const ok = window.confirm(`⚠️ DİKKAT: Çalışma dizinindeki ${dirtyCount} adet kaydedilmemiş değişiklik (staged ve untracked) tamamen sıfırlanacaktır.\n\nBu işlem geri alınamaz. Devam etmek istiyor musunuz?`);
  if (!ok) return;

  await runWithBtnBusy(document.getElementById('act-wt-discard'), '🗑️ Sıfırlanıyor...', async () => {
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.discardWorkingTree() : state.snapshot)
        : await apiCall('/api/working-tree/discard', 'POST', {});
      applySnapshot(snap);
      showToast('Çalışma alanındaki kaydedilmemiş değişiklikler temizlendi', 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });
}

async function handleFixCI(branchId) {
  showToast('🛠️ Test hatası düzeltildi, commit atıldı ve CI yeniden tetiklendi (Simülasyon)', 'ok');
  if (state.offlineMode && window.LocalFallbackEngine) {
    const snap = window.LocalFallbackEngine.fixCI(branchId);
    applySnapshot(snap);
  }
}

async function handleLocalTestRun(branchId, cmd) {
  showToast(`$ ${cmd} yürütülüyor...`, 'info');
  try {
    const res = state.offlineMode
      ? { ok: true, output: `=== RUN TestLocal\n--- PASS: TestLocal (0.04s)\nPASS\nok (offline mode)` }
      : await apiCall('/api/test/run', 'POST', { branchId, cmd });
    state.localRunOutput[branchId] = res.output;
    renderInspector(getActionCallbacks());
    showToast(res.ok ? 'Yerel test başarıyla geçti!' : 'Test başarısız oldu', res.ok ? 'ok' : 'danger');
  } catch (err) {
    showToast(err.message, 'danger');
  }
}

async function handleEditPR(branchId, title, body) {
  try {
    const snap = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.editPR(branchId, title, body) : state.snapshot)
      : await apiCall('/api/pr/edit', 'POST', { branchId, title, body });
    applySnapshot(snap);
    showToast('PR bilgileri güncellendi', 'ok');
  } catch (err) {
    showToast(err.message, 'danger');
  }
}

async function handleAmendCommit(branchId, message) {
  try {
    const snap = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.amendCommit(branchId, message) : state.snapshot)
      : await apiCall('/api/commit/amend', 'POST', { branchId, message });
    applySnapshot(snap);
    showToast('Son commit mesajı güncellendi', 'ok');
  } catch (err) {
    showToast(err.message, 'danger');
  }
}

// Master Snapshot Applicator
export function applySnapshot(snap) {
  state.snapshot = snap;
  state.lastSig = snapshotSignature(snap);

  // Auto-Merge Queue Watchdog
  const queuedIds = Object.keys(state.pendingAutoMerge || {});
  queuedIds.forEach((branchId) => {
    const queued = state.pendingAutoMerge[branchId];
    if (!queued) return;
    const target = (snap.branches || []).find((x) => x.id === branchId || (queued.branchName && x.name === queued.branchName));
    if (!target || target.state === 'merged') {
      delete state.pendingAutoMerge[branchId];
      return;
    }
    const ciSt = target.pr?.ci?.state;
    if (ciSt === 'SUCCESS' && !state.autoMergingInFlight[branchId]) {
      state.autoMergingInFlight[branchId] = true;
      delete state.pendingAutoMerge[branchId];
      showToast(`⚡ CI tamamlandı: '${target.name}' otomatik birleştiriliyor...`, 'ok');
      handleMergeBranch(target.id).finally(() => {
        delete state.autoMergingInFlight[branchId];
      });
    } else if (ciSt === 'FAILURE') {
      delete state.pendingAutoMerge[branchId];
      showToast(`❌ '${target.name}' için CI başarısız oldu; otomatik merge durduruldu!`, 'danger');
    }
  });

  const callbacks = getActionCallbacks();
  renderTopbar(callbacks);
  renderKPIs(() => {
    renderSVG(callbacks);
  });
  renderRadar(callbacks);
  renderSVG(callbacks);
  renderInspector(callbacks);
  renderRayPopovers(callbacks);
  populateBaseCommitOptions();
}

function populateBaseCommitOptions() {
  const nodes = state.snapshot?.nodes || [];
  if (!el.newBranchBase) return;
  el.newBranchBase.innerHTML = '';
  [...nodes].reverse().forEach((n) => {
    const opt = document.createElement('option');
    opt.value = n.shortSha;
    opt.textContent = `${n.shortSha} — ${truncate(n.subject, 36)}`;
    el.newBranchBase.appendChild(opt);
  });
}

// Security and Protection Modal
function renderSecurityUpdateModal() {
  const snap = state.snapshot;
  if (!snap) return;
  const isProtected = Boolean(snap.repo.branchProtected);
  if (el.secProtBadge) {
    el.secProtBadge.textContent = isProtected ? '🛡️ Aktif Korumalı' : '🔓 Korumasız';
    el.secProtBadge.className = `chip ${isProtected ? 'ok' : 'warn'}`;
  }
  if (el.secProtSummary) {
    el.secProtSummary.textContent = snap.repo.protectionSummary || (isProtected ? 'Korumalı' : 'Doğrudan push açık');
  }
  if (el.btnEnableProtection) {
    el.btnEnableProtection.hidden = isProtected;
  }
  if (el.secTeamChip) {
    el.secTeamChip.hidden = !snap.repo.teamMode;
  }
  if (el.secUpdateStatus) {
    el.secUpdateStatus.textContent = snap.repo.updateAvailable ? 'Yeni Sürüm Mevcut' : 'Güncel';
  }
  if (el.secUpdateCmd) {
    el.secUpdateCmd.textContent = 'git pull --ff-only origin main && go build -o gitarc .';
  }
}

// Bootstrap Application
async function bootstrap() {
  initElements();
  setupBottomDock(applySnapshot);
  setupCanvasInteractions(() => renderSVG(getActionCallbacks()));

  // UI Event Bindings
  document.getElementById('btn-sync-spine')?.addEventListener('click', async (ev) => {
    await runWithBtnBusy(ev.currentTarget, '🔄 Eşitleniyor...', async () => {
      try {
        const snap = state.offlineMode
          ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.syncSpine() : state.snapshot)
          : await apiCall('/api/spine/sync', 'POST', {});
        applySnapshot(snap);
        showToast('main omurgası origin ile eşitlendi', 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  });

  document.getElementById('btn-reset-sim')?.addEventListener('click', async () => {
    if (!window.confirm('Simülasyon deposunu başlangıç durumuna sıfırlamak istiyor musunuz?')) return;
    try {
      const snap = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.reset() : state.snapshot)
        : await apiCall('/api/simulation/reset', 'POST', {});
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      applySnapshot(snap);
      showToast('Simülasyon başlangıç durumuna sıfırlandı', 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('btn-prune-gone')?.addEventListener('click', handlePruneGone);

  // New Branch Modal
  document.getElementById('btn-new-branch')?.addEventListener('click', () => {
    if (!el.modal) return;
    el.modal.hidden = false;
    const branches = state.snapshot?.branches || [];
    const files = branches.find((b) => b.isHead)?.changedFiles || [];
    const suggested = suggestUniqueBranchName('feat', files, state.snapshot);
    if (el.newBranchName) {
      el.newBranchName.value = suggested;
      el.newBranchName.focus();
    }
  });

  document.getElementById('modal-cancel')?.addEventListener('click', () => {
    if (el.modal) el.modal.hidden = true;
  });

  el.newBranchForm?.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const name = el.newBranchName.value.trim();
    const base = el.newBranchBase.value;
    const submitBtn = el.newBranchForm.querySelector('button[type="submit"]');

    await runWithBtnBusy(submitBtn, 'Oluşturuluyor...', async () => {
      try {
        const snap = state.offlineMode
          ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.createBranch(name, base) : state.snapshot)
          : await apiCall('/api/branch/create', 'POST', { name, baseSha: base });
        el.modal.hidden = true;
        applySnapshot(snap);
        selectBranch(name);
        showToast(`✓ Yeni dal '${name}' oluşturuldu ve geçildi`, 'ok');
      } catch (err) {
        showToast(err.message, 'danger');
      }
    });
  });

  // Project Modal
  el.btnAddProject?.addEventListener('click', openAddProjectModal);
  el.addProjClose?.addEventListener('click', closeAddProjectModal);
  el.addProjCancel?.addEventListener('click', closeAddProjectModal);
  el.addProjectForm?.addEventListener('submit', (ev) => handleAddProjectSubmit(ev, applySnapshot));
  el.btnRemoveProject?.addEventListener('click', () => handleRemoveCurrentProject(applySnapshot));

  // Interactive Tour
  el.btnStartTour?.addEventListener('click', startCockpitTour);
  el.tourCloseBtn?.addEventListener('click', closeTour);
  el.tourPrevBtn?.addEventListener('click', prevTourStep);
  el.tourNextBtn?.addEventListener('click', nextTourStep);

  // Conflict Studio Modal Events
  document.getElementById('studio-close')?.addEventListener('click', closeConflictStudio);
  document.getElementById('studio-cancel')?.addEventListener('click', closeConflictStudio);
  el.studioApplyBtn?.addEventListener('click', () => applyConflictStudio(applySnapshot));

  // Security & Protection Modal Events
  el.btnProtectBranch?.addEventListener('click', () => {
    if (el.securityUpdateModal) el.securityUpdateModal.hidden = false;
    renderSecurityUpdateModal();
  });
  document.getElementById('sec-modal-close')?.addEventListener('click', () => {
    if (el.securityUpdateModal) el.securityUpdateModal.hidden = true;
  });
  el.btnEnableProtection?.addEventListener('click', async () => {
    try {
      const snap = await apiCall('/api/repo/protect', 'POST', {});
      applySnapshot(snap);
      showToast('🛡️ main dalı başarıyla korumaya alındı!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  // Global Keyboard Shortcuts
  window.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape') {
      if (el.modal && !el.modal.hidden) el.modal.hidden = true;
      if (el.conflictStudioModal && !el.conflictStudioModal.hidden) closeConflictStudio();
      if (el.securityUpdateModal && !el.securityUpdateModal.hidden) el.securityUpdateModal.hidden = true;
      if (el.addProjectModal && !el.addProjectModal.hidden) closeAddProjectModal();
      if (el.tourCard && !el.tourCard.hidden) closeTour();
    }
  });

  // Initial Fetch & Boot
  try {
    let snap = await apiCall('/api/snapshot');
    const savedId = localStorage.getItem(REPO_STORAGE_KEY);
    const isSavedSimulation = savedId && savedId.startsWith('sim-');

    if (savedId && savedId !== snap.repo.id && (snap.repos || []).some((r) => r.id === savedId)) {
      if (!(snap.repo.mode === 'live' && isSavedSimulation)) {
        try {
          snap = await apiCall('/api/repo/switch', 'POST', { repoId: savedId });
        } catch {}
      }
    }
    localStorage.setItem(REPO_STORAGE_KEY, snap.repo.id);
    applySnapshot(snap);
  } catch {
    state.offlineMode = true;
    if (window.LocalFallbackEngine) {
      const snap = window.LocalFallbackEngine.snapshot();
      applySnapshot(snap);
      showToast('Sunucusuz tarayıcı simülasyonu aktif (Offline Mode)', 'info');
    }
  }

  // 4-second live silent polling
  setInterval(async () => {
    if (state.offlineMode || state.drag || state.panning) return;
    if (el.modal && !el.modal.hidden) return;
    if (el.conflictStudioModal && !el.conflictStudioModal.hidden) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    try {
      const nextSnap = await apiCall('/api/snapshot');
      const sig = snapshotSignature(nextSnap);
      if (sig && sig !== state.lastSig) {
        applySnapshot(nextSnap);
      }
    } catch {}
  }, 4000);
}

// Start Application on DOM Ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bootstrap);
} else {
  bootstrap();
}

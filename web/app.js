// web/app.js - GitArc Modular Main Application Entry Point
import { REPO_STORAGE_KEY } from './modules/constants.js';
import { state, el, initElements, resetProjectScopedState } from './modules/state.js';
import { esc, truncate, showToast, runWithBtnBusy, snapshotSignature, suggestUniqueBranchName } from './modules/utils.js';
import { apiCall } from './modules/api.js';
import { initTheme } from './modules/theme.js';
import { protectionView, projectSwitchNotice } from './modules/syncsteps.js';
import { setupBottomDock, renderRadar, renderLogBadge, recordClientLog } from './modules/dock.js';
import { renderSVG } from './modules/scene.js';
import { setupCanvasInteractions } from './modules/interactions.js';
import { renderRayPopovers } from './modules/rails.js';
import { pollDecision } from './modules/polling.js';
import { detectPushes, playPushTraces } from './modules/trace.js';
import { renderKPIs } from './modules/kpi.js';
import { renderTopbar, updateStatusText, updateActionModel } from './modules/topbar.js';
import { renderInspector } from './modules/inspector.js';
import { openConflictStudio, closeConflictStudio, applyConflictStudio, bindConflictStudioEvents } from './modules/conflict-studio.js';
import {
  openAddProjectModal, closeAddProjectModal, handleAddProjectSubmit, handleRemoveCurrentProject,
  handleBrowseFolderClick, handleFallbackFolderPicker
} from './modules/project-modals.js';
import { startCockpitTour, closeTour, nextTourStep, prevTourStep } from './modules/tour.js';
import { openMCPConnectModal, closeMCPConnectModal, buildMCPConfigText } from './modules/mcp-modal.js';
import { openDiffViewer, closeDiffViewer, initDiffViewerListeners } from './modules/diff-viewer.js';
import { refreshProtectionPlan, applyShownProtection } from './modules/protection.js';
import { openQueueModal, closeQueueModal } from './modules/queue.js';
import { openPublishModal, closePublishModal } from './modules/publish.js';
import { mergeBranch } from './modules/merge.js';
import { promoteCopy } from './modules/inspector-model.js';

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
    onOpenQueue: () => openQueueModal(applySnapshot),
    onOpenDiff: (target, type, focusPath) => openDiffViewer(target, type, focusPath),
    onFixCI: handleFixCI,
    onLocalTestRun: handleLocalTestRun,
    onEditPR: handleEditPR,
    onAmendCommit: handleAmendCommit,
    onCommitWorkingTree: handleCommitWorkingTree,
    onDiscardWorkingTree: handleDiscardWorkingTree,
    onApplySnapshot: applySnapshot,
    onSetBaseBranch: handleSetBaseBranch,
  };
}

// Mutation Action Handlers
async function handleSetBaseBranch(branch) {
  try {
    const snap = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.setBaseBranch(branch) : state.snapshot)
      : await apiCall('/api/repo/base-branch', 'POST', { branch });
    applySnapshot(snap);
    showToast(branch ? `✓ Omurga dalı '${branch}' olarak sabitlendi` : '✓ Omurga dalı otomatik tespite döndü', 'ok');
  } catch (err) {
    showToast(err.message, 'danger');
  }
}

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

function handleMergeBranch(branchId, btnEl = null) {
  return mergeBranch(branchId, btnEl, {
    applySnapshot,
    rerender: () => { renderSVG(getActionCallbacks()); renderInspector(getActionCallbacks()); },
  });
}

async function handlePromoteBranch(branchId, title = undefined, body = undefined, btnEl = null) {
  const b = (state.snapshot?.branches || []).find((x) => x.id === branchId);
  const copy = promoteCopy(b || { name: branchId });
  const confirmed = window.confirm(copy.confirm);
  if (!confirmed) return;

  showToast(copy.companion ? '⏳ Yerel commit\'ler pushlanıyor...' : '⏳ Dal pushlanıyor ve GitHub PR oluşturuluyor...', 'info');
  await runWithBtnBusy(btnEl, copy.busy, async () => {
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
      showToast(copy.done, 'ok');
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
      const res = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.pruneGone() : state.snapshot)
        : await apiCall('/api/branch/prune', 'POST', {});
      const snap = res.snapshot || res;
      applySnapshot(snap);
      const count = typeof res.removed === 'number' ? res.removed : (Array.isArray(res.removed) ? res.removed.length : 0);
      showToast(count > 0 ? `✓ ${count} artık dal temizlendi` : 'Temizlenecek artık dal bulunamadı', 'ok');
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
  // "Uzağa gidiyor" izi için önceki görüntü ve önceki yerleşim (alt ray konumu) saklanır.
  const prevSnap = state.snapshot;
  const prevApex = state.apexLayout ? new Map(state.apexLayout) : null;
  if (prevSnap?.repo?.id && snap?.repo?.id && prevSnap.repo.id !== snap.repo.id) resetProjectScopedState();
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

  playPushTraces(detectPushes(prevSnap, snap), prevApex, state.apexLayout);
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
  const prot = protectionView(snap.repo);
  if (el.secProtBadge) {
    el.secProtBadge.textContent = prot.badge;
    el.secProtBadge.className = `chip ${prot.chip}`.trim();
  }
  if (el.secProtSummary) {
    el.secProtSummary.textContent = snap.repo.protectionSummary
      || (prot.state === 'active' ? 'Korumalı' : prot.state === 'unknown' ? 'Koruma durumu okunamadı' : 'Doğrudan push açık');
  }
  if (el.secTeamChip) {
    el.secTeamChip.hidden = !snap.repo.teamMode;
  }
  if (el.secUpdateStatus) {
    el.secUpdateStatus.textContent = updateStatusText(snap.repo);
  }
  if (el.secUpdateCmd) {
    el.secUpdateCmd.textContent = 'git pull --ff-only origin main && go build -o gitarc .';
  }
}

// Bootstrap Application
async function bootstrap() {
  initElements();
  initDiffViewerListeners();
  setupBottomDock(applySnapshot);
  setupCanvasInteractions(() => renderSVG(getActionCallbacks()));
  initTheme(document.getElementById('btn-theme'));

  // UI Event Bindings
  const handleSyncSpine = async (btnEl) => {
    await runWithBtnBusy(btnEl, '🔄 Eşitleniyor...', async () => {
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
  };

  document.getElementById('btn-canvas-sync')?.addEventListener('click', (ev) => handleSyncSpine(ev.currentTarget));
  document.getElementById('btn-sync-spine')?.addEventListener('click', (ev) => handleSyncSpine(ev.currentTarget));

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

  // Başka bir sekme projeyi değiştirdi: bu sekmedeki işlem YAPILMADI (sunucu 409 verdi); sunucunun projesini benimseyip yenile.
  window.addEventListener('gitarc:repo-mismatch', () => {
    showToast('⚠ Proje başka bir sekmede değiştirildi; işlem yapılmadı. Sayfa yenileniyor...', 'danger');
    setTimeout(() => window.location.reload(), 1400);
  });

  // Repo / Project Switcher
  el.repoSelect?.addEventListener('change', async () => {
    const targetRepoId = el.repoSelect.value;
    if (!targetRepoId || targetRepoId === state.snapshot?.repo?.id) return;

    // Süren kuyruk koşusu kendi projesine sabittir ve arka planda sürer: onay sorulmaz, geçişten sonra kısa bir not gösterilir.
    const switchNotice = projectSwitchNotice(state.snapshot);

    showToast('⏳ Proje yükleniyor...', 'info');
    try {
      let snap;
      if (!state.offlineMode) {
        snap = await apiCall('/api/repo/switch', 'POST', { repoId: targetRepoId });
      } else if (window.LocalFallbackEngine) {
        snap = window.LocalFallbackEngine.switchRepo(targetRepoId);
      }
      localStorage.setItem(REPO_STORAGE_KEY, targetRepoId);
      applySnapshot(snap);
      showToast(`✓ Proje değiştirildi: ${snap.repo.name}`, 'ok');
      if (switchNotice) showToast(switchNotice, 'info');
    } catch (err) {
      showToast(`Proje değiştirilemedi: ${err.message}`, 'danger');
      if (el.repoSelect && state.snapshot?.repo?.id) {
        el.repoSelect.value = state.snapshot.repo.id;
      }
    }
  });

  // Project Modal
  el.btnAddProject?.addEventListener('click', openAddProjectModal);
  el.addProjClose?.addEventListener('click', closeAddProjectModal);
  el.addProjCancel?.addEventListener('click', closeAddProjectModal);
  el.addProjectForm?.addEventListener('submit', (ev) => handleAddProjectSubmit(ev, applySnapshot));
  el.btnRemoveProject?.addEventListener('click', () => handleRemoveCurrentProject(applySnapshot));
  document.getElementById('btn-browse-folder')?.addEventListener('click', handleBrowseFolderClick);
  document.getElementById('fallback-folder-picker')?.addEventListener('change', handleFallbackFolderPicker);

  // Interactive Tour
  el.btnStartTour?.addEventListener('click', startCockpitTour);
  el.tourCloseBtn?.addEventListener('click', closeTour);
  el.tourPrevBtn?.addEventListener('click', prevTourStep);
  el.tourNextBtn?.addEventListener('click', nextTourStep);

  // Conflict Studio Modal Events
  document.getElementById('studio-close')?.addEventListener('click', closeConflictStudio);
  document.getElementById('studio-cancel')?.addEventListener('click', closeConflictStudio);
  el.studioApplyBtn?.addEventListener('click', () => applyConflictStudio(applySnapshot));
  bindConflictStudioEvents();

  // Security & Protection Modal Events
  el.btnNoProjectAdd?.addEventListener('click', () => openAddProjectModal());
  el.btnMainCI?.addEventListener('click', () => {
    const url = el.btnMainCI.dataset.url;
    if (url && url.startsWith('https://')) window.open(url, '_blank', 'noopener');
  });
  const openSecurityModal = () => {
    if (el.securityUpdateModal) el.securityUpdateModal.hidden = false;
    renderSecurityUpdateModal();
    refreshProtectionPlan();
  };
  el.btnProtectBranch?.addEventListener('click', openSecurityModal);
  el.btnUpdateCheck?.addEventListener('click', openSecurityModal); // sürüm rozeti: sürüm/güncelleme kartını açar (eskiden hiçbir şey yapmıyordu)
  // "Yeni sürüm var mı?": GitArc'ın KENDİ etiketlerine bakar (kullanıcı isteyince; ağ gerekir). Okunamazsa "güncel" denmez.
  el.btnRunUpdateCheck?.addEventListener('click', () => runWithBtnBusy(el.btnRunUpdateCheck, '🔄 Denetleniyor...', async () => {
    try {
      const res = state.offlineMode
        ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.checkUpdates() : null)
        : await apiCall('/api/system/update-check', 'POST', {});
      if (!res) return;
      if (state.snapshot) {
        // Rozet/durum satırı yeni bilgiyle çizilsin; denetlenemediyse "güncelleme var" bayrağına dokunma.
        const repo = { ...state.snapshot.repo, latestVersion: res.latestVersion, updateAvailable: Boolean(res.updateAvailable) };
        applySnapshot({ ...state.snapshot, repo });
      }
      if (el.secUpdateStatus) el.secUpdateStatus.textContent = updateStatusText(state.snapshot?.repo, res);
      showToast(res.checkError ? 'Sürüm denetlenemedi' : (res.updateAvailable ? `⬆ Yeni sürüm: v${res.latestVersion}` : '✓ GitArc güncel'), res.checkError ? 'danger' : 'ok');
      renderUpdateAction(res);
    } catch (err) {
      showToast(`Sürüm denetlenemedi: ${err.message}`, 'danger');
    }
  }));
  // "Güncelle": GitArc'ı İMZALI yeni sürüme günceller (sunucuda: indir → imza doğrula → ikiliyi değiştir). Önce açık onay; sonra yeniden başlatma kullanıcıda.
  function renderUpdateAction(res) {
    const m = updateActionModel(res);
    if (el.btnApplyUpdate) {
      el.btnApplyUpdate.hidden = !m.showApply;
      el.btnApplyUpdate.textContent = m.label || '⬇️ Güncelle';
      el.btnApplyUpdate.dataset.version = m.version;
    }
    if (el.secUpdateNote) { el.secUpdateNote.hidden = !m.note; el.secUpdateNote.textContent = m.note; }
  }
  el.btnApplyUpdate?.addEventListener('click', async () => {
    const version = el.btnApplyUpdate.dataset.version;
    if (!version) return;
    if (!window.confirm(`GitArc v${version} sürümüne güncellenecek.\n\nYeni ikili GitHub'dan indirilir, imzası doğrulanır (doğrulanamazsa HİÇBİR ŞEY kurulmaz) ve çalışan ikili değiştirilir; eskisi yedeklenir.\nSonra GitArc'ı yeniden başlatmanız gerekir.\n\nDevam edilsin mi?`)) return;
    await runWithBtnBusy(el.btnApplyUpdate, '⏳ Güncelleniyor...', async () => {
      try {
        const r = await apiCall('/api/system/update-apply', 'POST', { version });
        if (el.secUpdateStatus) el.secUpdateStatus.textContent = `✅ v${r.from} → v${r.to} kuruldu (imza doğrulandı). GitArc'ı YENİDEN BAŞLATIN: bu sayfa hâlâ eski sürümü çalıştırıyor.`;
        el.btnApplyUpdate.hidden = true;
        showToast(`✅ v${r.to} kuruldu: GitArc'ı yeniden başlatın`, 'ok');
      } catch (err) {
        showToast(`Güncelleme başarısız: ${err.message}`, 'danger');
        if (el.secUpdateNote) { el.secUpdateNote.hidden = false; el.secUpdateNote.textContent = `Güncelleme başarısız (çalışan sürüm değişmedi): ${err.message}`; }
      }
    });
  });
  el.secCopyUpdateCmd?.addEventListener('click', async () => {
    const cmd = el.secUpdateCmd?.textContent || '';
    try { await navigator.clipboard.writeText(cmd); showToast('Güncelleme komutu kopyalandı', 'ok'); } catch { showToast('Kopyalanamadı: komutu elle seçin', 'danger'); }
  });
  ['sec-close', 'sec-close-btn'].forEach((id) => {
    document.getElementById(id)?.addEventListener('click', () => {
      if (el.securityUpdateModal) el.securityUpdateModal.hidden = true;
    });
  });
  el.btnEnableProtection?.addEventListener('click', () => applyShownProtection(applySnapshot));

  // Merge kuyruğu planı penceresi
  el.btnPublish?.addEventListener('click', () => openPublishModal(applySnapshot));
  ['publish-close', 'publish-close-btn'].forEach((id) => document.getElementById(id)?.addEventListener('click', closePublishModal));
  el.btnQueue?.addEventListener('click', () => openQueueModal(applySnapshot));
  ['queue-close', 'queue-close-btn'].forEach((id) => document.getElementById(id)?.addEventListener('click', closeQueueModal));

  // MCP Connect Modal Events
  document.getElementById('btn-mcp-connect')?.addEventListener('click', openMCPConnectModal);
  document.getElementById('mcp-close')?.addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-close-btn')?.addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-copy-json')?.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(buildMCPConfigText());
      showToast('✓ MCP konfigürasyonu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  // Global Keyboard Shortcuts
  window.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape') {
      if (el.canvasInspector && !el.canvasInspector.hidden) {
        state.selectedBranchId = null;
        state.selectedNodeSha = null;
        el.canvasInspector.hidden = true;
        renderSVG(getActionCallbacks());
      }
      closeDiffViewer();
      if (el.modal && !el.modal.hidden) el.modal.hidden = true;
      if (el.conflictStudioModal && !el.conflictStudioModal.hidden) closeConflictStudio();
      if (el.securityUpdateModal && !el.securityUpdateModal.hidden) el.securityUpdateModal.hidden = true;
      if (el.addProjectModal && !el.addProjectModal.hidden) closeAddProjectModal();
      if (el.mcpModal && !el.mcpModal.hidden) closeMCPConnectModal();
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

  // Smart Git FS Event Listener / Lightweight Ping
  // Diskte .git/HEAD veya .git/index mtime değişmedikçe ağır snapshot çağrılmaz, sıfır zıplama!
  let lastGitSig = '';
  let lastFullAt = Date.now();
  let lastBg = null;
  setInterval(async () => {
    if (state.offlineMode || state.drag || state.panning) return;
    if (el.modal && !el.modal.hidden) return;
    if (el.conflictStudioModal && !el.conflictStudioModal.hidden) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;

    try {
      let pingChanged = false;
      let bgChanged = false;
      const ping = await apiCall('/api/system/git-status-ping', 'GET');
      if (ping && ping.sig) {
        if (lastGitSig && ping.sig !== lastGitSig) pingChanged = true;
        lastGitSig = ping.sig;
      }
      if (ping && typeof ping.bg === 'number') {
        if (lastBg !== null && ping.bg !== lastBg) bgChanged = true;
        lastBg = ping.bg;
      }

      // Karar tek yerde (polling.js): git değişikliği VEYA CI koşuyorsa aralıklı yenileme (seçili olsa bile).
      const decision = pollDecision({
        pingChanged,
        bgChanged,
        snap: state.snapshot,
        now: Date.now(),
        lastFullAt,
        selectionActive: Boolean(state.selectedBranchId || state.selectedNodeSha), // seçili öğeyi zıplatma
      });
      if (decision !== 'fetch') return;

      lastFullAt = Date.now();
      const nextSnap = await apiCall('/api/snapshot');
      const sig = snapshotSignature(nextSnap);
      if (sig && sig !== state.lastSig) {
        applySnapshot(nextSnap);
      }
    } catch {}
  }, 2500);
}

// Start Application on DOM Ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bootstrap);
} else {
  bootstrap();
}

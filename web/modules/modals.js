// web/modules/modals.js - Conflict Studio, Project Manager, Cockpit Tour and System Modals
import { state, el } from './state.js';
import { esc, truncate, showToast } from './utils.js';
import { apiCall } from './api.js';

const STUDIO_MODE_LABELS = {
  union: '1. Union (Her İkisini Koru)',
  ours: '2. Ours (Main Bazlı)',
  theirs: '3. Theirs (Dal Bazlı)',
  custom: '4. AI / Elle Düzenlendi',
};

export function getActiveStudioFile() {
  if (!state.studio) return null;
  const files = state.studio.payload.files || [];
  return files[state.studio.activeFileIdx] || files[0] || null;
}

export function studioPreviewForStrategy(file, strat, customByPath) {
  if (!file) return '';
  if (strat === 'custom') {
    return customByPath[file.path] !== undefined ? customByPath[file.path] : (file.unionPreview || '');
  }
  if (strat === 'ours') return file.oursPreview || '';
  if (strat === 'theirs') return file.theirsPreview || '';
  return file.unionPreview || '';
}

export function renderConflictStudio() {
  const st = state.studio;
  if (!st) return;
  const { payload, activeFileIdx, strategy, customByPath } = st;
  const files = payload.files || [];
  const curFile = files[activeFileIdx] || files[0];

  if (el.studioBranchBadge) el.studioBranchBadge.textContent = `${payload.branchName} ↔ ${payload.baseRef}`;

  if (el.studioFileTabs) {
    el.studioFileTabs.innerHTML = '';
    files.forEach((f, idx) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `studio-file-tab ${idx === activeFileIdx ? 'active' : ''}`;
      btn.textContent = `📄 ${f.path}`;
      btn.addEventListener('click', () => {
        if (st.strategy === 'custom' && curFile && el.studioRightPreview) {
          st.customByPath[curFile.path] = el.studioRightPreview.value;
        }
        st.activeFileIdx = idx;
        renderConflictStudio();
      });
      el.studioFileTabs.appendChild(btn);
    });
  }

  if (el.studioStrategyBar) {
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
  }

  if (el.studioGuideText && el.studioGuideBox) {
    if (payload.recommendationReason) {
      el.studioGuideText.textContent = payload.recommendationReason;
      el.studioGuideBox.hidden = false;
    } else {
      el.studioGuideBox.hidden = true;
    }
  }

  if (curFile) {
    if (el.studioLineBadge) el.studioLineBadge.textContent = `Satır ${curFile.startLine || 1}-${curFile.endLine || 1}`;
    if (el.studioLeftConflict) el.studioLeftConflict.textContent = curFile.conflictHunks || '';
    if (el.studioRightPreview) el.studioRightPreview.value = studioPreviewForStrategy(curFile, strategy, customByPath);
  }
  if (el.studioModeBadge) el.studioModeBadge.textContent = STUDIO_MODE_LABELS[strategy] || strategy;
}

export async function openConflictStudio(branchId) {
  try {
    const payload = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.inspectConflict(branchId) : { files: [] })
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
    if (el.studioRunVet) el.studioRunVet.checked = true;
    if (el.studioErrorBanner) el.studioErrorBanner.hidden = true;
    if (el.studioErrorText) el.studioErrorText.textContent = '';
    renderConflictStudio();
    if (el.conflictStudioModal) el.conflictStudioModal.hidden = false;
  } catch (err) {
    showToast(err.message, 'danger');
  }
}

export function closeConflictStudio() {
  if (el.conflictStudioModal) el.conflictStudioModal.hidden = true;
  state.studio = null;
}

export async function applyConflictStudio(onApplySnapshot = null) {
  const st = state.studio;
  if (!st) return;
  const curFile = getActiveStudioFile();
  if (st.strategy === 'custom' && curFile && el.studioRightPreview) {
    st.customByPath[curFile.path] = el.studioRightPreview.value;
  }

  const customFiles = st.strategy === 'custom' ? st.customByPath : undefined;
  const runVet = Boolean(el.studioRunVet?.checked);

  if (el.studioErrorBanner) el.studioErrorBanner.hidden = true;
  if (el.studioErrorText) el.studioErrorText.textContent = '';
  if (el.studioApplyBtn) {
    el.studioApplyBtn.disabled = true;
    el.studioApplyBtn.textContent = 'Uygulanıyor...';
  }

  try {
    const snap = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.resolveConflict(st.payload.branchId, st.strategy) : state.snapshot)
      : await apiCall('/api/branch/resolve', 'POST', {
        branchId: st.payload.branchId,
        strategy: st.strategy,
        customFiles,
        runVet,
      });

    closeConflictStudio();
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast(`✓ Çakışma başarıyla çözüldü (${st.strategy.toUpperCase()}) ve doğrulandı!`, 'ok');
  } catch (err) {
    if (el.studioErrorBanner) el.studioErrorBanner.hidden = false;
    if (el.studioErrorText) el.studioErrorText.textContent = err.message || 'Çakışma çözülemedi';
  } finally {
    if (el.studioApplyBtn) {
      el.studioApplyBtn.disabled = false;
      el.studioApplyBtn.textContent = '💾 Çözümü Uygula (Worktree)';
    }
  }
}

// Project Manager Modal Handlers
export function openAddProjectModal() {
  if (el.addProjectModal) el.addProjectModal.hidden = false;
  if (el.projectPathInput) {
    el.projectPathInput.value = '';
    el.projectPathInput.focus();
  }
  if (el.projectNameInput) el.projectNameInput.value = '';
}

export function closeAddProjectModal() {
  if (el.addProjectModal) el.addProjectModal.hidden = true;
}

export async function handleAddProjectSubmit(ev, onApplySnapshot = null) {
  ev.preventDefault();
  const repoPath = el.projectPathInput?.value.trim() || '';
  const repoName = el.projectNameInput?.value.trim() || '';
  if (!repoPath) {
    showToast('Lütfen geçerli bir yerel depo yolu girin', 'warn');
    return;
  }
  const btn = document.getElementById('add-proj-submit');
  if (btn) btn.disabled = true;

  try {
    let snap;
    if (state.offlineMode) {
      snap = window.LocalFallbackEngine?.addProject(repoPath, repoName) || state.snapshot;
    } else {
      snap = await apiCall('/api/repo/add', 'POST', { path: repoPath, name: repoName });
    }
    closeAddProjectModal();
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast(`✓ Proje başarıyla eklendi: ${snap.repo.name}`, 'ok');
  } catch (err) {
    showToast(`Proje eklenemedi: ${err.message}`, 'danger');
  } finally {
    if (btn) btn.disabled = false;
  }
}

export async function handleRemoveCurrentProject(onApplySnapshot = null) {
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
      snap = window.LocalFallbackEngine?.removeProject(currentRepo.id) || state.snapshot;
    } else {
      snap = await apiCall('/api/repo/remove', 'POST', { repoId: currentRepo.id });
    }
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast('Proje listeden kaldırıldı', 'info');
  } catch (err) {
    showToast(`Hata: ${err.message}`, 'danger');
  }
}

// Spotlight Interactive Tour Handlers
export const TOUR_STEPS = [
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
    targetSelector: '.topbar-right',
    posClass: 'tour-pos-top-right',
    badge: 'Adım 5 / 5',
    title: '🛡️ Güvenlik Kalkanı & MCP Entegrasyonu',
    body: '<p>GitArc, hassas verileri (secret/private key/env) depoya eklenmeden önce engeller.</p><p><strong>🔌 MCP</strong> butonu ile Claude Desktop, Cursor veya Antigravity gibi AI asistanlarınıza GitArc araçlarını bağlayabilirsiniz.</p>',
  },
];

let activeTourStep = 0;

export function renderTourStep(stepIdx) {
  if (stepIdx < 0 || stepIdx >= TOUR_STEPS.length) return;
  activeTourStep = stepIdx;
  const step = TOUR_STEPS[stepIdx];

  document.querySelectorAll('.tour-highlight').forEach((node) => node.classList.remove('tour-highlight'));

  if (step.targetSelector) {
    const targetEl = document.querySelector(step.targetSelector);
    if (targetEl) targetEl.classList.add('tour-highlight');
  }

  if (el.tourCard) {
    el.tourCard.className = `tour-card ${step.posClass || ''}`;
  }
  if (el.tourStepBadge) el.tourStepBadge.textContent = step.badge;
  if (el.tourStepTitle) el.tourStepTitle.textContent = step.title;
  if (el.tourStepBody) el.tourStepBody.innerHTML = step.body;

  if (el.tourPrevBtn) el.tourPrevBtn.disabled = stepIdx === 0;
  if (el.tourNextBtn) el.tourNextBtn.textContent = stepIdx === TOUR_STEPS.length - 1 ? 'Tamamla ✓' : 'Sonraki ▶';

  if (el.tourDots) {
    el.tourDots.innerHTML = '';
    TOUR_STEPS.forEach((_, idx) => {
      const dot = document.createElement('span');
      dot.className = `tour-dot ${idx === stepIdx ? 'active' : ''}`;
      el.tourDots.appendChild(dot);
    });
  }
}

export function startCockpitTour() {
  if (el.tourBackdrop) el.tourBackdrop.hidden = false;
  if (el.tourCard) el.tourCard.hidden = false;
  renderTourStep(0);
}

export function closeTour() {
  if (el.tourBackdrop) el.tourBackdrop.hidden = true;
  if (el.tourCard) el.tourCard.hidden = true;
  document.querySelectorAll('.tour-highlight').forEach((node) => node.classList.remove('tour-highlight'));
}

export function nextTourStep() {
  if (activeTourStep < TOUR_STEPS.length - 1) {
    renderTourStep(activeTourStep + 1);
  } else {
    closeTour();
  }
}

export function prevTourStep() {
  if (activeTourStep > 0) {
    renderTourStep(activeTourStep - 1);
  }
}

// MCP Connect Modal
export function buildMCPConfigText() {
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

export function openMCPConnectModal() {
  if (el.mcpConfigSnippet) {
    el.mcpConfigSnippet.textContent = buildMCPConfigText();
  }
  if (el.mcpModal) el.mcpModal.hidden = false;
}

export function closeMCPConnectModal() {
  if (el.mcpModal) el.mcpModal.hidden = true;
}


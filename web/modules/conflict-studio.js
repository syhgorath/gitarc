// web/modules/conflict-studio.js - Çakışma Stüdyosu (modal): önizleme, sekmeler, strateji ve uygulama
import { state, el } from './state.js';
import { showToast } from './utils.js';
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

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

const STRATEGY_KEYS = ['union', 'ours', 'theirs', 'custom'];
const STRATEGY_SHORT = { union: '✦ ikisi', ours: '🛡 main', theirs: '🚀 dal', custom: '🤖 özel' };

/** Saf: bir dosyanın stratejisi. Dosya başına seçilmediyse stüdyonun varsayılanı (sunucunun önerdiği). */
export function studioStrategyOf(st, file) {
  if (!st) return 'union';
  const own = file && st.strategyByPath ? st.strategyByPath[file.path] : undefined;
  return STRATEGY_KEYS.includes(own) ? own : (STRATEGY_KEYS.includes(st.strategy) ? st.strategy : 'union');
}

/** Saf (durumu değiştirir): strateji düğmesine basıldı; YALNIZCA etkin dosyayı etkiler. Dosya "özel" iken yazılan metin kaybolmasın diye
 *  önce kaydedilir. all=true ise (Tüm dosyalara uygula) seçim her dosyaya yazılır. */
export function chooseStudioStrategy(st, key, curFile, previewText, all = false) {
  if (!st || !curFile) return false;
  st.strategyByPath = st.strategyByPath || {};
  if (studioStrategyOf(st, curFile) === 'custom') st.customByPath[curFile.path] = previewText;
  const k = STRATEGY_KEYS.includes(key) ? key : 'union';
  if (all) {
    (st.payload?.files || []).forEach((f) => { st.strategyByPath[f.path] = k; });
  } else {
    st.strategyByPath[curFile.path] = k;
  }
  return true;
}

/** Saf (durumu değiştirir): önizleme alanına yazıldı. Yazılan metin yalnızca "özel" stratejide uygulanır; bu yüzden elle düzenleme O DOSYANIN
 *  stratejisini kendiliğinden "özel"e çevirir (yoksa yazılanlar sessizce yok sayılır). true = strateji değişti. */
export function editStudioPreview(st, curFile, text) {
  if (!st || !curFile) return false;
  st.strategyByPath = st.strategyByPath || {};
  st.customByPath[curFile.path] = text;
  if (studioStrategyOf(st, curFile) === 'custom') return false;
  st.strategyByPath[curFile.path] = 'custom';
  return true;
}

/** Saf: uygulama isteğinin gövdesi. Her dosya için strateji AÇIKÇA gönderilir; özel metin yalnızca "özel" dosyalar için. */
export function studioApplyPayload(st) {
  const files = st.payload?.files || [];
  const fileStrategies = {};
  const customFiles = {};
  files.forEach((f) => {
    const k = studioStrategyOf(st, f);
    fileStrategies[f.path] = k;
    if (k === 'custom' && st.customByPath[f.path] !== undefined) customFiles[f.path] = st.customByPath[f.path];
  });
  const distinct = new Set(Object.values(fileStrategies));
  return {
    strategy: st.strategy || 'union', // dosyada açık seçim yoksa varsayılan; hepsi açık gönderildiği için pratikte günlük etiketi
    fileStrategies,
    customFiles: Object.keys(customFiles).length ? customFiles : undefined,
    label: distinct.size === 1 ? [...distinct][0] : 'karma',
  };
}

/** Stüdyo düğmelerinin/alanının olay işleyicileri (modül bölünmesinde kaybolmuşlardı: düğmeler hiç tıklanamıyordu). Bir kez çağrılır. */
export function bindConflictStudioEvents() {
  el.studioStrategyBar?.querySelectorAll('.strat-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const st = state.studio;
      if (!chooseStudioStrategy(st, btn.getAttribute('data-strat'), getActiveStudioFile(), el.studioRightPreview?.value ?? '')) return;
      renderConflictStudio();
      if (studioStrategyOf(st, getActiveStudioFile()) === 'custom') el.studioRightPreview?.focus();
    });
  });
  document.getElementById('studio-strat-all')?.addEventListener('click', () => {
    const st = state.studio;
    const cur = getActiveStudioFile();
    if (!st || !cur) return;
    // etkin dosyanın şu anki stratejisini tüm dosyalara yay (özel ise yalnızca strateji; metinler dosyaya özgüdür)
    chooseStudioStrategy(st, studioStrategyOf(st, cur), cur, el.studioRightPreview?.value ?? '', true);
    renderConflictStudio();
  });
  el.studioRightPreview?.addEventListener('input', () => {
    const st = state.studio;
    if (editStudioPreview(st, getActiveStudioFile(), el.studioRightPreview.value)) {
      // Yeniden çizmek imleci/yazdığını bozardı: yalnızca etkin düğmeyi, rozeti ve dosya sekmesinin etiketini güncelle.
      el.studioStrategyBar?.querySelectorAll('.strat-btn').forEach((b) => b.classList.toggle('active', b.getAttribute('data-strat') === 'custom'));
      if (el.studioModeBadge) el.studioModeBadge.textContent = STUDIO_MODE_LABELS.custom;
      const tab = el.studioFileTabs?.querySelector('.studio-file-tab.active .studio-tab-strat');
      if (tab) tab.textContent = STRATEGY_SHORT.custom;
    }
  });
  document.getElementById('studio-copy-ai')?.addEventListener('click', async () => {
    const txt = state.studio?.payload?.aiPrompt;
    if (!txt) return;
    try {
      await navigator.clipboard.writeText(txt);
      showToast('AI / MCP çözüm istemi panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
}

export function renderConflictStudio() {
  const st = state.studio;
  if (!st) return;
  const { payload, activeFileIdx, customByPath } = st;
  const files = payload.files || [];
  const curFile = files[activeFileIdx] || files[0];
  const strategy = studioStrategyOf(st, curFile);

  if (el.studioBranchBadge) el.studioBranchBadge.textContent = `${payload.branchName} ↔ ${payload.baseRef}`;

  if (el.studioFileTabs) {
    el.studioFileTabs.innerHTML = '';
    files.forEach((f, idx) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `studio-file-tab ${idx === activeFileIdx ? 'active' : ''}`;
      btn.textContent = `📄 ${f.path}`;
      if (files.length > 1) {
        // Dosya başına seçim: hangi dosya hangi stratejiyle çözülecek sekmede görünür.
        const tag = document.createElement('span');
        tag.className = 'studio-tab-strat';
        tag.textContent = STRATEGY_SHORT[studioStrategyOf(st, f)];
        btn.append(' ', tag);
      }
      btn.addEventListener('click', () => {
        if (studioStrategyOf(st, curFile) === 'custom' && curFile && el.studioRightPreview) {
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
      strategy: payload.recommended || 'union', // varsayılan; dosya başına seçim strategyByPath'te
      strategyByPath: {},
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
  if (studioStrategyOf(st, curFile) === 'custom' && curFile && el.studioRightPreview) {
    st.customByPath[curFile.path] = el.studioRightPreview.value;
  }

  const body = studioApplyPayload(st);
  const runVet = Boolean(el.studioRunVet?.checked);

  if (el.studioErrorBanner) el.studioErrorBanner.hidden = true;
  if (el.studioErrorText) el.studioErrorText.textContent = '';
  if (el.studioApplyBtn) {
    el.studioApplyBtn.disabled = true;
    el.studioApplyBtn.textContent = 'Uygulanıyor...';
  }

  try {
    const snap = state.offlineMode
      ? (window.LocalFallbackEngine ? window.LocalFallbackEngine.resolveConflict(st.payload.branchId, body.strategy, body.fileStrategies) : state.snapshot)
      : await apiCall('/api/branch/resolve', 'POST', {
        branchId: st.payload.branchId,
        strategy: body.strategy,
        fileStrategies: body.fileStrategies,
        customFiles: body.customFiles,
        runVet,
      });

    closeConflictStudio();
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast(`✓ Çakışma başarıyla çözüldü (${body.label.toUpperCase()}) ve doğrulandı!`, 'ok');
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

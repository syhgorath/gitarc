// web/modules/studio.js - Alt "Teşhis Stüdyosu" panelleri: dal künyesi + CI adımları, orta durum, satır içi canlı diff
//
// HTML üretimi saf fonksiyonlardadır (Node'da test edilir); `updateStudioPanels` ve `renderStudioInlineDiff`
// bunları DOM'a yazıp olayları bağlar.
import { state, el } from './state.js';
import { esc, truncate, isWorkingTreeBranch } from './utils.js';
import { apiCall } from './api.js';
import { CHECK_STUDIO_VIEW, classifyCheck } from './inspector-model.js';

// Diff satırının türü: ekleme, silme, @@ başlığı veya bağlam.
export function diffLineClass(l) {
  let cls = 'ctx';
  if (l.startsWith('+')) cls = 'add';
  else if (l.startsWith('-')) cls = 'del';
  else if (l.startsWith('@@')) cls = 'hdr';
  return cls;
}

// İlk 150 satır (uzun diff'ler arayüzü dondurmasın).
export function diffLinesHtml(rawPatch) {
  const lines = rawPatch.split('\n').slice(0, 150);
  return lines.map((l) => {
    const cls = diffLineClass(l);
    return `<div class="studio-diff-line ${cls}">${esc(l)}</div>`;
  }).join('');
}

export function diffPanelHtml(firstFile, linesHtml) {
    return `
      <div class="studio-diff-container">
        <div class="studio-diff-file-head">📄 ${esc(firstFile.path)} (+${firstFile.additions || 0} / -${firstFile.deletions || 0})</div>
        ${linesHtml}
      </div>
    `;
}

// Özet CI durumunun (SUCCESS/FAILURE/RUNNING) kontrol görünümü sınıfı.
export function summaryCheckClass(state) {
  if (state === 'FAILURE') return 'fail';
  if (state === 'RUNNING') return 'running';
  return 'ok';
}

// Sol kolon: CI adımları. Gerçek kontrol listesi varsa onu, yoksa yalnızca özet durumu gösterir (adım UYDURULMAZ).
export function studioCiStepsHtml(b) {
  let ciStepsHtml = '';
  const checks = b.pr?.ci?.checks || [];
  if (checks.length > 0) {
    ciStepsHtml = `
      <div class="studio-step-tree">
        ${checks.map((chk) => {
          const view = CHECK_STUDIO_VIEW[classifyCheck(chk)];
          const badgeClass = view.cls;
          const statusText = view.text;
          return `
            <div class="studio-step-item">
              <span class="studio-step-name">${esc(chk.name || 'Adım')}</span>
              <span class="studio-step-badge ${badgeClass}">${statusText}</span>
            </div>
          `;
        }).join('')}
      </div>
    `;
  } else if (b.pr?.ci) {
    // Adım ayrıntısı yok: eskiden burada sabit "lint 14s / test 45s / build 52s" adımları UYDURULUYORDU. Artık yalnızca
    // bilinen özet gösterilir (adı ve süresi uydurulmaz).
    const view = CHECK_STUDIO_VIEW[summaryCheckClass(b.pr.ci.state)];
    ciStepsHtml = `
      <div class="studio-step-tree">
        <div class="studio-step-item">
          <span class="studio-step-name">${esc(b.pr.ci.workflow || 'CI kontrolleri')}</span>
          <span class="studio-step-badge ${view.cls}">${view.text}</span>
        </div>
        <div class="studio-step-item"><span class="studio-step-name muted">adım ayrıntısı bildirilmedi</span></div>
      </div>
    `;
  }
  return ciStepsHtml;
}

// Sol kolon aksiyon butonları (birleşmiş / çalışma alanı / normal dal).
export function studioActionsHtml(b) {
  const isHead = Boolean(b.isHead);
  const isMerged = b.state === 'merged';
  const hasPR = Boolean(b.pr && b.pr.number > 0);
  let actionButtonsHtml = '';
  if (isMerged) {
    actionButtonsHtml = `
      <div class="studio-actions-grid">
        <button type="button" class="btn ghost sm" id="studio-act-diff">🔍 Diff Gör</button>
        ${b.hasLocal ? '<button type="button" class="btn danger sm" id="studio-act-del">🧹 Kalıntıyı Sil</button>' : ''}
      </div>
    `;
  } else if (isWorkingTreeBranch(b)) {
    actionButtonsHtml = `
      <div class="studio-actions-grid">
        <button type="button" class="btn primary sm" id="studio-act-commit">🚀 Dala Aktar / Commit</button>
        <button type="button" class="btn ghost sm" id="studio-act-diff">🔍 Değişiklikler</button>
      </div>
    `;
  } else {
    actionButtonsHtml = `
      <div class="studio-actions-grid">
        ${!isHead ? '<button type="button" class="btn primary sm" id="studio-act-checkout">🎯 Dala Geç</button>' : '<button type="button" class="btn ok sm" id="studio-act-checkout-main">🌿 main\'e Geç</button>'}
        ${hasPR ? '<button type="button" class="btn ok sm" id="studio-act-merge">⚡ Merge Et</button>' : '<button type="button" class="btn primary sm" id="studio-act-promote">🚀 Push &amp; PR</button>'}
        <button type="button" class="btn ghost sm" id="studio-act-diff">🔍 Diff İncele</button>
        <button type="button" class="btn danger sm" id="studio-act-del">🗑️ Dalı Sil</button>
      </div>
    `;
  }
  return actionButtonsHtml;
}

export function studioBranchTypeLabel(b) {
  return b.hemisphere === 'upper' ? '☁️ Üst Ray · PR' : (isWorkingTreeBranch(b) ? '✏️ Alt Ray · Çalışma' : '💻 Alt Ray · Yerel');
}

export function studioDetailHtml(b) {
  const branchTypeLabel = studioBranchTypeLabel(b);
  const ciStepsHtml = studioCiStepsHtml(b);
  const actionButtonsHtml = studioActionsHtml(b);
  return `
    <div class="studio-detail-card">
      <div class="studio-card-row">
        <span class="studio-branch-name">${esc(truncate(b.name, 24))}</span>
        <span class="chip sm accent">${esc(branchTypeLabel)}</span>
      </div>
      <div class="studio-card-row">
        <span class="studio-author-pill">👤 ${esc(b.author || 'Geliştirici')}</span>
        <span class="studio-sha-badge">${esc(b.localSha ? b.localSha.slice(0, 7) : (b.baseSha ? b.baseSha.slice(0, 7) : 'HEAD'))}</span>
      </div>
      ${ciStepsHtml}
      ${actionButtonsHtml}
    </div>
  `;
}


export async function renderStudioInlineDiff(targetObj, onOpenDiff = null) {
  if (!el.studioDiffContent) return;

  if (!targetObj) {
    el.studioDiffContent.innerHTML = '<div class="studio-placeholder">Diff görüntülemek için bir dal veya commit seçin</div>';
    if (el.studioDiffMeta) el.studioDiffMeta.textContent = '';
    return;
  }

  el.studioDiffContent.innerHTML = '<div class="studio-placeholder">Diff yükleniyor...</div>';
  if (el.studioDiffMeta) el.studioDiffMeta.textContent = 'Yükleniyor...';

  const isCommit = Boolean(targetObj.shortSha && !targetObj.name);
  const isWorkingTree = isWorkingTreeBranch(targetObj);
  const target = isWorkingTree ? 'working-tree' : (isCommit ? (targetObj.sha || targetObj.shortSha) : targetObj.name);
  const diffType = isWorkingTree ? 'working-tree' : (isCommit ? 'commit' : 'branch');

  if (el.studioDiffFullscreen) {
    el.studioDiffFullscreen.onclick = () => {
      if (onOpenDiff) onOpenDiff(target, diffType);
    };
  }

  try {
    let diffData = null;
    if (!state.offlineMode) {
      diffData = await apiCall(`/api/diff?target=${encodeURIComponent(target)}&type=${diffType}`, 'GET');
    } else if (window.LocalFallbackEngine) {
      diffData = window.LocalFallbackEngine.getDiff(target, diffType);
    }

    if (!diffData || !diffData.files || diffData.files.length === 0) {
      el.studioDiffContent.innerHTML = '<div class="studio-placeholder">Bu dalda veya committe farklı dosya değişikliği yok.</div>';
      if (el.studioDiffMeta) el.studioDiffMeta.textContent = '0 dosya';
      return;
    }

    const firstFile = diffData.files[0];
    if (el.studioDiffMeta) {
      el.studioDiffMeta.textContent = `${diffData.files.length} dosya (${truncate(firstFile.path || '', 22)})`;
    }

    const rawPatch = firstFile.patch || '';
    if (!rawPatch) {
      el.studioDiffContent.innerHTML = `<div class="studio-placeholder">Dosya: ${esc(firstFile.path)} (İkili veya boş içerik)</div>`;
      return;
    }

    const linesHtml = diffLinesHtml(rawPatch);
    el.studioDiffContent.innerHTML = diffPanelHtml(firstFile, linesHtml);
  } catch (err) {
    el.studioDiffContent.innerHTML = `<div class="studio-placeholder">Diff alınamadı: ${esc(err.message)}</div>`;
    if (el.studioDiffMeta) el.studioDiffMeta.textContent = 'Hata';
  }
}

// Stüdyo sol kolonundaki butonların (id) karşılığı geri çağrılar.
export const STUDIO_ACTION_IDS = [
  'studio-act-checkout', 'studio-act-checkout-main', 'studio-act-merge', 'studio-act-promote',
  'studio-act-del', 'studio-act-diff', 'studio-act-commit',
];

export function dispatchStudioAction(actId, b, callbacks, btn) {
  const { onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenDiff, onSelectBranch } = callbacks;
  switch (actId) {
    case 'studio-act-checkout':
      if (onCheckoutBranch) onCheckoutBranch(b.id, btn);
      break;
    case 'studio-act-checkout-main':
      if (onCheckoutBranch) onCheckoutBranch('main', btn);
      break;
    case 'studio-act-merge':
      if (onMergeBranch) onMergeBranch(b.id, btn);
      break;
    case 'studio-act-promote':
      if (onPromoteBranch) onPromoteBranch(b.id, undefined, undefined, btn);
      break;
    case 'studio-act-del':
      if (onDeleteBranch) onDeleteBranch(b.id, btn);
      break;
    case 'studio-act-diff':
      if (onOpenDiff) {
        if (isWorkingTreeBranch(b)) onOpenDiff('working-tree', 'working-tree');
        else onOpenDiff(b.name, 'branch');
      }
      break;
    case 'studio-act-commit':
      // Çalışma alanını seçer: sağ Inspector'daki Konsolsuz Commit & PR formu açılır.
      if (onSelectBranch) onSelectBranch(b.id);
      break;
    default:
      break;
  }
}

export async function updateStudioPanels(b, callbacks = {}) {
  if (!el.studioBranchDetail) return;

  const { onOpenDiff } = callbacks;

  if (!b) {
    if (el.studioBranchChip) el.studioBranchChip.textContent = 'Seçim Yok';
    el.studioBranchDetail.innerHTML = '<div class="studio-placeholder">Tuvalden bir dal veya commit seçin</div>';
    if (el.studioDiffContent) el.studioDiffContent.innerHTML = '<div class="studio-placeholder">Diff görüntülemek için bir dal veya commit seçin</div>';
    if (el.studioDiffMeta) el.studioDiffMeta.textContent = '';
    return;
  }
  if (el.studioBranchChip) {
    el.studioBranchChip.textContent = b.name;
    el.studioBranchChip.title = b.name;
  }

  el.studioBranchDetail.innerHTML = studioDetailHtml(b);

  // Sol Kolon Aksiyon Listener'ları
  STUDIO_ACTION_IDS.forEach((id) => {
    document.getElementById(id)?.addEventListener('click', (ev) => {
      dispatchStudioAction(id, b, callbacks, ev.currentTarget);
    });
  });

  // 2. Orta Kolon Durum & Hata Vurgusu
  if (b.pr?.ci?.state === 'FAILURE' && el.studioMidStatus) {
    el.studioMidStatus.textContent = '❌ CI Hatası Bulundu';
  } else if (el.studioMidStatus) {
    el.studioMidStatus.textContent = 'Canlı Terminal';
  }

  // 3. Sağ Kolon: Satır İçi Canlı Diff Yükle
  await renderStudioInlineDiff(b, onOpenDiff);
}

// web/modules/inspector.js - Sağ detay paneli (Inspector) orkestratörü
//
// Kararlar inspector-model.js'te, HTML kartları inspector-cards.js'te, alt stüdyo panelleri studio.js'te,
// AI öneri olayları proposal.js'te. Burada yalnızca hangi kartların hangi sırayla basılacağı ve olay bağlama var.
import { state, el } from './state.js';
import { esc, isSecretFile, showToast, diffArgsFor, mergeCommitOf } from './utils.js';
import { apiCall } from './api.js';
import {
  branchViewModel, isMergedRemnant, workingTreeDefaults, promoteDefaults, findSelectedBranch, findSelectedNode,
} from './inspector-model.js';
import {
  nodeInspectorHtml, nodeStudioHtml, proposalCardHtml, branchHeaderHtml, layersHtml, conflictCardHtml,
  crossPrCardHtml, ciCardHtml, mergedRemnantCardHtml, commitStudioCardHtml, promoteCardHtml, syncedCardHtml,
  changedFilesCardHtml, prEditCardHtml, prDetailCardHtml, amendCardHtml, suggestedCmdCardHtml,
  mergedFilesCardHtml, mergedFilesRowsHtml,
} from './inspector-cards.js';
import { renderStudioInlineDiff, updateStudioPanels } from './studio.js';
import { wireProposalActions } from './proposal.js';

export { renderStudioInlineDiff, updateStudioPanels, wireProposalActions };

// Dal seçiliyken inspector'daki tüm kontrollerin olay bağlamaları.
function wireBranchControls(b, v, callbacks) {
  const { isVirtualWip } = v;
  const {
    onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch,
    onOpenConflictStudio, onFixCI, onLocalTestRun, onEditPR, onAmendCommit,
    onCommitWorkingTree, onDiscardWorkingTree, onOpenDiff
  } = callbacks;

  document.getElementById('act-checkout')?.addEventListener('click', (ev) => {
    if (onCheckoutBranch) onCheckoutBranch(b.id, ev.currentTarget);
  });
  document.getElementById('act-checkout-main')?.addEventListener('click', (ev) => {
    if (onCheckoutBranch) onCheckoutBranch('main', ev.currentTarget);
  });
  document.getElementById('act-checkout-main-synced')?.addEventListener('click', (ev) => {
    if (onCheckoutBranch) onCheckoutBranch('main', ev.currentTarget);
  });
  document.getElementById('act-delete-synced')?.addEventListener('click', (ev) => {
    if (onDeleteBranch) onDeleteBranch(b.id, ev.currentTarget);
  });
  document.getElementById('act-promote')?.addEventListener('click', (ev) => {
    if (onPromoteBranch) onPromoteBranch(b.id, undefined, undefined, ev.currentTarget);
  });

  const formWt = document.getElementById('form-wt-commit');
  formWt?.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const brName = document.getElementById('wt-branch-name').value;
    const msg = document.getElementById('wt-commit-msg').value;
    const prTitle = document.getElementById('wt-pr-title')?.value || '';
    const prBody = document.getElementById('wt-pr-body')?.value || '';
    const btn = document.getElementById('act-wt-commit-only');
    if (onCommitWorkingTree) onCommitWorkingTree(brName, msg, false, prTitle, prBody, btn);
  });

  document.getElementById('act-wt-commit-push')?.addEventListener('click', () => {
    const brName = document.getElementById('wt-branch-name').value;
    const msg = document.getElementById('wt-commit-msg').value;
    const prTitle = document.getElementById('wt-pr-title')?.value || '';
    const prBody = document.getElementById('wt-pr-body')?.value || '';
    const btn = document.getElementById('act-wt-commit-push');
    if (onCommitWorkingTree) onCommitWorkingTree(brName, msg, true, prTitle, prBody, btn);
  });

  document.getElementById('act-wt-discard')?.addEventListener('click', () => {
    if (onDiscardWorkingTree) onDiscardWorkingTree();
  });

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
    if (onPromoteBranch) onPromoteBranch(b.id, pTitle, pBody, btn);
  });

  document.getElementById('merge-strategy')?.addEventListener('change', (ev) => {
    state.mergeStrategy = ev.target.value;
  });
  document.getElementById('act-merge')?.addEventListener('click', (ev) => {
    if (onMergeBranch) onMergeBranch(b.id, ev.currentTarget);
  });
  document.getElementById('act-branch-diff')?.addEventListener('click', () => {
    if (onOpenDiff) {
      onOpenDiff(...diffArgsFor(b, state.snapshot));
    }
  });
  el.detail.querySelectorAll('.file-tag').forEach((tag) => {
    tag.addEventListener('click', () => {
      if (onOpenDiff) {
        onOpenDiff(...diffArgsFor(b, state.snapshot));
      }
    });
  });
  document.getElementById('act-resolve')?.addEventListener('click', () => {
    if (onOpenConflictStudio) onOpenConflictStudio(b.id);
  });
  document.getElementById('act-resolve-card')?.addEventListener('click', () => {
    if (onOpenConflictStudio) onOpenConflictStudio(b.id);
  });
  document.getElementById('act-fix-ci')?.addEventListener('click', () => {
    if (onFixCI) onFixCI(b.id);
  });
  document.getElementById('canvas-inspector-close')?.addEventListener('click', () => {
    state.selectedBranchId = null;
    state.selectedNodeSha = null;
    if (el.canvasInspector) el.canvasInspector.hidden = true;
    if (callbacks.onSelectBranch) callbacks.onSelectBranch(null);
  });
  document.getElementById('act-delete')?.addEventListener('click', (ev) => {
    if (onDeleteBranch) onDeleteBranch(b.id, ev.currentTarget);
  });
  document.getElementById('act-clean-and-switch-main')?.addEventListener('click', (ev) => {
    if (onDeleteBranch) onDeleteBranch(b.id, ev.currentTarget);
  });
  document.getElementById('act-open-new-branch-modal')?.addEventListener('click', () => {
    document.getElementById('btn-new-branch')?.click();
  });

  document.getElementById('form-edit-pr')?.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const title = document.getElementById('pr-edit-title').value;
    const body = document.getElementById('pr-edit-body').value;
    if (onEditPR) onEditPR(b.id, title, body);
  });

  document.getElementById('form-amend-commit')?.addEventListener('submit', (ev) => {
    ev.preventDefault();
    const msg = document.getElementById('commit-amend-msg').value;
    if (onAmendCommit) onAmendCommit(b.id, msg);
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
      await navigator.clipboard.writeText(b.pr?.ci?.rawLog || '');
      showToast('CI hata logu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi; logu seçip kopyalayabilirsiniz', 'warn');
    }
  });

  document.getElementById('act-run-local')?.addEventListener('click', () => {
    const cmd = b.pr?.ci?.extracts?.[0]?.reproCmd || 'go test -v ./...';
    if (onLocalTestRun) onLocalTestRun(b.id, cmd);
  });
}

export function renderInspector(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap || !el.detail) return;

  const {
    onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch,
    onOpenConflictStudio, onFixCI, onLocalTestRun, onEditPR, onAmendCommit,
    onCommitWorkingTree, onDiscardWorkingTree, onApplySnapshot, onOpenDiff
  } = callbacks;

  if (state.selectedNodeSha) {
    const node = findSelectedNode(snap, state.selectedNodeSha);
    if (node) {
      if (el.canvasInspector) el.canvasInspector.hidden = false;
      if (el.canvasInspectorTitle) el.canvasInspectorTitle.textContent = `Commit ${node.shortSha}`;

      el.detail.innerHTML = nodeInspectorHtml(node);
      document.getElementById('canvas-inspector-close')?.addEventListener('click', () => {
        state.selectedNodeSha = null;
        state.selectedBranchId = null;
        if (el.canvasInspector) el.canvasInspector.hidden = true;
        if (callbacks.onSelectNode) callbacks.onSelectNode(null);
      });
      document.getElementById('act-node-diff')?.addEventListener('click', () => {
        if (onOpenDiff) onOpenDiff(node.sha || node.shortSha, 'commit');
      });

      // Studio Panellerini güncelle (Commit modu)
      if (el.studioBranchChip) el.studioBranchChip.textContent = `Commit ${node.shortSha}`;
      if (el.studioBranchDetail) {
        el.studioBranchDetail.innerHTML = nodeStudioHtml(node);
        document.getElementById('studio-act-node-diff')?.addEventListener('click', () => {
          if (onOpenDiff) onOpenDiff(node.sha || node.shortSha, 'commit');
        });
      }
      renderStudioInlineDiff(node, onOpenDiff);
      return;
    }
  }

  const proposalHtml = snap.pendingProposal
    ? proposalCardHtml(snap.pendingProposal, state.proposalDraftEdits)
    : '';

  const b = findSelectedBranch(snap, state.selectedBranchId);
  if (!b) {
    if (snap.pendingProposal) {
      if (el.canvasInspector) el.canvasInspector.hidden = false;
      if (el.canvasInspectorTitle) el.canvasInspectorTitle.textContent = '🤖 AI Commit & PR Önerisi';
      el.detail.innerHTML = proposalHtml;
      document.getElementById('canvas-inspector-close')?.addEventListener('click', () => {
        if (el.canvasInspector) el.canvasInspector.hidden = true;
      });
      wireProposalActions(onApplySnapshot);
      return;
    }
    if (el.canvasInspector) el.canvasInspector.hidden = true;
    el.detail.innerHTML = '';

    const defaultBranch = (snap.branches || []).find((br) => br.isHead) || (snap.branches || [])[0];
    if (defaultBranch) {
      updateStudioPanels(defaultBranch, callbacks);
    } else {
      updateStudioPanels(null, callbacks);
    }
    return;
  }

  // Alt Teşhis Stüdyosu Panellerini güncelle (Dal modu)
  updateStudioPanels(b, callbacks);

  const v = branchViewModel(b, snap, state);
  const {
    isOpen, isVirtualWip, isAlreadySynced, canPromote, canCommitWorkingTree, isLive,
  } = v;
  if (el.canvasInspector) el.canvasInspector.hidden = false;
  if (el.canvasInspectorTitle) el.canvasInspectorTitle.textContent = v.title;

  let html = branchHeaderHtml({ ...v, proposalHtml, mergeStrategy: state.mergeStrategy });
  html += layersHtml(v);

  if (b.conflict) {
    html += conflictCardHtml(b);
  }

  if (b.crossPr?.length) {
    html += crossPrCardHtml(b);
  }

  if (b.pr?.ci) {
    html += ciCardHtml(b, { isLive, localRunOut: state.localRunOutput[b.id] || '' });
  }

  if (canCommitWorkingTree) {
    const candidateFiles = (b.changedFiles || []).filter(Boolean);
    const secretFile = candidateFiles.find((f) => isSecretFile(f, snap));

    if (isMergedRemnant(b) && !isVirtualWip) {
      html += mergedRemnantCardHtml(b, candidateFiles);
    } else {
      const defaults = workingTreeDefaults(b, candidateFiles, snap, isVirtualWip);
      html += commitStudioCardHtml(b, snap, { secretFile, candidateFiles, ...defaults });
    }
  }

  if (canPromote) {
    const promote = promoteDefaults(b, state.draftPrEdits[b.name]);
    html += promoteCardHtml(b, promote.title, promote.body);
  } else if (isAlreadySynced && b.hemisphere === 'lower') {
    html += syncedCardHtml(b);
  }

  // Değişen dosyalar kartı HER dal için aynı: dosya dosya +/- ve tıklayınca o dosyanın diff'i (eskiden yalnızca adlar vardı
  // ve içeriğe ulaşılamıyordu). Birleşmiş dal birleşme commit'inden, diğerleri dal/çalışma alanından okunur.
  const mergeCommit = mergeCommitOf(b, snap);
  const hasFiles = Boolean(mergeCommit) || (b.changedFiles || []).length > 0;
  if (hasFiles) {
    html += mergedFilesCardHtml();
  }

  if (b.pr && isOpen) {
    html += prEditCardHtml(b);
  } else if (b.pr && !isOpen) {
    html += prDetailCardHtml(b);
  }

  if (b.canAmendCommit) {
    html += amendCardHtml(b);
  }

  if (b.suggestedCmd) {
    html += suggestedCmdCardHtml(b);
  }

  el.detail.innerHTML = html;

  if (snap.pendingProposal) {
    wireProposalActions(callbacks.onApplySnapshot);
  }

  wireBranchControls(b, v, callbacks);
  if (hasFiles) loadMergedFiles(b, mergeCommit, callbacks);
}

// Dal pencerelerindeki dosya listesi: diff'ten (dosya dosya +/-) yüklenir; satıra tıklayınca O DOSYANIN diff'i açılır.
// Diff alınamazsa (örn. çevrimdışı) bilinen dosya adları istatistiksiz gösterilir.
async function loadMergedFiles(b, mergeCommit, callbacks) {
  const [target, type] = diffArgsFor(b, state.snapshot);
  const fallbackFiles = (b.changedFiles || []).map((path) => ({ path, status: 'M' }));
  const paint = (files, countText) => {
    if (state.selectedBranchId !== b.id) return; // kullanıcı başka bir öğeye geçti
    const list = document.getElementById('merged-files-list');
    const count = document.getElementById('merged-files-count');
    if (!list) return;
    list.innerHTML = mergedFilesRowsHtml(files);
    if (count) count.textContent = countText;
    list.querySelectorAll('.merged-file-row').forEach((row) => {
      row.addEventListener('click', () => {
        const f = files[Number(row.getAttribute('data-file-idx'))];
        if (callbacks.onOpenDiff) callbacks.onOpenDiff(target, type, f?.path || '');
      });
    });
  };
  try {
    let res;
    if (state.offlineMode || window.location.protocol === 'file:') {
      res = window.LocalFallbackEngine?.getDiff ? window.LocalFallbackEngine.getDiff(target, type) : { files: [] };
    } else {
      res = await apiCall(`/api/diff?target=${encodeURIComponent(target)}&type=${encodeURIComponent(type)}`);
    }
    const files = res?.files?.length ? res.files : fallbackFiles;
    paint(files, `${files.length} dosya · +${res?.totalAdds || 0} / -${res?.totalDels || 0}`);
  } catch (err) {
    paint(fallbackFiles, fallbackFiles.length ? `${fallbackFiles.length} dosya` : 'okunamadı');
  }
}

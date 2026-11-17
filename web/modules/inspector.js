// web/modules/inspector.js - Right detail inspector panel and form controller
import { state, el } from './state.js';
import {
  esc, truncate, isMergedOrGone, isSecretFile, suggestUniqueBranchName, showToast
} from './utils.js';
import { apiCall } from './api.js';

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
  const isWorkingTree = Boolean(targetObj.isWorkingTreeWip);
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

    const lines = rawPatch.split('\n').slice(0, 150);
    const linesHtml = lines.map((l) => {
      let cls = 'ctx';
      if (l.startsWith('+')) cls = 'add';
      else if (l.startsWith('-')) cls = 'del';
      else if (l.startsWith('@@')) cls = 'hdr';
      return `<div class="studio-diff-line ${cls}">${esc(l)}</div>`;
    }).join('');

    el.studioDiffContent.innerHTML = `
      <div class="studio-diff-container">
        <div class="studio-diff-file-head">📄 ${esc(firstFile.path)} (+${firstFile.additions || 0} / -${firstFile.deletions || 0})</div>
        ${linesHtml}
      </div>
    `;
  } catch (err) {
    el.studioDiffContent.innerHTML = `<div class="studio-placeholder">Diff alınamadı: ${esc(err.message)}</div>`;
    if (el.studioDiffMeta) el.studioDiffMeta.textContent = 'Hata';
  }
}

export async function updateStudioPanels(b, callbacks = {}) {
  if (!el.studioBranchDetail) return;

  const { onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenDiff } = callbacks;

  if (!b) {
    if (el.studioBranchChip) el.studioBranchChip.textContent = 'Seçim Yok';
    el.studioBranchDetail.innerHTML = '<div class="studio-placeholder">Tuvalden bir dal veya commit seçin</div>';
    if (el.studioDiffContent) el.studioDiffContent.innerHTML = '<div class="studio-placeholder">Diff görüntülemek için bir dal veya commit seçin</div>';
    if (el.studioDiffMeta) el.studioDiffMeta.textContent = '';
    return;
  }

  // 1. Sol Kolon: Dal Künyesi & CI Adımları
  const isHead = Boolean(b.isHead);
  const isMerged = b.state === 'merged';
  const hasPR = Boolean(b.pr && b.pr.number > 0);
  const branchTypeLabel = b.hemisphere === 'upper' ? '☁️ Üst Ray · PR' : (b.isWorkingTreeWip ? '✏️ Alt Ray · Çalışma' : '💻 Alt Ray · Yerel');

  if (el.studioBranchChip) {
    el.studioBranchChip.textContent = b.name;
    el.studioBranchChip.title = b.name;
  }

  let ciStepsHtml = '';
  const checks = b.pr?.ci?.checks || [];
  if (checks.length > 0) {
    ciStepsHtml = `
      <div class="studio-step-tree">
        ${checks.map((chk) => {
          const conc = (chk.conclusion || chk.status || '').toUpperCase();
          let badgeClass = 'ok';
          let statusText = 'Başarılı';
          if (conc === 'FAILURE' || conc === 'TIMED_OUT' || conc === 'CANCELLED') {
            badgeClass = 'fail';
            statusText = 'Hatalı';
          } else if (conc === 'IN_PROGRESS' || conc === 'QUEUED') {
            badgeClass = 'running';
            statusText = 'Çalışıyor';
          } else if (conc === 'SKIPPED') {
            badgeClass = 'skip';
            statusText = 'Atlandı';
          }
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
    const ci = b.pr.ci;
    const isFail = ci.state === 'FAILURE';
    ciStepsHtml = `
      <div class="studio-step-tree">
        <div class="studio-step-item">
          <span class="studio-step-name">lint</span>
          <span class="studio-step-badge ok">Başarılı</span>
          <span class="studio-step-time">14s</span>
        </div>
        <div class="studio-step-item">
          <span class="studio-step-name">test</span>
          <span class="studio-step-badge ${isFail ? 'fail' : 'ok'}">${isFail ? 'Hatalı' : 'Başarılı'}</span>
          <span class="studio-step-time">${isFail ? '38s' : '45s'}</span>
        </div>
        <div class="studio-step-item">
          <span class="studio-step-name">build</span>
          <span class="studio-step-badge ${isFail ? 'skip' : 'ok'}">${isFail ? 'Atlandı' : 'Başarılı'}</span>
          <span class="studio-step-time">${isFail ? '—' : '52s'}</span>
        </div>
      </div>
    `;
  }

  // Sol Kolon Aksiyon Butonları
  let actionButtonsHtml = '';
  if (isMerged) {
    actionButtonsHtml = `
      <div class="studio-actions-grid">
        <button type="button" class="btn ghost sm" id="studio-act-diff">🔍 Diff Gör</button>
        ${b.hasLocal ? '<button type="button" class="btn danger sm" id="studio-act-del">🧹 Kalıntıyı Sil</button>' : ''}
      </div>
    `;
  } else if (b.isWorkingTreeWip) {
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

  el.studioBranchDetail.innerHTML = `
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

  // Sol Kolon Aksiyon Listener'ları
  document.getElementById('studio-act-checkout')?.addEventListener('click', (ev) => {
    if (onCheckoutBranch) onCheckoutBranch(b.id, ev.currentTarget);
  });
  document.getElementById('studio-act-checkout-main')?.addEventListener('click', (ev) => {
    if (onCheckoutBranch) onCheckoutBranch('main', ev.currentTarget);
  });
  document.getElementById('studio-act-merge')?.addEventListener('click', (ev) => {
    if (onMergeBranch) onMergeBranch(b.id, ev.currentTarget);
  });
  document.getElementById('studio-act-promote')?.addEventListener('click', (ev) => {
    if (onPromoteBranch) onPromoteBranch(b.id, undefined, undefined, ev.currentTarget);
  });
  document.getElementById('studio-act-del')?.addEventListener('click', (ev) => {
    if (onDeleteBranch) onDeleteBranch(b.id, ev.currentTarget);
  });
  document.getElementById('studio-act-diff')?.addEventListener('click', () => {
    if (onOpenDiff) {
      if (b.isWorkingTreeWip) onOpenDiff('working-tree', 'working-tree');
      else onOpenDiff(b.name, 'branch');
    }
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

export function wireProposalActions(onApplySnapshot = null) {
  const prop = state.snapshot?.pendingProposal;
  if (!prop) return;

  const bIn = document.getElementById('prop-edit-branch');
  const mIn = document.getElementById('prop-edit-message');
  const tIn = document.getElementById('prop-edit-title');
  const dIn = document.getElementById('prop-edit-body');

  const saveDraft = () => {
    state.proposalDraftEdits = {
      id: prop.id,
      branch: bIn ? bIn.value.trim() : prop.branchName,
      message: mIn ? mIn.value : prop.message,
      title: tIn ? tIn.value.trim() : prop.prTitle,
      body: dIn ? dIn.value : prop.prBody,
    };
  };

  bIn?.addEventListener('input', saveDraft);
  mIn?.addEventListener('input', saveDraft);
  tIn?.addEventListener('input', saveDraft);
  dIn?.addEventListener('input', saveDraft);

  document.getElementById('act-proposal-approve')?.addEventListener('click', async () => {
    const branchName = bIn ? bIn.value.trim() : prop.branchName;
    const message = mIn ? mIn.value.trim() : prop.message;
    const prTitle = tIn ? tIn.value.trim() : prop.prTitle;
    const prBody = dIn ? dIn.value : prop.prBody;

    if (!branchName) {
      showToast('Lütfen geçerli bir dal adı girin', 'danger');
      return;
    }
    if (!message) {
      showToast('Commit mesajı boş olamaz', 'danger');
      return;
    }

    const payload = {
      proposalId: prop.id,
      branchName,
      message,
      prTitle,
      prBody,
    };

    if (!state.offlineMode) {
      try {
        const snap = await apiCall('/api/proposal/approve', 'POST', payload);
        delete state.proposalDraftEdits;
        showToast(`✓ AI önerisi onaylandı: ${branchName} dalı oluşturuldu ve PR açıldı!`, 'ok');
        if (onApplySnapshot) onApplySnapshot(snap);
      } catch (err) {
        showToast(`Hata: ${err.message}`, 'danger');
      }
    } else if (window.LocalFallbackEngine) {
      try {
        const snap = window.LocalFallbackEngine.approveProposal(payload);
        delete state.proposalDraftEdits;
        showToast(`✓ AI önerisi onaylandı (Offline Mod): ${branchName}`, 'ok');
        if (onApplySnapshot) onApplySnapshot(snap);
      } catch (err) {
        showToast(`Hata: ${err.message}`, 'danger');
      }
    }
  });

  document.getElementById('act-proposal-dismiss')?.addEventListener('click', async () => {
    delete state.proposalDraftEdits;
    showToast('AI commit/PR önerisi reddedildi (dosyalarınız korundu)', 'info');
    if (!state.offlineMode) {
      try {
        const dismissedSnap = await apiCall('/api/proposal/dismiss', 'POST', {});
        if (onApplySnapshot) onApplySnapshot(dismissedSnap);
      } catch (_) {}
    } else if (window.LocalFallbackEngine) {
      window.LocalFallbackEngine.dismissProposal();
      if (onApplySnapshot) onApplySnapshot(state.snapshot);
    }
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
    const node = snap.nodes.find((n) => n.shortSha === state.selectedNodeSha);
    if (node) {
      if (el.canvasInspector) el.canvasInspector.hidden = false;
      if (el.canvasInspectorTitle) el.canvasInspectorTitle.textContent = `Commit ${node.shortSha}`;

      el.detail.innerHTML = `
        <div class="det-header">
          <div class="det-title-row">
            <h3 class="det-branch-name">Commit ${esc(node.shortSha)}</h3>
            ${node.tag ? `<span class="chip accent">${esc(node.tag)}</span>` : ''}
          </div>
          <div class="det-badges">
            <span class="chip">👤 ${esc(node.author)}</span>
            <span class="chip">${esc(node.relativeTime)}</span>
            ${node.isMerge ? '<span class="chip ok">Merge Node</span>' : ''}
          </div>
          <div class="det-actions">
            <button type="button" class="btn primary sm" id="act-node-diff">🔍 Commit Değişikliklerini Gör (Diff)</button>
          </div>
        </div>
        <div class="diag-card">
          <h4>Commit Mesajı</h4>
          <div class="layer-line">${esc(node.subject)}</div>
        </div>
        <div class="lock-note">🔒 <strong>main</strong> omurgasındaki commit mesajları tarihçe güvenliği için kilitlidir.</div>
      `;
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
        el.studioBranchDetail.innerHTML = `
          <div class="studio-detail-card">
            <div class="studio-card-row">
              <span class="studio-branch-name">Commit ${esc(node.shortSha)}</span>
              ${node.tag ? `<span class="chip sm accent">${esc(node.tag)}</span>` : '<span class="chip sm">Spine</span>'}
            </div>
            <div class="studio-card-row">
              <span class="studio-author-pill">👤 ${esc(node.author)}</span>
              <span class="studio-sha-badge">${esc(node.relativeTime)}</span>
            </div>
            <div class="layer-line">${esc(node.subject)}</div>
            <div class="studio-actions-grid">
              <button type="button" class="btn primary sm" id="studio-act-node-diff">🔍 Diff Gör</button>
            </div>
          </div>
        `;
        document.getElementById('studio-act-node-diff')?.addEventListener('click', () => {
          if (onOpenDiff) onOpenDiff(node.sha || node.shortSha, 'commit');
        });
      }
      renderStudioInlineDiff(node, onOpenDiff);
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
          <textarea class="textarea-control" id="prop-edit-body" rows="4" placeholder="PR açıklaması...">${esc(bodyVal)}</textarea>
        </div>

        <div class="det-actions">
          <button type="button" class="btn primary sm" id="act-proposal-approve">✓ Onayla: Commit At &amp; PR Aç</button>
          <button type="button" class="btn ghost sm" id="act-proposal-dismiss">✕ Öneriyi Reddet (Temizle)</button>
        </div>
      </div>
    `;
  }

  const b = snap.branches.find((x) => x.id === state.selectedBranchId);
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

  const isOpen = b.state === 'open';
  const isVirtualWip = b.id === 'local-wip-working-tree';
  if (el.canvasInspector) el.canvasInspector.hidden = false;
  if (el.canvasInspectorTitle) el.canvasInspectorTitle.textContent = isVirtualWip ? '✏️ Çalışma Alanı (Taslak)' : b.name;
  const hasConflict = Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
  const isRemoteOnly = !b.hasLocal && b.hasRemote && isOpen;
  const isLive = snap.repo.mode === 'live';
  const ciState = b.pr?.ci?.state || '';
  const isRunning = ciState === 'RUNNING';
  const isQueued = Boolean(state.pendingAutoMerge[b.id]);
  const canCommitWorkingTree = isVirtualWip || (b.uncommittedCount || 0) > 0 || (b.isHead && (state.snapshot?.repo?.dirtyCount || 0) > 0);
  const isAlreadySynced = b.aheadMain === 0 && (b.uncommittedCount || 0) === 0 && !isVirtualWip;
  const canPromote = isOpen && b.hemisphere === 'lower' && !isVirtualWip && !canCommitWorkingTree && !isAlreadySynced;
  const hasOpenPR = Boolean(b.pr && b.pr.number > 0 && b.pr.reviewState !== 'MERGED');
  const canMerge = isOpen && hasOpenPR && !hasConflict && !isVirtualWip && ciState !== 'FAILURE';

  const authorName = (b.author || '').trim();
  const currentUser = (snap.repo?.currentUser || '').trim().toLowerCase();
  const isMe = authorName && currentUser && authorName.toLowerCase() === currentUser;

  let html = `
    ${proposalHtml}
    <div class="det-header">
      <div class="det-title-row">
        <h3 class="det-branch-name">${isVirtualWip ? '✏️ Çalışma Alanı (Kaydedilmemiş Değişiklikler)' : esc(b.name)}</h3>
        <span class="chip ${b.state === 'merged' ? 'ok' : (isVirtualWip ? 'purple' : (b.hemisphere === 'lower' ? 'purple' : 'accent'))}">
          ${isVirtualWip ? 'Yerel Taslak · Uncommitted' : (b.state === 'merged' ? 'Tam Parabol' : (isAlreadySynced ? '✓ main ile Eşit (Kalıntı Dal)' : (b.hemisphere === 'lower' && b.aheadMain === 0 ? '🌱 Yeni Yerel Dal' : b.hemisphere === 'lower' ? 'Alt Ray · Local' : 'Üst Ray · PR')))}
        </span>
      </div>
      <div class="det-badges">
        ${b.isHead ? '<span class="chip ok">HEAD (Aktif Dal)</span>' : ''}
        ${authorName ? `<span class="chip ${isMe ? 'accent' : 'purple'}" title="Dal/PR Sahibi">👤 @${esc(authorName)} ${isMe ? '(Ben)' : ''}</span>` : ''}
        ${b.pr?.ci ? `<span class="chip ${b.pr.ci.state === 'SUCCESS' ? 'ok' : (b.pr.ci.state === 'FAILURE' ? 'danger' : 'warn')}">CI: ${esc(b.pr.ci.state)}</span>` : ''}
        ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
      </div>
      <div class="det-actions">
        <button type="button" class="btn sm" id="act-branch-diff" title="Kod değişikliklerini (diff) görüntüle">🔍 Diff Gör</button>
        ${!b.isHead && isOpen && !isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-checkout" ${snap.repo.teamMode ? 'disabled title="Team Server modunda paylaşılan HEAD kilitlidir"' : ''}>${isRemoteOnly ? '🎯 Yerelde İndir &amp; Geç' : '🎯 Dala Geç'}</button>` : ''}
        ${b.isHead && isAlreadySynced && !isVirtualWip ? `<button type="button" class="btn primary sm" id="act-checkout-main">🎯 main Omurgasına Geç</button>` : ''}
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
          ${b.pr?.ci ? `
            <div class="ci-merge-gate ${isRunning ? 'running' : (b.pr.ci.state === 'FAILURE' ? 'fail' : 'ok')}">
              ${b.pr.ci.state === 'FAILURE' ? '❌ CI kontrolleri başarısız; merge kilidi aktif' : (isRunning ? '⏳ CI kontrolleri sürüyor; otomatik merge kuyrukta' : '✓ Tüm CI kontrolleri başarılı; güvenle birleştirilebilir')}
            </div>
          ` : ''}
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

  if (b.pr?.ci) {
    const ci = b.pr.ci;
    const ciState = ci.state || 'SUCCESS';
    const isFailed = ciState === 'FAILURE';
    const isRunningCI = ciState === 'RUNNING';
    const checks = ci.checks || [];
    const extracts = ci.extracts || [];
    const localRunOut = state.localRunOutput[b.id] || '';

    let cardModifier = 'ci-ok';
    let statusText = '✓ Tüm Kontroller Başarılı';
    let chipTone = 'ok';
    if (isFailed) {
      cardModifier = '';
      statusText = '❌ Başarısız Kontrol';
      chipTone = 'danger';
    } else if (isRunningCI) {
      cardModifier = 'ci-running';
      statusText = '⏳ Kontroller Devam Ediyor...';
      chipTone = 'warn';
    }

    html += `
      <div class="diag-card ci ${cardModifier}">
        <h4>
          <span>⚙️ GitHub Actions CI Adımları ${checks.length > 0 ? `(${checks.length} Kontrol)` : ''}</span>
          <span class="chip ${chipTone}">${statusText}</span>
        </h4>
        <div class="wt-note">
          <strong>İş Akışı:</strong> ${esc(ci.workflow || 'ci.yml')} ${ci.duration ? `· Süre: ${esc(ci.duration)}` : ''}
          ${ci.failedStep ? `<br><span class="ci-failed-tag">Patlayan Adım: ${esc(ci.failedStep)}</span>` : ''}
        </div>

        ${checks.length > 0 ? `
          <div class="ci-checks-list">
            ${checks.map((chk) => {
              const conc = (chk.conclusion || chk.status || '').toUpperCase();
              let icon = '✅';
              let chipClass = 'ok';
              if (conc === 'FAILURE' || conc === 'TIMED_OUT' || conc === 'CANCELLED') {
                icon = '❌';
                chipClass = 'danger';
              } else if (conc === 'IN_PROGRESS' || conc === 'QUEUED' || conc === 'PENDING') {
                icon = '🟡';
                chipClass = 'warn';
              } else if (conc === 'SKIPPED') {
                icon = '⚪';
                chipClass = 'ghost';
              }
              return `
                <div class="ci-check-row">
                  <div class="ci-check-info">
                    <span class="ci-check-icon">${icon}</span>
                    <span class="ci-check-name">${esc(chk.name || 'check')}</span>
                  </div>
                  <div class="ci-check-status">
                    <span class="chip ${chipClass} sm mono">${esc(conc || 'COMPLETED')}</span>
                    ${chk.detailsUrl ? `<a href="${esc(chk.detailsUrl)}" target="_blank" rel="noopener" class="btn ghost sm ci-link-btn" title="GitHub Actions üzerinde incele">🔗 Detay</a>` : ''}
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        ` : ''}

        ${isFailed ? `
          <div class="ci-fail-section">
            <div class="layer-line"><strong>Gürültüsüz CI Hata Ayıklayıcı (${extracts.length} kritik bulgu):</strong></div>
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
            <pre class="terminal-box" id="ci-raw-log">${esc(localRunOut || ci.rawLog || 'Ham log bulunamadı.')}</pre>
          </div>
        ` : ''}
      </div>
    `;
  }

  if (canCommitWorkingTree) {
    const isMerged = isMergedOrGone(b) || b.state === 'merged' || b.pr?.reviewState === 'MERGED';
    const candidateFiles = (b.changedFiles || []).filter(Boolean);
    const secretFile = candidateFiles.find((f) => isSecretFile(f, snap));

    if (isMerged && !isVirtualWip) {
      html += `
        <div class="diag-card working-tree-card">
          <h4>
            <span>⚠️ Birleşmiş Kalıntı Dalda Açık Dosyalar</span>
            <span class="chip warn">${candidateFiles.length} Dosya Bekliyor</span>
          </h4>
          <div class="wt-note">
            Aktif dalınız (<strong>${esc(b.name)}</strong>) GitHub üzerinde birleşmiş ve silinmiştir (origin: gone). Birleşmiş bir dalın üzerine yeni commit veya PR açılamaz.<br><br>
            Açık <strong>${candidateFiles.length}</strong> dosyanız güvenle korunmaktadır. Bu kalıntıyı temizleyip ana omurgaya geçebilir veya dosyaları yeni bir dala aktarabilirsiniz.
          </div>
          <div class="det-actions">
            ${b.hasLocal ? '<button type="button" class="btn primary sm" id="act-clean-and-switch-main">🧹 Kalıntıyı Temizle ve main\'e Geç</button>' : ''}
            <button type="button" class="btn emerald sm" id="act-open-new-branch-modal">+ Yeni Dala Aktar</button>
            <button type="button" class="btn warn sm" id="act-wt-discard">🗑️ Değişiklikleri Geri Al (Discard)</button>
          </div>
        </div>
      `;
    } else {
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

      if (isVirtualWip) {
        defaultWtBranch = suggestUniqueBranchName('feat', candidateFiles, snap);
        branchPlaceholder = 'Örn: ' + defaultWtBranch;

        let prefix = 'feat';
        let scope = '';
        const first = (candidateFiles[0] || '').toLowerCase();
        if (first.startsWith('internal/mcp')) { scope = '(mcp)'; prefix = 'feat'; }
        else if (first.startsWith('web/')) { scope = '(ui)'; prefix = 'feat'; }
        else if (first.startsWith('docs/') || first.endsWith('.md')) { scope = '(docs)'; prefix = 'docs'; }
        else if (first.includes('test')) { scope = '(test)'; prefix = 'test'; }
        else if (first.startsWith('.github/')) { scope = '(ci)'; prefix = 'ci'; }

        const count = candidateFiles.length;
        const countLabel = count === 1 ? '1 dosya' : `${count} dosya`;
        const fileSummary = candidateFiles.slice(0, 2).map((x) => x.split('/').pop()).join(', ');
        const moreSuffix = count > 2 ? '...' : '';
        const desc = count > 0 ? `${countLabel} (${fileSummary}${moreSuffix}) güncellemeleri ve iyileştirmeler` : 'yeni özellik geliştirmeleri';

        defaultCommitMsg = `${prefix}${scope}: ${desc}`;
        defaultPrTitle = defaultCommitMsg;
        defaultPrBody = `## Neler Değişti?\n- ${candidateFiles.slice(0, 8).map((f) => `\`${f}\` üzerinde geliştirmeler`).join('\n- ')}\n\n## Doğrulama\n- Yerel testler ve kontroller tamamlandı.`;
      }

      html += `
        <div class="diag-card working-tree-card">
          <h4>
            <span>🚀 Konsolsuz Commit &amp; PR Stüdyosu</span>
            <span class="chip ${secretFile ? 'danger' : 'purple'}">${secretFile ? '🛡️ Güvenlik Engeli' : (candidateFiles.length + ' Dosya Bekliyor')}</span>
          </h4>
          <div class="wt-note">
            Terminal açmadan çalışma alanındaki açık dosyaları yeni bir dala aktarabilir, commit atabilir veya doğrudan origin'e pushlayıp GitHub PR açabilirsiniz.
          </div>

          ${secretFile ? `
            <div class="wt-alert danger">
              <strong>GÜVENLİK ENGELİ:</strong> '${esc(secretFile)}' dosyası gizli/hassas bilgi (secret/private key/env) olarak tespit edildi! Bu dosya güvenliğiniz için commitlenemez.
            </div>
          ` : ''}

          <form id="form-wt-commit" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="wt-branch-name">Hedef Dal Adı</label>
              <input type="text" id="wt-branch-name" class="input-control mono" value="${esc(defaultWtBranch)}" placeholder="${esc(branchPlaceholder)}" required ${snap.repo.teamMode ? 'disabled' : ''}>
            </div>
            <div class="wt-field">
              <label class="wt-label">Kaydedilecek Açık Dosyalar (${candidateFiles.length} dosya)</label>
              <div class="file-list">
                ${candidateFiles.map((f) => {
                  const isSec = isSecretFile(f, snap);
                  return `<span class="file-tag ${isSec ? 'special danger-file' : ''}" title="${isSec ? 'Güvenlik kalkanı tarafından engellendi' : ''}">${isSec ? '🛡️ ' : ''}${esc(f)}</span>`;
                }).join('')}
              </div>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-commit-msg">Commit Mesajı</label>
              <input type="text" id="wt-commit-msg" class="input-control mono" value="${esc(defaultCommitMsg)}" placeholder="Örn: feat(ui): add new interactive panel" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-title">PR Başlığı (Opsiyonel)</label>
              <input type="text" id="wt-pr-title" class="input-control mono" value="${esc(defaultPrTitle)}" placeholder="Örn: feat: add new interactive panel">
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
  } else if (isAlreadySynced && b.hemisphere === 'lower') {
    html += `
      <div class="diag-card">
        <h4>
          <span>✓ main Omurgası ile Senkronize</span>
          <span class="chip ok">Kalıntı Dal</span>
        </h4>
        <div class="wt-note">
          Bu dal (<strong>${esc(b.name)}</strong>) GitHub <strong>main</strong> omurgasıyla tamamen eşitlendi. Push edilecek yeni bir commit veya kaydedilmemiş açık değişiklik bulunmuyor.
        </div>
        <div class="det-actions">
          ${b.isHead ? '<button type="button" class="btn primary sm" id="act-checkout-main-synced">🎯 main Omurgasına Geç</button>' : ''}
          <button type="button" class="btn warn sm" id="act-delete-synced">🧹 Kalıntı Dalı Temizle</button>
        </div>
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
  } else if (b.pr && !isOpen) {
    html += `
      <div class="diag-card">
        <h4>
          <span>📜 PR #${b.pr.number} Detayı</span>
          <span class="chip ok">${esc(b.pr.reviewState || 'MERGED')}</span>
        </h4>
        <div class="layer-line"><strong>Başlık:</strong> ${esc(b.pr.title)}</div>
        ${b.pr.author ? `<div class="layer-line"><strong>Yazar:</strong> @${esc(b.pr.author)}</div>` : ''}
        ${b.pr.ci ? `
          <div class="layer-line"><strong>CI Durumu:</strong> <span class="chip ${b.pr.ci.state === 'SUCCESS' ? 'ok' : 'danger'} sm mono">CI: ${esc(b.pr.ci.state)}</span> (${esc(b.pr.ci.workflow || 'ci.yml')})</div>
        ` : ''}
        ${b.pr.body ? `
          <div class="wt-label">PR Açıklaması</div>
          <pre class="pr-body-box">${esc(b.pr.body)}</pre>
        ` : '<div class="wt-note">Bu PR için GitHub açıklaması kaydedilmemiş veya gövde boş.</div>'}
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
  }

  if (b.suggestedCmd) {
    html += `
      <div class="diag-card">
        <h4>
          <span>Tek Tıkla Terminal Komutu</span>
          <button type="button" class="btn ghost sm" id="act-copy-cmd">Kopyala</button>
        </h4>
        <pre class="terminal-box">${esc(b.suggestedCmd)}</pre>
      </div>
    `;
  }

  el.detail.innerHTML = html;

  if (snap.pendingProposal) {
    wireProposalActions(onApplySnapshot);
  }

  // Bind Event Listeners
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
      if (isVirtualWip) onOpenDiff('working-tree', 'working-tree');
      else onOpenDiff(b.name, 'branch');
    }
  });
  el.detail.querySelectorAll('.file-tag').forEach((tag) => {
    tag.addEventListener('click', () => {
      if (onOpenDiff) {
        if (isVirtualWip) onOpenDiff('working-tree', 'working-tree');
        else onOpenDiff(b.name, 'branch');
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

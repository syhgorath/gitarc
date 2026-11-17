// web/modules/canvas.js - Parabolic SVG Canvas, Topology Geometry and Capsule Renderer
import { state, el } from './state.js';
import {
  SVG_NS, XHTML_NS, MAIN_Y, SPINE_START_X, SPINE_STEP_X,
  LANE_BASE_OFFSET, LANE_STEP_Y, FO_W, FO_H, MERGED_FO_W, MERGED_FO_H,
  OPEN_CAPSULE_MIN_GAP_X
} from './constants.js';
import {
  esc, truncate, svgEl, isMergedOrGone, isOpenPR, isLocalOpen, isMyBranch
} from './utils.js';

export function xOfSpine(idx) {
  return SPINE_START_X + idx * SPINE_STEP_X;
}

export function yOfLane(hemisphere, lane, isMerged = false) {
  const l = Math.max(1, lane || 1);
  const offset = isMerged ? (84 + (l - 1) * 56) : (LANE_BASE_OFFSET + (l - 1) * LANE_STEP_Y);
  return hemisphere === 'lower' ? MAIN_Y + offset : MAIN_Y - offset;
}

export function layoutOpenApexPositions(branches) {
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
      const minClearX = hemi === 'upper' && lastMergedEndX > 0 ? Math.max(baseX, lastMergedEndX + 20) : baseX;
      const apexX = prevX === -Infinity ? minClearX : Math.max(minClearX, prevX + OPEN_CAPSULE_MIN_GAP_X);
      const apexY = hemi === 'lower' ? MAIN_Y + LANE_BASE_OFFSET : MAIN_Y - LANE_BASE_OFFSET;
      prevX = apexX;
      map.set(b.id, { apexX, apexY });
    });
  });
  return map;
}

export function computeBranchGeometry(b, tipXOverride = null, tipYOverride = null, apexLayout = null) {
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

export function branchToneClass(b) {
  if (b.pr?.ci?.state === 'FAILURE') return 'ci-fail';
  if (b.conflict?.hasConflict && !b.conflict.resolved) return 'conflict';
  if (b.pr?.ci?.state === 'RUNNING') return 'ci-running';
  if (b.state === 'merged') return 'merged';
  if (b.originGone || b.state === 'gone') return 'gone';
  return b.hemisphere === 'lower' ? 'lower-open' : 'upper-open';
}

export function branchStrokeColor(b) {
  if (b.pr?.ci?.state === 'FAILURE') return '#f85149';
  if (b.conflict?.hasConflict && !b.conflict.resolved) return '#d29922';
  if (b.pr?.ci?.state === 'RUNNING') return '#e3b341';
  if (b.state === 'merged') return '#3fb950';
  if (b.originGone || b.state === 'gone') return '#8b949e';
  return b.hemisphere === 'lower' ? '#bc8cff' : '#58a6ff';
}

export function matchesFilter(b) {
  const q = (state.pathLens || '').trim().toLowerCase();
  if (q) {
    const nameHit = (b.name || '').toLowerCase().includes(q);
    const fileHit = (b.changedFiles || []).some((f) => f.toLowerCase().includes(q));
    if (!nameHit && !fileHit) return false;
  }
  switch (state.filter) {
    case 'mine':
      return Boolean(b.state === 'open' && !isMergedOrGone(b) && isMyBranch(b, state.snapshot?.repo?.currentUser));
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

export function renderKPIs(onFilterChange = null) {
  const branches = state.snapshot?.branches || [];
  const localBranches = branches.filter((b) => b.hasLocal && !b.id.startsWith('gh-merged-') && b.id !== 'local-wip-working-tree');
  const localCount = localBranches.length;
  const mergedPRCount = branches.filter((b) => b.id.startsWith('gh-merged-') || (b.state === 'merged' && !b.hasLocal)).length;
  const dirtyCount = state.snapshot?.repo?.dirtyCount || branches.find((b) => b.isHead)?.uncommittedCount || 0;

  const upperCount = branches.filter(isOpenPR).length;
  const lowerCount = branches.filter(isLocalOpen).length;
  const mineCount = branches.filter((b) => b.state === 'open' && !isMergedOrGone(b) && isMyBranch(b, state.snapshot?.repo?.currentUser)).length;
  const conflictCount = branches.filter((b) => b.state === 'open' && !isMergedOrGone(b) && ((b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length > 0))).length;
  const failCount = branches.filter((b) => b.state === 'open' && b.pr?.ci?.state === 'FAILURE').length;
  const goneCount = branches.filter(isMergedOrGone).length;

  const allTitle = `Tümü (${branches.length} düğüm):\n• ${localCount} yerel dal (klasik 'git branch' sayısı)\n• ${mergedPRCount} GitHub birleşmiş PR parabolü\n• ${branches.length - localCount - mergedPRCount} diğer uzak/sanal dal.`;

  const defs = [
    { key: 'all', label: 'Tümü', val: branches.length, tone: '', title: allTitle },
    { key: 'mine', label: '👤 Benim Dallarım (@me)', val: mineCount, tone: mineCount ? 'accent' : '', title: 'Giriş yapan kullanıcıya ait açık dallar' },
    { key: 'lower', label: 'Alt Ray (Local)', val: lowerCount, tone: lowerCount ? 'purple' : '', title: 'PR açılmamış açık yerel dallar' },
    { key: 'dirty', label: 'Değişiklik (Dirty)', val: dirtyCount, tone: dirtyCount ? 'warn' : '', title: `Çalışma ağacında ${dirtyCount} adet kaydedilmemiş dosya var.` },
    { key: 'issues', label: 'Çakışma / Cross-PR', val: conflictCount, tone: conflictCount ? 'warn' : '', title: 'Çakışan veya aynı dosyaları değiştiren açık dallar' },
    { key: 'ci_fail', label: 'CI Hatası', val: failCount, tone: failCount ? 'danger' : '', title: 'CI testleri başarısız olan dallar' },
    { key: 'gone', label: 'Birleşmiş / Gone', val: goneCount, tone: goneCount ? 'ok' : '', title: 'Main ile birleşmiş veya uzaktaki dalı silinmiş temizleme adayları' },
  ];

  if (!el.kpiPills) return;
  el.kpiPills.innerHTML = '';
  defs.forEach((d) => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = `kpi-pill ${d.tone} ${state.filter === d.key ? 'active' : ''}`;
    if (d.title) btn.title = d.title;
    btn.innerHTML = `<span>${esc(d.label)}</span><strong>${d.val}</strong>`;
    btn.addEventListener('click', () => {
      state.filter = d.key;
      renderKPIs(onFilterChange);
      if (onFilterChange) onFilterChange();
    });
    el.kpiPills.appendChild(btn);
  });

  const prBadge = document.createElement('span');
  prBadge.className = `kpi-pill ${upperCount ? 'accent' : ''}`;
  prBadge.title = "GitHub üzerinde açık olan Pull Request'ler";
  prBadge.innerHTML = `<span>Üst Ray (PR)</span><strong>${upperCount}</strong>`;
  el.kpiPills.appendChild(prBadge);

  if (el.dragHint) {
    el.dragHint.innerHTML = `💡 <strong>Tümü Sayacı (${branches.length}):</strong> ${localCount} yerel dal + ${mergedPRCount} birleşmiş PR parabolüdür.`;
  }
}

export function renderTopbar(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap) return;

  const liveLabel = state.offlineMode ? 'Offline Simülasyon' : (snap.repo.mode === 'live' ? 'Canlı Git · Aktif' : 'Simülasyon · Etkileşimli');
  if (el.modeBadge) el.modeBadge.textContent = `${snap.repo.name} · ${liveLabel}`;
  if (el.engineStatus) el.engineStatus.textContent = snap.repo.mode === 'live' ? '● Canlı Repo Bağlı' : '● Simülasyon Hazır';

  if (el.repoSelect) {
    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = r.name;
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });
  }

  if (el.btnRemoveProject) el.btnRemoveProject.hidden = snap.repo.mode !== 'live';
  const cleanableCount = (snap.branches || []).filter((b) => b.originGone || b.state === 'gone' || (b.state === 'merged' && b.hasLocal)).length;
  if (el.pruneCount) el.pruneCount.textContent = String(cleanableCount);

  const btnPrune = document.getElementById('btn-prune-gone');
  if (btnPrune) btnPrune.disabled = cleanableCount === 0 && snap.repo.mode !== 'live';
  const btnSync = document.getElementById('btn-sync-spine');
  if (btnSync) btnSync.disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;

  const isProtected = Boolean(snap.repo.branchProtected);
  if (el.btnProtectBranch) {
    el.btnProtectBranch.textContent = isProtected ? '🛡️ main Korumalı' : '🔓 Korumasız';
    el.btnProtectBranch.className = `btn ${isProtected ? 'ok' : 'warn'}`;
  }

  const verTxt = `v${snap.repo.appVersion || '0.8.0'}`;
  const teamTag = snap.repo.teamMode ? ' · TEAM' : '';
  const updTag = snap.repo.updateAvailable ? ' ⬆ Güncelle' : '';
  if (el.btnUpdateCheck) {
    el.btnUpdateCheck.textContent = verTxt + teamTag + updTag;
    el.btnUpdateCheck.className = `btn mono ${snap.repo.updateAvailable ? 'warn' : 'ghost'}`;
  }

  if (el.perfCacheBadge) el.perfCacheBadge.textContent = `⚡ ${snap.repo.cacheHits || 0} SHA hit`;

  const ahead = snap.repo.ahead || 0;
  const behind = snap.repo.behind || 0;
  const remoteName = snap.repo.remoteName || 'origin';
  const defBranch = snap.repo.defaultBranch || 'main';
  if (el.spineSyncBadge) {
    const isSynced = ahead === 0 && behind === 0;
    const txt = isSynced
      ? `${defBranch} ↔ ${remoteName}/${defBranch} (Synced)`
      : `${defBranch} ↔ ${remoteName}/${defBranch}  ↑${ahead} · ↓${behind}`;
    if (el.canvasSyncText) {
      el.canvasSyncText.textContent = txt;
    } else {
      el.spineSyncBadge.textContent = txt;
    }
    if (el.canvasSyncDot) {
      el.canvasSyncDot.className = `sync-dot ${isSynced ? 'synced' : 'pending'}`;
    }
    if (isSynced) {
      el.spineSyncBadge.classList.remove('skewed');
      el.spineSyncBadge.title = `${defBranch} ile ${remoteName}/${defBranch} eşitlendi · Eşitlemek için tıklayın (git fetch & fast-forward)`;
    } else {
      el.spineSyncBadge.classList.add('skewed');
      el.spineSyncBadge.title = `Fark var (↑${ahead} / ↓${behind}) · Şimdi senkronize etmek için tıklayın!`;
    }
  }
}

export function capsulePositionForApex(b, apexX, apexY) {
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

export function buildCapsuleForeignObject(b, geom, isSelected, visible, tone, callbacks = {}) {
  const { onSelectBranch, onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenConflictStudio, onOpenDiff } = callbacks;
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
    const authorTxt = b.author ? `<span class="cap-merged-author" title="Geliştirici: @${esc(b.author)}">@${esc(truncate(b.author, 8))}</span>` : '';
    const localTagHtml = hasLocalLeftover
      ? `<span class="cap-merged-local-tag" title="Yerelde dal kopyası duruyor">${b.isHead ? 'HEAD·Local' : '💻 Local'}</span>`
      : '';
    const diffBtnHtml = '<button type="button" class="cap-merged-del cap-btn-diff" data-act="diff" title="Birleşen Kod Değişikliklerini Gör (Diff)">🔍</button>';
    const delBtnHtml = hasLocalLeftover
      ? '<button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>'
      : '';
    pill.innerHTML = `
      <span class="cap-merged-txt">✓ ${esc(prTxt + truncate(b.name, hasLocalLeftover ? 10 : 16))}</span>
      ${authorTxt}
      ${localTagHtml}
      ${diffBtnHtml}
      ${delBtnHtml}
    `;
    pill.querySelector('button[data-act="diff"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
    pill.querySelector('button[data-act="diff"]')?.addEventListener('click', (ev) => {
      ev.stopPropagation();
      if (onOpenDiff) onOpenDiff(b.name || (b.pr ? `refs/pull/${b.pr.number}/head` : b.localSha), 'branch');
    });
    pill.querySelector('button[data-act="delete"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
    pill.querySelector('button[data-act="delete"]')?.addEventListener('click', (ev) => {
      ev.stopPropagation();
      if (onDeleteBranch) onDeleteBranch(b.id);
    });
    pill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
    pill.addEventListener('click', (ev) => {
      ev.stopPropagation();
      if (onSelectBranch) onSelectBranch(b.id);
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
  const delTitle = isRemoteOnly ? 'PR Kapat & Uzak Dalı Sil' : 'Dalı Sil';

  let badgeTone = 'sky';
  let badgePulse = false;
  let badgeText = b.pr ? `PR #${b.pr.number} · ${b.pr.reviewState}` : 'Remote Ray';
  const isFreshLocal = isLower && !isWorkingTreeWip && b.aheadMain === 0;
  const isSyncedLocal = isLower && !isWorkingTreeWip && b.aheadMain === 0 && (b.uncommittedCount || 0) === 0;
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
    if (isWorkingTreeWip) {
      badgeText = 'Yerel Taslak · Uncommitted';
    } else if (isSyncedLocal) {
      badgeText = '✓ main ile Senkronize';
    } else if (isFreshLocal) {
      badgeText = '🌱 Yeni Yerel Dal';
    } else {
      badgeText = 'Yarım Parabol · Local';
    }
  }

  const subText = isWorkingTreeWip
    ? `${b.uncommittedCount || 0} dosya bekliyor · İncelemek için tıklayın`
    : (isSyncedLocal
      ? 'main omurgasıyla eşitlendi · Yeni commit veya değişiklik yok'
      : (isFreshLocal
        ? 'main omurgasında taze dal · İlk commitinizi bekliyor'
        : (b.lastCommitMsg ? b.lastCommitMsg : (isLower ? `↑${b.aheadMain || 0} yerel · ${b.uncommittedCount || 0} dirty` : `↑${b.aheadMain || 0} ↓${b.behindMain || 0}`))));

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

  const checkoutTitle = isRemoteOnly ? 'Yerelde İndir & Geç (git switch --track)' : 'Bu dala geç (git switch)';
  const checkoutBtn = (!b.isHead && !isWorkingTreeWip)
    ? `<button type="button" class="cap-btn-ico" data-act="checkout" title="${checkoutTitle}">🎯</button>`
    : '';
  const diffBtn = `<button type="button" class="cap-btn-ico cap-btn-diff" data-act="diff" title="Kod Değişikliklerini Gör (Diff)">🔍</button>`;

  let actionRowHtml = '';
  if (isFailed) {
    actionRowHtml = `
      <button type="button" class="cap-btn danger" data-act="inspect">🔍 Hatayı Gör</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  } else if (hasConflict) {
    actionRowHtml = `
      <button type="button" class="cap-btn amber" data-act="resolve">⚡ Çakışma Stüdyosu</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  } else if (isLower) {
    if (isWorkingTreeWip) {
      if (snap?.pendingProposal) {
        actionRowHtml = `
          <button type="button" class="cap-btn sky running-pulse" data-act="inspect">✨ AI Önerisi</button>
          ${diffBtn}
        `;
      } else {
        actionRowHtml = `
          <button type="button" class="cap-btn sky" data-act="inspect">🚀 Commit &amp; PR</button>
          ${diffBtn}
        `;
      }
    } else {
      const hasOpenPR = Boolean(b.pr && b.pr.number > 0 && b.pr.reviewState !== 'MERGED');
      const isSynced = b.aheadMain === 0 && (b.uncommittedCount || 0) === 0;
      if (isSynced) {
        actionRowHtml = `
          ${b.isHead ? '<button type="button" class="cap-btn sky" data-act="checkout-main">🎯 main\'e Geç</button>' : checkoutBtn}
          ${diffBtn}
          <button type="button" class="cap-btn-del" data-act="delete" title="Kalıntı dalı sil">🧹</button>
        `;
      } else {
        actionRowHtml = `
          <button type="button" class="cap-btn sky" data-act="promote">🚀 Push &amp; PR</button>
          ${hasOpenPR ? `<button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge</button>` : ''}
          ${diffBtn}
          ${checkoutBtn}
          <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
        `;
      }
    }
  } else if (isRunning) {
    actionRowHtml = `
      <button type="button" class="cap-btn amber ${isQueued ? 'running-pulse' : ''}" data-act="merge">${isQueued ? '⏳ CI Bekleniyor (İptal)' : '⏳ CI Bitince Merge Et'}</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  } else {
    actionRowHtml = `
      <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge (Parabolü Kapat)</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  }

  const authorName = (b.author || '').trim();
  const currentUser = (snap?.repo?.currentUser || '').trim().toLowerCase();
  const isMe = authorName && currentUser && authorName.toLowerCase() === currentUser;
  const authorBadgeHtml = authorName
    ? `<span class="cap-author-badge ${isMe ? 'author-me' : 'author-peer'}" title="${isMe ? 'Bu dal sizin tarafınızdan açıldı (@me)' : `Geliştirici: @${esc(authorName)}`}">${isMe ? '👤 Ben' : `👤 @${esc(truncate(authorName, 10))}`}</span>`
    : '';

  card.innerHTML = `
    <div class="cap-head">
      <span class="cap-title" title="${esc(b.name)}">${isWorkingTreeWip ? '✏️ Çalışma Alanı' : esc(truncate(b.name, 18))}</span>
      ${authorBadgeHtml}
      <span class="cap-badge ${badgeTone} ${badgePulse ? 'running-pulse' : ''}">${badgeText}</span>
    </div>
    <div class="cap-sub" title="${esc(subText)}">${esc(truncate(subText, 32))}</div>
    ${alertsHtml ? `<div class="cap-alerts">${alertsHtml}</div>` : ''}
    <div class="cap-actions">${actionRowHtml}</div>
  `;

  card.querySelectorAll('button[data-act]').forEach((btn) => {
    btn.addEventListener('pointerdown', (ev) => ev.stopPropagation());
    btn.addEventListener('click', (ev) => {
      ev.stopPropagation();
      const act = btn.getAttribute('data-act');
      if (act === 'diff' && onOpenDiff) {
        if (isWorkingTreeWip) onOpenDiff('working-tree', 'working-tree');
        else onOpenDiff(b.name, 'branch');
      }
      else if (act === 'inspect' && onSelectBranch) onSelectBranch(b.id);
      else if (act === 'checkout' && onCheckoutBranch) onCheckoutBranch(b.id, btn);
      else if (act === 'checkout-main' && onCheckoutBranch) onCheckoutBranch('main', btn);
      else if (act === 'merge' && onMergeBranch) onMergeBranch(b.id, btn);
      else if (act === 'promote' && onPromoteBranch) onPromoteBranch(b.id, undefined, undefined, btn);
      else if (act === 'resolve' && onOpenConflictStudio) onOpenConflictStudio(b.id);
      else if (act === 'delete' && onDeleteBranch) onDeleteBranch(b.id, btn);
    });
  });

  card.addEventListener('pointerdown', (ev) => {
    if (ev.target.closest('button')) return;
    state.drag = {
      branchId: b.id,
      startX: ev.clientX,
      startY: ev.clientY,
      currentX: ev.clientX,
      currentY: ev.clientY,
      hasMoved: false,
    };
    try {
      card.setPointerCapture?.(ev.pointerId);
    } catch {}
  });

  card.addEventListener('click', (ev) => {
    if (state.drag?.hasMoved) return;
    ev.stopPropagation();
    if (onSelectBranch) onSelectBranch(b.id);
  });

  fo.appendChild(card);
  return fo;
}

export function renderSVG(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap || !el.svg) return;

  const { onSelectBranch, onSelectNode, onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenConflictStudio, onOpenDiff } = callbacks;

  const nodes = snap.nodes || [];
  const branches = snap.branches || [];
  const visibleBranches = branches.filter((b) => state.showMergedArches || b.state !== 'merged');
  const apexLayout = layoutOpenApexPositions(visibleBranches);
  state.apexLayout = apexLayout;

  const maxSpineIdx = Math.max(nodes.length - 1, ...visibleBranches.map((b) => b.endSpineIndex || 0), 6);
  const maxMergedUpperLane = Math.max(1, ...visibleBranches.filter((b) => b.hemisphere === 'upper' && b.state === 'merged').map((b) => b.lane || 1));
  const maxApexX = Math.max(0, ...Array.from(apexLayout.values()).map((v) => v.apexX));

  // Dinamik dikey sınır hesaplama (ferah ve dengeli tuval):
  const upperBranches = visibleBranches.filter((b) => b.hemisphere === 'upper');
  const lowerBranches = visibleBranches.filter((b) => b.hemisphere === 'lower');

  let calcMinY = MAIN_Y - 80;
  if (upperBranches.length > 0) {
    const highestUpperY = Math.min(...upperBranches.map((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout);
      return geom.apexY;
    }));
    calcMinY = Math.min(calcMinY, highestUpperY - 70);
  } else {
    calcMinY = MAIN_Y - 100;
  }

  let calcMaxY = MAIN_Y + 80;
  if (lowerBranches.length > 0) {
    const lowestLowerY = Math.max(...lowerBranches.map((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout);
      return geom.apexY;
    }));
    calcMaxY = Math.max(calcMaxY, lowestLowerY + 125);
  } else {
    calcMaxY = MAIN_Y + 100;
  }

  const minY = calcMinY - 30;
  const maxY = calcMaxY + 40;
  const sceneW = Math.max(1280, SPINE_START_X + (maxSpineIdx + 2) * SPINE_STEP_X + 140, maxApexX + 200);
  const sceneH = Math.max(340, maxY - minY);

  const viewW = sceneW / state.zoom;
  const viewH = sceneH / state.zoom;
  el.svg.setAttribute('viewBox', `${state.panX} ${minY + state.panY} ${viewW} ${viewH}`);
  if (el.zoomReset) el.zoomReset.textContent = `${Math.round(state.zoom * 100)}%`;

  el.svg.innerHTML = '';

  // 1. SVG <defs>
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

  // 2. Background horizontal dashed guide rails (temiz, metinsiz zarif çizgiler)
  el.svg.appendChild(svgEl('line', { x1: 20, y1: MAIN_Y - LANE_BASE_OFFSET, x2: sceneW - 20, y2: MAIN_Y - LANE_BASE_OFFSET, class: 'svg-guide-line' }));
  el.svg.appendChild(svgEl('line', { x1: 20, y1: MAIN_Y + LANE_BASE_OFFSET, x2: sceneW - 20, y2: MAIN_Y + LANE_BASE_OFFSET, class: 'svg-guide-line' }));

  const upperCount = branches.filter((b) => b.hemisphere === 'upper' && b.state === 'open').length;
  const lowerCount = branches.filter((b) => b.hemisphere === 'lower' && b.state === 'open').length;
  if (el.upperRayCount) el.upperRayCount.textContent = upperCount > 0 ? String(upperCount) : '';
  if (el.lowerRayCount) el.lowerRayCount.textContent = lowerCount > 0 ? String(lowerCount) : '';

  // 3. Parabolic Curves Layer
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
      if (onSelectBranch) onSelectBranch(b.id);
    });
    arcLayer.appendChild(openPath);

    if (b.state === 'merged' && geom.closeD) {
      const closePath = svgEl('path', { d: geom.closeD, class: openCls.join(' ') });
      closePath.addEventListener('click', (ev) => {
        ev.stopPropagation();
        if (onSelectBranch) onSelectBranch(b.id);
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
          if (onSelectBranch) onSelectBranch(b.id);
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
          if (onDeleteBranch) onDeleteBranch(b.id);
        });
        remPill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
        remPill.addEventListener('click', (ev) => {
          ev.stopPropagation();
          if (onSelectBranch) onSelectBranch(b.id);
        });
        remFo.appendChild(remPill);
        capsuleLayer.appendChild(remFo);
      }
    } else {
      arcLayer.appendChild(svgEl('line', {
        x1: geom.apexX,
        y1: geom.apexY,
        x2: geom.apexX + 95,
        y2: geom.apexY,
        class: `svg-open-tail ${tone} ${visible ? '' : 'dimmed'}`,
      }));
    }

    const dot = svgEl('circle', {
      cx: geom.apexX,
      cy: geom.apexY,
      r: isSelected ? 6 : 4.5,
      class: `svg-apex-dot ${tone}`,
    });
    arcLayer.appendChild(dot);

    const fo = buildCapsuleForeignObject(b, geom, isSelected, visible, tone, callbacks);
    capsuleLayer.appendChild(fo);
  });

  el.svg.appendChild(arcLayer);

  // 4. Central Spine Rail Line
  const spineLineX1 = 40;
  const spineLineX2 = sceneW - 60;
  el.svg.appendChild(svgEl('line', {
    x1: spineLineX1,
    y1: MAIN_Y,
    x2: spineLineX2,
    y2: MAIN_Y,
    stroke: 'url(#main-rail-grad)',
    filter: 'url(#glow-green)',
    class: 'svg-spine-line',
  }));

  const defBranch = snap.repo?.defaultBranch || 'main';
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
      if (onSelectNode) onSelectNode(n.shortSha);
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

export function setupCanvasInteractions(onRenderSVG = null) {
  if (!el.svg) return;

  el.svg.addEventListener('pointerdown', (ev) => {
    if (ev.target.closest('.fo-capsule') || ev.target.closest('circle') || ev.target.closest('path')) return;
    state.panning = {
      startX: ev.clientX,
      startY: ev.clientY,
      initialPanX: state.panX,
      initialPanY: state.panY,
    };
    try {
      el.svg.setPointerCapture?.(ev.pointerId);
    } catch {}
  });

  window.addEventListener('pointermove', (ev) => {
    if (state.panning) {
      const dx = (ev.clientX - state.panning.startX) / state.zoom;
      const dy = (ev.clientY - state.panning.startY) / state.zoom;
      state.panX = state.panning.initialPanX - dx;
      state.panY = state.panning.initialPanY - dy;
      if (onRenderSVG) onRenderSVG();
      return;
    }

    if (state.drag) {
      const dx = ev.clientX - state.drag.startX;
      const dy = ev.clientY - state.drag.startY;
      if (Math.abs(dx) > 4 || Math.abs(dy) > 4) {
        state.drag.hasMoved = true;
      }
    }
  });

  window.addEventListener('pointerup', () => {
    state.panning = null;
    state.drag = null;
  });

  el.svg.addEventListener('wheel', (ev) => {
    ev.preventDefault();
    const factor = ev.deltaY < 0 ? 1.08 : 0.92;
    state.zoom = Math.min(2.5, Math.max(0.4, state.zoom * factor));
    if (onRenderSVG) onRenderSVG();
  }, { passive: false });

  if (el.zoomReset) {
    el.zoomReset.addEventListener('click', () => {
      state.zoom = 1;
      state.panX = 0;
      state.panY = 0;
      if (onRenderSVG) onRenderSVG();
    });
  }

  const btnZoomIn = document.getElementById('zoom-in');
  if (btnZoomIn) {
    btnZoomIn.addEventListener('click', () => {
      state.zoom = Math.min(2.5, state.zoom * 1.15);
      if (onRenderSVG) onRenderSVG();
    });
  }

  const btnZoomOut = document.getElementById('zoom-out');
  if (btnZoomOut) {
    btnZoomOut.addEventListener('click', () => {
      state.zoom = Math.max(0.4, state.zoom * 0.85);
      if (onRenderSVG) onRenderSVG();
    });
  }

  if (el.toggleMerged) {
    el.toggleMerged.addEventListener('change', (ev) => {
      state.showMergedArches = ev.target.checked;
      if (onRenderSVG) onRenderSVG();
    });
  }

  // Upper & Lower Ray Interactive Track Focus & Filter
  el.btnUpperRay?.addEventListener('click', (ev) => {
    ev.stopPropagation();
    state.filter = (state.filter === 'upper' ? 'all' : 'upper');
    renderKPIs();
    if (onRenderSVG) onRenderSVG();
  });

  el.btnLowerRay?.addEventListener('click', (ev) => {
    ev.stopPropagation();
    state.filter = (state.filter === 'lower' ? 'all' : 'lower');
    renderKPIs();
    if (onRenderSVG) onRenderSVG();
  });

  // Clicking empty canvas deselects active branch and closes canvas inspector
  el.svg.addEventListener('click', (ev) => {
    const target = ev.target;
    if (target === el.svg || target.id === 'rail-svg' || target.classList?.contains('svg-guide-line') || target.classList?.contains('svg-spine-line') || target.classList?.contains('dot-grid-canvas')) {
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      if (el.canvasInspector) el.canvasInspector.hidden = true;
      if (onRenderSVG) onRenderSVG();
    }
  });
}

export function renderRayPopovers(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap) return;

  const upperCount = (snap.branches || []).filter((b) => b.hemisphere === 'upper' && b.state === 'open').length;
  const lowerBranches = (snap.branches || []).filter((b) => b.hemisphere === 'lower' && b.state === 'open');
  const dirtyCount = snap.repo?.dirtyCount || 0;
  const lowerTotal = lowerBranches.length + (dirtyCount > 0 ? 1 : 0);

  if (el.upperRayCount) el.upperRayCount.textContent = upperCount > 0 ? String(upperCount) : '';
  if (el.lowerRayCount) el.lowerRayCount.textContent = lowerTotal > 0 ? String(lowerTotal) : '';
}



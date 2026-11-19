// web/modules/capsule.js - Dal kapsülleri: birleşmiş "hap" ve açık dal kartı
//
// Hangi rozetin/butonun/metnin gösterileceği kararı DOM'suz modellerde (`mergedPillModel`, `openCardModel` ve
// yardımcıları) verilir ve Node'da test edilir; `buildCapsuleForeignObject` bunları DOM'a yazar ve olay
// dinleyicilerini bağlar. HTML şablonlarının boşlukları bilerek korunmuştur (satır içi öğeler arasındaki
// boşluk düzeni etkiler).
import { state } from './state.js';
import { XHTML_NS, FO_W, FO_H, MERGED_FO_W, MERGED_FO_H } from './constants.js';
import { esc, truncate, svgEl } from './utils.js';

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

// ---------------------------------------------------------------------------
// Birleşmiş dal "hap"ı
// ---------------------------------------------------------------------------

export function mergedPillModel(b, isSelected) {
  const hasLocalLeftover = Boolean(b.originGone || b.hasLocal);
  const className = `cap-merged-pill ${isSelected ? 'selected' : ''} ${hasLocalLeftover ? 'gone' : ''}`;
  const prTxt = b.pr ? `#${b.pr.number} ` : '';
  const authorTxt = b.author ? `<span class="cap-merged-author" title="Geliştirici: @${esc(b.author)}">@${esc(truncate(b.author, 8))}</span>` : '';
  const localTagHtml = hasLocalLeftover
    ? `<span class="cap-merged-local-tag" title="Yerelde dal kopyası duruyor">${b.isHead ? 'HEAD·Local' : '💻 Local'}</span>`
    : '';
  const diffBtnHtml = '<button type="button" class="cap-merged-del cap-btn-diff" data-act="diff" title="Birleşen Kod Değişikliklerini Gör (Diff)">🔍</button>';
  const delBtnHtml = hasLocalLeftover
    ? '<button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>'
    : '';
  const html = `
      <span class="cap-merged-txt">✓ ${esc(prTxt + truncate(b.name, hasLocalLeftover ? 10 : 16))}</span>
      ${authorTxt}
      ${localTagHtml}
      ${diffBtnHtml}
      ${delBtnHtml}
    `;
  return { className, html, hasLocalLeftover };
}

// ---------------------------------------------------------------------------
// Açık dal kartı: kararlar
// ---------------------------------------------------------------------------

// Kartın kararlarını etkileyen tüm dal durumları tek yerde.
export function branchFlags(b) {
  const isLower = b.hemisphere === 'lower';
  const isWorkingTreeWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-') || b.name === 'wip/working-tree';
  return {
    isLower,
    isWorkingTreeWip,
    isFailed: b.pr?.ci?.state === 'FAILURE',
    isRunning: b.pr?.ci?.state === 'RUNNING',
    hasConflict: Boolean(b.conflict?.hasConflict && !b.conflict.resolved),
    isRemoteOnly: !b.hasLocal && b.hasRemote && b.state === 'open',
    isFreshLocal: isLower && !isWorkingTreeWip && b.aheadMain === 0,
    isSyncedLocal: isLower && !isWorkingTreeWip && b.aheadMain === 0 && (b.uncommittedCount || 0) === 0,
  };
}

// Rozet: renk tonu, nabız animasyonu ve metin.
export function badgeModel(b) {
  const f = branchFlags(b);
  let tone = 'sky';
  let pulse = false;
  let text = b.pr ? `PR #${b.pr.number} · ${b.pr.reviewState}` : 'Remote Ray';
  if (f.isFailed) {
    tone = 'danger';
    text = `PR #${b.pr.number} · CI Failed`;
  } else if (f.hasConflict) {
    tone = 'warn';
    text = b.pr ? `PR #${b.pr.number} · Çakışma` : 'Merge Çakışması';
  } else if (f.isRunning) {
    tone = 'warn';
    pulse = true;
    text = `PR #${b.pr.number} · CI Çalışıyor`;
  } else if (b.originGone || b.state === 'gone') {
    tone = 'warn';
    text = 'origin: gone';
  } else if (f.isLower) {
    tone = 'purple';
    if (f.isWorkingTreeWip) {
      text = 'Yerel Taslak · Uncommitted';
    } else if (f.isSyncedLocal) {
      text = '✓ main ile Senkronize';
    } else if (f.isFreshLocal) {
      text = '🌱 Yeni Yerel Dal';
    } else {
      text = 'Yarım Parabol · Local';
    }
  }
  return { tone, pulse, text };
}

export function subTextOf(b) {
  const f = branchFlags(b);
  return f.isWorkingTreeWip
    ? `${b.uncommittedCount || 0} dosya bekliyor · İncelemek için tıklayın`
    : (f.isSyncedLocal
      ? 'main omurgasıyla eşitlendi · Yeni commit veya değişiklik yok'
      : (f.isFreshLocal
        ? 'main omurgasında taze dal · İlk commitinizi bekliyor'
        : (b.lastCommitMsg ? b.lastCommitMsg : (f.isLower ? `↑${b.aheadMain || 0} yerel · ${b.uncommittedCount || 0} dirty` : `↑${b.aheadMain || 0} ↓${b.behindMain || 0}`))));
}

export function alertsHtmlOf(b) {
  const f = branchFlags(b);
  let html = '';
  if (f.isRemoteOnly) {
    html += '<span class="cap-alert-pill cross">☁ Sadece Origin</span>';
  }
  if (f.hasConflict) {
    html += `<span class="cap-alert-pill conflict">${b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lock' : '⚠ Conflict'}</span>`;
  }
  if (b.crossPr?.length) {
    html += `<span class="cap-alert-pill cross">⇄ Cross-PR ${esc(b.crossPr.map((c) => '#' + (c.otherPrNumber || '?')).join(','))}</span>`;
  }
  return html;
}

export function authorBadgeHtmlOf(b, currentUserRaw) {
  const authorName = (b.author || '').trim();
  const currentUser = (currentUserRaw || '').trim().toLowerCase();
  const isMe = authorName && currentUser && authorName.toLowerCase() === currentUser;
  return authorName
    ? `<span class="cap-author-badge ${isMe ? 'author-me' : 'author-peer'}" title="${isMe ? 'Bu dal sizin tarafınızdan açıldı (@me)' : `Geliştirici: @${esc(authorName)}`}">${isMe ? '👤 Ben' : `👤 @${esc(truncate(authorName, 10))}`}</span>`
    : '';
}

// Kartın alt satırındaki butonlar. ctx = { isQueued, pendingProposal }
export function actionRowHtmlOf(b, ctx = {}) {
  const f = branchFlags(b);
  const delTitle = f.isRemoteOnly ? 'PR Kapat & Uzak Dalı Sil' : 'Dalı Sil';
  const checkoutTitle = f.isRemoteOnly ? 'Yerelde İndir & Geç (git switch --track)' : 'Bu dala geç (git switch)';
  const checkoutBtn = (!b.isHead && !f.isWorkingTreeWip)
    ? `<button type="button" class="cap-btn-ico" data-act="checkout" title="${checkoutTitle}">🎯</button>`
    : '';
  const diffBtn = `<button type="button" class="cap-btn-ico cap-btn-diff" data-act="diff" title="Kod Değişikliklerini Gör (Diff)">🔍</button>`;

  let actionRowHtml = '';
  if (f.isFailed) {
    actionRowHtml = `
      <button type="button" class="cap-btn danger" data-act="inspect">🔍 Hatayı Gör</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  } else if (f.hasConflict) {
    actionRowHtml = `
      <button type="button" class="cap-btn amber" data-act="resolve">⚡ Çakışma Stüdyosu</button>
      ${diffBtn}
      ${checkoutBtn}
      <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
    `;
  } else if (f.isLower) {
    if (f.isWorkingTreeWip) {
      if (ctx.pendingProposal) {
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
  } else if (f.isRunning) {
    actionRowHtml = `
      <button type="button" class="cap-btn amber ${ctx.isQueued ? 'running-pulse' : ''}" data-act="merge">${ctx.isQueued ? '⏳ CI Bekleniyor (İptal)' : '⏳ CI Bitince Merge Et'}</button>
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
  return actionRowHtml;
}

// Açık dal kartının sınıfı ve HTML'i. ctx = { tone, isSelected, isQueued, pendingProposal, currentUser }
export function openCardModel(b, ctx) {
  const f = branchFlags(b);
  const badge = badgeModel(b);
  const subText = subTextOf(b);
  const alertsHtml = alertsHtmlOf(b);
  const actionRowHtml = actionRowHtmlOf(b, ctx);
  const authorBadgeHtml = authorBadgeHtmlOf(b, ctx.currentUser);
  const className = `cap-card ${ctx.tone} ${ctx.isSelected ? 'selected' : ''}`;
  const html = `
    <div class="cap-head">
      <span class="cap-title" title="${esc(b.name)}">${f.isWorkingTreeWip ? '✏️ Çalışma Alanı' : esc(truncate(b.name, 18))}</span>
      ${authorBadgeHtml}
      <span class="cap-badge ${badge.tone} ${badge.pulse ? 'running-pulse' : ''}">${esc(badge.text)}</span>
    </div>
    <div class="cap-sub" title="${esc(subText)}">${esc(truncate(subText, 32))}</div>
    ${alertsHtml ? `<div class="cap-alerts">${alertsHtml}</div>` : ''}
    <div class="cap-actions">${actionRowHtml}</div>
  `;
  return { className, html, isWorkingTreeWip: f.isWorkingTreeWip };
}

// Kart butonunun (data-act) karşılığı: ilgili geri çağrıyı çalıştırır.
export function dispatchCardAction(act, btn, b, isWorkingTreeWip, cb) {
  const { onSelectBranch, onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenConflictStudio, onOpenDiff } = cb;
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
}

// ---------------------------------------------------------------------------
// DOM yazıcıları
// ---------------------------------------------------------------------------

function buildMergedPill(b, isSelected, callbacks) {
  const { onSelectBranch, onDeleteBranch, onOpenDiff } = callbacks;
  const m = mergedPillModel(b, isSelected);
  const pill = document.createElementNS(XHTML_NS, 'div');
  pill.className = m.className;
  pill.innerHTML = m.html;
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
  return pill;
}

function buildOpenCard(b, ctx, callbacks) {
  const { onSelectBranch } = callbacks;
  const m = openCardModel(b, ctx);
  const card = document.createElementNS(XHTML_NS, 'div');
  card.className = m.className;
  card.innerHTML = m.html;

  card.querySelectorAll('button[data-act]').forEach((btn) => {
    btn.addEventListener('pointerdown', (ev) => ev.stopPropagation());
    btn.addEventListener('click', (ev) => {
      ev.stopPropagation();
      dispatchCardAction(btn.getAttribute('data-act'), btn, b, m.isWorkingTreeWip, callbacks);
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
  return card;
}

export function buildCapsuleForeignObject(b, geom, isSelected, visible, tone, callbacks = {}) {
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
    fo.appendChild(buildMergedPill(b, isSelected, callbacks));
    return fo;
  }

  fo.appendChild(buildOpenCard(b, {
    tone,
    isSelected,
    isQueued: Boolean(state.pendingAutoMerge[b.id]),
    pendingProposal: snap?.pendingProposal,
    currentUser: snap?.repo?.currentUser,
  }, callbacks));
  return fo;
}

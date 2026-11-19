// web/modules/canvas.js - Parabolic SVG Canvas, Topology Geometry and Capsule Renderer
import { state, el } from './state.js';
import { XHTML_NS, MAIN_Y, LANE_BASE_OFFSET } from './constants.js';
import { esc, truncate, svgEl } from './utils.js';
import { filterContext, matchesBranchFilter } from './filters.js';
import { renderKPIs } from './kpi.js';
import { buildCapsuleForeignObject } from './capsule.js';
import {
  xOfSpine, layoutSpinePositions, layoutBranchCrowd, layoutOpenApexPositions,
  computeBranchGeometry, computeView, splitHistory
} from './layout.js';

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

// Tuvaldeki süzme: koşullar filters.js tablosundan gelir (KPI çipleriyle aynı kaynak).
export function matchesFilter(b) {
  return matchesBranchFilter(b, {
    filter: state.filter,
    pathLens: state.pathLens,
    ctx: filterContext(state.snapshot),
  });
}

export function renderSVG(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap || !el.svg) return;

  const { onSelectBranch, onSelectNode, onCheckoutBranch, onMergeBranch, onPromoteBranch, onDeleteBranch, onOpenConflictStudio, onOpenDiff } = callbacks;

  const nodes = snap.nodes || [];
  const branches = snap.branches || [];

  // Tüm birleşmiş dalları göster (kullanıcı tercihi: dallar silinmez, 2 merge noktası kalabalığa göre açılır)
  // historyOnly dallar (merge noktası bilinmeyen) parabol olarak çizilmez; sol raydaki "Geçmiş" çipinde listelenir.
  const { drawn } = splitHistory(branches);
  const visibleBranches = drawn.filter((b) => {
    if (b.state !== 'merged') return true;
    return state.showMergedArches !== false;
  });

  // Dinamik Omurga Yayılımı: Dal kalabalığına göre omurga düğümlerini ve merge mesafesini esnet
  state.spinePositions = layoutSpinePositions(nodes, visibleBranches);

  const branchCrowdLayout = layoutBranchCrowd(visibleBranches);
  state.branchCrowdLayout = branchCrowdLayout;

  const apexLayout = layoutOpenApexPositions(visibleBranches, branchCrowdLayout);
  state.apexLayout = apexLayout;

  const maxSpineIdx = Math.max(nodes.length - 1, ...visibleBranches.map((b) => Math.max(b.startSpineIndex || 0, b.endSpineIndex || 0)), 6);
  const maxMergedUpperLane = Math.max(1, ...visibleBranches.filter((b) => b.hemisphere === 'upper' && b.state === 'merged').map((b) => b.lane || 1));
  const maxApexX = Math.max(0, ...Array.from(apexLayout.values()).map((v) => v.apexX));

  // Dinamik dikey sınır hesaplama (ferah ve kuleyi önleyen tavanlı tuval):
  const upperBranches = visibleBranches.filter((b) => b.hemisphere === 'upper');
  const lowerBranches = visibleBranches.filter((b) => b.hemisphere === 'lower');

  let calcMinY = MAIN_Y - 90;
  if (upperBranches.length > 0) {
    const highestUpperY = Math.min(...upperBranches.map((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout, branchCrowdLayout);
      return geom.apexY;
    }));
    calcMinY = Math.min(calcMinY, highestUpperY - 70);
  }

  let calcMaxY = MAIN_Y + 90;
  if (lowerBranches.length > 0) {
    const lowestLowerY = Math.max(...lowerBranches.map((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout, branchCrowdLayout);
      return geom.apexY;
    }));
    calcMaxY = Math.max(calcMaxY, lowestLowerY + 115);
  }

  const minY = calcMinY - 25;
  const maxY = calcMaxY + 30;
  const lastSpineX = xOfSpine(maxSpineIdx);
  const sceneW = Math.max(1280, lastSpineX + 240, maxApexX + 200);
  const sceneH = Math.max(340, maxY - minY);

  // Depo değişince (veya yakınlaştırma sıfırlanınca) en güncel bölgeye (sağ uç) yaslan.
  const repoId = snap.repo?.id || '';
  if (state.anchoredRepoId !== repoId) {
    state.anchoredRepoId = repoId;
    state.anchorEnd = true;
  }
  const view = computeView({
    sceneW, sceneH, minY,
    zoom: state.zoom,
    panX: state.anchorEnd ? Infinity : state.panX,
    panY: state.panY,
    containerW: el.svg.getBoundingClientRect?.().width || 0,
  });
  state.anchorEnd = false;
  state.panX = view.x;          // kullanıcı kaydırması bu noktadan devam eder
  state.unitsPerPx = view.unitsPerPx;
  el.svg.setAttribute('viewBox', `${view.x} ${view.y} ${view.viewW} ${view.viewH}`);
  if (el.zoomReset) {
    el.zoomReset.textContent = `${Math.round(state.zoom * 100)}%`;
    el.zoomReset.title = view.truncated
      ? 'Tuval ekrandan geniş: okunabilir ölçekte gösteriliyor, kaydırarak gezin. Tıklayınca en güncel bölgeye döner.'
      : 'Yakınlaştırmayı sıfırla';
  }

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
    const geom = computeBranchGeometry(b, null, null, apexLayout, branchCrowdLayout);
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
      const upp = state.unitsPerPx || 1 / Math.max(0.01, state.zoom);
      const dx = (ev.clientX - state.panning.startX) * upp;
      const dy = (ev.clientY - state.panning.startY) * upp;
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
      state.anchorEnd = true;
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

  // "Geçmiş" çipi: aç/kapat, dışarı tıklayınca veya Esc ile kapat
  const histBtn = document.getElementById('btn-history-lane');
  const histPop = document.getElementById('history-popover');
  const setHistoryOpen = (open) => {
    state.historyOpen = Boolean(open);
    if (histPop) histPop.hidden = !state.historyOpen;
    histBtn?.setAttribute('aria-expanded', String(state.historyOpen));
  };
  histBtn?.addEventListener('click', (ev) => { ev.stopPropagation(); setHistoryOpen(!state.historyOpen); });
  document.getElementById('history-close')?.addEventListener('click', () => setHistoryOpen(false));
  document.addEventListener('click', (ev) => {
    if (state.historyOpen && !ev.target.closest?.('#history-lane-wrap')) setHistoryOpen(false);
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && state.historyOpen) setHistoryOpen(false);
  });

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

  renderHistoryCard(callbacks);
}

// Sol raydaki "Geçmiş" çipi: omurgada merge noktası bulunamayan birleşmiş dalları listeler.
export function renderHistoryCard(callbacks = {}) {
  const wrap = document.getElementById('history-lane-wrap');
  const list = document.getElementById('history-list');
  const pop = document.getElementById('history-popover');
  const btn = document.getElementById('btn-history-lane');
  if (!wrap || !list || !pop || !btn) return;

  const { history } = splitHistory(state.snapshot?.branches);
  wrap.hidden = history.length === 0;
  const countEl = document.getElementById('history-count');
  if (countEl) countEl.textContent = history.length > 0 ? String(history.length) : '';
  if (history.length === 0) state.historyOpen = false;

  pop.hidden = !state.historyOpen;
  btn.setAttribute('aria-expanded', String(Boolean(state.historyOpen)));

  list.innerHTML = '';
  history.forEach((b) => {
    const item = document.createElement('div');
    item.className = 'ray-item history-item';
    item.setAttribute('role', 'button');
    item.tabIndex = 0;
    item.dataset.branchId = b.id;
    const pr = b.pr?.number ? `#${b.pr.number} · ` : '';
    const who = b.author || b.pr?.author || '';
    item.innerHTML = `<div><div class="ray-item-title">${esc(truncate(b.name, 34))}</div>`
      + `<div class="ray-item-sub">${esc(pr + (who ? '@' + who : 'birleşmiş'))}</div></div>`;
    const choose = () => {
      state.historyOpen = false;
      pop.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
      if (callbacks.onSelectBranch) callbacks.onSelectBranch(b.id);
    };
    item.addEventListener('click', choose);
    item.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); choose(); }
    });
    list.appendChild(item);
  });
}



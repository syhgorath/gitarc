// web/modules/scene.js - Parabolik SVG sahnesinin çizimi (`renderSVG`)
//
// Yerleşim sayıları layout.js'ten, kapsül HTML'i capsule.js'ten gelir. Burada yalnızca sahne sınırları
// (saf `computeSceneBounds`), çizim sırası ve her katmanın (tanımlar, kılavuz çizgileri, yaylar, omurga,
// düğümler) DOM'a yazılması var.
import { state, el } from './state.js';
import { XHTML_NS, MAIN_Y, LANE_BASE_OFFSET } from './constants.js';
import { esc, truncate, svgEl } from './utils.js';
import { filterContext, matchesBranchFilter } from './filters.js';
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

// ---------------------------------------------------------------------------
// Saf yardımcılar
// ---------------------------------------------------------------------------

// Çizilecek dallar: geçmiş (merge noktası bilinmeyen) dallar çizilmez; birleşmişler anahtar kapalıysa gizlenir.
export function selectVisibleBranches(branches, showMergedArches) {
  const { drawn } = splitHistory(branches);
  return drawn.filter((b) => {
    if (b.state !== 'merged') return true;
    return showMergedArches !== false;
  });
}

// Birleşmiş yaylar altta, açık dallar üstte çizilir (kararlı sıralama).
export function drawOrder(visibleBranches) {
  return [...visibleBranches].sort((a, b) => (a.state === 'merged' ? 0 : 1) - (b.state === 'merged' ? 0 : 1));
}

// Sahne boyutu ve dikey sınırlar. state.spinePositions önceden ayarlanmış olmalıdır (xOfSpine kullanır).
export function computeSceneBounds({ nodes, visibleBranches, apexLayout, branchCrowdLayout }) {
  const maxSpineIdx = Math.max(nodes.length - 1, ...visibleBranches.map((b) => Math.max(b.startSpineIndex || 0, b.endSpineIndex || 0)), 6);
  const maxApexX = Math.max(0, ...Array.from(apexLayout.values()).map((v) => v.apexX));

  // Dinamik dikey sınır hesaplama (ferah ve kuleyi önleyen tavanlı tuval):
  const upperBranches = visibleBranches.filter((b) => b.hemisphere === 'upper');
  const lowerBranches = visibleBranches.filter((b) => b.hemisphere === 'lower');

  let calcMinY = MAIN_Y - 90;
  if (upperBranches.length > 0) {
    const highestUpperY = Math.min(...upperBranches.map((b) => computeBranchGeometry(b, null, null, apexLayout, branchCrowdLayout).apexY));
    calcMinY = Math.min(calcMinY, highestUpperY - 86);
  }

  let calcMaxY = MAIN_Y + 90;
  if (lowerBranches.length > 0) {
    const lowestLowerY = Math.max(...lowerBranches.map((b) => computeBranchGeometry(b, null, null, apexLayout, branchCrowdLayout).apexY));
    calcMaxY = Math.max(calcMaxY, lowestLowerY + 131);
  }

  const minY = calcMinY - 25;
  const maxY = calcMaxY + 30;
  const lastSpineX = xOfSpine(maxSpineIdx);
  const sceneW = Math.max(1280, lastSpineX + 240, maxApexX + 200);
  const sceneH = Math.max(340, maxY - minY);
  return { minY, sceneW, sceneH, maxSpineIdx };
}

// Birleşmiş dalın yerelde kalan kopyası için alt rayda kesikli "kalıntı" yolu.
export function remnantGeometry(b, geom) {
  const mergeX = geom.endX || xOfSpine(b.endSpineIndex || b.startSpineIndex || 0);
  const remY = MAIN_Y + 56 + (((b.endSpineIndex || 0) % 2) * 30);
  const remX = mergeX + 18;
  return { mergeX, remX, remY, d: `M ${mergeX} ${MAIN_Y} C ${mergeX} ${MAIN_Y + 30}, ${remX - 16} ${remY}, ${remX} ${remY}` };
}

// Omurgadaki bir commit düğümünün görünümü (DOM yok).
export function nodeVisual(n, idx, total) {
  const isLatest = idx === total - 1;
  const cls = ['svg-node-circle'];
  if (n.isMerge) cls.push('merge-node');
  if (n.isHead || isLatest) cls.push('head-node');
  return { isLatest, className: cls.join(' '), r: isLatest ? 8.5 : (n.isMerge ? 7 : 5.5) };
}

// ---------------------------------------------------------------------------
// DOM katmanları
// ---------------------------------------------------------------------------

function applyView(view) {
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
}

function renderDefs() {
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
}

// Arka plandaki yatay kesikli kılavuz çizgileri (temiz, metinsiz zarif çizgiler)
function renderGuideLines(sceneW) {
  el.svg.appendChild(svgEl('line', { x1: 20, y1: MAIN_Y - LANE_BASE_OFFSET, x2: sceneW - 20, y2: MAIN_Y - LANE_BASE_OFFSET, class: 'svg-guide-line' }));
  el.svg.appendChild(svgEl('line', { x1: 20, y1: MAIN_Y + LANE_BASE_OFFSET, x2: sceneW - 20, y2: MAIN_Y + LANE_BASE_OFFSET, class: 'svg-guide-line' }));
}

function selectOnClick(node, branchId, onSelectBranch) {
  node.addEventListener('click', (ev) => {
    ev.stopPropagation();
    if (onSelectBranch) onSelectBranch(branchId);
  });
}

// Yerel kalıntı: kesikli yol + "💻 dal-adı 🧹" kapsülü
function renderRemnant(b, geom, { isSelected, visible }, arcLayer, capsuleLayer, callbacks) {
  const { onSelectBranch, onDeleteBranch } = callbacks;
  const rem = remnantGeometry(b, geom);
  const remPath = svgEl('path', {
    d: rem.d,
    class: `svg-local-remnant-path ${isSelected ? 'selected' : ''} ${visible ? '' : 'dimmed'}`,
  });
  selectOnClick(remPath, b.id, onSelectBranch);
  arcLayer.appendChild(remPath);

  const remFo = svgEl('foreignObject', {
    x: rem.remX,
    y: rem.remY - 12,
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

// Her dal için yay(lar), uç noktası ve kapsülü
function renderBranchShapes(b, { apexLayout, branchCrowdLayout }, arcLayer, capsuleLayer, callbacks) {
  const { onSelectBranch } = callbacks;
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
  selectOnClick(openPath, b.id, onSelectBranch);
  arcLayer.appendChild(openPath);

  if (b.state === 'merged' && geom.closeD) {
    const closePath = svgEl('path', { d: geom.closeD, class: openCls.join(' ') });
    selectOnClick(closePath, b.id, onSelectBranch);
    arcLayer.appendChild(closePath);

    if (b.hasLocal || b.originGone) {
      renderRemnant(b, geom, { isSelected, visible }, arcLayer, capsuleLayer, callbacks);
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

  arcLayer.appendChild(svgEl('circle', {
    cx: geom.apexX,
    cy: geom.apexY,
    r: isSelected ? 6 : 4.5,
    class: `svg-apex-dot ${tone}`,
  }));

  capsuleLayer.appendChild(buildCapsuleForeignObject(b, geom, isSelected, visible, tone, callbacks));
}

// Merkezdeki omurga çizgisi ve soldaki dal adı etiketi
function renderSpineRail(sceneW, defBranch) {
  el.svg.appendChild(svgEl('line', {
    x1: 40,
    y1: MAIN_Y,
    x2: sceneW - 60,
    y2: MAIN_Y,
    stroke: 'url(#main-rail-grad)',
    filter: 'url(#glow-green)',
    class: 'svg-spine-line',
  }));

  const mainTagW = Math.max(58, defBranch.length * 8 + 22);
  el.svg.appendChild(svgEl('rect', { x: 22, y: MAIN_Y - 13, width: mainTagW, height: 26, rx: 8, class: 'svg-main-tag-bg' }));
  el.svg.appendChild(svgEl('text', { x: 22 + mainTagW / 2, y: MAIN_Y + 4, class: 'svg-main-tag-txt' }, defBranch));
}

// Omurgadaki commit düğümleri
function renderNodes(nodes, onSelectNode) {
  const nodeLayer = svgEl('g', { id: 'node-layer' });
  nodes.forEach((n, idx) => {
    const cx = xOfSpine(n.spineIndex);
    const v = nodeVisual(n, idx, nodes.length);

    const circle = svgEl('circle', {
      cx,
      cy: MAIN_Y,
      r: v.r,
      class: v.className,
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
      class: `svg-node-sha ${v.isLatest ? 'latest' : ''}`,
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
  return nodeLayer;
}

export function renderSVG(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap || !el.svg) return;

  const nodes = snap.nodes || [];
  const visibleBranches = selectVisibleBranches(snap.branches || [], state.showMergedArches);

  // Dinamik Omurga Yayılımı: Dal kalabalığına göre omurga düğümlerini ve merge mesafesini esnet
  state.spinePositions = layoutSpinePositions(nodes, visibleBranches);

  const branchCrowdLayout = layoutBranchCrowd(visibleBranches);
  state.branchCrowdLayout = branchCrowdLayout;

  const apexLayout = layoutOpenApexPositions(visibleBranches, branchCrowdLayout);
  state.apexLayout = apexLayout;

  const { minY, sceneW, sceneH } = computeSceneBounds({ nodes, visibleBranches, apexLayout, branchCrowdLayout });

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
  applyView(view);

  el.svg.innerHTML = '';

  renderDefs();
  renderGuideLines(sceneW);

  // Parabolic Curves Layer
  const arcLayer = svgEl('g', { id: 'arc-layer' });
  const capsuleLayer = svgEl('g', { id: 'capsule-layer' });
  drawOrder(visibleBranches).forEach((b) => {
    renderBranchShapes(b, { apexLayout, branchCrowdLayout }, arcLayer, capsuleLayer, callbacks);
  });
  el.svg.appendChild(arcLayer);

  renderSpineRail(sceneW, snap.repo?.defaultBranch || 'main');

  el.svg.appendChild(renderNodes(nodes, callbacks.onSelectNode));
  el.svg.appendChild(capsuleLayer);
  el.svg.appendChild(svgEl('path', { id: 'drag-preview-path', class: 'svg-drag-preview', d: '' }));
}

// web/modules/scene.js - Parabolik SVG sahnesinin çizimi (`renderSVG`)
//
// Yerleşim sayıları layout.js'ten, kapsül HTML'i capsule.js'ten gelir. Burada yalnızca sahne sınırları
// (saf `computeSceneBounds`), çizim sırası ve her katmanın (tanımlar, kılavuz çizgileri, yaylar, omurga,
// düğümler) DOM'a yazılması var.
import { state, el } from './state.js';
import { XHTML_NS, MAIN_Y, LANE_BASE_OFFSET, SPINE_STEP_X } from './constants.js';
import { esc, truncate, svgEl } from './utils.js';
import { mainCIBadge, mergeIntentBadge } from './syncsteps.js';
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

// Çalışan kuyruk koşusunda şu an işlenen PR'ın "birleşeceği yer": omurganın ucundaki hayalet düğüm ve ona giden sarı kesik ok.
// Saf geometri (DOM yok). geom: computeBranchGeometry sonucu (apexX/apexY); lastSpineX: omurgadaki son düğümün x'i.
export function queueLandingGeometry(geom, lastSpineX) {
  const landingX = lastSpineX + SPINE_STEP_X * 0.7;
  const sx = geom.apexX;
  const sy = geom.apexY;
  const dir = sy < MAIN_Y ? -1 : 1; // yay omurganın üstünde mi altında mı
  const endY = MAIN_Y + dir * 8;     // hayalet düğümün hemen yanında biter (ok ucu düğüme değer)
  const d = `M ${sx} ${sy} C ${sx + 60} ${sy}, ${landingX} ${sy + (endY - sy) * 0.55}, ${landingX} ${endY}`;
  return { d, landingX, landingY: MAIN_Y };
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
    <marker id="queue-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto">
      <path d="M2 1L8 5L2 9" class="svg-queue-arrow-head"/>
    </marker>
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

// Birleşme niyeti olan HER PR için ("şu an işlenen" kuyruk PR'ı ya da tekil "CI bitince merge et"): yayın tepe noktasından omurganın
// ucuna sarı kesik hedef çizgisi + hayalet düğüm ("geliyor"); birleşince yay yeşile kapanır ("gitti"). Niyeti olmayan sıradan açık
// PR'lara çizgi çekilmez; sırada bekleyen kuyruk PR'ları yalnızca etiket alır.
function renderQueueTarget(snap, visibleBranches, layouts, nodes, arcLayer) {
  if (!nodes.length) return;
  const intents = visibleBranches.filter((b) => b.state === 'open' && matchesFilter(b) && mergeIntentBadge(snap, b, state.pendingAutoMerge)?.tone === 'current');
  if (!intents.length) return;
  const lastX = xOfSpine(nodes[nodes.length - 1].spineIndex);
  let drewLanding = false;
  intents.forEach((cur) => {
    const geom = computeBranchGeometry(cur, null, null, layouts.apexLayout, layouts.branchCrowdLayout);
    const t = queueLandingGeometry(geom, lastX);
    const g = svgEl('g', { class: 'svg-queue-target', 'data-branch-id': cur.id });
    g.appendChild(svgEl('title', {}, `${cur.name} sıradaki merge olarak ${snap.repo?.defaultBranch || 'main'} ucuna birleşecek`));
    g.appendChild(svgEl('path', { d: t.d, class: 'svg-queue-target-path', 'marker-end': 'url(#queue-arrow)' }));
    if (!drewLanding) { // birden çok niyet aynı hayalet düğüme gider
      drewLanding = true;
      g.appendChild(svgEl('circle', { cx: t.landingX, cy: t.landingY, r: 7, class: 'svg-queue-landing' }));
      g.appendChild(svgEl('text', { x: t.landingX, y: t.landingY + 24, class: 'svg-queue-landing-txt' }, 'birleşecek'));
    }
    arcLayer.appendChild(g);
  });
}

// Omurgadaki commit düğümleri
function renderNodes(nodes, onSelectNode, ciBadge = null) {
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

    if (ciBadge && ciBadge.sha === n.shortSha) {
      // Merge sonrası main testleri: yalnızca SHA'sı eşleşen düğümün altında (eski düğümlerde "yeşil" iddia edilmez).
      const bw = Math.round(ciBadge.text.length * 6.4 + 18);
      const g = svgEl('g', { class: `svg-mainci-badge ${ciBadge.tone}`, role: 'button', tabindex: 0, 'aria-label': `${ciBadge.text}: ayrıntı için tıklayın` });
      g.appendChild(svgEl('title', {}, ciBadge.detail || ciBadge.text));
      g.appendChild(svgEl('rect', { x: cx - bw / 2, y: MAIN_Y + 34, width: bw, height: 20, rx: 6, class: 'svg-mainci-bg' }));
      g.appendChild(svgEl('text', { x: cx, y: MAIN_Y + 48, class: 'svg-mainci-txt' }, ciBadge.text));
      const open = (ev) => {
        ev.stopPropagation();
        state.selectedNodeSha = n.shortSha;
        state.selectedBranchId = null;
        if (onSelectNode) onSelectNode(n.shortSha);
      };
      g.addEventListener('click', open);
      g.addEventListener('keydown', (ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); open(ev); } });
      nodeLayer.appendChild(g);
    }

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
  if (snap.repo?.mode === 'none') { // proje yok: tuval boş, karşılama paneli gösterilir
    el.svg.innerHTML = '';
    return;
  }

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
  renderQueueTarget(snap, visibleBranches, { apexLayout, branchCrowdLayout }, nodes, arcLayer);
  el.svg.appendChild(arcLayer);

  renderSpineRail(sceneW, snap.repo?.defaultBranch || 'main');

  el.svg.appendChild(renderNodes(nodes, callbacks.onSelectNode, mainCIBadge(snap)));
  el.svg.appendChild(capsuleLayer);
  el.svg.appendChild(svgEl('path', { id: 'drag-preview-path', class: 'svg-drag-preview', d: '' }));
}

// web/modules/layout.js - Parabolik tuvalin saf yerleşim matematiği (DOM yok, Node'da test edilir)
//
// Omurga konumları, şerit yükseklikleri, yay geometrisi, açık kapsül yerleşimi ve görünür pencere
// (viewBox) hesabı burada. canvas.js yalnızca bunları çizer. Tek bağımlılık: state (omurga konumları
// ve kalabalık haritası) ve constants.
import { state } from './state.js';
import {
  MAIN_Y, SPINE_START_X, SPINE_STEP_X,
  LANE_BASE_OFFSET, LANE_STEP_Y, MERGED_LANE_BASE, MERGED_LANE_STEP,
  OPEN_CAPSULE_MIN_GAP_X
} from './constants.js';

// Bir dalın omurgadaki [başlangıç, bitiş] aralığı. Bitiş en az başlangıç + 1'dir (tek adımlık yay).
// Aralık normalleştirmesi daha önce dört yerde ayrı ayrı yazılmıştı.
export function branchSpan(b) {
  const s = b.startSpineIndex || 0;
  return { s, e: Math.max(s + 1, b.endSpineIndex || 0) };
}

// Omurga i. adımı (i → i+1) için bir aralığın "kalabalık" katkısı: kapsıyorsa 1, ucu değiyorsa 0.5.
function segmentCrowd({ s, e }, i) {
  if (s <= i && e >= i + 1) return 1;
  if (s === i || e === i + 1) return 0.5;
  return 0;
}

// İki aralık (uçları değmek dahil) üst üste biniyor mu?
function spansOverlap(a, b) {
  return Math.max(a.s, b.s) <= Math.min(a.e, b.e);
}

// Şerit yüksekliği tek yerde tanımlı: birleşmiş yaylar ve açık kapsüller için taban + şerit adımı.
export function laneOffset(lane, isMerged = false) {
  const l = Math.max(1, lane || 1);
  return isMerged
    ? MERGED_LANE_BASE + (l - 1) * MERGED_LANE_STEP
    : LANE_BASE_OFFSET + (l - 1) * LANE_STEP_Y;
}

export function xOfSpine(idx) {
  if (state.spinePositions && state.spinePositions[idx] !== undefined) {
    return state.spinePositions[idx];
  }
  if (state.spinePositions && state.spinePositions.length > 0 && idx >= state.spinePositions.length) {
    const lastIdx = state.spinePositions.length - 1;
    return state.spinePositions[lastIdx] + (idx - lastIdx) * SPINE_STEP_X;
  }
  return SPINE_START_X + idx * SPINE_STEP_X;
}

export function layoutSpinePositions(nodes, branches) {
  const maxSpineIdx = Math.max(
    (nodes || []).length - 1,
    ...(branches || []).map((b) => Math.max(b.startSpineIndex || 0, b.endSpineIndex || 0)),
    6
  );

  const positions = new Array(maxSpineIdx + 1);
  positions[0] = SPINE_START_X;

  // Kaç birleşen dal varsa (örneğin 10 adet), önceki ve sonraki nokta arası ona göre genişlesin.
  // Yalnızca birleşmiş yaylar omurgada yer ister (iki merge noktası birbirinden uzaklaşır);
  // açık dalların kapsülleri layoutOpenApexPositions ile ayrı yerleşir, bu yüzden sayılmaz.
  for (let i = 0; i < maxSpineIdx; i++) {
    let crowd = 0;
    (branches || []).forEach((b) => {
      if (b.state !== 'merged') return;
      crowd += segmentCrowd(branchSpan(b), i);
    });

    // Dal kalabalığı arttıkça omurgada önceki ve sonraki düğüm arası mesafe orantılı biçimde açılır
    const extraStep = Math.round(crowd * 85);
    const step = SPINE_STEP_X + extraStep;
    positions[i + 1] = positions[i] + step;
  }

  return positions;
}

export function layoutBranchCrowd(branches) {
  const map = new Map();
  const merged = (branches || []).filter((b) => b.state === 'merged');

  merged.forEach((b) => {
    const span = branchSpan(b);
    const overlapping = merged.filter((other) => spansOverlap(span, branchSpan(other)));
    overlapping.sort((a, b) => (a.lane || 1) - (b.lane || 1) || (a.id || '').localeCompare(b.id || ''));
    const rank = Math.max(0, overlapping.findIndex((o) => o.id === b.id));
    map.set(b.id, { crowd: overlapping.length, rank });
  });

  return map;
}

// Okunabilirlik tabanı: 1 sahne birimi en az bu kadar piksel olmalı. Sahne bundan daha
// genişse tamamı ekrana sığdırılmaz (metinler okunmaz hale gelir); bunun yerine bu
// ölçekte çizilir ve en güncel bölge (sağ uç) gösterilir, geri kalanı kaydırılarak gezilir.
export const MIN_READABLE_SCALE = 0.5;

// Saf fonksiyon: görünür SVG penceresini (viewBox) hesaplar.
//  - zoom/pan kullanıcı durumudur; zoom=1 "okunabilir taban" demektir.
//  - panX = Infinity en sağ (güncel) bölgeye yaslar.
//  - pan sahnenin dışına taşamaz.
export function computeView({ sceneW, sceneH, minY, zoom = 1, panX = 0, panY = 0, containerW = 0, minScale = MIN_READABLE_SCALE }) {
  const cw = containerW > 0 ? containerW : sceneW; // genişlik bilinmiyorsa eski davranış
  const fitScale = cw / sceneW;
  const baseZoom = fitScale < minScale ? minScale / fitScale : 1;
  const effZoom = Math.max(0.01, zoom) * baseZoom;
  const viewW = sceneW / effZoom;
  const viewH = sceneH / effZoom;
  const maxPanX = Math.max(0, sceneW - viewW);
  const x = Math.min(maxPanX, Math.max(0, panX));
  return {
    x, y: minY + panY, viewW, viewH,
    baseZoom, truncated: viewW < sceneW - 1,
    unitsPerPx: viewW / cw,
  };
}

// Birleşmiş ama merge noktası omurga penceresinde bulunamayan dallar (historyOnly) gerçek bir
// konuma sahip değildir; parabol olarak çizilmez, "geçmiş" çipinde listelenir.
export function splitHistory(branches) {
  const drawn = [];
  const history = [];
  (branches || []).forEach((b) => (b && b.historyOnly ? history : drawn).push(b));
  return { drawn, history };
}

// Birleşmiş yayların iki ucunun kalabalık yüzünden gerçek merge konumundan en fazla ne kadar
// uzaklaştırılabileceği (sahne birimi). Sınırsız büyürse çizim gerçeği yansıtmaz.
export const MAX_MERGE_SPREAD = 2 * SPINE_STEP_X;

export function yOfLane(hemisphere, lane, isMerged = false) {
  const offset = laneOffset(lane, isMerged);
  return hemisphere === 'lower' ? MAIN_Y + offset : MAIN_Y - offset;
}

export function layoutOpenApexPositions(branches, branchCrowdLayout = null) {
  const map = new Map();
  const mergedBranches = branches.filter((b) => b.state === 'merged');
  const lastMergedEndX = mergedBranches.reduce((acc, b) => {
    const geom = computeBranchGeometry(b, null, null, null, branchCrowdLayout);
    return Math.max(acc, geom.endX || xOfSpine(branchSpan(b).e));
  }, 0);

  ['upper', 'lower'].forEach((hemi) => {
    const openList = branches
      .filter((b) => b.state !== 'merged' && b.hemisphere === hemi)
      .sort((a, b) => a.startSpineIndex - b.startSpineIndex || a.endSpineIndex - b.endSpineIndex || a.name.localeCompare(b.name));

    let prevX = -Infinity;
    openList.forEach((b) => {
      const span = branchSpan(b);
      const startX = xOfSpine(span.s);
      const baseX = Math.max(startX + 150, xOfSpine(span.e) - 10);
      const minClearX = hemi === 'upper' && lastMergedEndX > 0 ? Math.max(baseX, lastMergedEndX + 20) : baseX;
      const apexX = prevX === -Infinity ? minClearX : Math.max(minClearX, prevX + OPEN_CAPSULE_MIN_GAP_X);
      const apexY = hemi === 'lower' ? MAIN_Y + LANE_BASE_OFFSET : MAIN_Y - LANE_BASE_OFFSET;
      prevX = apexX;
      map.set(b.id, { apexX, apexY });
    });
  });
  return map;
}

export function computeBranchGeometry(b, tipXOverride = null, tipYOverride = null, apexLayout = null, branchCrowdLayout = null) {
  const span = branchSpan(b);
  const baseStartX = xOfSpine(span.s);
  const isMerged = b.state === 'merged';

  if (isMerged && tipXOverride === null) {
    const baseEndX = xOfSpine(span.e);

    // Dal kalabalığına göre 2 merge noktasının birbirinden uzaklaşması:
    const lane = Math.max(1, b.lane || 1);
    const crowdInfo = branchCrowdLayout?.get(b.id) || state.branchCrowdLayout?.get(b.id) || { crowd: 1, rank: 0 };
    const spreadStep = Math.min(48, 16 + crowdInfo.crowd * 3);
    const spread = Math.min(MAX_MERGE_SPREAD, (lane - 1) * spreadStep + (crowdInfo.rank * 8));

    const startX = Math.max(SPINE_START_X - 50, baseStartX - spread);
    const endX = baseEndX + spread;
    const apexX = (startX + endX) / 2;

    // Kat kat fırlasın: şerit yüksekliği yOfLane ile tek yerden gelir
    const apexY = yOfLane(b.hemisphere, lane, true);

    // Geniş açılı yatık Bezier gerilimi:
    const cp1X = startX + (apexX - startX) * 0.65;
    const cp2X = apexX + (endX - apexX) * 0.35;
    const openD = `M ${startX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
    const closeD = `M ${apexX} ${apexY} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y}`;
    const areaD = `${openD} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y} Z`;
    return { startX, endX, apexX, apexY, openD, closeD, areaD };
  }

  const laid = apexLayout?.get(b.id);
  const defaultApexX = laid ? laid.apexX : Math.max(baseStartX + 150, xOfSpine(span.e) - 10);
  const defaultApexY = laid ? laid.apexY : yOfLane(b.hemisphere, b.lane, isMerged);
  const apexX = tipXOverride !== null ? tipXOverride : defaultApexX;
  const apexY = tipYOverride !== null ? tipYOverride : defaultApexY;
  const cp1X = baseStartX + (apexX - baseStartX) * 0.55;
  const openD = `M ${baseStartX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
  return { startX: baseStartX, endX: null, apexX, apexY, openD, closeD: null, areaD: null };
}

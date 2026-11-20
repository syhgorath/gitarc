// web/modules/interactions.js - Tuval etkileşimleri: sürükleyerek kaydırma, tekerlek/düğme ile yakınlaştırma,
// ray süzgeçleri, "Birleşmiş Paraboller" anahtarı ve boş alana tıklayınca seçimi kaldırma.
// Sayısal kurallar (yakınlaştırma sınırları, kaydırma hesabı, süzgeç değiştirme) saf fonksiyonlardadır.
import { state, el } from './state.js';
import { renderKPIs } from './kpi.js';
import { setupHistoryChip, setupLongLivedChip, setupBasePicker } from './rails.js';

export const ZOOM_MIN = 0.4;
export const ZOOM_MAX = 2.5;

// Tekerlek: yukarı yakınlaştırır, aşağı uzaklaştırır; her iki yönde sınırlanır.
export function wheelZoom(zoom, deltaY) {
  const factor = deltaY < 0 ? 1.08 : 0.92;
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, zoom * factor));
}
export function zoomInStep(zoom) { return Math.min(ZOOM_MAX, zoom * 1.15); }
export function zoomOutStep(zoom) { return Math.max(ZOOM_MIN, zoom * 0.85); }

// Sürükleme sırasındaki yeni kaydırma konumu. unitsPerPx: 1 piksel kaç sahne birimi (bilinmiyorsa 1/zoom).
export function panFromDrag(panning, clientX, clientY, unitsPerPx, zoom) {
  const upp = unitsPerPx || 1 / Math.max(0.01, zoom);
  return {
    panX: panning.initialPanX - (clientX - panning.startX) * upp,
    panY: panning.initialPanY - (clientY - panning.startY) * upp,
  };
}

// Bir kapsül sürüklemesi 4 pikselden fazla oynadıysa "hareket etti" sayılır (tıklama değil).
export function dragHasMoved(drag, clientX, clientY) {
  return Math.abs(clientX - drag.startX) > 4 || Math.abs(clientY - drag.startY) > 4;
}

// Aynı süzgece tekrar tıklamak süzgeci kapatır (all'a döner).
export function toggleFilterKey(current, key) {
  return current === key ? 'all' : key;
}

// Boş tuval alanına (tuval, kılavuz/omurga çizgisi, nokta ızgarası) tıklanınca seçim kalkar.
export function isEmptyCanvasTarget(target, svgEl) {
  return Boolean(target === svgEl || target.id === 'rail-svg' || target.classList?.contains('svg-guide-line') || target.classList?.contains('svg-spine-line') || target.classList?.contains('dot-grid-canvas'));
}

// Tuval kaydırmasını (ve işaretçi yakalamayı) BAŞLATMAMASI gereken etkileşimli öğeler. Yakalama başlarsa tarayıcı `click`'i
// öğeden alıp svg'ye yönlendirir ve öğenin kendi click işleyicisi hiç çalışmaz (gerçek fareyle tıklanamayan rozet hatası).
export function isPanExemptTarget(target) {
  return Boolean(target?.closest && (target.closest('.fo-capsule') || target.closest('circle') || target.closest('path') || target.closest('.svg-mainci-badge')));
}

function setupPanAndZoom(onRenderSVG) {
  el.svg.addEventListener('pointerdown', (ev) => {
    if (isPanExemptTarget(ev.target)) return;
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
      const next = panFromDrag(state.panning, ev.clientX, ev.clientY, state.unitsPerPx, state.zoom);
      state.panX = next.panX;
      state.panY = next.panY;
      if (onRenderSVG) onRenderSVG();
      return;
    }

    if (state.drag && dragHasMoved(state.drag, ev.clientX, ev.clientY)) {
      state.drag.hasMoved = true;
    }
  });

  window.addEventListener('pointerup', () => {
    state.panning = null;
    state.drag = null;
  });

  el.svg.addEventListener('wheel', (ev) => {
    ev.preventDefault();
    state.zoom = wheelZoom(state.zoom, ev.deltaY);
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
      state.zoom = zoomInStep(state.zoom);
      if (onRenderSVG) onRenderSVG();
    });
  }

  const btnZoomOut = document.getElementById('zoom-out');
  if (btnZoomOut) {
    btnZoomOut.addEventListener('click', () => {
      state.zoom = zoomOutStep(state.zoom);
      if (onRenderSVG) onRenderSVG();
    });
  }
}

function setupFilters(onRenderSVG) {
  if (el.toggleMerged) {
    el.toggleMerged.addEventListener('change', (ev) => {
      state.showMergedArches = ev.target.checked;
      if (onRenderSVG) onRenderSVG();
    });
  }

  // Upper & Lower Ray Interactive Track Focus & Filter
  el.btnUpperRay?.addEventListener('click', (ev) => {
    ev.stopPropagation();
    state.filter = toggleFilterKey(state.filter, 'upper');
    renderKPIs();
    if (onRenderSVG) onRenderSVG();
  });

  el.btnLowerRay?.addEventListener('click', (ev) => {
    ev.stopPropagation();
    state.filter = toggleFilterKey(state.filter, 'lower');
    renderKPIs();
    if (onRenderSVG) onRenderSVG();
  });
}

export function setupCanvasInteractions(onRenderSVG = null) {
  if (!el.svg) return;

  setupPanAndZoom(onRenderSVG);
  setupHistoryChip();
  setupLongLivedChip();
  setupBasePicker();
  setupFilters(onRenderSVG);

  // Clicking empty canvas deselects active branch and closes canvas inspector
  el.svg.addEventListener('click', (ev) => {
    if (isEmptyCanvasTarget(ev.target, el.svg)) {
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      if (el.canvasInspector) el.canvasInspector.hidden = true;
      if (onRenderSVG) onRenderSVG();
    }
  });
}

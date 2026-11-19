// web/modules/trace.js - "Uzağa gidiyor" izi: yerel (alt ray) bir dal pushlanıp PR olunca, eski konumundan yeni üst ray
// konumuna kısa bir kesik çizgi + giden nokta çizilir. Saf kısımlar (`detectPushes`, `tracePathD`) DOM'suz test edilir.
//
// Sezgi: iki anlık görüntü arasında aynı adlı dal alt rayda açıkken üst rayda açık hale geldiyse (push + PR) ya da
// kaydedilmemiş "çalışma alanı" taslağı yerine yeni bir üst ray dalı belirdiyse bir push olmuştur. Terminalden yapılan
// push da (yoklama ile algılanınca) aynı izi çizer.
import { state, el } from './state.js';
import { svgEl } from './utils.js';

export const WIP_ID = 'local-wip-working-tree';
export const TRACE_MS = 1700;

const isOpen = (b) => b && b.state === 'open';

// -> [{ name, fromId, toId, kind: 'promote' | 'wip' }]
export function detectPushes(prev, next) {
  if (!prev || !next) return [];
  if ((prev.repo?.id || '') !== (next.repo?.id || '')) return []; // depo değişimi bir push değildir
  const prevBranches = prev.branches || [];
  const nextBranches = next.branches || [];
  const prevByName = new Map(prevBranches.map((b) => [b.name, b]));
  const out = [];

  nextBranches.forEach((n) => {
    if (!isOpen(n) || n.hemisphere !== 'upper') return;
    const p = prevByName.get(n.name);
    if (p) {
      if (isOpen(p) && p.hemisphere === 'lower') out.push({ name: n.name, fromId: p.id, toId: n.id, kind: 'promote' });
      return;
    }
    // Yeni bir üst ray dalı ve önceki görüntüde kaydedilmemiş çalışma alanı vardı: Commit + Push & PR.
    const wip = prevBranches.find((b) => b.id === WIP_ID);
    if (wip) out.push({ name: n.name, fromId: wip.id, toId: n.id, kind: 'wip' });
  });

  // Aynı anda birden çok "wip" izi çıkmasın (tek taslak tek dala dönüşür).
  const seenWip = new Set();
  return out.filter((t) => (t.kind !== 'wip' ? true : (seenWip.has(t.fromId) ? false : seenWip.add(t.fromId))));
}

// İki nokta arasında omurgayı çaprazlayan yumuşak S eğrisi.
export function tracePathD(from, to) {
  const x1 = from.x;
  const y1 = from.y;
  const x2 = to.x;
  const y2 = to.y;
  const midY = (y1 + y2) / 2;
  const bend = Math.min(60, Math.abs(x2 - x1) * 0.5 + 24);
  return `M ${round(x1)} ${round(y1)} C ${round(x1 + bend)} ${round(midY)}, ${round(x2 - bend)} ${round(midY)}, ${round(x2)} ${round(y2)}`;
}

function round(n) {
  return Math.round(n * 10) / 10;
}

// Koordinatlar: yerleşim haritasından (id -> {apexX, apexY}) gelir.
export function traceEndpoints(trace, prevApex, nextApex) {
  const a = prevApex?.get?.(trace.fromId);
  const b = nextApex?.get?.(trace.toId);
  if (!a || !b) return null;
  return { from: { x: a.apexX, y: a.apexY }, to: { x: b.apexX, y: b.apexY } };
}

// DOM: izi sahnenin üstüne çizer, kısa süre sonra kaldırır.
export function playPushTraces(traces, prevApex, nextApex) {
  if (!el.svg || !traces?.length) return 0;
  let layer = el.svg.querySelector?.('#trace-layer');
  if (layer) layer.remove();
  layer = svgEl('g', { id: 'trace-layer', 'aria-hidden': 'true' });
  let drawn = 0;
  traces.forEach((t) => {
    const ends = traceEndpoints(t, prevApex, nextApex);
    if (!ends) return;
    const d = tracePathD(ends.from, ends.to);
    layer.appendChild(svgEl('path', { class: 'svg-push-trace', d }));
    const dot = svgEl('circle', { class: 'svg-push-dot', r: '5' });
    dot.appendChild(svgEl('animateMotion', { dur: '1.1s', path: d, fill: 'freeze', calcMode: 'spline', keyTimes: '0;1', keySplines: '0.4 0 0.2 1' }));
    layer.appendChild(dot);
    drawn++;
  });
  if (!drawn) return 0;
  el.svg.appendChild(layer);
  setTimeout(() => layer.remove(), TRACE_MS);
  return drawn;
}

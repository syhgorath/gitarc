// Tests for the pure rules in web/modules/interactions.js and web/modules/rails.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  ZOOM_MIN, ZOOM_MAX, wheelZoom, zoomInStep, zoomOutStep, panFromDrag, dragHasMoved, toggleFilterKey, isEmptyCanvasTarget,
} from '../../web/modules/interactions.js';
import { rayCounts, historyItemModel } from '../../web/modules/rails.js';

// ---------- zoom ----------
test('wheelZoom zooms in on scroll up, out on scroll down and never leaves [ZOOM_MIN, ZOOM_MAX]', () => {
  assert.ok(wheelZoom(1, -1) > 1);
  assert.ok(wheelZoom(1, 1) < 1);
  let z = 1;
  for (let i = 0; i < 100; i++) z = wheelZoom(z, -1);
  assert.equal(z, ZOOM_MAX);
  for (let i = 0; i < 200; i++) z = wheelZoom(z, 1);
  assert.equal(z, ZOOM_MIN);
});

test('zoom buttons step by 15% and respect only their own limit', () => {
  assert.ok(Math.abs(zoomInStep(1) - 1.15) < 1e-12);
  assert.equal(zoomInStep(2.4), ZOOM_MAX);
  assert.ok(Math.abs(zoomOutStep(1) - 0.85) < 1e-12);
  assert.equal(zoomOutStep(0.41), ZOOM_MIN);
});

// ---------- pan ----------
test('panFromDrag: dragging right/down moves the view left/up by pixels × units-per-pixel', () => {
  const panning = { startX: 100, startY: 50, initialPanX: 1000, initialPanY: 20 };
  assert.deepEqual(panFromDrag(panning, 160, 90, 2, 1), { panX: 1000 - 120, panY: 20 - 80 });
  assert.deepEqual(panFromDrag(panning, 100, 50, 2, 1), { panX: 1000, panY: 20 }, 'no movement, no change');
});

test('panFromDrag falls back to 1/zoom when units-per-pixel is unknown (first frame)', () => {
  const panning = { startX: 0, startY: 0, initialPanX: 0, initialPanY: 0 };
  assert.deepEqual(panFromDrag(panning, 10, 0, 0, 2), { panX: -5, panY: 0 });
  assert.ok(Number.isFinite(panFromDrag(panning, 10, 10, 0, 0).panX), 'zoom 0 is guarded');
});

test('dragHasMoved: more than 4px on either axis counts as a drag, exactly 4px does not', () => {
  const drag = { startX: 10, startY: 10 };
  assert.equal(dragHasMoved(drag, 14, 10), false);
  assert.equal(dragHasMoved(drag, 15, 10), true);
  assert.equal(dragHasMoved(drag, 10, 5), true);
  assert.equal(dragHasMoved(drag, 6, 14), false);
});

// ---------- filters / deselect ----------
test('toggleFilterKey: same key turns the filter off, another key switches', () => {
  assert.equal(toggleFilterKey('all', 'upper'), 'upper');
  assert.equal(toggleFilterKey('upper', 'upper'), 'all');
  assert.equal(toggleFilterKey('lower', 'upper'), 'upper');
  assert.equal(toggleFilterKey('ci_fail', 'lower'), 'lower');
});

test('isEmptyCanvasTarget: only the canvas itself, guide/spine lines and the dot grid deselect', () => {
  const svg = { id: 'rail-svg' };
  const cls = (...names) => ({ contains: (c) => names.includes(c) });
  assert.equal(isEmptyCanvasTarget(svg, svg), true);
  assert.equal(isEmptyCanvasTarget({ id: 'rail-svg' }, svg), true);
  assert.equal(isEmptyCanvasTarget({ id: 'x', classList: cls('svg-guide-line') }, svg), true);
  assert.equal(isEmptyCanvasTarget({ id: 'x', classList: cls('svg-spine-line') }, svg), true);
  assert.equal(isEmptyCanvasTarget({ id: 'x', classList: cls('dot-grid-canvas') }, svg), true);
  assert.equal(isEmptyCanvasTarget({ id: 'capsule', classList: cls('fo-capsule') }, svg), false);
  assert.equal(isEmptyCanvasTarget({ id: 'p' }, svg), false, 'no classList is fine');
});

// ---------- rails ----------
test('rayCounts: one rule for both the first render and every re-render (regression: Alt Ray lost the dirty +1 after pan/zoom)', () => {
  const branches = [
    { hemisphere: 'upper', state: 'open' }, { hemisphere: 'upper', state: 'open' }, { hemisphere: 'upper', state: 'merged' },
    { hemisphere: 'lower', state: 'open' },
  ];
  assert.deepEqual(rayCounts({ repo: { dirtyCount: 0 }, branches }), { upper: 2, lower: 1 });
  assert.deepEqual(rayCounts({ repo: { dirtyCount: 5 }, branches }), { upper: 2, lower: 2 }, 'a dirty working tree adds one draft to the lower ray');
  assert.deepEqual(rayCounts({ repo: {}, branches: [] }), { upper: 0, lower: 0 });
  assert.deepEqual(rayCounts(null), { upper: 0, lower: 0 });
  assert.deepEqual(rayCounts({ repo: { dirtyCount: 1 }, branches: [] }), { upper: 0, lower: 1 });
});

test('historyItemModel: PR number and author, truncation and fallbacks', () => {
  assert.deepEqual(historyItemModel({ name: 'feat/x', pr: { number: 7, author: 'bob' }, author: 'ziya' }), { title: 'feat/x', sub: '#7 · @ziya' });
  assert.deepEqual(historyItemModel({ name: 'feat/x', pr: { number: 7, author: 'bob' } }), { title: 'feat/x', sub: '#7 · @bob' });
  assert.deepEqual(historyItemModel({ name: 'feat/x' }), { title: 'feat/x', sub: 'birleşmiş' });
  assert.equal(historyItemModel({ name: 'x'.repeat(50) }).title.length, 34);
});

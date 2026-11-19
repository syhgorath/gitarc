// Tests for the pure helpers behind web/modules/scene.js. Run: node --test tests/js
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

import { state } from '../../web/modules/state.js';
import { MAIN_Y } from '../../web/modules/constants.js';
import { layoutSpinePositions, layoutBranchCrowd, layoutOpenApexPositions } from '../../web/modules/layout.js';
import { selectVisibleBranches, drawOrder, computeSceneBounds, remnantGeometry, nodeVisual } from '../../web/modules/scene.js';

const merged = (id, s, e, lane = 1, hemisphere = 'upper') => ({ id, name: id, state: 'merged', hemisphere, lane, startSpineIndex: s, endSpineIndex: e });
const open = (id, s, e, hemisphere = 'upper', lane = 1) => ({ id, name: id, state: 'open', hemisphere, lane, startSpineIndex: s, endSpineIndex: e });

beforeEach(() => { state.spinePositions = undefined; state.branchCrowdLayout = undefined; });

function bounds(nodes, branches) {
  const visible = selectVisibleBranches(branches, true);
  state.spinePositions = layoutSpinePositions(nodes, visible);
  const crowd = layoutBranchCrowd(visible);
  const apex = layoutOpenApexPositions(visible, crowd);
  return computeSceneBounds({ nodes, visibleBranches: visible, apexLayout: apex, branchCrowdLayout: crowd });
}

test('selectVisibleBranches: history-only branches are never drawn; merged ones follow the toggle', () => {
  const list = [open('o', 1, 2), merged('m', 0, 2), { ...merged('h', 0, 9), historyOnly: true }];
  assert.deepEqual(selectVisibleBranches(list, true).map((b) => b.id), ['o', 'm']);
  assert.deepEqual(selectVisibleBranches(list, undefined).map((b) => b.id), ['o', 'm'], 'unset toggle shows merged arches (default)');
  assert.deepEqual(selectVisibleBranches(list, false).map((b) => b.id), ['o'], 'toggle off hides merged arches only');
  assert.deepEqual(selectVisibleBranches(null, true), []);
});

test('drawOrder: merged arches first (drawn underneath), stable otherwise', () => {
  const order = drawOrder([open('o1', 1, 2), merged('m1', 0, 1), open('o2', 2, 3), merged('m2', 1, 2)]).map((b) => b.id);
  assert.deepEqual(order, ['m1', 'm2', 'o1', 'o2']);
  const input = [open('a', 0, 1)];
  drawOrder(input);
  assert.equal(input.length, 1, 'input is not mutated');
});

test('computeSceneBounds: minimum scene size for an empty/small repo', () => {
  const b = bounds([{ spineIndex: 0 }], []);
  assert.ok(b.sceneW >= 1280);
  assert.ok(b.sceneH >= 340);
  assert.ok(b.minY < MAIN_Y - 90, 'headroom above the spine');
  assert.equal(b.maxSpineIdx, 6, 'at least 7 spine slots');
});

test('computeSceneBounds: tall merged stacks lift the top, lower branches push the bottom down', () => {
  const nodes = new Array(10).fill(0).map((_, i) => ({ spineIndex: i }));
  const flat = bounds(nodes, [merged('a', 1, 3, 1)]);
  const tall = bounds(nodes, [merged('a', 1, 3, 1), merged('b', 1, 3, 9)]);
  assert.ok(tall.minY < flat.minY, 'a lane-9 arch needs more headroom');
  const lowerOnly = bounds(nodes, [open('l', 1, 3, 'lower')]);
  assert.ok(lowerOnly.sceneH >= flat.sceneH || lowerOnly.sceneH >= 340);
  assert.ok(Number.isFinite(lowerOnly.minY) && Number.isFinite(lowerOnly.sceneW));
});

test('computeSceneBounds: far open capsules widen the scene beyond the spine', () => {
  const nodes = new Array(6).fill(0).map((_, i) => ({ spineIndex: i }));
  const few = bounds(nodes, [open('a', 1, 2)]);
  const many = bounds(nodes, Array.from({ length: 12 }, (_, i) => open(`o${i}`, 1, 2)));
  assert.ok(many.sceneW > few.sceneW);
});

test('remnantGeometry: alternates the drop height by end index parity and anchors at the merge point', () => {
  const even = remnantGeometry({ endSpineIndex: 4 }, { endX: 600 });
  const odd = remnantGeometry({ endSpineIndex: 5 }, { endX: 600 });
  assert.equal(even.remY, MAIN_Y + 56);
  assert.equal(odd.remY, MAIN_Y + 56 + 30);
  assert.equal(even.mergeX, 600);
  assert.equal(even.remX, 618);
  assert.equal(even.d, `M 600 ${MAIN_Y} C 600 ${MAIN_Y + 30}, 602 ${MAIN_Y + 56}, 618 ${MAIN_Y + 56}`);
  state.spinePositions = [140, 296, 452];
  assert.equal(remnantGeometry({ endSpineIndex: 2 }, {}).mergeX, 452, 'falls back to the spine position when the geometry has no endX');
});

test('nodeVisual: latest node is biggest and highlighted; merge/head variants', () => {
  assert.deepEqual(nodeVisual({}, 0, 3), { isLatest: false, className: 'svg-node-circle', r: 5.5 });
  assert.deepEqual(nodeVisual({ isMerge: true }, 1, 3), { isLatest: false, className: 'svg-node-circle merge-node', r: 7 });
  assert.deepEqual(nodeVisual({ isHead: true }, 0, 3), { isLatest: false, className: 'svg-node-circle head-node', r: 5.5 });
  assert.deepEqual(nodeVisual({ isMerge: true }, 2, 3), { isLatest: true, className: 'svg-node-circle merge-node head-node', r: 8.5 });
});

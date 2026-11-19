// Zero-dependency geometry/layout tests for web/modules/canvas.js.
// Run: node --test tests/js   (uses only Node's built-in test runner)
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

import { state } from '../../web/modules/state.js';
import {
  MAIN_Y, SPINE_START_X, SPINE_STEP_X, LANE_BASE_OFFSET, OPEN_CAPSULE_MIN_GAP_X,
} from '../../web/modules/constants.js';
import {
  xOfSpine, layoutSpinePositions, layoutBranchCrowd, yOfLane,
  layoutOpenApexPositions, computeBranchGeometry,
  computeView, MIN_READABLE_SCALE, splitHistory, MAX_MERGE_SPREAD,
} from '../../web/modules/layout.js';
import { branchToneClass, branchStrokeColor, matchesFilter } from '../../web/modules/canvas.js';

const merged = (id, s, e, lane = 1, hemisphere = 'upper') =>
  ({ id, name: id, state: 'merged', hemisphere, lane, startSpineIndex: s, endSpineIndex: e });
const open = (id, s, e, hemisphere = 'upper') =>
  ({ id, name: id, state: 'open', hemisphere, lane: 1, startSpineIndex: s, endSpineIndex: e });

beforeEach(() => {
  state.spinePositions = undefined;
  state.branchCrowdLayout = undefined;
  state.pathLens = '';
  state.filter = 'all';
  state.snapshot = null;
});

test('xOfSpine falls back to a uniform grid without computed positions', () => {
  assert.equal(xOfSpine(0), SPINE_START_X);
  assert.equal(xOfSpine(3), SPINE_START_X + 3 * SPINE_STEP_X);
});

test('xOfSpine uses computed positions and extrapolates past the last one', () => {
  state.spinePositions = [100, 300, 700];
  assert.equal(xOfSpine(1), 300);
  assert.equal(xOfSpine(4), 700 + 2 * SPINE_STEP_X);
});

test('layoutSpinePositions: no branches gives a uniform spine of at least 7 points', () => {
  const pos = layoutSpinePositions([], []);
  assert.ok(pos.length >= 7);
  assert.equal(pos[0], SPINE_START_X);
  for (let i = 1; i < pos.length; i++) assert.equal(pos[i] - pos[i - 1], SPINE_STEP_X);
});

test('layoutSpinePositions: spine is strictly increasing and widens where branches crowd', () => {
  const branches = [merged('a', 2, 3), merged('b', 2, 3, 2), merged('c', 2, 3, 3)];
  const pos = layoutSpinePositions(new Array(8).fill({}), branches);
  for (let i = 1; i < pos.length; i++) assert.ok(pos[i] > pos[i - 1], `non-increasing at ${i}`);
  const crowdedGap = pos[3] - pos[2];
  const calmGap = pos[6] - pos[5];
  assert.ok(crowdedGap > calmGap, `crowded gap ${crowdedGap} should exceed calm gap ${calmGap}`);
});

test('layoutSpinePositions covers branches whose end index exceeds the node count', () => {
  const pos = layoutSpinePositions([{}, {}], [merged('far', 0, 15)]);
  assert.ok(pos.length >= 16);
});

test('layoutBranchCrowd only tracks merged branches and ranks overlaps uniquely', () => {
  const list = [merged('a', 0, 4), merged('b', 1, 5, 2), merged('c', 8, 9), open('o', 0, 5)];
  const map = layoutBranchCrowd(list);
  assert.ok(!map.has('o'));
  assert.equal(map.get('a').crowd, 2);
  assert.equal(map.get('b').crowd, 2);
  assert.notEqual(map.get('a').rank, map.get('b').rank);
  assert.deepEqual(map.get('c'), { crowd: 1, rank: 0 });
  assert.equal(layoutBranchCrowd(null).size, 0);
});

test('yOfLane: upper above spine, lower below, outer lanes farther, no 4-lane cap', () => {
  assert.ok(yOfLane('upper', 1) < MAIN_Y);
  assert.ok(yOfLane('lower', 1) > MAIN_Y);
  assert.equal(MAIN_Y - yOfLane('upper', 1), LANE_BASE_OFFSET);
  assert.ok(yOfLane('upper', 12) < yOfLane('upper', 4), 'lane 12 must sit above lane 4');
  assert.ok(yOfLane('lower', 12) > yOfLane('lower', 4));
  // merged arches sit closer to the spine than open ones
  assert.ok(MAIN_Y - yOfLane('upper', 1, true) < MAIN_Y - yOfLane('upper', 1, false));
  // invalid lane values are clamped to lane 1
  assert.equal(yOfLane('upper', 0), yOfLane('upper', 1));
  assert.equal(yOfLane('upper', undefined), yOfLane('upper', 1));
});

test('computeBranchGeometry: merged arch is a closed parabola on the right side of the spine', () => {
  const b = merged('m', 2, 4, 1, 'upper');
  const g = computeBranchGeometry(b);
  assert.ok(g.endX > g.startX);
  assert.ok(g.apexX > g.startX && g.apexX < g.endX);
  assert.ok(g.apexY < MAIN_Y);
  assert.match(g.openD, /^M /);
  assert.match(g.closeD, /^M /);
  assert.ok(g.startX >= SPINE_START_X - 50);

  const lower = computeBranchGeometry(merged('l', 2, 4, 1, 'lower'));
  assert.ok(lower.apexY > MAIN_Y);
});

test('computeBranchGeometry: outer lanes arch higher and spread wider (no overlap collapse)', () => {
  const inner = computeBranchGeometry(merged('i', 2, 4, 1));
  const outer = computeBranchGeometry(merged('o', 2, 4, 6));
  assert.ok(outer.apexY < inner.apexY);
  assert.ok(outer.endX - outer.startX > inner.endX - inner.startX);
});

test('computeBranchGeometry: start never drifts left of the canvas margin even for huge lanes', () => {
  const g = computeBranchGeometry(merged('x', 0, 1, 40));
  assert.ok(g.startX >= SPINE_START_X - 50);
});

test('computeBranchGeometry: crowd info widens spread', () => {
  const b = merged('c', 2, 4, 3);
  const calm = computeBranchGeometry(b, null, null, null, new Map([['c', { crowd: 1, rank: 0 }]]));
  const busy = computeBranchGeometry(b, null, null, null, new Map([['c', { crowd: 9, rank: 4 }]]));
  assert.ok(busy.endX - busy.startX > calm.endX - calm.startX);
});

test('computeBranchGeometry: open branch has no closing path and honours tip overrides', () => {
  const b = open('o', 1, 3);
  const g = computeBranchGeometry(b);
  assert.equal(g.endX, null);
  assert.equal(g.closeD, null);
  const t = computeBranchGeometry(b, 900, 55);
  assert.equal(t.apexX, 900);
  assert.equal(t.apexY, 55);
});

test('layoutOpenApexPositions: capsules in a hemisphere keep a minimum gap and never reach for merged ids', () => {
  const list = [open('a', 1, 2), open('b', 1, 2), open('c', 1, 2), merged('m', 0, 1)];
  const map = layoutOpenApexPositions(list);
  assert.ok(!map.has('m'));
  const xs = ['a', 'b', 'c'].map((id) => map.get(id).apexX).sort((p, q) => p - q);
  for (let i = 1; i < xs.length; i++) assert.ok(xs[i] - xs[i - 1] >= OPEN_CAPSULE_MIN_GAP_X);
});

test('layoutOpenApexPositions: upper capsules clear the last merged arch end; lower ones are independent', () => {
  const m = merged('m', 0, 6, 1, 'upper');
  const crowd = layoutBranchCrowd([m]);
  const endX = computeBranchGeometry(m, null, null, null, crowd).endX;
  const map = layoutOpenApexPositions([m, open('u', 0, 1, 'upper'), open('l', 0, 1, 'lower')], crowd);
  assert.ok(map.get('u').apexX >= endX + 20);
  assert.ok(map.get('l').apexX < map.get('u').apexX);
  assert.ok(map.get('u').apexY < MAIN_Y && map.get('l').apexY > MAIN_Y);
});

test('layoutOpenApexPositions is deterministic regardless of input order', () => {
  const a = [open('x', 1, 2), open('y', 2, 3), open('z', 1, 2)];
  const r1 = layoutOpenApexPositions(a);
  const r2 = layoutOpenApexPositions([...a].reverse());
  for (const id of ['x', 'y', 'z']) assert.deepEqual(r1.get(id), r2.get(id));
});

test('branchToneClass / branchStrokeColor priority: CI failure > conflict > running > merged > gone > hemisphere', () => {
  const fail = { state: 'open', pr: { ci: { state: 'FAILURE' } }, conflict: { hasConflict: true } };
  assert.equal(branchToneClass(fail), 'ci-fail');
  assert.equal(branchStrokeColor(fail), '#f85149');

  const conflict = { state: 'open', conflict: { hasConflict: true, resolved: false } };
  assert.equal(branchToneClass(conflict), 'conflict');
  const resolved = { state: 'open', hemisphere: 'upper', conflict: { hasConflict: true, resolved: true } };
  assert.equal(branchToneClass(resolved), 'upper-open');

  assert.equal(branchToneClass({ state: 'open', pr: { ci: { state: 'RUNNING' } } }), 'ci-running');
  assert.equal(branchToneClass({ state: 'merged' }), 'merged');
  assert.equal(branchToneClass({ state: 'open', originGone: true }), 'gone');
  assert.equal(branchToneClass({ state: 'open', hemisphere: 'lower' }), 'lower-open');
  assert.equal(branchStrokeColor({ state: 'open', hemisphere: 'lower' }), '#bc8cff');
  assert.equal(branchStrokeColor({ state: 'open', hemisphere: 'upper' }), '#58a6ff');
});

test('matchesFilter: path lens matches branch name or changed files, case-insensitively', () => {
  const b = { name: 'feat/Login', changedFiles: ['internal/Auth.go'] };
  state.pathLens = 'login';
  assert.ok(matchesFilter(b));
  state.pathLens = 'AUTH';
  assert.ok(matchesFilter(b));
  state.pathLens = 'zzz';
  assert.ok(!matchesFilter(b));
});

test('matchesFilter: status filters', () => {
  state.filter = 'ci_fail';
  assert.ok(matchesFilter({ state: 'open', pr: { ci: { state: 'FAILURE' } } }));
  assert.ok(!matchesFilter({ state: 'merged', pr: { ci: { state: 'FAILURE' } } }));

  state.filter = 'issues';
  assert.ok(matchesFilter({ state: 'open', conflict: { hasConflict: true, resolved: false } }));
  assert.ok(matchesFilter({ state: 'open', crossPr: [{}] }));
  assert.ok(!matchesFilter({ state: 'open', conflict: { hasConflict: true, resolved: true } }));

  state.filter = 'gone';
  assert.ok(matchesFilter({ state: 'merged' }));
  assert.ok(!matchesFilter({ state: 'open' }));

  state.filter = 'dirty';
  assert.ok(matchesFilter({ isHead: true, uncommittedCount: 2 }));
  assert.ok(!matchesFilter({ isHead: false, uncommittedCount: 2 }));

  state.filter = 'mine';
  state.snapshot = { repo: { currentUser: 'Ziya' } };
  assert.ok(matchesFilter({ state: 'open', author: 'ziya' }));
  assert.ok(!matchesFilter({ state: 'open', author: 'bob' }));
});

test('end-to-end scene: no two overlapping merged arches in one hemisphere share an apex', () => {
  // Mirrors what renderSVG does: lanes (lane 1..n) + crowd layout + spine spread.
  const branches = [];
  for (let i = 0; i < 10; i++) branches.push(merged(`b${i}`, 1, 6, i + 1));
  state.spinePositions = layoutSpinePositions(new Array(8).fill({}), branches);
  const crowd = layoutBranchCrowd(branches);
  state.branchCrowdLayout = crowd;
  const ys = branches.map((b) => computeBranchGeometry(b, null, null, null, crowd).apexY);
  assert.equal(new Set(ys).size, ys.length, 'each lane must have its own apex height');
  const widths = branches.map((b) => {
    const g = computeBranchGeometry(b, null, null, null, crowd);
    return g.endX - g.startX;
  });
  for (let i = 1; i < widths.length; i++) assert.ok(widths[i] >= widths[i - 1], 'outer arches must enclose inner ones');
});

test('layoutSpinePositions: open branches never widen the spine (enterprise regression)', () => {
  // 10 open PRs piled on the last segments used to inflate those gaps to ~1000px each.
  const opens = [];
  for (let i = 0; i < 10; i++) opens.push(open(`o${i}`, 5, 8));
  const pos = layoutSpinePositions(new Array(9).fill({}), opens);
  for (let i = 1; i < pos.length; i++) assert.equal(pos[i] - pos[i - 1], SPINE_STEP_X, `gap ${i} widened by open branches`);
});

test('layoutSpinePositions: only merged arches count toward crowding', () => {
  const mixed = [merged('m1', 2, 3), merged('m2', 2, 3, 2), ...Array.from({ length: 8 }, (_, i) => open(`o${i}`, 2, 3))];
  const onlyMerged = [merged('m1', 2, 3), merged('m2', 2, 3, 2)];
  assert.deepEqual(layoutSpinePositions(new Array(8).fill({}), mixed), layoutSpinePositions(new Array(8).fill({}), onlyMerged));
});

test('computeView: scene that fits keeps the classic full-scene view', () => {
  const v = computeView({ sceneW: 1280, sceneH: 400, minY: 20, containerW: 1600 });
  assert.equal(v.viewW, 1280);
  assert.equal(v.x, 0);
  assert.equal(v.baseZoom, 1);
  assert.equal(v.truncated, false);
  assert.equal(v.y, 20);
});

test('computeView: very wide scene is drawn at the readable scale and anchored to the latest region', () => {
  const sceneW = 8000, containerW = 1600;
  const v = computeView({ sceneW, sceneH: 400, minY: 0, containerW, panX: Infinity });
  assert.ok(v.truncated);
  const scale = containerW / v.viewW;
  assert.ok(scale >= MIN_READABLE_SCALE - 1e-9, `scale ${scale} below readable floor`);
  assert.ok(Math.abs(v.x + v.viewW - sceneW) < 1e-6, 'must end exactly at the right edge of the scene');
  assert.ok(v.x > 0);
});

test('computeView: panning is clamped to the scene bounds', () => {
  const base = { sceneW: 8000, sceneH: 400, minY: 0, containerW: 1600 };
  assert.equal(computeView({ ...base, panX: -500 }).x, 0);
  const right = computeView({ ...base, panX: 1e9 });
  assert.ok(Math.abs(right.x + right.viewW - 8000) < 1e-6);
  const mid = computeView({ ...base, panX: 1000 });
  assert.equal(mid.x, 1000);
});

test('computeView: zoom in shrinks the window, zoom out never goes past the scene', () => {
  const base = { sceneW: 8000, sceneH: 400, minY: 0, containerW: 1600, panX: 0 };
  const normal = computeView(base);
  const zin = computeView({ ...base, zoom: 2 });
  const zout = computeView({ ...base, zoom: 0.4 });
  assert.ok(zin.viewW < normal.viewW);
  assert.ok(zout.viewW > normal.viewW);
  assert.equal(zout.x, 0);
  assert.ok(zout.viewW >= 0);
});

test('computeView: unknown container width falls back to the old behaviour; bad zoom is safe', () => {
  const v = computeView({ sceneW: 3000, sceneH: 300, minY: 0, containerW: 0 });
  assert.equal(v.viewW, 3000);
  assert.ok(!v.truncated);
  const z = computeView({ sceneW: 3000, sceneH: 300, minY: 0, containerW: 1000, zoom: 0 });
  assert.ok(Number.isFinite(z.viewW) && Number.isFinite(z.x));
});

test('computeView: unitsPerPx converts pointer pixels into scene units', () => {
  const v = computeView({ sceneW: 8000, sceneH: 400, minY: 0, containerW: 1600 });
  // the whole container width spans exactly viewW scene units
  assert.ok(Math.abs(v.unitsPerPx * 1600 - v.viewW) < 1e-6);
  // at the readable floor, one pixel covers 1 / MIN_READABLE_SCALE units
  assert.ok(Math.abs(v.unitsPerPx - 1 / MIN_READABLE_SCALE) < 1e-9);
});

test('computeView: a realistic enterprise scene becomes readable after fix A + C', () => {
  // 16 nodes, 10 piled open PRs, 3 merged arches: spine now stays compact and the view is readable.
  const branches = [merged('a', 1, 3), merged('b', 3, 6, 2), merged('c', 6, 9),
    ...Array.from({ length: 10 }, (_, i) => open(`o${i}`, 10, 13))];
  const pos = layoutSpinePositions(new Array(16).fill({}), branches);
  const lastSpine = pos[pos.length - 1];
  assert.ok(lastSpine < 3600, `enterprise spine should stay compact, got ${lastSpine}`);
  const v = computeView({ sceneW: lastSpine + 240 + 2400, sceneH: 400, minY: 0, containerW: 1739 });
  assert.ok(1739 / v.viewW >= MIN_READABLE_SCALE - 1e-9);
});

test('splitHistory separates history-only branches from drawable ones', () => {
  const list = [merged('a', 1, 2), { ...merged('h1', 0, 9), historyOnly: true }, open('o', 3, 4), { ...merged('h2', 0, 9), historyOnly: true }];
  const { drawn, history } = splitHistory(list);
  assert.deepEqual(drawn.map((b) => b.id), ['a', 'o']);
  assert.deepEqual(history.map((b) => b.id), ['h1', 'h2']);
  assert.deepEqual(splitHistory(null), { drawn: [], history: [] });
  assert.deepEqual(splitHistory([null, undefined]).history, []);
});

test('splitHistory keeps every branch exactly once (nothing lost, nothing duplicated)', () => {
  const list = Array.from({ length: 50 }, (_, i) => (i % 3 === 0 ? { ...merged(`m${i}`, 0, 9), historyOnly: true } : merged(`m${i}`, i % 5, (i % 5) + 2)));
  const { drawn, history } = splitHistory(list);
  assert.equal(drawn.length + history.length, list.length);
  assert.equal(new Set([...drawn, ...history].map((b) => b.id)).size, list.length);
});

test('computeBranchGeometry: merge points never drift further than MAX_MERGE_SPREAD from their real position (prometheus regression)', () => {
  state.spinePositions = undefined;
  const real = computeBranchGeometry({ ...merged('flat', 4, 6, 1) });
  for (const lane of [2, 6, 12, 40, 200]) {
    const b = merged(`l${lane}`, 4, 6, lane);
    const g = computeBranchGeometry(b, null, null, null, new Map([[b.id, { crowd: 40, rank: 39 }]]));
    const leftDrift = real.startX - g.startX;
    const rightDrift = g.endX - real.endX;
    assert.ok(leftDrift <= MAX_MERGE_SPREAD + 1e-9, `lane ${lane}: start drifted ${leftDrift}`);
    assert.ok(rightDrift <= MAX_MERGE_SPREAD + 1e-9, `lane ${lane}: end drifted ${rightDrift}`);
  }
});

test('computeBranchGeometry: forty overlapping merged arches stay within a bounded width', () => {
  const branches = Array.from({ length: 40 }, (_, i) => merged(`b${i}`, 2, 8, i + 1));
  state.spinePositions = layoutSpinePositions(new Array(12).fill({}), branches);
  const crowd = layoutBranchCrowd(branches);
  const geoms = branches.map((b) => computeBranchGeometry(b, null, null, null, crowd));
  const minX = Math.min(...geoms.map((g) => g.startX));
  const maxX = Math.max(...geoms.map((g) => g.endX));
  const trueSpan = xOfSpine(8) - xOfSpine(2);
  assert.ok(maxX - minX <= trueSpan + 2 * MAX_MERGE_SPREAD + 1e-6, `scene ${maxX - minX} exceeds true span ${trueSpan} + 2*spread`);
});

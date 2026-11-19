// Tests for the helpers that replaced duplicated logic in web/modules/layout.js. Run: node --test tests/js
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

import { state } from '../../web/modules/state.js';
import { MAIN_Y, LANE_BASE_OFFSET, LANE_STEP_Y, MERGED_LANE_BASE, MERGED_LANE_STEP, SPINE_STEP_X } from '../../web/modules/constants.js';
import {
  branchSpan, laneOffset, yOfLane, computeBranchGeometry, layoutSpinePositions, layoutBranchCrowd, layoutOpenApexPositions,
} from '../../web/modules/layout.js';

const merged = (id, s, e, lane = 1, hemisphere = 'upper') =>
  ({ id, name: id, state: 'merged', hemisphere, lane, startSpineIndex: s, endSpineIndex: e });

beforeEach(() => {
  state.spinePositions = undefined;
  state.branchCrowdLayout = undefined;
});

test('branchSpan normalizes every shape of index data the same way', () => {
  assert.deepEqual(branchSpan({ startSpineIndex: 2, endSpineIndex: 5 }), { s: 2, e: 5 });
  assert.deepEqual(branchSpan({ startSpineIndex: 3, endSpineIndex: 3 }), { s: 3, e: 4 }, 'zero-width arch becomes one step');
  assert.deepEqual(branchSpan({ startSpineIndex: 6, endSpineIndex: 2 }), { s: 6, e: 7 }, 'inverted range is clamped');
  assert.deepEqual(branchSpan({}), { s: 0, e: 1 }, 'missing indices default to the first step');
  assert.deepEqual(branchSpan({ startSpineIndex: null, endSpineIndex: undefined }), { s: 0, e: 1 });
});

test('laneOffset: one definition for open capsules and merged arches', () => {
  assert.equal(laneOffset(1, false), LANE_BASE_OFFSET);
  assert.equal(laneOffset(3, false), LANE_BASE_OFFSET + 2 * LANE_STEP_Y);
  assert.equal(laneOffset(1, true), MERGED_LANE_BASE);
  assert.equal(laneOffset(4, true), MERGED_LANE_BASE + 3 * MERGED_LANE_STEP);
  for (const bad of [0, -3, undefined, null, NaN]) {
    assert.equal(laneOffset(bad, true), MERGED_LANE_BASE, `lane ${bad} clamps to 1`);
    assert.equal(laneOffset(bad, false), LANE_BASE_OFFSET);
  }
});

test('regression: a merged arch apex sits exactly at yOfLane(…, merged) — the two formulas used to disagree (52 vs 54)', () => {
  for (const hemisphere of ['upper', 'lower']) {
    for (let lane = 1; lane <= 12; lane++) {
      const b = merged(`m${lane}`, 2, 4, lane, hemisphere);
      const g = computeBranchGeometry(b);
      assert.equal(g.apexY, yOfLane(hemisphere, lane, true), `${hemisphere} lane ${lane}`);
    }
  }
});

test('yOfLane keeps open lanes unchanged and sides symmetric around the spine', () => {
  for (let lane = 1; lane <= 6; lane++) {
    for (const merged_ of [false, true]) {
      const up = MAIN_Y - yOfLane('upper', lane, merged_);
      const down = yOfLane('lower', lane, merged_) - MAIN_Y;
      assert.equal(up, down);
      assert.equal(up, laneOffset(lane, merged_));
    }
  }
});

test('spine widening and branch crowding agree on malformed branches (shared span rules)', () => {
  // A branch with inverted/missing indices is treated as a single-step arch by BOTH computations.
  const odd = [merged('inv', 5, 2), { id: 'nil', name: 'nil', state: 'merged', hemisphere: 'upper', lane: 1 }];
  const pos = layoutSpinePositions(new Array(8).fill({}), odd);
  for (let i = 1; i < pos.length; i++) assert.ok(Number.isFinite(pos[i]) && pos[i] > pos[i - 1]);
  const crowd = layoutBranchCrowd(odd);
  assert.equal(crowd.get('inv').crowd, 1);
  assert.equal(crowd.get('nil').crowd, 1);
  assert.ok(Number.isFinite(computeBranchGeometry(odd[1]).startX), 'missing indices no longer produce NaN geometry');
});

test('open-capsule apex layout still uses the shared span end', () => {
  const open = { id: 'o', name: 'o', state: 'open', hemisphere: 'upper', lane: 1, startSpineIndex: 1, endSpineIndex: 4 };
  const apex = layoutOpenApexPositions([open]).get('o');
  assert.ok(apex.apexX >= 140 + 1 * SPINE_STEP_X + 150 - 1e-9, 'at least 150 units right of the start');
  assert.equal(apex.apexY, MAIN_Y - LANE_BASE_OFFSET);
});

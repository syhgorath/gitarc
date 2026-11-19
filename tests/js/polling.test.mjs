// Tests for the background poll decision (web/modules/polling.js). Regression: a PR whose CI was running stayed on
// "Kontroller Devam Ediyor..." forever, because GitHub data only refreshes when a snapshot is requested and polling
// stopped while a branch was selected. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { CI_REFRESH_MS, hasPendingCI, pollDecision } from '../../web/modules/polling.js';

const running = { branches: [{ state: 'open', pr: { ci: { state: 'RUNNING' } } }] };
const done = { branches: [{ state: 'open', pr: { ci: { state: 'SUCCESS' } } }] };

test('hasPendingCI: only open PRs with a running CI count', () => {
  assert.equal(hasPendingCI(running), true);
  assert.equal(hasPendingCI(done), false);
  assert.equal(hasPendingCI({ branches: [{ state: 'merged', pr: { ci: { state: 'RUNNING' } } }] }), false);
  assert.equal(hasPendingCI({ branches: [{ state: 'open' }] }), false);
  assert.equal(hasPendingCI(null), false);
});

test('running CI keeps refreshing on an interval, even while a branch is selected', () => {
  const base = { snap: running, selectionActive: true, pingChanged: false, lastFullAt: 1000 };
  assert.equal(pollDecision({ ...base, now: 1000 + CI_REFRESH_MS - 1 }), 'skip', 'not yet due');
  assert.equal(pollDecision({ ...base, now: 1000 + CI_REFRESH_MS }), 'fetch', 'due: pull fresh GitHub data');
});

test('once CI has finished the loop goes quiet again (no needless snapshots)', () => {
  assert.equal(pollDecision({ snap: done, now: 999999, lastFullAt: 0, selectionActive: true, pingChanged: false }), 'skip');
  assert.equal(pollDecision({ snap: done, now: 999999, lastFullAt: 0, selectionActive: false, pingChanged: false }), 'skip');
});

test('a git change fetches, but never yanks the canvas away from a selected item', () => {
  assert.equal(pollDecision({ snap: done, pingChanged: true, selectionActive: false, now: 1, lastFullAt: 1 }), 'fetch');
  assert.equal(pollDecision({ snap: done, pingChanged: true, selectionActive: true, now: 1, lastFullAt: 1 }), 'skip');
});

test('defensive defaults', () => {
  assert.equal(pollDecision(), 'skip');
  assert.equal(pollDecision({}), 'skip');
});

test('main CI running after a merge keeps the loop refreshing until it finishes', () => {
  const snap = { repo: { mainCI: { state: 'RUNNING' } }, branches: [] };
  assert.equal(hasPendingCI(snap), true);
  assert.equal(pollDecision({ snap, now: 100000, lastFullAt: 0, selectionActive: true }), 'fetch');
  assert.equal(hasPendingCI({ repo: { mainCI: { state: 'SUCCESS' } }, branches: [] }), false);
  assert.equal(hasPendingCI({ repo: { mainCI: { state: 'FAILURE' } }, branches: [] }), false);
});

test('fresh background data fetches even while a branch is selected (server shows stale data first now)', () => {
  assert.equal(pollDecision({ bgChanged: true, selectionActive: true, snap: { branches: [] }, now: 1, lastFullAt: 1 }), 'fetch');
  assert.equal(pollDecision({ bgChanged: false, pingChanged: false, selectionActive: true, snap: { branches: [] }, now: 1, lastFullAt: 1 }), 'skip');
});

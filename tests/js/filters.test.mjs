// Tests for web/modules/filters.js and its agreement with the KPI chips. Run: node --test tests/js
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

import { state, el } from '../../web/modules/state.js';
import {
  FILTERS, filterContext, filterTest, matchesPathLens, matchesBranchFilter, countMatching, dirtyFileCount,
} from '../../web/modules/filters.js';
import { matchesFilter } from '../../web/modules/scene.js';
import { renderKPIs } from '../../web/modules/kpi.js';

// ---- fixtures ----
const open = (id, extra = {}) => ({ id, name: id, state: 'open', hemisphere: 'upper', hasLocal: false, ...extra });
const withPR = (n, ci = 'SUCCESS') => ({ number: n, ci: { state: ci } });

const FIXTURE = [
  open('pr-green', { pr: withPR(1), author: 'ziya' }),
  open('pr-red', { pr: withPR(2, 'FAILURE'), author: 'bob' }),
  open('pr-conflict', { pr: withPR(3), conflict: { hasConflict: true, resolved: false }, author: 'ziya' }),
  open('pr-conflict-fixed', { pr: withPR(4), conflict: { hasConflict: true, resolved: true } }),
  open('pr-cross', { pr: withPR(5), crossPr: [{ otherPrNumber: 9 }] }),
  open('local-only', { hemisphere: 'lower', hasLocal: true, author: 'ziya', changedFiles: ['internal/gitengine/lanes.go'] }),
  open('head-dirty', { hemisphere: 'lower', hasLocal: true, isHead: true, uncommittedCount: 2 }),
  { id: 'merged-1', name: 'merged-1', state: 'merged', hemisphere: 'upper', pr: withPR(6, 'FAILURE'), conflict: { hasConflict: true } },
  { id: 'gone-1', name: 'gone-1', state: 'open', hemisphere: 'upper', originGone: true, hasLocal: true },
  { id: 'gone-2', name: 'gone-2', state: 'gone', hemisphere: 'upper' },
];

const snapshot = (over = {}) => ({ repo: { currentUser: 'ziya', dirtyCount: 3, ...over }, branches: FIXTURE, nodes: [] });
const ids = (list) => list.map((b) => b.id).sort();
const where = (key, snap = snapshot()) => ids(snap.branches.filter((b) => matchesBranchFilter(b, { filter: key, pathLens: '', ctx: filterContext(snap) })));

beforeEach(() => {
  state.snapshot = snapshot();
  state.filter = 'all';
  state.pathLens = '';
});

// ---- each predicate ----
test('all and unknown keys match everything', () => {
  assert.equal(where('all').length, FIXTURE.length);
  assert.equal(where('definitely-not-a-filter').length, FIXTURE.length);
  assert.equal(filterTest('nope'), FILTERS.all.test);
});

test('upper filter shows open pull requests (regression: it was undefined and matched every branch)', () => {
  assert.deepEqual(where('upper'), ['pr-conflict', 'pr-conflict-fixed', 'pr-cross', 'pr-green', 'pr-red']);
  assert.ok(where('upper').length < FIXTURE.length);
});

test('lower filter: open local branches without a PR', () => {
  assert.deepEqual(where('lower'), ['head-dirty', 'local-only']);
});

test('mine filter follows the current user and falls back to HEAD when unknown', () => {
  assert.deepEqual(where('mine'), ['head-dirty', 'local-only', 'pr-conflict', 'pr-green']);
  const noUser = snapshot({ currentUser: '' });
  assert.deepEqual(where('mine', noUser), ['head-dirty']);
});

test('issues filter: unresolved conflicts and Cross-PR collisions on open branches only', () => {
  assert.deepEqual(where('issues'), ['pr-conflict', 'pr-cross']);
});

test('ci_fail filter: open branches whose CI failed (merged ones do not count)', () => {
  assert.deepEqual(where('ci_fail'), ['pr-red']);
});

test('gone filter: merged, gone and origin-gone branches', () => {
  assert.deepEqual(where('gone'), ['gone-1', 'gone-2', 'merged-1']);
});

test('dirty filter shows only the HEAD branch and honours the repo-wide dirty count', () => {
  assert.deepEqual(where('dirty'), ['head-dirty']);
  const clean = snapshot({ dirtyCount: 0 });
  clean.branches = clean.branches.map((b) => (b.isHead ? { ...b, uncommittedCount: 0 } : b));
  assert.deepEqual(where('dirty', clean), []);
});

// ---- path lens ----
test('matchesPathLens: name or changed file, case-insensitive; empty query matches all', () => {
  const b = { name: 'feat/Login', changedFiles: ['internal/Auth.go'] };
  assert.ok(matchesPathLens(b, ''));
  assert.ok(matchesPathLens(b, '  '));
  assert.ok(matchesPathLens(b, 'LOGIN'));
  assert.ok(matchesPathLens(b, 'auth'));
  assert.ok(!matchesPathLens(b, 'zzz'));
  assert.ok(matchesPathLens({ name: undefined }, ''));
});

test('matchesBranchFilter ANDs the filter with the path lens', () => {
  const ctx = filterContext(snapshot());
  const lane = FIXTURE.find((b) => b.id === 'local-only');
  assert.ok(matchesBranchFilter(lane, { filter: 'lower', pathLens: 'lanes', ctx }));
  assert.ok(!matchesBranchFilter(lane, { filter: 'lower', pathLens: 'nope', ctx }));
  assert.ok(!matchesBranchFilter(lane, { filter: 'ci_fail', pathLens: 'lanes', ctx }));
});

// ---- counts ----
test('countMatching equals the number of branches the filter shows', () => {
  const snap = snapshot();
  const ctx = filterContext(snap);
  for (const key of Object.keys(FILTERS)) {
    assert.equal(countMatching(key, snap.branches, ctx), where(key, snap).length, key);
  }
  assert.equal(countMatching('mine', null, ctx), 0);
});

test('dirtyFileCount counts FILES (repo-wide), falling back to the HEAD branch', () => {
  assert.equal(dirtyFileCount(snapshot({ dirtyCount: 7 })), 7);
  assert.equal(dirtyFileCount(snapshot({ dirtyCount: 0 })), 2, 'falls back to HEAD uncommittedCount');
  assert.equal(dirtyFileCount({ repo: {}, branches: [] }), 0);
  assert.equal(dirtyFileCount(null), 0);
});

// ---- the invariant this refactor exists for: chip number == what the canvas shows ----
function fakeDom() {
  const mk = () => ({
    children: [], className: '', title: '', _h: '',
    appendChild(c) { this.children.push(c); }, addEventListener() {},
    set innerHTML(v) { this._h = v; if (v === '') this.children = []; }, get innerHTML() { return this._h; },
  });
  globalThis.document = { createElement: mk, getElementById: () => null };
  el.kpiPills = mk();
  el.dragHint = mk();
}

test('KPI chip values always equal the number of branches the same filter shows on the canvas', () => {
  fakeDom();
  renderKPIs();
  const chips = new Map(el.kpiPills.children.map((c) => {
    const label = c._h.match(/<span>(.*?)<\/span>/)[1];
    const value = Number(c._h.match(/<strong>(\d+)<\/strong>/)[1]);
    return [label, value];
  }));
  const byFilter = { 'Benim Dallarım': 'mine', 'Yerel (Local)': 'lower', 'Çakışma / Cross-PR': 'issues', 'CI Hatası': 'ci_fail', 'Birleşmiş / Gone': 'gone', 'Uzak (PR)': 'upper' };
  for (const [labelPart, key] of Object.entries(byFilter)) {
    const label = [...chips.keys()].find((l) => l.includes(labelPart));
    assert.ok(label, `chip "${labelPart}" must exist`);
    state.filter = key;
    state.pathLens = '';
    const shown = FIXTURE.filter((b) => matchesFilter(b)).length;
    assert.equal(chips.get(label), shown, `chip "${label}" (${chips.get(label)}) must equal branches shown by filter "${key}" (${shown})`);
  }
  // "Tümü" is the total, "Dirty" is a FILE count by design.
  assert.equal(chips.get('Tümü'), FIXTURE.length);
  assert.equal(chips.get('Değişiklik (Dirty)'), 3);
});

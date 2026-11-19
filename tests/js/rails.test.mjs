// Zero-dependency tests for the canvas label/chip helpers (web/modules/rails.js). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { protectionView } from '../../web/modules/syncsteps.js';

test('top bar protection button names the real default branch too', () => {
  assert.equal(protectionView({ protectionState: 'active', defaultBranch: 'master' }).button, '🛡️ master Korumalı');
  assert.equal(protectionView({ protectionState: 'active' }).button, '🛡️ main Korumalı');
});

import { longLivedList, longLivedItemModel } from '../../web/modules/rails.js';
import { actionRowHtmlOf, mergedPillModel } from '../../web/modules/capsule.js';
import { cleanableCount } from '../../web/modules/topbar.js';

test('long-lived list takes the server flag and skips deleted remotes', () => {
  const bs = [
    { name: 'develop', longLived: true, state: 'merged', hasLocal: true, hasRemote: true, originGone: true }, // birleşmiş ama hâlâ duruyor
    { name: 'feat/x', hasLocal: true },
    { name: 'release/1', longLived: true, hasRemote: true },
    { name: 'stable', longLived: true, state: 'gone', hasLocal: true },
    { name: 'support/old', longLived: true, state: 'merged' }, // hiçbir kopyası kalmamış
  ];
  assert.deepEqual(longLivedList(bs).map((b) => b.name), ['develop', 'release/1']);
  assert.deepEqual(longLivedList(undefined), []);
});

test('long-lived item shows ahead/behind and where the branch lives', () => {
  assert.deepEqual(longLivedItemModel({ name: 'develop', aheadMain: 3, behindMain: 1, hasLocal: true, hasRemote: true }), { title: 'develop', sub: '↑3 ↓1 · yerel + uzak' });
  assert.equal(longLivedItemModel({ name: 'r', hasRemote: true }).sub, '↑0 ↓0 · uzak');
  assert.equal(longLivedItemModel({ name: 'r', hasLocal: true }).sub, '↑0 ↓0 · yerel');
});

test('long-lived branches never offer a delete/clean button and are not counted as cleanable', () => {
  const base = { id: 'b', name: 'develop', hemisphere: 'upper', state: 'open', hasRemote: true, aheadMain: 2, pr: { number: 5, reviewState: 'OPEN', ci: { state: 'SUCCESS' } } };
  assert.match(actionRowHtmlOf(base), /data-act="delete"/);
  assert.ok(!/data-act="delete"/.test(actionRowHtmlOf({ ...base, longLived: true })));
  const merged = { name: 'develop', state: 'merged', hasLocal: true, originGone: true };
  assert.match(mergedPillModel(merged, false).html, /data-act="delete"/);
  assert.ok(!/data-act="delete"/.test(mergedPillModel({ ...merged, longLived: true }, false).html));
  assert.equal(cleanableCount([{ ...merged, prunable: true }, { ...merged, longLived: true }]), 1);
});

import { basePickerModel } from '../../web/modules/rails.js';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

test('base picker only shows for live repos with a real choice', () => {
  const live = { mode: 'live', defaultBranch: 'develop', baseCandidates: ['develop', 'main', 'feat/x'], baseBranchPinned: true };
  const m = basePickerModel(live);
  assert.equal(m.visible, true);
  assert.equal(m.pinned, true);
  assert.deepEqual(m.items.map((i) => [i.name, i.current]), [['develop', true], ['main', false], ['feat/x', false]]);
  assert.equal(basePickerModel({ ...live, mode: 'simulation' }).visible, false);
  assert.equal(basePickerModel({ ...live, baseCandidates: ['main'] }).visible, false);
  assert.equal(basePickerModel({ mode: 'live' }).visible, false);
  assert.equal(basePickerModel(undefined).current, 'main');
});

test('offline engine refuses to change the base branch like the server does in simulation', () => {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  assert.throws(() => ctx.window.LocalFallbackEngine.setBaseBranch('develop'), /Simülasyon/);
});

test('base picker button shows the current spine branch and says whether it was chosen by hand', () => {
  const auto = basePickerModel({ mode: 'live', defaultBranch: 'develop', baseCandidates: ['develop', 'main'] });
  assert.equal(auto.label, '⚙ develop');
  assert.match(auto.title, /otomatik tespit/);
  const pinned = basePickerModel({ mode: 'live', defaultBranch: 'main', baseCandidates: ['develop', 'main'], baseBranchPinned: true });
  assert.match(pinned.title, /elle seçildi/);
  assert.equal(basePickerModel({ mode: 'live', defaultBranch: 'x'.repeat(40), baseCandidates: ['a', 'b'] }).label.length <= 26, true);
});

import { closeChipPopovers } from '../../web/modules/rails.js';
import { state } from '../../web/modules/state.js';

test('opening one chip popover closes the others (they now sit side by side)', () => {
  const nodes = {};
  globalThis.document = { getElementById: (id) => (nodes[id] ||= { hidden: false, attrs: {}, setAttribute(k, v) { this.attrs[k] = v; } }) };
  state.baseOpen = true; state.historyOpen = true; state.longLivedOpen = true;
  closeChipPopovers('history');
  assert.equal(state.historyOpen, true, 'the one being opened is left alone');
  assert.equal(state.baseOpen, false);
  assert.equal(state.longLivedOpen, false);
  assert.equal(nodes['base-popover'].hidden, true);
  assert.equal(nodes['btn-longlived-lane'].attrs['aria-expanded'], 'false');
  closeChipPopovers();
  assert.equal(state.historyOpen, false);
});

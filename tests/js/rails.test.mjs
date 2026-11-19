// Zero-dependency tests for the left-rail spine card (web/modules/rails.js). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { spineCardModel } from '../../web/modules/rails.js';
import { protectionView } from '../../web/modules/syncsteps.js';

test('spine card name follows the default branch (master/develop), main only as fallback', () => {
  assert.equal(spineCardModel({ defaultBranch: 'master' }).name, 'master');
  assert.equal(spineCardModel({ defaultBranch: 'develop' }).name, 'develop');
  assert.equal(spineCardModel({}).name, 'main');
  assert.equal(spineCardModel(undefined).name, 'main');
});

test('spine card never claims "Korunan" unless protection is really active', () => {
  assert.match(spineCardModel({ protectionState: 'active' }).sub, /Korunan/);
  assert.equal(spineCardModel({ protectionState: 'active' }).tone, 'prot-active');
  const none = spineCardModel({ protectionState: 'none' });
  assert.match(none.sub, /Korumasız/);
  assert.equal(none.tone, 'prot-none');
  const unknown = spineCardModel({ protectionState: 'unknown' });
  assert.match(unknown.sub, /okunamadı/);
  assert.ok(!/Korunan/.test(unknown.sub));
  assert.equal(unknown.tone, 'prot-unknown');
});

test('spine card stays compatible with servers that only send branchProtected', () => {
  assert.equal(spineCardModel({ branchProtected: true }).tone, 'prot-active');
  assert.equal(spineCardModel({ branchProtected: false }).tone, 'prot-none');
});

test('spine card tooltip prefers the server summary', () => {
  assert.equal(spineCardModel({ protectionSummary: 'Ruleset: 2 kural' }).title, 'Ruleset: 2 kural');
  assert.match(spineCardModel({ defaultBranch: 'master', protectionState: 'none' }).title, /^master:/);
});

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
  assert.equal(cleanableCount([merged, { ...merged, longLived: true }]), 1);
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

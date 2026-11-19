// Tests for the pure models behind web/modules/kpi.js and web/modules/topbar.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { buildKpiModel } from '../../web/modules/kpi.js';
import { modeLabel, versionView, cleanableCount, syncBadgeModel } from '../../web/modules/topbar.js';

// ---------- kpi ----------
const open = (id, extra = {}) => ({ id, name: id, state: 'open', hemisphere: 'upper', ...extra });
const snap = {
  repo: { currentUser: 'ziya', dirtyCount: 4 },
  branches: [
    open('a', { pr: { number: 1, ci: { state: 'FAILURE' } }, author: 'ziya' }),
    open('b', { hemisphere: 'lower', hasLocal: true, author: 'ziya' }),
    { id: 'gh-merged-9', name: 'm', state: 'merged', hemisphere: 'upper' },
    { id: 'merged-x', name: 'mx', state: 'merged', hemisphere: 'upper', hasLocal: true },
    { id: 'local-wip-working-tree', name: 'wip', state: 'open', hemisphere: 'lower', hasLocal: true },
  ],
};

test('buildKpiModel: chip definitions in a fixed order with the expected keys', () => {
  const m = buildKpiModel(snap);
  assert.deepEqual(m.defs.map((d) => d.key), ['all', 'mine', 'lower', 'dirty', 'issues', 'ci_fail', 'gone']);
  assert.equal(m.total, 5);
  assert.equal(m.defs.find((d) => d.key === 'all').val, 5);
  assert.equal(m.defs.find((d) => d.key === 'ci_fail').val, 1);
  assert.equal(m.defs.find((d) => d.key === 'dirty').val, 4, 'dirty chip counts FILES');
  assert.equal(m.upperCount, 1);
});

test('buildKpiModel: tone is empty for zero counts and coloured otherwise', () => {
  const m = buildKpiModel({ repo: {}, branches: [] });
  for (const d of m.defs) assert.equal(d.tone, '', `${d.key} must have no tone when 0`);
  const n = buildKpiModel(snap);
  assert.equal(n.defs.find((d) => d.key === 'ci_fail').tone, 'danger');
  assert.equal(n.defs.find((d) => d.key === 'dirty').tone, 'warn');
  assert.equal(n.defs.find((d) => d.key === 'gone').tone, 'ok');
});

test('buildKpiModel: local/merged-PR counters exclude the virtual working-tree branch and gh-merged arches', () => {
  const m = buildKpiModel(snap);
  assert.equal(m.localCount, 2, 'b and merged-x are local; wip virtual branch and gh-merged-9 are not');
  assert.equal(m.mergedPRCount, 1, 'only the gh-merged PR (merged-x has a local copy)');
  assert.match(m.hintHtml, /Tümü Sayacı \(5\)/);
  assert.match(m.hintHtml, /2 yerel dal \+ 1 birleşmiş PR/);
  assert.match(m.defs[0].title, /2 yerel dal/);
});

test('buildKpiModel tolerates missing snapshot data', () => {
  for (const v of [null, undefined, {}, { repo: null, branches: null }]) {
    const m = buildKpiModel(v);
    assert.equal(m.total, 0);
    assert.equal(m.defs.length, 7);
  }
});

// ---------- topbar ----------
test('modeLabel', () => {
  assert.equal(modeLabel({ mode: 'live' }, false), 'Canlı Git · Aktif');
  assert.equal(modeLabel({ mode: 'simulation' }, false), 'Simülasyon · Etkileşimli');
  assert.equal(modeLabel({ mode: 'live' }, true), 'Offline Simülasyon', 'offline wins over live');
});

test('versionView: team and update tags, fallback version', () => {
  assert.deepEqual(versionView({ appVersion: '1.2.3' }), { text: 'v1.2.3', className: 'btn mono ghost' });
  assert.deepEqual(versionView({ appVersion: '1.2.3', teamMode: true, updateAvailable: true }), { text: 'v1.2.3 · TEAM ⬆ Güncelle', className: 'btn mono warn' });
  assert.match(versionView({}).text, /^v\d+\.\d+\.\d+$/);
});

test('cleanableCount: gone, origin-gone and merged-with-local-copy branches', () => {
  assert.equal(cleanableCount([
    { state: 'gone' }, { originGone: true, state: 'open' }, { state: 'merged', hasLocal: true },
    { state: 'merged', hasLocal: false }, { state: 'open' },
  ]), 3);
  assert.equal(cleanableCount(null), 0);
});

test('syncBadgeModel: in sync with all steps ok is healthy', () => {
  const m = syncBadgeModel({ defaultBranch: 'main', remoteName: 'origin', syncSteps: [{ id: 'fetch', label: 'git fetch', state: 'ok' }] });
  assert.equal(m.text, 'main ↔ origin/main (Synced)');
  assert.equal(m.isSynced, true);
  assert.equal(m.healthy, true);
  assert.equal(m.skewed, false);
  assert.match(m.title, /eşitlendi/);
});

test('syncBadgeModel: ahead/behind shows the arrows and is always skewed', () => {
  const m = syncBadgeModel({ ahead: 2, behind: 1 });
  assert.equal(m.text, 'main ↔ origin/main  ↑2 · ↓1');
  assert.equal(m.isSynced, false);
  assert.equal(m.healthy, false);
  assert.equal(m.skewed, true);
  assert.match(m.title, /Fark var \(↑2 \/ ↓1\)/);
});

test('syncBadgeModel: "Synced" is NOT healthy while a data step is pending or stale (the whole point of the 2nd line)', () => {
  for (const state of ['pending', 'stale', 'failed']) {
    const m = syncBadgeModel({ syncSteps: [{ id: 'prs', label: 'PR listesi', state }] });
    assert.equal(m.isSynced, true);
    assert.equal(m.healthy, false, state);
    assert.equal(m.skewed, true, state);
  }
  // unknown protection (no admin rights) is permanent + harmless
  assert.equal(syncBadgeModel({ syncSteps: [{ id: 'protection', label: 'Koruma', state: 'unknown' }] }).healthy, true);
});

test('syncBadgeModel: steps are included in the tooltip and as items; defaults for missing repo fields', () => {
  const m = syncBadgeModel({ syncSteps: [{ id: 'fetch', label: 'git fetch', state: 'ok', count: 0 }] });
  assert.equal(m.steps.length, 1);
  assert.match(m.title, /Veri toplama:\ngit fetch:/);
  const bare = syncBadgeModel({});
  assert.equal(bare.text, 'main ↔ origin/main (Synced)');
  assert.deepEqual(bare.steps, []);
  assert.doesNotMatch(bare.title, /Veri toplama/);
});

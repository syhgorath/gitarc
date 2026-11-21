// "CI Bitince Merge Et" ne zaman sunulur: CI sürüyorsa YA DA CI henüz belirmemişken zorunlu kontrol varsa.
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { ciPendingForMerge } from '../../web/modules/syncsteps.js';
import { branchViewModel } from '../../web/modules/inspector-model.js';
import { actionRowHtmlOf } from '../../web/modules/capsule.js';

const pr = (ci) => ({ number: 7, reviewState: 'OPEN', ...(ci ? { ci: { state: ci } } : {}) });
const open = (extra = {}) => ({ id: 'a', name: 'a', state: 'open', hemisphere: 'upper', hasLocal: false, hasRemote: true, aheadMain: 1, pr: pr(), ...extra });
const snap = (requiredChecks) => ({ repo: { mode: 'live', requiredChecks } });

test('CI sürüyorsa her zaman beklenir', () => {
  assert.equal(ciPendingForMerge(open({ pr: pr('RUNNING') }), snap()), true);
});

test('CI henüz yok + zorunlu kontrol var → bekle (düz Merge GitHub reddine çarpar)', () => {
  assert.equal(ciPendingForMerge(open(), snap(['go (macos-latest)', 'web'])), true);
});

test('CI henüz yok + zorunlu kontrol yok/okunamadı → düz Merge (CI\'ı olmayan depoda boşuna beklenmez)', () => {
  assert.equal(ciPendingForMerge(open(), snap([])), false);
  assert.equal(ciPendingForMerge(open(), snap(undefined)), false);
  assert.equal(ciPendingForMerge(open(), { repo: {} }), false);
});

test('CI biliniyorsa (yeşil/kırmızı) beklenmez; PR yoksa ya da açık değilse hayır', () => {
  assert.equal(ciPendingForMerge(open({ pr: pr('SUCCESS') }), snap(['x'])), false);
  assert.equal(ciPendingForMerge(open({ pr: pr('FAILURE') }), snap(['x'])), false);
  assert.equal(ciPendingForMerge(open({ pr: null }), snap(['x'])), false);
  assert.equal(ciPendingForMerge(open({ state: 'merged' }), snap(['x'])), false);
  assert.equal(ciPendingForMerge(null, snap(['x'])), false);
});

test('inspector modeli: bekleyen CI "mergeWaitsForCI" olur ama isRunning (rozet/yeşil-kırmızı metni) uydurulmaz', () => {
  const v = branchViewModel(open(), snap(['web']));
  assert.equal(v.mergeWaitsForCI, true);
  assert.equal(v.isRunning, false, 'CI bilinmiyorken "çalışıyor" denmez');
});

test('kart düğmesi: CI belirmemişken "CI Bitince Merge Et", yoksa "Merge"', () => {
  const html = (ctx) => actionRowHtmlOf(open({ hemisphere: 'lower', hasLocal: true }), ctx);
  assert.match(html({ ciPending: true, isQueued: false }), /CI Bitince Merge Et/);
  assert.match(html({ ciPending: false, isQueued: false }), /⚡ Merge/);
});

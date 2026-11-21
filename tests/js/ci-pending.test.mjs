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

// Proje değiştirirken süren kuyruk koşusu varsa uyarı (koşu etkin proje değişince durur, yarım kalabilir).
import { projectSwitchWarning } from '../../web/modules/syncsteps.js';

test('projectSwitchWarning: koşu yoksa null; koşu sürüyorsa ilerlemeyi ve "yarım kalabilir"i söyler', () => {
  assert.equal(projectSwitchWarning(null), null);
  assert.equal(projectSwitchWarning({ repo: { name: 'GitArc' } }), null);
  const w = projectSwitchWarning({ repo: { name: 'GitArc', queueRun: { total: 5, merged: 2, items: [] } } });
  assert.match(w, /GitArc: 2\/5 PR'lık kuyruk koşusu sürüyor/);
  assert.match(w, /DURUR ve yarım kalabilir/);
  assert.match(w, /Yine de projeyi değiştirmek istiyor musunuz\?/);
});

import { openPRsTruncationNote } from '../../web/modules/queue.js';

test('openPRsTruncationNote: liste sınıra dayandıysa söyler, yoksa boş', () => {
  assert.equal(openPRsTruncationNote(null), '');
  assert.equal(openPRsTruncationNote({ repo: {} }), '');
  assert.match(openPRsTruncationNote({ repo: { openPRsTruncated: true } }), /yalnızca ilk 20 açık PR/);
});

import { updateStatusText } from '../../web/modules/topbar.js';

test('updateStatusText: hiç denetlenmediyse "güncel" DENMEZ; denetlenemediyse nedeni söyler', () => {
  assert.match(updateStatusText({ appVersion: '0.10.0' }), /Henüz denetlenmedi/);
  assert.match(updateStatusText({ appVersion: '0.10.0' }, { currentVersion: '0.10.0', checkError: 'ağ yok' }), /Denetlenemedi.*ağ yok/);
  assert.match(updateStatusText({ appVersion: '0.10.0' }, { currentVersion: '0.10.0', latestVersion: '0.11.0', updateAvailable: true }), /Yeni sürüm mevcut: v0\.11\.0 \(şu an v0\.10\.0\)/);
  assert.match(updateStatusText({ appVersion: '0.10.0' }, { currentVersion: '0.10.0', latestVersion: '0.10.0', updateAvailable: false }), /Güncel \(v0\.10\.0\)/);
  assert.match(updateStatusText({ appVersion: '0.10.0', latestVersion: '0.10.0' }), /Güncel/);
});

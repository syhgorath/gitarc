// Zero-dependency tests for web/modules/syncsteps.js (+ snapshotSignature). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatAgo, formatDuration, stepTone, describeStep, summarizeSync, protectionView } from '../../web/modules/syncsteps.js';

const NOW = Date.parse('2026-11-19T12:00:00Z');
const at = (secAgo) => new Date(NOW - secAgo * 1000).toISOString();

test('formatAgo buckets and bad input', () => {
  assert.equal(formatAgo(at(1), NOW), 'az önce');
  assert.equal(formatAgo(at(12), NOW), '12 sn önce');
  assert.equal(formatAgo(at(180), NOW), '3 dk önce');
  assert.equal(formatAgo(at(7200), NOW), '2 sa önce');
  assert.equal(formatAgo(at(86400 * 3), NOW), '3 gün önce');
  assert.equal(formatAgo('', NOW), '');
  assert.equal(formatAgo('not a date', NOW), '');
  assert.equal(formatAgo(at(-30), NOW), 'az önce'); // clock skew never goes negative
});

test('formatDuration', () => {
  assert.equal(formatDuration(0), '');
  assert.equal(formatDuration(420), '420 ms');
  assert.equal(formatDuration(4800), '4.8 sn');
  assert.equal(formatDuration(-5), '');
});

test('stepTone maps every state', () => {
  assert.equal(stepTone({ state: 'ok' }), 'ok');
  assert.equal(stepTone({ state: 'failed' }), 'bad');
  assert.equal(stepTone({ state: 'skipped' }), 'muted');
  assert.equal(stepTone({ state: 'unknown' }), 'muted'); // permanent + harmless (no admin rights)
  for (const s of ['pending', 'stale']) assert.equal(stepTone({ state: s }), 'warn');
  assert.equal(stepTone(undefined), 'warn');
});

test('describeStep: ok step shows count, duration and age', () => {
  const d = describeStep({ id: 'prs', label: 'PR listesi', state: 'ok', count: 20, durationMs: 4800, at: at(12) }, NOW);
  assert.equal(d.text, 'PR listesi ✓ 20');
  assert.equal(d.tone, 'ok');
  assert.match(d.title, /20 kayıt/);
  assert.match(d.title, /4\.8 sn/);
  assert.match(d.title, /12 sn önce/);
});

test('describeStep: pending / stale / failed / unknown / skipped explain themselves', () => {
  assert.match(describeStep({ label: 'PR listesi', state: 'pending' }, NOW).title, /arka planda/);
  assert.match(describeStep({ label: 'PR listesi', state: 'stale' }, NOW).title, /önceki sonuç/);
  const failed = describeStep({ label: 'git fetch', state: 'failed', detail: 'zaman aşımı' }, NOW);
  assert.equal(failed.tone, 'bad');
  assert.match(failed.title, /zaman aşımı/);
  assert.equal(describeStep({ label: 'Koruma', state: 'unknown', detail: 'yetki yok' }, NOW).text, 'Koruma ?');
  assert.equal(describeStep({ label: 'Birleşmiş PR', state: 'skipped' }, NOW).tone, 'muted');
});

test('describeStep falls back to id when label is missing and never throws', () => {
  assert.equal(describeStep({ id: 'fetch', state: 'ok' }, NOW).text, 'fetch ✓');
  assert.doesNotThrow(() => describeStep({ state: 'weird' }, NOW));
});

test('summarizeSync: overall tone is the worst step', () => {
  const ok = { state: 'ok', label: 'a' };
  assert.equal(summarizeSync([ok, ok], NOW).tone, 'ok');
  assert.equal(summarizeSync([ok, ok], NOW).allOk, true);
  assert.equal(summarizeSync([ok, { state: 'pending', label: 'b' }], NOW).tone, 'warn');
  // an unknown protection state (non-admin) must not keep the badge amber forever
  assert.equal(summarizeSync([ok, { state: 'unknown', label: 'b' }], NOW).allOk, true);
  assert.equal(summarizeSync([ok, { state: 'stale', label: 'b' }], NOW).allOk, false);
  assert.equal(summarizeSync([ok, { state: 'failed', label: 'b' }, { state: 'pending', label: 'c' }], NOW).tone, 'bad');
  // skipped steps (e.g. non-GitHub remote) do not make the badge unhealthy
  assert.equal(summarizeSync([ok, { state: 'skipped', label: 'b' }], NOW).tone, 'ok');
});

test('summarizeSync tolerates missing data (old servers / simulation)', () => {
  for (const v of [undefined, null, []]) {
    const s = summarizeSync(v, NOW);
    assert.deepEqual(s.items, []);
    assert.equal(s.allOk, false);
    assert.equal(s.title, '');
  }
});

test('protectionView: unknown is NOT shown as unprotected (prometheus regression)', () => {
  const unknown = protectionView({ protectionState: 'unknown', branchProtected: false });
  assert.equal(unknown.state, 'unknown');
  assert.notEqual(unknown.button, '🔓 Korumasız');
  assert.equal(unknown.canEnable, false);

  const none = protectionView({ protectionState: 'none' });
  assert.equal(none.button, '🔓 Korumasız');
  assert.equal(none.canEnable, true);

  const active = protectionView({ protectionState: 'active' });
  assert.equal(active.button, '🛡️ main Korumalı');
  assert.equal(active.canEnable, false);
});

test('protectionView stays backwards compatible with servers that only send branchProtected', () => {
  assert.equal(protectionView({ branchProtected: true }).state, 'active');
  assert.equal(protectionView({ branchProtected: false }).state, 'none');
  assert.equal(protectionView(undefined).state, 'none');
});

import { snapshotSignature } from '../../web/modules/utils.js';

test('snapshotSignature reacts to sync step / protection changes but not to timestamps', () => {
  const base = (steps, prot) => ({ repo: { id: 'r', ahead: 0, behind: 0, dirtyCount: 0, syncSteps: steps, protectionState: prot }, nodes: [], branches: [], logs: [] });
  const pending = base([{ id: 'prs', state: 'pending' }], 'unknown');
  const ok = base([{ id: 'prs', state: 'ok', count: 20 }], 'unknown');
  const okLater = base([{ id: 'prs', state: 'ok', count: 20, at: '2026-11-19T12:00:09Z' }], 'unknown');
  assert.notEqual(snapshotSignature(pending), snapshotSignature(ok), 'pending → ok must re-render');
  assert.equal(snapshotSignature(ok), snapshotSignature(okLater), 'a new timestamp alone must not re-render');
  assert.notEqual(snapshotSignature(ok), snapshotSignature(base([{ id: 'prs', state: 'ok', count: 20 }], 'active')), 'protection change must re-render');
});

test('snapshotSignature reacts to the main CI result (regression: the chip never changed after the first load)', () => {
  const mk = (mainCI) => ({ repo: { id: 'r', mainCI }, branches: [], nodes: [], logs: [] });
  const running = snapshotSignature(mk({ state: 'RUNNING', sha: 'abc' }));
  const failed = snapshotSignature(mk({ state: 'FAILURE', sha: 'abc' }));
  const newCommit = snapshotSignature(mk({ state: 'RUNNING', sha: 'def' }));
  assert.notEqual(running, failed);
  assert.notEqual(running, newCommit);
  assert.notEqual(running, snapshotSignature(mk(undefined)));
});

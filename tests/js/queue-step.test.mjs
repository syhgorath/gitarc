// Merge kuyruğu 3a (adım adım): tarayıcı yedek motoru, internal/gitengine/queue_step.go ile AYNI kuralları taşır. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { stepPlanModel } from '../../web/modules/queue.js';

function loadEngine() {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  return ctx.window.LocalFallbackEngine;
}

const base = { mode: 'live', branch: 'feat/x', branchId: 'x', pr: 1 };
const upd = (status, behind) => ({ status, behind, headSha: 'h1', baseSha: 'b1', baseRef: 'origin/main', conflictFiles: ['f.txt'], reason: 'neden' });

test('decision table matches the Go engine (decideQueueStep)', () => {
  const eng = loadEngine();
  const cases = [
    [{ prState: 'OPEN', ci: 'SUCCESS', update: upd('up_to_date', 0) }, 'merge'],
    [{ prState: 'OPEN', ci: 'SUCCESS', update: upd('ready', 2) }, 'update'],
    [{ prState: 'OPEN', ci: 'RUNNING', update: upd('ready', 2) }, 'update'],
    [{ prState: 'OPEN', ci: 'FAILURE', update: upd('ready', 2) }, 'blocked'],
    [{ prState: 'OPEN', ci: 'RUNNING', update: upd('up_to_date', 0) }, 'wait'],
    [{ prState: 'OPEN', ci: 'UNKNOWN', update: upd('up_to_date', 0) }, 'wait'],
    [{ prState: 'OPEN', ci: '', update: upd('up_to_date', 0) }, 'wait'],
    [{ prState: 'OPEN', ci: 'SUCCESS', update: upd('conflict', 1) }, 'blocked'],
    [{ prState: 'OPEN', draft: true, ci: 'SUCCESS', update: upd('up_to_date', 0) }, 'blocked'],
    [{ prState: 'MERGED', ci: 'SUCCESS', update: upd('up_to_date', 0) }, 'blocked'],
    [{ prState: 'OPEN', ci: 'SUCCESS', review: 'REVIEW_REQUIRED', update: upd('up_to_date', 0) }, 'blocked'],
    [{ prState: 'OPEN', ci: 'SUCCESS', review: 'APPROVED', update: upd('up_to_date', 0) }, 'merge'],
    [{ prState: 'OPEN', ci: 'SUCCESS', ghHeadSha: 'baska', update: upd('up_to_date', 0) }, 'blocked'],
  ];
  cases.forEach(([input, action], i) => {
    const p = eng.queueStepDecide(base, input);
    assert.equal(p.action, action, `case ${i}: ${p.reason}`);
    assert.equal(Boolean(p.fingerprint), action === 'update' || action === 'merge', `case ${i} fingerprint`);
  });
  const unknown = eng.queueStepDecide(base, { prState: 'OPEN', ci: 'UNKNOWN', update: upd('up_to_date', 0) });
  assert.match(unknown.reason, /yeşil varsayılmaz/);
});

test('simulation: wait / blocked steps cannot be applied; green PR updates, merges, and others fall behind', () => {
  const eng = loadEngine();
  assert.equal(eng.queueStepPlan('br-open-421').action, 'blocked');
  assert.equal(eng.queueStepPlan('br-open-422').action, 'blocked');
  assert.equal(eng.queueStepPlan('br-open-424').action, 'wait');
  assert.throws(() => eng.queueStep('br-open-424', 'x'), /şimdilik yapılacak bir şey yok/);

  const behindBefore = eng.queueStepPlan('br-open-422').behind;
  eng.fixCI('br-open-424');
  const merge = eng.queueStepPlan('br-open-424');
  assert.equal(merge.action, 'merge');
  assert.throws(() => eng.queueStep('br-open-424', 'yanlis'), /onaylanmadı/);
  assert.throws(() => eng.queueStep('br-open-424', ''), /onaylanmadı/);
  eng.queueStep('br-open-424', merge.fingerprint);
  assert.equal(eng.queueStepPlan('br-open-424').action, 'blocked', 'merged PR takes no further step');

  eng.fixCI('br-open-422');
  const update = eng.queueStepPlan('br-open-422');
  assert.equal(update.action, 'update');
  assert.equal(update.behind, behindBefore + 1, 'merging another PR leaves this one one commit further behind');
  assert.throws(() => eng.queueStep('br-open-422', merge.fingerprint), /onaylanmadı/, 'a stale/other fingerprint never confirms');
  eng.queueStep('br-open-422', update.fingerprint);
  const next = eng.queueStepPlan('br-open-422');
  assert.equal(next.behind, 0);
  assert.equal(next.action, 'merge');
});

test('step plan view model: apply only for update/merge with a fingerprint; unknown CI is never shown as green', () => {
  const m = stepPlanModel({ action: 'merge', pr: 3, headSha: 'abcdef123', ci: 'SUCCESS', fingerprint: 'fp', reason: 'ok' });
  assert.equal(m.canApply, true);
  assert.match(m.confirmText, /#3 birleştir/);
  assert.ok(m.lines.some((l) => l.includes('abcdef1')), 'head sha shown');
  assert.equal(stepPlanModel({ action: 'update', pr: 3, ci: 'SUCCESS', fingerprint: 'fp' }).canApply, true);
  assert.equal(stepPlanModel({ action: 'merge', pr: 3, ci: 'SUCCESS' }).canApply, false, 'no fingerprint, no apply');
  for (const action of ['wait', 'blocked']) {
    assert.equal(stepPlanModel({ action, ci: 'UNKNOWN', fingerprint: 'fp' }).canApply, false, action);
  }
  assert.ok(stepPlanModel({ action: 'wait', ci: 'UNKNOWN' }).lines.some((l) => /yeşil varsayılmaz/.test(l)));
});

import { pruneQueueView } from '../../web/modules/queue.js';

test('merged/closed PRs are dropped from the queue view so the next plan request never names a closed branch', () => {
  const snap = { branches: [
    { id: 'a', state: 'open', pr: { number: 1 } },
    { id: 'b', state: 'merged', pr: { number: 2 } },
    { id: 'c', state: 'open', pr: { number: 3 } },
  ] };
  const r = pruneQueueView(['a', 'b', 'c'], new Set(['a', 'b']), snap);
  assert.equal(JSON.stringify(r.order), JSON.stringify(['a', 'c']));
  assert.equal(JSON.stringify([...r.included]), JSON.stringify(['a']));
});

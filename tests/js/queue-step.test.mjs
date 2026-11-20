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

// ---- 3b: otomatik koşu ----
import { runPlanModel, runStatusModel } from '../../web/modules/queue.js';

test('run plan model: only a startable plan with a fingerprint can be confirmed; excluded PRs are listed, not run', () => {
  const m = runPlanModel({ startable: true, fingerprint: 'fp', items: [{ position: 1, pr: 4, branch: 'feat/d' }, { position: 2, pr: 5, branch: 'feat/e' }], excluded: [{ pr: 6, branch: 'feat/f' }], notes: ['not 1'] });
  assert.equal(m.canStart, true);
  assert.match(m.confirmText, /2 PR/);
  assert.ok(m.lines.includes('1. #4 feat/d') && m.lines.some((l) => l.includes('koşuya girmez') && l.includes('#6')) && m.lines.includes('not 1'));
  const no = runPlanModel({ startable: false, reason: 'ilk PR çakışıyor', items: [], excluded: [] });
  assert.equal(no.canStart, false);
  assert.ok(no.lines.some((l) => l.includes('ilk PR çakışıyor')));
  assert.equal(runPlanModel({ startable: true, items: [{}] }).canStart, false, 'no fingerprint, no start');
});

test('run status model: running shows progress; every stop status has a title, and only stops can be re-planned', () => {
  const items = [{ pr: 4, branch: 'feat/d' }, { pr: 5, branch: 'feat/e' }];
  const run = runStatusModel({ status: 'running', items, current: 1, merged: 1, events: [{ time: '2026-11-20T15:04:05Z', message: 'x birleştirildi' }] });
  assert.equal(run.running, true);
  assert.ok(run.lines[0].includes('2/2') && run.lines[0].includes('#5') && run.lines[0].includes('1 birleşti'));
  assert.ok(run.lines.some((l) => l.includes('15:04:05') && l.includes('x birleştirildi')));
  for (const status of ['completed', 'stopped', 'blocked', 'timeout', 'interrupted']) {
    const m = runStatusModel({ status, items, current: 0, merged: 0, stopReason: 'neden' });
    assert.equal(m.running, false, status);
    assert.ok(m.title && !m.title.includes('undefined'), status);
  }
  assert.equal(runStatusModel({ status: 'blocked', items }).resumable, true);
  assert.equal(runStatusModel({ status: 'completed', items }).resumable, false);
  assert.equal(runStatusModel(null), null);
  assert.equal(runStatusModel({ status: 'running', items, current: 99, merged: 0 }).lines[0].includes('2/2'), true, 'out-of-range index is clamped');
});

test('offline engine run: needs the fingerprint, merges the green prefix, stops at the first problem and never waits', () => {
  const eng = loadEngine();
  assert.equal(eng.queueRunPlan().startable, false, 'first PR (#421) conflicts');
  const plan = eng.queueRunPlan(['br-open-424', 'br-open-422']);
  assert.equal(plan.startable, true);
  assert.throws(() => eng.queueRun(['br-open-424', 'br-open-422'], 'yanlis'), /onaylanmadı/);
  assert.throws(() => eng.queueRun(['br-open-424', 'br-open-422'], ''), /onaylanmadı/);
  assert.equal(eng.queueRunStatus(), null, 'rejected start creates no run');
  // 424: CI sürüyor → bekleme yok, zaman aşımı gibi durur, hiçbir şey birleşmez
  const waiting = eng.queueRun(['br-open-424', 'br-open-422'], plan.fingerprint);
  assert.equal(waiting.status, 'timeout');
  assert.equal(waiting.merged, 0);
  // 424 yeşile dönünce: 424 birleşir, 422 (CI kırmızı) engel
  eng.fixCI('br-open-424');
  const plan2 = eng.queueRunPlan(['br-open-424', 'br-open-422']);
  const run = eng.queueRun(['br-open-424', 'br-open-422'], plan2.fingerprint);
  assert.equal(run.status, 'blocked');
  assert.equal(run.merged, 1);
  assert.match(run.stopReason, /CI başarısız/);
  assert.equal(eng.queueRunStatus().id, run.id);
  assert.throws(() => eng.queueRunStop(), /çalışan/);
});

// ---- koşu paneli: adım görünümü, kompakt satır, kaydırılabilir pencere ----
test('run steps: finished PRs are done, the current one is highlighted, the stop point is marked, the rest wait', () => {
  const items = [{ pr: 10, branch: 'feat/j' }, { pr: 11, branch: 'feat/k' }, { pr: 12, branch: 'feat/l' }];
  const states = (run) => runStatusModel({ items, ...run }).steps.map((s) => s.state).join(',');
  assert.equal(states({ status: 'running', merged: 1, current: 1 }), 'done,current,waiting');
  assert.equal(states({ status: 'completed', merged: 3, current: 3 }), 'done,done,done');
  assert.equal(states({ status: 'blocked', merged: 1 }), 'done,failed,waiting');
  assert.equal(states({ status: 'timeout', merged: 0 }), 'failed,waiting,waiting');
  assert.equal(states({ status: 'stopped', merged: 2 }), 'done,done,stopped');
  assert.equal(states({ status: 'interrupted', merged: 0 }), 'stopped,waiting,waiting');
  assert.equal(runStatusModel({ items, status: 'running', merged: 99 }).steps.length, 3, 'merged is clamped');
  assert.equal(runStatusModel({ items, status: 'running', merged: 0 }).counter, '0/3');
});

test('run phase follows the current PR\'s last event and the log is capped', () => {
  const items = [{ pr: 10, branch: 'feat/j' }, { pr: 11, branch: 'feat/k' }];
  const ev = (kind, branch) => ({ time: '2026-11-20T19:42:22+03:00', kind, branch, message: `${kind} ${branch}` });
  assert.equal(runStatusModel({ items, status: 'running', merged: 0, events: [ev('info', '')] }).phase, 'planlanıyor');
  assert.equal(runStatusModel({ items, status: 'running', merged: 1, events: [ev('merge', 'feat/j'), ev('update', 'feat/k')] }).phase, 'CI bekleniyor');
  assert.equal(runStatusModel({ items, status: 'completed', merged: 2 }).phase, '', 'only a running run has a phase');
  const many = Array.from({ length: 500 }, (_, i) => ev('wait', `b${i}`));
  assert.equal(runStatusModel({ items, status: 'running', merged: 0, events: many }).events.length, 60, 'log is capped (scrolls inside its box)');
});

test('compact line for a finished run is short and names the result', () => {
  const m = runStatusModel({ items: [{ pr: 7, branch: 'a' }, { pr: 8, branch: 'b' }, { pr: 9, branch: 'c' }], status: 'completed', merged: 3, events: [{ time: '2026-11-20T19:43:56+03:00', kind: 'stop', message: 'bitti' }] });
  assert.match(m.compactLine, /^Son koşu: Koşu bitti · 3\/3 PR · 19:43:56$/);
});

test('queue modal never outgrows the screen: body scrolls inside the card, head and close stay reachable', () => {
  const css = readFileSync(new URL('../../web/tasarim.css', import.meta.url), 'utf8');
  assert.match(css, /#queue-modal \.modal-card\s*\{[^}]*max-height:\s*calc\(100vh/);
  assert.match(css, /#queue-modal \.modal-card\s*\{[^}]*box-sizing:\s*border-box/, 'padding/border must be inside max-height (content-box overflowed by 38px)');
  assert.match(css, /#queue-modal \.modal-body\s*\{[^}]*overflow-y:\s*auto/);
  assert.match(css, /#queue-modal \.modal-foot\s*\{[^}]*position:\s*sticky/);
  assert.match(css, /\.queue-run-log\s*\{[^}]*max-height:[^}]*overflow-y:\s*auto/, 'the event log scrolls inside its own box');
});

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

// ---- kartlarda "kuyrukta" rozeti ----
import { queueRunBadge } from '../../web/modules/syncsteps.js';
import { openCardModel } from '../../web/modules/capsule.js';
import { hasPendingCI } from '../../web/modules/polling.js';
import { snapshotSignature } from '../../web/modules/utils.js';

const qrSnap = (extra = {}) => ({ repo: { queueRun: { total: 3, merged: 1, current: 1, phase: 'CI bekleniyor', items: [
  { branchId: 'a', branch: 'feat/a', pr: 1, state: 'done' }, { branchId: 'b', branch: 'feat/b', pr: 2, state: 'current' }, { branchId: 'c', branch: 'feat/c', pr: 3, state: 'waiting' }], ...extra } } });

test('queue badge: waiting PRs show their place, the current one shows its phase, finished/other PRs show nothing', () => {
  const snap = qrSnap();
  assert.equal(queueRunBadge(snap, { id: 'c', name: 'feat/c' }).text, '🚦 Kuyrukta 3/3');
  assert.equal(queueRunBadge(snap, { id: 'c', name: 'feat/c' }).tone, 'waiting');
  const cur = queueRunBadge(snap, { id: 'b', name: 'feat/b' });
  assert.equal(cur.text, '⏳ İşleniyor · CI bekleniyor');
  assert.equal(cur.tone, 'current');
  assert.equal(queueRunBadge(snap, { id: 'a', name: 'feat/a' }), null, 'merged PR is a merged pill, no tag');
  assert.equal(queueRunBadge(snap, { id: 'z', name: 'feat/z' }), null, 'a PR outside the run gets nothing');
  assert.equal(queueRunBadge({ repo: {} }, { id: 'c', name: 'feat/c' }), null, 'no run, no tag');
  assert.equal(queueRunBadge(snap, null), null);
  assert.equal(queueRunBadge(qrSnap({ phase: '' }), { id: 'b', name: 'feat/b' }).text, '⏳ İşleniyor · planlanıyor');
  assert.equal(queueRunBadge(snap, { id: 'x', name: 'feat/c' }).text, '🚦 Kuyrukta 3/3', 'matched by branch name too');
});

test('open card carries the queue tag (escaped) only when a badge is given', () => {
  const b = { id: 'c', name: 'feat/c', state: 'open', hasLocal: true, hasRemote: true, pr: { number: 3, ci: { state: 'SUCCESS' } } };
  const base = { tone: 'emerald', isSelected: false, isQueued: false };
  assert.ok(openCardModel(b, { ...base, queueRun: { text: '🚦 Kuyrukta 3/3', tone: 'waiting', title: 't' } }).html.includes('cap-queue-tag waiting'));
  assert.ok(!openCardModel(b, { ...base, queueRun: null }).html.includes('cap-queue-tag'));
  const evil = openCardModel(b, { ...base, queueRun: { text: '<img src=x onerror=1>', tone: 'waiting', title: '"><script>' } }).html;
  assert.ok(!evil.includes('<img') && !evil.includes('<script'), 'untrusted text is escaped');
});

test('a running queue keeps polling fast and changes the snapshot signature (cards refresh while the modal is closed)', () => {
  assert.equal(hasPendingCI(qrSnap()), true);
  assert.equal(hasPendingCI({ repo: {}, branches: [] }), false);
  const sigOf = (s) => snapshotSignature({ repo: { id: 'r', ...s.repo }, nodes: [], branches: [], logs: [] });
  assert.notEqual(sigOf(qrSnap()), sigOf({ repo: {} }));
  assert.notEqual(sigOf(qrSnap()), sigOf(qrSnap({ merged: 2, current: 2 })), 'progress changes the signature');
});

// ---- sarı kesik hedef çizgisi ----
import { queueLandingGeometry } from '../../web/modules/scene.js';
import { MAIN_Y, SPINE_STEP_X } from '../../web/modules/constants.js';

test('landing geometry: ghost node sits just right of the spine tip and the arrow ends next to it, from above or below', () => {
  const up = queueLandingGeometry({ apexX: 900, apexY: 80 }, 700);
  assert.equal(up.landingX, 700 + SPINE_STEP_X * 0.7);
  assert.equal(up.landingY, MAIN_Y);
  assert.ok(up.d.startsWith('M 900 80 C '), 'starts at the apex dot');
  assert.ok(up.d.endsWith(`${up.landingX} ${MAIN_Y - 8}`), 'an upper arc ends just above the ghost node');
  const down = queueLandingGeometry({ apexX: 900, apexY: 400 }, 700);
  assert.ok(down.d.endsWith(`${down.landingX} ${MAIN_Y + 8}`), 'a lower arc ends just below the ghost node');
  assert.ok(!up.d.includes('NaN') && !down.d.includes('NaN'));
});

// ---- tek dil: tekil "CI bitince merge et" da aynı niyet rozeti + oku alır ----
import { mergeIntentBadge } from '../../web/modules/syncsteps.js';

test('merge intent badge: a single "CI bitince merge et" PR gets the same tag language as a queued PR; no intent, no tag', () => {
  const b = { id: 'x', name: 'feat/x' };
  const single = mergeIntentBadge({ repo: {} }, b, { x: { branchId: 'x' } });
  assert.equal(single.tone, 'current', 'same tone as a queue run\'s processed PR (it also gets the arrow)');
  assert.match(single.text, /CI bitince birleşecek/);
  assert.equal(mergeIntentBadge({ repo: {} }, b, {}), null, 'an ordinary open PR gets nothing');
  assert.equal(mergeIntentBadge({ repo: {} }, b, undefined), null);
  assert.equal(mergeIntentBadge({ repo: {} }, null, { x: {} }), null);
  // kuyruk koşusu önceliklidir: aynı PR iki rozet almaz
  const inRun = mergeIntentBadge(qrSnap(), { id: 'c', name: 'feat/c' }, { c: { branchId: 'c' } });
  assert.equal(inRun.text, '🚦 Kuyrukta 3/3');
});

// ---- geride kalan PR: Merge yerine "Güncelle & Birleştir" ----
import { needsUpdateBeforeMerge } from '../../web/modules/syncsteps.js';
import { actionRowHtmlOf } from '../../web/modules/capsule.js';

const behindPR = (extra = {}) => ({ id: 'p', name: 'feat/p', state: 'open', behindMain: 2, pr: { number: 22, ci: { state: 'SUCCESS' } }, ...extra });
const strict = { repo: { strictUpToDate: true } };

test('needs-update: only a behind, conflict-free open PR in a repo that REQUIRES an up-to-date branch', () => {
  assert.equal(needsUpdateBeforeMerge(behindPR(), strict), true);
  assert.equal(needsUpdateBeforeMerge(behindPR(), { repo: { strictUpToDate: false } }), false, 'rule off: a behind PR merges fine');
  assert.equal(needsUpdateBeforeMerge(behindPR(), { repo: {} }), false, 'unknown rule is never assumed on');
  assert.equal(needsUpdateBeforeMerge(behindPR({ behindMain: 0 }), strict), false, 'up to date');
  assert.equal(needsUpdateBeforeMerge(behindPR({ conflict: { hasConflict: true } }), strict), false, 'a conflict is the Conflict Studio\'s job');
  assert.equal(needsUpdateBeforeMerge(behindPR({ pr: null }), strict), false, 'no PR');
  assert.equal(needsUpdateBeforeMerge(behindPR({ state: 'merged' }), strict), false);
  assert.equal(needsUpdateBeforeMerge(null, strict), false);
});

test('card merge button: "Güncelle & Birleştir" when behind under strict protection, the usual labels otherwise', () => {
  const open = behindPR({ hemisphere: 'upper', hasRemote: true });
  assert.match(actionRowHtmlOf(open, { needsUpdate: true }), /Güncelle &amp; Birleştir/);
  assert.ok(!actionRowHtmlOf(open, { needsUpdate: true }).includes('Merge (Parabolü Kapat)'));
  assert.match(actionRowHtmlOf(open, { needsUpdate: false }), /Merge \(Parabolü Kapat\)/);
  const running = behindPR({ hemisphere: 'upper', hasRemote: true, pr: { number: 22, ci: { state: 'RUNNING' } } });
  assert.match(actionRowHtmlOf(running, { needsUpdate: true, isQueued: false }), /Güncelle &amp; Birleştir/, 'behind + CI running: waiting is pointless, update first');
  assert.match(actionRowHtmlOf(running, { needsUpdate: false, isQueued: false }), /CI Bitince Merge Et/);
});

test('offline engine: applying a protection that requires an up-to-date branch sets repo.strictUpToDate', () => {
  const eng = loadEngine();
  assert.ok(!eng.snapshot().repo.strictUpToDate, 'unprotected demo: rule is not on');
  const plan = eng.protectionPlan('ci', ['test']);
  const snap = eng.applyProtection('ci', ['test'], plan.fingerprint);
  assert.equal(snap.repo.strictUpToDate, true);
});

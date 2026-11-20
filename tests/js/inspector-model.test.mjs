// Tests for the pure decisions in web/modules/inspector-model.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  checkConclusion, classifyCheck, CHECK_STUDIO_VIEW, CHECK_INSPECTOR_VIEW, branchViewModel, layerSummaries,
  isMergedRemnant, commitScope, workingTreeDefaults, promoteDefaults, proposalFormValues, buildApprovePayload,
} from '../../web/modules/inspector-model.js';

const open = (id, extra = {}) => ({ id, name: id, state: 'open', hemisphere: 'upper', hasLocal: false, hasRemote: false, aheadMain: 1, ...extra });
const snap = (repo = {}) => ({ repo: { mode: 'live', currentUser: 'ziya', dirtyCount: 0, ...repo } });

// ---------- CI checks ----------
test('classifyCheck: one rule for both panels', () => {
  for (const c of ['FAILURE', 'timed_out', 'Cancelled']) assert.equal(classifyCheck({ conclusion: c }), 'fail', c);
  for (const c of ['IN_PROGRESS', 'queued', 'PENDING']) assert.equal(classifyCheck({ status: c }), 'running', c);
  assert.equal(classifyCheck({ conclusion: 'skipped' }), 'skip');
  for (const c of ['success', 'SUCCESS', 'completed', '']) assert.equal(classifyCheck({ conclusion: c }), 'ok', c);
  assert.equal(classifyCheck({}), 'ok');
});

test('regression: PENDING is "running" in the studio panel too (it used to show as Başarılı there but 🟡 in the inspector)', () => {
  const view = CHECK_STUDIO_VIEW[classifyCheck({ status: 'PENDING' })];
  assert.deepEqual(view, { cls: 'running', text: 'Çalışıyor' });
  assert.equal(CHECK_INSPECTOR_VIEW[classifyCheck({ status: 'PENDING' })].icon, '🟡');
});

test('checkConclusion prefers conclusion over status and upper-cases', () => {
  assert.equal(checkConclusion({ conclusion: 'failure', status: 'completed' }), 'FAILURE');
  assert.equal(checkConclusion({ status: 'queued' }), 'QUEUED');
  assert.equal(checkConclusion({}), '');
});

test('view tables cover every class', () => {
  for (const k of ['ok', 'fail', 'running', 'skip']) {
    assert.ok(CHECK_STUDIO_VIEW[k].cls && CHECK_STUDIO_VIEW[k].text, k);
    assert.ok(CHECK_INSPECTOR_VIEW[k].icon && CHECK_INSPECTOR_VIEW[k].chip, k);
  }
});

// ---------- branch view model ----------
test('branchViewModel: open PR with green CI can merge; failing CI or conflict blocks it', () => {
  const green = open('a', { pr: { number: 1, reviewState: 'APPROVED', ci: { state: 'SUCCESS' } } });
  assert.equal(branchViewModel(green, snap()).canMerge, true);
  assert.equal(branchViewModel({ ...green, pr: { ...green.pr, ci: { state: 'FAILURE' } } }, snap()).canMerge, false);
  assert.equal(branchViewModel({ ...green, conflict: { hasConflict: true, resolved: false } }, snap()).canMerge, false);
  assert.equal(branchViewModel({ ...green, conflict: { hasConflict: true, resolved: true } }, snap()).canMerge, true, 'resolved conflicts do not block');
  assert.equal(branchViewModel({ ...green, pr: { ...green.pr, reviewState: 'MERGED' } }, snap()).hasOpenPR, false);
});

test('branchViewModel: virtual working tree, remote-only, queued auto-merge, live flag', () => {
  const wip = branchViewModel(open('local-wip-working-tree', { hemisphere: 'lower', hasLocal: true }), snap());
  assert.equal(wip.isVirtualWip, true);
  assert.equal(wip.canCommitWorkingTree, true);
  assert.equal(wip.canPromote, false);
  assert.equal(wip.title, '✏️ Çalışma Alanı (Taslak)');
  assert.equal(branchViewModel(open('r', { hasRemote: true }), snap()).isRemoteOnly, true);
  assert.equal(branchViewModel(open('r', { hasRemote: true, hasLocal: true }), snap()).isRemoteOnly, false);
  assert.equal(branchViewModel(open('q'), snap(), { pendingAutoMerge: { q: true } }).isQueued, true);
  // Sunucu koşusundaki PR de "kuyrukta" sayılır (yenilemeden sonra da sürer).
  const sq = snap();
  sq.repo.queueRun = { total: 1, phase: 'CI bekleniyor', items: [{ branchId: 'q', branch: 'q', state: 'current' }] };
  assert.equal(branchViewModel(open('q'), sq, {}).isQueued, true);
  assert.equal(branchViewModel(open('q'), snap(), {}).isQueued, false);
  assert.equal(branchViewModel(open('q'), snap({ mode: 'simulation' })).isLive, false);
  assert.equal(branchViewModel(open('plain-name'), snap()).title, 'plain-name');
});

test('branchViewModel: promote vs synced vs commit-working-tree are mutually consistent', () => {
  const ahead = branchViewModel(open('l', { hemisphere: 'lower', hasLocal: true, aheadMain: 2, uncommittedCount: 0 }), snap());
  assert.deepEqual([ahead.canPromote, ahead.isAlreadySynced, ahead.canCommitWorkingTree], [true, false, false]);
  const synced = branchViewModel(open('l', { hemisphere: 'lower', hasLocal: true, aheadMain: 0, uncommittedCount: 0 }), snap());
  assert.deepEqual([synced.canPromote, synced.isAlreadySynced], [false, true]);
  const dirty = branchViewModel(open('l', { hemisphere: 'lower', hasLocal: true, aheadMain: 2, uncommittedCount: 3 }), snap());
  assert.deepEqual([dirty.canCommitWorkingTree, dirty.canPromote], [true, false], 'uncommitted work must be committed first');
  const headDirtyRepo = branchViewModel(open('h', { hemisphere: 'lower', hasLocal: true, isHead: true, aheadMain: 2 }), snap({ dirtyCount: 4 }));
  assert.equal(headDirtyRepo.canCommitWorkingTree, true, 'HEAD inherits the repo-wide dirty count');
});

test('branchViewModel: author badge — me vs peer, case-insensitive, trimmed', () => {
  assert.equal(branchViewModel(open('a', { author: ' Ziya ' }), snap()).isMe, true);
  assert.ok(!branchViewModel(open('a', { author: 'bob' }), snap()).isMe);
  assert.ok(!branchViewModel(open('a', { author: 'ziya' }), snap({ currentUser: '' })).isMe);
});

test('layerSummaries: upper/lower accordion headings', () => {
  const s = snap({ dirtyCount: 2 });
  assert.equal(layerSummaries(open('a', { pr: { number: 7, reviewState: 'APPROVED' } }), s, {}).upperSummary, 'PR #7 · APPROVED');
  assert.equal(layerSummaries(open('a'), s, { isRemoteOnly: true }).upperSummary, 'origin/a');
  assert.equal(layerSummaries(open('a', { originGone: true }), s, {}).upperSummary, 'origin: gone');
  assert.equal(layerSummaries(open('a'), s, {}).upperSummary, 'PR Yok');
  assert.equal(layerSummaries(open('a', { hasLocal: true, isHead: true }), s, {}).lowerSummary, 'HEAD (Aktif)');
  assert.equal(layerSummaries(open('a', { hasLocal: true, aheadMain: 3, behindMain: 1 }), s, {}).lowerSummary, '↑3 / ↓1');
  assert.equal(layerSummaries(open('a'), s, {}).lowerSummary, 'Yerelde Yok');
  assert.equal(layerSummaries(open('w', { uncommittedCount: 5 }), s, { isVirtualWip: true }).lowerSummary, '5 dosya bekliyor');
  assert.equal(layerSummaries(open('w'), s, { isVirtualWip: true }).lowerSummary, '2 dosya bekliyor');
});

test('isMergedRemnant: merged, gone, origin-gone or MERGED review', () => {
  assert.equal(isMergedRemnant(open('a', { state: 'merged' })), true);
  assert.equal(isMergedRemnant(open('a', { originGone: true })), true);
  assert.equal(isMergedRemnant(open('a', { pr: { reviewState: 'MERGED' } })), true);
  assert.equal(isMergedRemnant(open('a')), false);
});

// ---------- commit studio defaults ----------
test('commitScope maps the first changed file to a Conventional Commit type and scope', () => {
  assert.deepEqual(commitScope('internal/mcp/server.go'), { prefix: 'feat', scope: '(mcp)' });
  assert.deepEqual(commitScope('web/app.js'), { prefix: 'feat', scope: '(ui)' });
  assert.deepEqual(commitScope('docs/PLAN.md'), { prefix: 'docs', scope: '(docs)' });
  assert.deepEqual(commitScope('README.md'), { prefix: 'docs', scope: '(docs)' });
  assert.deepEqual(commitScope('internal/x_test.go'), { prefix: 'test', scope: '(test)' });
  assert.deepEqual(commitScope('.github/workflows/ci.yml'), { prefix: 'ci', scope: '(ci)' });
  assert.deepEqual(commitScope('main.go'), { prefix: 'feat', scope: '' });
  assert.deepEqual(commitScope(undefined), { prefix: 'feat', scope: '' });
});

test('workingTreeDefaults: the virtual working tree derives branch, message and PR body from the files', () => {
  const files = ['web/app.js', 'web/modules/x.js', 'README.md'];
  const d = workingTreeDefaults(open('local-wip-working-tree', { name: 'wip/working-tree' }), files, { branches: [] }, true);
  assert.match(d.defaultWtBranch, /^feat\//);
  assert.equal(d.branchPlaceholder, 'Örn: ' + d.defaultWtBranch);
  assert.equal(d.defaultCommitMsg, 'feat(ui): 3 dosya (app.js, x.js...) güncellemeleri ve iyileştirmeler');
  assert.equal(d.defaultPrTitle, d.defaultCommitMsg);
  assert.match(d.defaultPrBody, /^## Neler Değişti\?\n- `web\/app\.js` üzerinde geliştirmeler\n- /);
  assert.match(d.defaultPrBody, /## Doğrulama/);
  const one = workingTreeDefaults(open('w', { name: 'wip/working-tree' }), ['a.go'], { branches: [] }, true);
  assert.match(one.defaultCommitMsg, /1 dosya \(a\.go\)/);
  const none = workingTreeDefaults(open('w', { name: 'wip/working-tree' }), [], { branches: [] }, true);
  assert.equal(none.defaultCommitMsg, 'feat: yeni özellik geliştirmeleri');
});

test('workingTreeDefaults: a real branch keeps its own name; auto-generated "update N files" messages are dropped', () => {
  const b = open('feat/mine', { lastCommitMsg: 'update 5 files', draftPrTitle: 'update 3 files', draftPrBody: 'my body' });
  const d = workingTreeDefaults(b, ['a.go'], { branches: [] }, false);
  assert.equal(d.defaultWtBranch, 'feat/mine');
  assert.equal(d.branchPlaceholder, 'feat/ozellik-adi');
  assert.equal(d.defaultCommitMsg, '');
  assert.equal(d.defaultPrTitle, '');
  assert.equal(d.defaultPrBody, 'my body');
  assert.equal(workingTreeDefaults(open('x', { lastCommitMsg: 'Pushlanmamış yerel çalışma 3' }), [], {}, false).defaultCommitMsg, '');
  assert.equal(workingTreeDefaults(open('x', { lastCommitMsg: 'feat: real' }), [], {}, false).defaultPrTitle, 'feat: real');
});

test('promoteDefaults: user draft wins (even an empty one), then the branch draft, then the last commit, then a fallback', () => {
  const b = open('feat/p', { draftPrTitle: 'D', draftPrBody: 'DB', lastCommitMsg: 'L' });
  assert.deepEqual(promoteDefaults(b, { title: 'mine', body: 'mine body' }), { title: 'mine', body: 'mine body' });
  assert.deepEqual(promoteDefaults(b, { title: '', body: '' }), { title: '', body: '' }, 'clearing the field is respected');
  assert.deepEqual(promoteDefaults(b, undefined), { title: 'D', body: 'DB' });
  assert.deepEqual(promoteDefaults(open('feat/p', { lastCommitMsg: 'L' }), undefined), { title: 'L', body: '' });
  assert.deepEqual(promoteDefaults(open('feat/p'), undefined), { title: 'feat: feat/p', body: '' });
});

// ---------- AI proposal ----------
const prop = { id: 'p1', branchName: 'feat/ai', message: 'feat: ai', prTitle: 'T', prBody: 'B' };

test('proposalFormValues: draft edits apply only to the same proposal', () => {
  assert.deepEqual(proposalFormValues(prop, null), { branch: 'feat/ai', message: 'feat: ai', title: 'T', body: 'B' });
  const draft = { id: 'p1', branch: 'x', message: 'y', title: 'z', body: 'w' };
  assert.deepEqual(proposalFormValues(prop, draft), { branch: 'x', message: 'y', title: 'z', body: 'w' });
  assert.deepEqual(proposalFormValues(prop, { ...draft, id: 'other' }), { branch: 'feat/ai', message: 'feat: ai', title: 'T', body: 'B' });
  assert.deepEqual(proposalFormValues({ ...prop, prTitle: undefined, prBody: undefined }, null).title, '');
});

test('buildApprovePayload validates in order (branch first) and builds the payload', () => {
  assert.deepEqual(buildApprovePayload(prop, { branch: '', message: '', title: 't', body: 'b' }), { error: 'Lütfen geçerli bir dal adı girin' });
  assert.deepEqual(buildApprovePayload(prop, { branch: 'feat/x', message: '', title: 't', body: 'b' }), { error: 'Commit mesajı boş olamaz' });
  assert.deepEqual(buildApprovePayload(prop, { branch: 'feat/x', message: 'm', title: 't', body: 'b' }), {
    payload: { proposalId: 'p1', branchName: 'feat/x', message: 'm', prTitle: 't', prBody: 'b' },
  });
});

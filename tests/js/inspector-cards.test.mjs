// Tests for the HTML cards (web/modules/inspector-cards.js) and the studio panels (web/modules/studio.js). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { branchViewModel } from '../../web/modules/inspector-model.js';
import {
  nodeInspectorHtml, nodeStudioHtml, proposalCardHtml, branchHeaderHtml, layersHtml, conflictCardHtml, crossPrCardHtml,
  ciCardHtml, mergedRemnantCardHtml, commitStudioCardHtml, promoteCardHtml, syncedCardHtml, changedFilesCardHtml,
  prEditCardHtml, prDetailCardHtml, amendCardHtml, suggestedCmdCardHtml,
} from '../../web/modules/inspector-cards.js';
import {
  diffLineClass, diffLinesHtml, diffPanelHtml, studioCiStepsHtml, studioActionsHtml, studioBranchTypeLabel, studioDetailHtml,
  STUDIO_ACTION_IDS, dispatchStudioAction,
} from '../../web/modules/studio.js';
import { isWorkingTreeBranch } from '../../web/modules/utils.js';

const open = (id, extra = {}) => ({ id, name: id, state: 'open', hemisphere: 'upper', hasLocal: true, hasRemote: false, aheadMain: 1, ...extra });
const pr = (n, ci, extra = {}) => ({ number: n, title: 'T', reviewState: 'APPROVED', ...(ci ? { ci } : {}), ...extra });
const snapOf = (repo = {}) => ({ repo: { mode: 'live', currentUser: 'ziya', ...repo }, branches: [] });
const ids = (html) => [...html.matchAll(/\bid="([^"]+)"/g)].map((m) => m[1]);
const EVIL = '<img src=x onerror=alert(1)>';

function header(b, snap = snapOf(), st = {}) {
  const v = branchViewModel(b, snap, st);
  return branchHeaderHtml({ ...v, proposalHtml: '', mergeStrategy: st.mergeStrategy || 'merge' });
}

// ---------- commit node ----------
test('node cards: commit details, tag chip, merge flag and a diff button, all escaped', () => {
  const node = { shortSha: 'a1b2c3d', subject: `fix ${EVIL}`, author: 'ziya', relativeTime: '2 g', tag: 'v1.0', isMerge: true };
  const html = nodeInspectorHtml(node);
  assert.match(html, /Commit a1b2c3d/);
  assert.match(html, /chip accent">v1\.0</);
  assert.match(html, /Merge Node/);
  assert.deepEqual(ids(html), ['act-node-diff']);
  assert.doesNotMatch(html, /<img/);
  const plain = nodeInspectorHtml({ ...node, tag: '', isMerge: false });
  assert.doesNotMatch(plain, /chip accent|Merge Node/);
  assert.deepEqual(ids(nodeStudioHtml(node)), ['studio-act-node-diff']);
  assert.match(nodeStudioHtml({ ...node, tag: '' }), /Spine/);
});

// ---------- header / actions ----------
test('branchHeaderHtml: which action buttons exist in each situation', () => {
  assert.deepEqual(ids(header(open('a', { pr: pr(1, { state: 'SUCCESS' }), hasRemote: true }))).filter((i) => i.startsWith('act-') || i === 'merge-strategy'),
    ['act-branch-diff', 'act-checkout', 'merge-strategy', 'act-merge', 'act-delete']);
  assert.ok(ids(header(open('h', { isHead: true, hemisphere: 'lower', aheadMain: 0, uncommittedCount: 0 }))).includes('act-checkout-main'), 'synced HEAD offers main');
  assert.ok(ids(header(open('l', { hemisphere: 'lower', aheadMain: 2 }))).includes('act-promote'));
  assert.ok(!ids(header(open('l', { hemisphere: 'lower', aheadMain: 2, pr: pr(3) }))).includes('act-promote'), 'already has a PR');
  assert.ok(ids(header(open('c', { conflict: { hasConflict: true } }))).includes('act-resolve'));
  const wip = ids(header(open('local-wip-working-tree', { hemisphere: 'lower' })));
  assert.deepEqual(wip.filter((i) => i.startsWith('act-')), ['act-branch-diff'], 'working tree: diff only');
});

test('branchHeaderHtml: merge button follows CI state; Team mode locks checkout', () => {
  const run = header(open('a', { pr: pr(1, { state: 'RUNNING' }) }));
  assert.match(run, /CI Bitince Otomatik Merge Et/);
  assert.match(header(open('a', { pr: pr(1, { state: 'RUNNING' }) }), snapOf(), { pendingAutoMerge: { a: true } }), /CI Bekleniyor \(İptal Et\)[\s\S]*|running-pulse/);
  const fail = header(open('a', { pr: pr(1, { state: 'FAILURE' }) }));
  assert.match(fail, /id="act-merge" disabled/);
  assert.match(fail, /merge kilidi aktif/);
  assert.doesNotMatch(header(open('a', { pr: pr(1, { state: 'SUCCESS' }) })), /id="act-merge" disabled/);
  assert.match(header(open('a'), snapOf({ teamMode: true })), /id="act-checkout" disabled title="Team Server modunda/);
  assert.match(header(open('a', { hasRemote: true, hasLocal: false })), /Yerelde İndir &amp; Geç/);
  assert.match(header(open('a', { hasRemote: true, hasLocal: false })), /PR Kapat &amp; Uzak Dalı Sil/);
});

test('branchHeaderHtml: merge strategy select marks the saved choice and escapes hostile names', () => {
  const html = header(open('a', { pr: pr(1) }), snapOf(), { mergeStrategy: 'squash' });
  assert.match(html, /<option value="squash" selected>/);
  assert.doesNotMatch(html, /<option value="merge" selected>/);
  assert.doesNotMatch(header(open(EVIL, { author: EVIL })), /<img/);
  assert.match(header(open('a', { author: 'ziya' })), /\(Ben\)/);
  assert.doesNotMatch(header(open('a', { author: 'bob' })), /\(Ben\)/);
});

test('layersHtml: upper/lower accordions with the right facts', () => {
  const b = open('a', { pr: pr(5, { workflow: 'CI', duration: '2m' }), localSha: 'abc', aheadMain: 2, behindMain: 1 });
  const html = layersHtml(branchViewModel(b, snapOf()));
  assert.match(html, /PR #5 · APPROVED/);
  assert.match(html, /CI: CI \(2m\)/);
  assert.match(html, /main farkı: ↑2 \/ ↓1/);
  const gone = layersHtml(branchViewModel(open('g', { originGone: true, hasLocal: false, state: 'merged' }), snapOf()));
  assert.match(gone, /Yerelde dal yok|Yerel kalıntı dal temizlendi/);
  assert.doesNotMatch(layersHtml(branchViewModel(open(EVIL, { hasLocal: false, hasRemote: true }), snapOf())), /<img/);
});

// ---------- diagnostics cards ----------
test('conflict / cross-PR cards', () => {
  const lock = conflictCardHtml(open('a', { conflict: { hasConflict: true, resolved: false, isChangelogOrLock: true, files: ['CHANGELOG.md'], previewUnified: '<<<<' } }));
  assert.match(lock, /CHANGELOG \/ Lockfile Çakışması/);
  assert.match(lock, /id="act-resolve-card"/);
  assert.match(lock, /&lt;&lt;&lt;&lt;/);
  const fixed = conflictCardHtml(open('a', { conflict: { hasConflict: true, resolved: true, resolvedNote: 'ok' } }));
  assert.match(fixed, /Çakışma Çözüldü/);
  assert.doesNotMatch(fixed, /act-resolve-card/);
  const cross = crossPrCardHtml(open('a', { crossPr: [{ otherPrNumber: 9, otherBranch: 'feat/o', sharedFiles: ['a.go', 'b.go'] }, { otherBranch: EVIL, sharedFiles: ['c.go'] }] }));
  assert.match(cross, /PR #9 \(feat\/o\)<\/strong>: a\.go, b\.go/);
  assert.doesNotMatch(cross, /<img/);
});

test('ciCardHtml: failing CI shows findings, repro and the local-run/copy buttons; fix button only in simulation', () => {
  const b = open('a', { pr: pr(1, { state: 'FAILURE', failedStep: 'test', rawLog: `RAW ${EVIL}`, extracts: [{ kind: 'test_fail', symbol: 'TestX', fileLine: 'a_test.go:9', message: 'm', reproCmd: 'go test -run TestX' }],
    checks: [{ name: 'test', conclusion: 'failure', detailsUrl: 'https://x/y?a=1&b=2' }, { name: 'lint', conclusion: 'success' }, { name: 'e2e', status: 'pending' }, { name: 'docs', conclusion: 'skipped' }] }) });
  const live = ciCardHtml(b, { isLive: true, localRunOut: '' });
  assert.deepEqual(ids(live).filter((i) => i.startsWith('act-')), ['act-copy-log', 'act-run-local'], 'no fix-CI button on a live repo');
  assert.ok(ids(ciCardHtml(b, { isLive: false, localRunOut: '' })).includes('act-fix-ci'));
  assert.match(live, /Patlayan Adım: test/);
  assert.match(live, /\$ go test -run TestX/);
  assert.match(live, /href="https:\/\/x\/y\?a=1&amp;b=2"/);
  assert.doesNotMatch(live, /<img/);
  // per-check icons from the single classifier: fail ❌, ok ✅, pending 🟡, skipped ⚪
  for (const icon of ['❌', '✅', '🟡', '⚪']) assert.ok(live.includes(`ci-check-icon">${icon}<`), icon);
  assert.match(ciCardHtml(b, { isLive: true, localRunOut: 'LOCAL OUT' }), /id="ci-raw-log">LOCAL OUT</);
});

test('ciCardHtml: running and successful CI cards', () => {
  assert.match(ciCardHtml(open('a', { pr: pr(1, { state: 'RUNNING' }) }), { isLive: true, localRunOut: '' }), /ci-running[\s\S]*Kontroller Devam Ediyor/);
  const ok = ciCardHtml(open('a', { pr: pr(1, { state: 'SUCCESS', workflow: 'ci', duration: '1m' }) }), { isLive: true, localRunOut: '' });
  assert.match(ok, /ci-ok/);
  assert.match(ok, /Tüm Kontroller Başarılı/);
  assert.doesNotMatch(ok, /act-run-local/);
});

// ---------- working tree / promote / synced ----------
test('commitStudioCardHtml: secret files and Team mode disable committing', () => {
  const b = open('wip', { id: 'local-wip-working-tree', pr: null });
  const d = { secretFile: '.env', candidateFiles: ['.env', 'a.go'], defaultWtBranch: 'feat/x', branchPlaceholder: 'p', defaultCommitMsg: 'm', defaultPrTitle: 't', defaultPrBody: 'b' };
  const blocked = commitStudioCardHtml(b, snapOf({ secretPatterns: ['.env*'] }), d);
  assert.match(blocked, /GÜVENLİK ENGELİ/);
  assert.match(blocked, /id="act-wt-commit-only" disabled/);
  assert.match(blocked, /id="act-wt-commit-push" disabled/);
  assert.match(blocked, /🛡️ \.env/);
  const fine = commitStudioCardHtml(b, snapOf(), { ...d, secretFile: undefined, candidateFiles: ['a.go'] });
  assert.doesNotMatch(fine, /GÜVENLİK ENGELİ/);
  assert.doesNotMatch(fine, /id="act-wt-commit-only" disabled/);
  assert.match(fine, /1 Dosya Bekliyor/);
  const team = commitStudioCardHtml(b, snapOf({ teamMode: true }), { ...d, secretFile: undefined, candidateFiles: ['a.go'] });
  assert.match(team, /id="wt-branch-name"[^>]* disabled/);
  assert.match(team, /id="act-wt-discard" disabled/);
  assert.match(commitStudioCardHtml({ ...b, pr: { number: 7 } }, snapOf(), { ...d, secretFile: undefined }), /PR #7&#39;a Pushla|PR #7'a Pushla/);
});

test('mergedRemnantCardHtml / promoteCardHtml / syncedCardHtml', () => {
  const rem = mergedRemnantCardHtml(open('m', { hasLocal: true }), ['a.go', 'b.go']);
  assert.match(rem, /2 Dosya Bekliyor/);
  assert.deepEqual(ids(rem), ['act-clean-and-switch-main', 'act-open-new-branch-modal', 'act-wt-discard']);
  assert.ok(!ids(mergedRemnantCardHtml(open('m', { hasLocal: false }), [])).includes('act-clean-and-switch-main'));
  const promote = promoteCardHtml(open('p'), `T ${EVIL}`, 'B');
  assert.deepEqual(ids(promote), ['form-promote-pr', 'promote-pr-title', 'promote-pr-body', 'act-promote-submit']);
  assert.doesNotMatch(promote, /<img/);
  assert.deepEqual(ids(syncedCardHtml(open('s', { isHead: true }))), ['act-checkout-main-synced', 'act-delete-synced']);
  assert.deepEqual(ids(syncedCardHtml(open('s', { isHead: false }))), ['act-delete-synced']);
});

test('changed files / PR cards / amend / suggested command', () => {
  const files = changedFilesCardHtml(open('a', { changedFiles: ['CHANGELOG.md', 'go.mod', 'x.go', `${EVIL}.go`] }));
  assert.match(files, /Değişen Dosyalar \(4\)/);
  assert.equal((files.match(/file-tag special/g) || []).length, 2, 'changelog and go.mod are special');
  assert.doesNotMatch(files, /<img/);
  assert.deepEqual(ids(prEditCardHtml(open('a', { pr: pr(7) }))), ['form-edit-pr', 'pr-edit-title', 'pr-edit-body']);
  const detail = prDetailCardHtml(open('a', { state: 'merged', pr: pr(8, { state: 'SUCCESS', workflow: 'ci' }, { author: 'bob', body: `B ${EVIL}`, reviewState: 'MERGED' }) }));
  assert.match(detail, /@bob/);
  assert.match(detail, /CI: SUCCESS/);
  assert.doesNotMatch(detail, /<img/);
  assert.match(prDetailCardHtml(open('a', { pr: pr(8, null, { body: '' }) })), /GitHub açıklaması kaydedilmemiş/);
  assert.deepEqual(ids(amendCardHtml(open('a', { lastCommitMsg: 'feat: "q"' }))), ['form-amend-commit', 'commit-amend-msg']);
  assert.match(amendCardHtml(open('a', { lastCommitMsg: 'feat: "q"' })), /value="feat: &quot;q&quot;"/);
  assert.deepEqual(ids(suggestedCmdCardHtml(open('a', { suggestedCmd: 'git switch a' }))), ['act-copy-cmd']);
});

test('proposalCardHtml: draft edits are used only for the same proposal; everything escaped', () => {
  const prop = { id: 'p1', branchName: 'feat/ai', message: 'msg', prTitle: 'T', prBody: 'B', baseBranch: 'origin/main', files: ['a.go', EVIL], diffStat: ' a | 1', shieldResult: '', countCheckMatch: false, countCheckMessage: 'sayı uyuşmuyor', createdAt: '12:00' };
  const base = proposalCardHtml(prop, null);
  assert.match(base, /id="prop-edit-branch" value="feat\/ai"/);
  assert.deepEqual(ids(base).filter((i) => i.startsWith('act-')), ['act-proposal-approve', 'act-proposal-dismiss']);
  assert.match(base, /layer-line warn/);
  assert.match(base, /sayı uyuşmuyor/);
  assert.match(base, /0 secret, 0 özel anahtar/);
  assert.doesNotMatch(base, /<img/);
  const edited = proposalCardHtml(prop, { id: 'p1', branch: 'my/branch', message: 'my msg', title: 'my title', body: 'my body' });
  assert.match(edited, /value="my\/branch"/);
  assert.match(edited, />my msg<\/textarea>/);
  assert.match(proposalCardHtml(prop, { id: 'OTHER', branch: 'nope' }), /value="feat\/ai"/);
  assert.doesNotMatch(proposalCardHtml({ ...prop, diffStat: '' }, null), /git diff --stat/);
});

// ---------- studio panels ----------
test('diffLineClass and diffLinesHtml: classes, escaping and the 150-line cap', () => {
  assert.equal(diffLineClass('+a'), 'add');
  assert.equal(diffLineClass('-a'), 'del');
  assert.equal(diffLineClass('@@ -1 +1 @@'), 'hdr');
  assert.equal(diffLineClass(' ctx'), 'ctx');
  assert.equal(diffLineClass(''), 'ctx');
  const html = diffLinesHtml(`@@ h\n+${EVIL}\n-x\n` + 'y\n'.repeat(500));
  assert.equal((html.match(/studio-diff-line/g) || []).length, 150);
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /studio-diff-line hdr">@@ h/);
});

test('diffPanelHtml: file head with additions/deletions and escaped path', () => {
  const html = diffPanelHtml({ path: `a/${EVIL}.go`, additions: 3, deletions: 1 }, '<div>L</div>');
  assert.match(html, /\(\+3 \/ -1\)/);
  assert.match(html, /<div>L<\/div>/);
  assert.doesNotMatch(html, /<img/);
  assert.match(diffPanelHtml({ path: 'a.go' }, ''), /\(\+0 \/ -0\)/);
});

test('studioCiStepsHtml: real checks use the shared classifier; summary-only CI shows ONLY the known summary (no invented steps); no CI shows nothing', () => {
  const checks = studioCiStepsHtml(open('a', { pr: pr(1, { state: 'FAILURE', checks: [
    { name: 'build', status: 'IN_PROGRESS' }, { name: 'lint', status: 'PENDING' }, { name: 'e2e', status: 'QUEUED' }, { name: 'z', conclusion: 'failure' }, { name: 'd', conclusion: 'skipped' }, { name: 'ok', conclusion: 'success' }] }) }));
  assert.equal((checks.match(/studio-step-badge running">Çalışıyor/g) || []).length, 3, 'IN_PROGRESS, PENDING and QUEUED all run');
  assert.match(checks, /badge fail">Hatalı/);
  assert.match(checks, /badge skip">Atlandı/);
  assert.match(checks, /badge ok">Başarılı/);
  const failSummary = studioCiStepsHtml(open('a', { pr: pr(1, { state: 'FAILURE', workflow: 'CI' }) }));
  assert.match(failSummary, /studio-step-name">CI</);
  assert.match(failSummary, /badge fail">Hatalı/);
  assert.match(studioCiStepsHtml(open('a', { pr: pr(1, { state: 'RUNNING' }) })), /badge running">Çalışıyor/);
  assert.match(studioCiStepsHtml(open('a', { pr: pr(1, { state: 'SUCCESS' }) })), /badge ok">Başarılı/);
  for (const st of ['FAILURE', 'SUCCESS', 'RUNNING']) {
    const html = studioCiStepsHtml(open('a', { pr: pr(1, { state: st }) }));
    assert.doesNotMatch(html, /lint|build|\d+s</, `no fabricated steps or durations (${st})`);
  }
  assert.equal(studioCiStepsHtml(open('a')), '');
});

test('studioActionsHtml: buttons per branch kind (the working-tree commit button exists in the markup)', () => {
  assert.deepEqual(ids(studioActionsHtml(open('m', { state: 'merged', hasLocal: true }))), ['studio-act-diff', 'studio-act-del']);
  assert.deepEqual(ids(studioActionsHtml(open('m', { state: 'merged', hasLocal: false }))), ['studio-act-diff']);
  assert.deepEqual(ids(studioActionsHtml(open('local-wip-working-tree', { hemisphere: 'lower' }))), ['studio-act-commit', 'studio-act-diff'], 'the real working-tree branch (by id) gets the commit button');
  assert.deepEqual(ids(studioActionsHtml(open('x', { name: 'wip/working-tree' }))), ['studio-act-commit', 'studio-act-diff'], 'recognised by name too');
  assert.deepEqual(ids(studioActionsHtml(open('a', { isWorkingTreeWip: true }))), ['studio-act-checkout', 'studio-act-promote', 'studio-act-diff', 'studio-act-del'], 'the old never-set property no longer matters');
  assert.deepEqual(ids(studioActionsHtml(open('a', { pr: pr(1) }))), ['studio-act-checkout', 'studio-act-merge', 'studio-act-diff', 'studio-act-del']);
  assert.deepEqual(ids(studioActionsHtml(open('a', { isHead: true }))), ['studio-act-checkout-main', 'studio-act-promote', 'studio-act-diff', 'studio-act-del']);
});

test('studioDetailHtml / studioBranchTypeLabel', () => {
  assert.equal(studioBranchTypeLabel(open('a')), '☁️ Üst Ray · PR');
  assert.equal(studioBranchTypeLabel(open('a', { hemisphere: 'lower' })), '💻 Alt Ray · Yerel');
  assert.equal(studioBranchTypeLabel(open('local-wip-working-tree', { hemisphere: 'lower' })), '✏️ Alt Ray · Çalışma');
  const html = studioDetailHtml(open('a-very-long-branch-name-that-needs-cutting', { author: EVIL, localSha: 'abcdef1234', pr: pr(1, { state: 'SUCCESS' }) }));
  assert.match(html, /abcdef1/);
  assert.doesNotMatch(html, /<img/);
  assert.match(studioDetailHtml(open('a')), /HEAD/, 'falls back to HEAD when no sha is known');
});

// ---------- working tree detection and studio action routing ----------
test('isWorkingTreeBranch: the virtual working tree and local-wip-* drafts, nothing else', () => {
  assert.equal(isWorkingTreeBranch({ id: 'local-wip-working-tree', name: 'wip/working-tree' }), true);
  assert.equal(isWorkingTreeBranch({ id: 'local-wip-foo', name: 'foo (yerel)' }), true);
  assert.equal(isWorkingTreeBranch({ id: 'x', name: 'wip/working-tree' }), true);
  assert.equal(isWorkingTreeBranch({ id: 'br-local-wip', name: 'wip/keychain-probe' }), false, 'a normal branch that merely starts with wip/ is a real branch');
  assert.equal(isWorkingTreeBranch({ id: 'feat', name: 'feat/x', isWorkingTreeWip: true }), false, 'the old never-set property is not a signal');
  assert.equal(isWorkingTreeBranch(null), false);
  assert.equal(isWorkingTreeBranch({}), false);
});

test('dispatchStudioAction: every studio button maps to a callback — including the working-tree commit button (it used to be dead)', () => {
  const calls = [];
  const rec = (name) => (...a) => calls.push([name, ...a]);
  const cb = { onCheckoutBranch: rec('checkout'), onMergeBranch: rec('merge'), onPromoteBranch: rec('promote'), onDeleteBranch: rec('delete'), onOpenDiff: rec('diff'), onSelectBranch: rec('select') };
  const b = { id: 'b1', name: 'feat/b1' };
  const wip = { id: 'local-wip-working-tree', name: 'wip/working-tree' };
  const btn = { tag: 'btn' };
  dispatchStudioAction('studio-act-checkout', b, cb, btn);
  dispatchStudioAction('studio-act-checkout-main', b, cb, btn);
  dispatchStudioAction('studio-act-merge', b, cb, btn);
  dispatchStudioAction('studio-act-promote', b, cb, btn);
  dispatchStudioAction('studio-act-del', b, cb, btn);
  dispatchStudioAction('studio-act-diff', b, cb, btn);
  dispatchStudioAction('studio-act-diff', wip, cb, btn);
  dispatchStudioAction('studio-act-commit', wip, cb, btn);
  dispatchStudioAction('nope', b, cb, btn);
  assert.deepEqual(calls, [
    ['checkout', 'b1', btn], ['checkout', 'main', btn], ['merge', 'b1', btn], ['promote', 'b1', undefined, undefined, btn], ['delete', 'b1', btn],
    ['diff', 'feat/b1', 'branch'], ['diff', 'working-tree', 'working-tree'], ['select', 'local-wip-working-tree'],
  ]);
  assert.doesNotThrow(() => dispatchStudioAction('studio-act-commit', wip, {}, btn), 'missing callbacks are ignored');
});

test('every button id the studio markup can produce has a dispatcher entry', () => {
  const produced = new Set();
  const samples = [open('m', { state: 'merged', hasLocal: true }), open('local-wip-working-tree', { hemisphere: 'lower' }), open('a', { pr: pr(1) }), open('h', { isHead: true })];
  for (const b of samples) for (const id of ids(studioActionsHtml(b))) produced.add(id);
  for (const id of produced) assert.ok(STUDIO_ACTION_IDS.includes(id), `${id} has no listener (dead button)`);
});

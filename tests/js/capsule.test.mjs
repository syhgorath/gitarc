// Tests for the pure models in web/modules/capsule.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MERGED_FO_W, MERGED_FO_H, FO_W, FO_H } from '../../web/modules/constants.js';
import {
  capsulePositionForApex, mergedPillModel, branchFlags, badgeModel, subTextOf, alertsHtmlOf,
  authorBadgeHtmlOf, actionRowHtmlOf, openCardModel, dispatchCardAction,
} from '../../web/modules/capsule.js';

const open = (id, extra = {}) => ({ id, name: id, state: 'open', hemisphere: 'upper', hasLocal: false, hasRemote: false, ...extra });
const pr = (n, review = 'APPROVED', ci) => ({ number: n, reviewState: review, ...(ci ? { ci: { state: ci } } : {}) });
const acts = (html) => [...html.matchAll(/data-act="([^"]+)"/g)].map((m) => m[1]);

// ---------- position ----------
test('capsulePositionForApex: merged pills sit above the apex, open cards above (upper) or below (lower)', () => {
  assert.deepEqual(capsulePositionForApex({ state: 'merged' }, 500, 100), { x: 500 - MERGED_FO_W / 2, y: 100 - MERGED_FO_H - 6 });
  assert.deepEqual(capsulePositionForApex({ state: 'open', hemisphere: 'upper' }, 500, 100), { x: 500 - FO_W / 2, y: 100 - FO_H + 12 });
  assert.deepEqual(capsulePositionForApex({ state: 'open', hemisphere: 'lower' }, 500, 100), { x: 500 - FO_W / 2, y: 114 });
});

// ---------- merged pill ----------
test('mergedPillModel: plain merged branch has diff only; leftover local copy adds delete + Local tag', () => {
  const plain = mergedPillModel({ name: 'merged/plain', pr: { number: 21 } }, false);
  assert.deepEqual(acts(plain.html), ['diff']);
  assert.equal(plain.hasLocalLeftover, false);
  assert.match(plain.html, /✓ #21 merged\/plain/);
  assert.doesNotMatch(plain.className, /gone|selected/);

  const left = mergedPillModel({ name: 'merged/x', hasLocal: true, isHead: false }, true);
  assert.deepEqual(acts(left.html), ['diff', 'delete']);
  assert.match(left.html, /💻 Local/);
  assert.match(left.className, /selected/);
  assert.match(left.className, /gone/);
  assert.match(mergedPillModel({ name: 'm', originGone: true, isHead: true }, false).html, /HEAD·Local/);
});

test('mergedPillModel: truncates the name harder when the Local tag needs room, and escapes author/name', () => {
  const long = 'merged/a-very-long-branch-name';
  assert.ok(mergedPillModel({ name: long }, false).html.includes('✓ merged/a-very-l…'), 'plain variant truncates to 16 chars');
  const withLocal = mergedPillModel({ name: long, hasLocal: true }, false).html;
  assert.ok(withLocal.includes('✓ merged/a-…'), 'local variant truncates to 10 chars');
  const evil = mergedPillModel({ name: '<b>x</b>', author: '<img src=x onerror=1>' }, false).html;
  assert.doesNotMatch(evil, /<img|<b>/);
  assert.match(evil, /&lt;img/);
});

// ---------- flags / badge ----------
test('branchFlags: working tree, remote-only, fresh/synced local', () => {
  assert.equal(branchFlags(open('local-wip-working-tree', { hemisphere: 'lower' })).isWorkingTreeWip, true);
  assert.equal(branchFlags(open('local-wip-foo', { hemisphere: 'lower' })).isWorkingTreeWip, true);
  assert.equal(branchFlags(open('x', { name: 'wip/working-tree' })).isWorkingTreeWip, true);
  assert.equal(branchFlags(open('x', { hasRemote: true })).isRemoteOnly, true);
  assert.equal(branchFlags(open('x', { hasRemote: true, hasLocal: true })).isRemoteOnly, false);
  const synced = branchFlags(open('x', { hemisphere: 'lower', aheadMain: 0, uncommittedCount: 0 }));
  assert.equal(synced.isFreshLocal, true);
  assert.equal(synced.isSyncedLocal, true);
  assert.equal(branchFlags(open('x', { hemisphere: 'lower', aheadMain: 0, uncommittedCount: 2 })).isSyncedLocal, false);
  assert.equal(branchFlags(open('x', { hemisphere: 'upper', aheadMain: 0 })).isFreshLocal, false, 'only the lower ray has fresh local branches');
});

test('badgeModel priority: CI failed > conflict > CI running > origin gone > lower states > default', () => {
  assert.deepEqual(badgeModel(open('a', { pr: pr(1, 'APPROVED', 'FAILURE'), conflict: { hasConflict: true } })), { tone: 'danger', pulse: false, text: 'PR #1 · CI Failed' });
  assert.deepEqual(badgeModel(open('a', { pr: pr(2), conflict: { hasConflict: true, resolved: false } })), { tone: 'warn', pulse: false, text: 'PR #2 · Çakışma' });
  assert.equal(badgeModel(open('a', { conflict: { hasConflict: true } })).text, 'Merge Çakışması');
  assert.deepEqual(badgeModel(open('a', { pr: pr(3, 'REVIEW_REQUIRED', 'RUNNING') })), { tone: 'warn', pulse: true, text: 'PR #3 · CI Çalışıyor' });
  assert.equal(badgeModel(open('a', { originGone: true })).text, 'origin: gone');
  assert.equal(badgeModel(open('a', { state: 'gone' })).text, 'origin: gone');
  const lower = (extra) => badgeModel(open('a', { hemisphere: 'lower', ...extra }));
  assert.equal(lower({ id: 'local-wip-working-tree' }).text, 'Yerel Taslak · Uncommitted');
  assert.equal(lower({ aheadMain: 0, uncommittedCount: 0 }).text, '✓ main ile Senkronize');
  assert.equal(lower({ aheadMain: 0, uncommittedCount: 3 }).text, '🌱 Yeni Yerel Dal');
  assert.equal(lower({ aheadMain: 2 }).text, 'Yarım Parabol · Local');
  assert.equal(lower({}).tone, 'purple');
  assert.deepEqual(badgeModel(open('a', { pr: pr(4, 'APPROVED') })), { tone: 'sky', pulse: false, text: 'PR #4 · APPROVED' });
  assert.equal(badgeModel(open('a')).text, 'Remote Ray');
  assert.equal(badgeModel(open('a', { conflict: { hasConflict: true, resolved: true } })).text, 'Remote Ray', 'resolved conflicts do not show');
});

test('subTextOf covers each state and falls back to counters', () => {
  assert.match(subTextOf(open('local-wip-working-tree', { uncommittedCount: 3 })), /^3 dosya bekliyor/);
  assert.match(subTextOf(open('a', { hemisphere: 'lower', aheadMain: 0, uncommittedCount: 0 })), /eşitlendi/);
  assert.match(subTextOf(open('a', { hemisphere: 'lower', aheadMain: 0, uncommittedCount: 1 })), /taze dal/);
  assert.equal(subTextOf(open('a', { lastCommitMsg: 'feat: x', aheadMain: 1 })), 'feat: x');
  assert.equal(subTextOf(open('a', { hemisphere: 'lower', aheadMain: 2, uncommittedCount: 1 })), '↑2 yerel · 1 dirty');
  assert.equal(subTextOf(open('a', { aheadMain: 2, behindMain: 5 })), '↑2 ↓5');
});

test('alertsHtmlOf: origin-only, conflict flavours, Cross-PR numbers (escaped, missing → ?)', () => {
  assert.equal(alertsHtmlOf(open('a')), '');
  assert.match(alertsHtmlOf(open('a', { hasRemote: true })), /☁ Sadece Origin/);
  assert.match(alertsHtmlOf(open('a', { hasLocal: true, conflict: { hasConflict: true, isChangelogOrLock: true } })), /CHANGELOG \/ Lock/);
  assert.match(alertsHtmlOf(open('a', { hasLocal: true, conflict: { hasConflict: true } })), /⚠ Conflict/);
  assert.match(alertsHtmlOf(open('a', { crossPr: [{ otherPrNumber: 20 }, {}] })), /Cross-PR #20,#\?/);
  assert.doesNotMatch(alertsHtmlOf(open('a', { crossPr: [{ otherPrNumber: '<script>' }] })), /<script>/);
});

test('authorBadgeHtmlOf: me vs peer, case-insensitive, escaped, empty author', () => {
  assert.equal(authorBadgeHtmlOf(open('a'), 'ziya'), '');
  assert.match(authorBadgeHtmlOf(open('a', { author: 'Ziya' }), ' ziya '), /author-me.*👤 Ben/s);
  assert.match(authorBadgeHtmlOf(open('a', { author: 'bob' }), 'ziya'), /author-peer.*@bob/s);
  assert.match(authorBadgeHtmlOf(open('a', { author: 'bob' }), ''), /author-peer/);
  assert.doesNotMatch(authorBadgeHtmlOf(open('a', { author: '<img src=x onerror=1>' }), ''), /<img/);
  assert.match(authorBadgeHtmlOf(open('a', { author: 'a-very-long-author-name' }), ''), /@a-very-lo…/);
});

// ---------- action rows ----------
test('actionRowHtmlOf: the buttons offered in each situation', () => {
  const a = (b, ctx) => acts(actionRowHtmlOf(b, ctx));
  assert.deepEqual(a(open('a', { pr: pr(1, 'APPROVED', 'FAILURE'), hasLocal: true })), ['inspect', 'diff', 'checkout', 'delete']);
  assert.deepEqual(a(open('a', { hasLocal: true, isHead: true, conflict: { hasConflict: true } })), ['resolve', 'diff', 'delete'], 'no checkout button on HEAD');
  assert.deepEqual(a(open('a', { hasLocal: true, pr: pr(2) })), ['merge', 'diff', 'checkout', 'delete']);
  assert.deepEqual(a(open('a', { hasLocal: true, pr: pr(3, 'REVIEW_REQUIRED', 'RUNNING') })), ['merge', 'diff', 'checkout', 'delete']);
  // lower ray
  const lower = (extra) => open('a', { hemisphere: 'lower', hasLocal: true, ...extra });
  assert.deepEqual(a(lower({ id: 'local-wip-working-tree' })), ['inspect', 'diff'], 'working tree: no checkout/delete');
  assert.deepEqual(a(lower({ aheadMain: 2 })), ['promote', 'diff', 'checkout', 'delete']);
  assert.deepEqual(a(lower({ aheadMain: 2, pr: pr(9, 'APPROVED') })), ['promote', 'merge', 'diff', 'checkout', 'delete']);
  assert.deepEqual(a(lower({ aheadMain: 2, pr: pr(9, 'MERGED') })), ['promote', 'diff', 'checkout', 'delete'], 'merged PR is not mergeable again');
  assert.deepEqual(a(lower({ aheadMain: 0, uncommittedCount: 0 })), ['checkout', 'diff', 'delete']);
  assert.deepEqual(a(lower({ aheadMain: 0, uncommittedCount: 0, isHead: true })), ['checkout-main', 'diff', 'delete']);
});

test('actionRowHtmlOf: AI proposal and queued auto-merge states', () => {
  const wip = open('local-wip-working-tree', { hemisphere: 'lower', hasLocal: true });
  assert.match(actionRowHtmlOf(wip, { pendingProposal: { id: 1 } }), /✨ AI Önerisi/);
  assert.match(actionRowHtmlOf(wip, {}), /Commit &amp; PR/);
  const running = open('a', { hasLocal: true, pr: pr(3, 'REVIEW_REQUIRED', 'RUNNING') });
  assert.match(actionRowHtmlOf(running, {}), /CI Bitince Merge Et/);
  const queued = actionRowHtmlOf(running, { isQueued: true });
  assert.match(queued, /CI Bekleniyor \(İptal\)/);
  assert.match(queued, /running-pulse/);
});

test('actionRowHtmlOf: remote-only branches say what the buttons really do', () => {
  const html = actionRowHtmlOf(open('a', { hasRemote: true, pr: pr(5) }), {});
  assert.match(html, /PR Kapat &amp; Uzak Dalı Sil|PR Kapat & Uzak Dalı Sil/);
  assert.match(html, /git switch --track/);
});

// ---------- whole card ----------
test('openCardModel assembles class, title, badge, sub text and alerts', () => {
  const b = open('feat/x', { pr: pr(7, 'APPROVED', 'FAILURE'), hasLocal: true, author: 'ziya', crossPr: [{ otherPrNumber: 9 }] });
  const m = openCardModel(b, { tone: 'ci-fail', isSelected: true, isQueued: false, pendingProposal: null, currentUser: 'ziya' });
  assert.equal(m.className, 'cap-card ci-fail selected');
  assert.match(m.html, /class="cap-title" title="feat\/x">feat\/x</);
  assert.match(m.html, /cap-badge danger[^>]*>PR #7 · CI Failed</);
  assert.match(m.html, /👤 Ben/);
  assert.match(m.html, /<div class="cap-alerts">/);
  assert.equal(m.isWorkingTreeWip, false);
  assert.equal(openCardModel(open('local-wip-working-tree', { hemisphere: 'lower' }), { tone: 'x' }).isWorkingTreeWip, true);
  assert.match(openCardModel(open('local-wip-working-tree', { hemisphere: 'lower' }), { tone: 'x' }).html, /✏️ Çalışma Alanı/);
  assert.doesNotMatch(openCardModel(open('plain', { hasLocal: true }), { tone: 'x' }).html, /cap-alerts/, 'no alerts container when there is nothing to warn about');
});

test('openCardModel escapes hostile data everywhere it is interpolated', () => {
  const evil = '<img src=x onerror=alert(1)>';
  const b = open(evil, { author: evil, lastCommitMsg: evil, hasRemote: true, crossPr: [{ otherPrNumber: evil }], pr: { number: 1, reviewState: evil } });
  const html = openCardModel(b, { tone: 'x', currentUser: '' }).html;
  assert.doesNotMatch(html, /<img/, 'no raw <img> may survive in the card HTML');
  assert.match(html, /&lt;img/);
});

// ---------- click dispatch ----------
test('dispatchCardAction routes each data-act to the right callback with the right arguments', () => {
  const calls = [];
  const rec = (name) => (...a) => calls.push([name, ...a]);
  const cb = { onSelectBranch: rec('select'), onCheckoutBranch: rec('checkout'), onMergeBranch: rec('merge'), onPromoteBranch: rec('promote'), onDeleteBranch: rec('delete'), onOpenConflictStudio: rec('studio'), onOpenDiff: rec('diff') };
  const b = { id: 'b1', name: 'feat/b1' };
  const btn = { tag: 'btn' };
  dispatchCardAction('diff', btn, b, false, cb);
  dispatchCardAction('diff', btn, b, true, cb);
  dispatchCardAction('inspect', btn, b, false, cb);
  dispatchCardAction('checkout', btn, b, false, cb);
  dispatchCardAction('checkout-main', btn, b, false, cb);
  dispatchCardAction('merge', btn, b, false, cb);
  dispatchCardAction('promote', btn, b, false, cb);
  dispatchCardAction('resolve', btn, b, false, cb);
  dispatchCardAction('delete', btn, b, false, cb);
  dispatchCardAction('unknown', btn, b, false, cb);
  assert.deepEqual(calls, [
    ['diff', 'feat/b1', 'branch'], ['diff', 'working-tree', 'working-tree'], ['select', 'b1'], ['checkout', 'b1', btn],
    ['checkout', 'main', btn], ['merge', 'b1', btn], ['promote', 'b1', undefined, undefined, btn], ['studio', 'b1'], ['delete', 'b1', btn],
  ]);
  assert.doesNotThrow(() => dispatchCardAction('merge', btn, b, false, {}), 'missing callbacks are ignored');
});

import { whereModel, whereHtmlOf } from '../../web/modules/capsule.js';

const ob = (extra = {}) => ({ id: 'b', name: 'feat/x', state: 'open', hemisphere: 'upper', hasLocal: true, hasRemote: true, aheadMain: 0, uncommittedCount: 0, ...extra });

test('whereModel: remote chip says PR / exists / not pushed / deleted', () => {
  assert.equal(whereModel(ob({ pr: { number: 83 } })).remote.text, '☁ PR #83');
  assert.equal(whereModel(ob()).remote.text, '☁ Uzak: var');
  assert.equal(whereModel(ob({ hasRemote: false, hemisphere: 'lower' })).remote.text, '☁ Uzakta yok');
  const gone = whereModel(ob({ originGone: true }));
  assert.equal(gone.remote.text, '☁ Uzakta silinmiş');
  assert.equal(gone.remote.tone, 'warn');
});

test('whereModel: local chip says missing / equal / ahead and dirty / HEAD', () => {
  assert.equal(whereModel(ob({ hasLocal: false })).local.text, '💻 Yerelde yok');
  assert.equal(whereModel(ob()).local.text, '💻 eşit');
  assert.equal(whereModel(ob({ aheadMain: 3, uncommittedCount: 2 })).local.text, '💻 ↑3 · 2 dirty');
  assert.equal(whereModel(ob({ aheadMain: 1, isHead: true })).local.text, '💻 ↑1 · HEAD');
  const wip = whereModel({ id: 'local-wip-working-tree', name: 'wip', state: 'open', hemisphere: 'lower', hasLocal: true, uncommittedCount: 4 });
  assert.equal(wip.remote.text, '☁ Uzakta yok');
  assert.equal(wip.local.text, '💻 4 dosya');
});

test('the open card carries the Uzak / Yerel row and escapes hostile data', () => {
  const html = openCardModel(ob({ pr: { number: 5, reviewState: 'OPEN' } }), { tone: 'sky', isSelected: false }).html;
  assert.match(html, /class="cap-where"/);
  assert.match(html, /cap-loc remote sky/);
  assert.match(html, /cap-loc local purple/);
  const evil = whereHtmlOf(ob({ pr: { number: '<img src=x onerror=1>' } }));
  assert.doesNotMatch(evil, /<img/);
});

// Dürüstlük: GitHub'ın boş bıraktığı PR bilgisi (inceleme kararı, CI) arayüzde uydurulmaz. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { prHeadline } from '../../web/modules/utils.js';
import { badgeModel } from '../../web/modules/capsule.js';
import { branchViewModel } from '../../web/modules/inspector-model.js';
import { ciCardHtml } from '../../web/modules/inspector-cards.js';

const openPR = (reviewState, ci) => ({
  id: 'b', name: 'feat/x', state: 'open', hemisphere: 'upper', hasLocal: true, hasRemote: true,
  pr: { number: 12, title: 't', reviewState, ...(ci ? { ci } : {}) },
});

test('PR headline: an empty review decision is simply left out, never invented as REVIEW_REQUIRED', () => {
  assert.equal(prHeadline({ number: 12, reviewState: '' }), 'PR #12');
  assert.equal(prHeadline({ number: 12 }), 'PR #12');
  assert.equal(prHeadline({ number: 12, reviewState: 'APPROVED' }), 'PR #12 · APPROVED');
  assert.equal(prHeadline({ number: 7, reviewState: 'CHANGES_REQUESTED' }), 'PR #7 · CHANGES_REQUESTED');
});

test('capsule badge for a PR without a review decision shows no trailing separator or invented review state', () => {
  const m = badgeModel(openPR('', { state: 'SUCCESS' }));
  assert.ok(m.text.includes('PR #12'), m.text);
  assert.ok(!m.text.includes('REVIEW_REQUIRED') && !/·\s*$/.test(m.text), m.text);
});

test('a PR with UNKNOWN CI (no checks reported) is not "running": the merge button stays a normal merge, not "wait for CI"', () => {
  const unknown = branchViewModel(openPR('', undefined), { repo: { mode: 'live' }, branches: [] });
  assert.equal(unknown.isRunning, false);
  assert.equal(unknown.canMerge, true, 'with no CI information the user can still merge (GitHub decides)');
  const running = branchViewModel(openPR('', { state: 'RUNNING' }), { repo: { mode: 'live' }, branches: [] });
  assert.equal(running.isRunning, true, 'a really running CI still waits');
});

test('CI card: a CI with no state says it is unknown instead of "all checks passed"', () => {
  const html = ciCardHtml({ pr: { ci: { state: '', checks: [] } } }, { isLive: true });
  assert.match(html, /CI Durumu Bilinmiyor/);
  assert.doesNotMatch(html, /Tüm Kontroller Başarılı/);
  assert.match(ciCardHtml({ pr: { ci: { state: 'SUCCESS' } } }, { isLive: true }), /Tüm Kontroller Başarılı/);
  assert.match(ciCardHtml({ pr: { ci: { state: 'RUNNING' } } }, { isLive: true }), /Devam Ediyor/);
});

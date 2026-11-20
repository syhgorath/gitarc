// main CI chip model (web/modules/syncsteps.js: mainCIView) and the offline engine's post-merge behaviour. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

import { mainCIView } from '../../web/modules/syncsteps.js';

test('no data means no chip: green is never assumed', () => {
  assert.equal(mainCIView(undefined), null);
  assert.equal(mainCIView({}), null);
  assert.equal(mainCIView({ mainCI: {} }), null);
});

test('chip text, tone and link per state', () => {
  const base = { defaultBranch: 'develop', mainCI: { workflow: 'CI', sha: 'abc1234', title: 'Merge PR #7', url: 'https://github.com/a/b/actions/runs/1' } };
  const ok = mainCIView({ ...base, mainCI: { ...base.mainCI, state: 'SUCCESS', branch: 'develop' } });
  assert.equal(ok.btnClass, 'ok');
  assert.equal(ok.text, '✓ develop CI');
  const bad = mainCIView({ ...base, mainCI: { ...base.mainCI, state: 'FAILURE' } });
  assert.equal(bad.btnClass, 'danger');
  assert.match(bad.text, /kırık/);
  assert.match(bad.title, /BAŞARISIZ/);
  assert.equal(bad.url, 'https://github.com/a/b/actions/runs/1');
  const run = mainCIView({ ...base, mainCI: { ...base.mainCI, state: 'RUNNING' } });
  assert.match(run.btnClass, /running-pulse/);
  assert.match(run.text, /koşuyor/);
});

test('only https links are ever offered (never javascript: or http:)', () => {
  assert.equal(mainCIView({ mainCI: { state: 'FAILURE', url: 'javascript:alert(1)' } }).url, '');
  assert.equal(mainCIView({ mainCI: { state: 'FAILURE', url: 'http://x' } }).url, '');
});

function loadEngine() {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  return ctx.window.LocalFallbackEngine;
}

test('offline demo: green by default, tests start running right after a merge', () => {
  const eng = loadEngine();
  assert.equal(eng.snapshot().repo.mainCI.state, 'SUCCESS');
  const target = eng.snapshot().branches.find((b) => b.state === 'open' && b.pr && !(b.conflict?.hasConflict && !b.conflict.resolved) && b.pr.ci?.state !== 'FAILURE');
  assert.ok(target, 'a mergeable demo branch exists');
  const snap = eng.mergeBranch(target.id);
  assert.equal(snap.repo.mainCI.state, 'RUNNING');
  assert.ok(snap.repo.mainCI.sha);
});

// ---- tuvaldeki merge düğümü rozeti + Inspector kartı ----
import { mainCIBadge, mainCICardModel } from '../../web/modules/syncsteps.js';
import { mainCICardHtml } from '../../web/modules/inspector-cards.js';

const snapOf = (mainCI, nodes) => ({ repo: { defaultBranch: 'main', mainCI }, nodes });
const nodes = [{ shortSha: '1111111', isMerge: true }, { shortSha: 'abc1234', isMerge: true }];

test('badge is pinned ONLY to the node whose SHA matches the CI run; never to older nodes', () => {
  const snap = snapOf({ state: 'RUNNING', sha: 'abc1234', workflow: 'CI', url: 'https://github.com/a/b/actions/runs/9' }, nodes);
  const b = mainCIBadge(snap);
  assert.equal(b.sha, 'abc1234');
  assert.equal(b.tone, 'running');
  assert.match(b.text, /testler koşuyor/);
  assert.equal(mainCICardModel(snap, nodes[0]), null, 'older merge node gets no card (green is never assumed for it)');
  assert.ok(mainCICardModel(snap, nodes[1]));
});

test('no badge without data, without a matching node, or with an unreadable SHA', () => {
  assert.equal(mainCIBadge(snapOf(undefined, nodes)), null);
  assert.equal(mainCIBadge(snapOf({ state: 'SUCCESS' }, nodes)), null, 'no sha');
  assert.equal(mainCIBadge(snapOf({ state: 'SUCCESS', sha: 'ffffff0' }, nodes)), null, 'sha not on the visible spine');
  assert.equal(mainCIBadge(snapOf({ state: 'WHATEVER', sha: 'abc1234' }, nodes)), null, 'unknown state is not drawn');
  assert.equal(mainCIBadge({ repo: null, nodes }), null);
});

test('tone and text per state', () => {
  const t = (state) => mainCIBadge(snapOf({ state, sha: 'abc1234' }, nodes));
  assert.deepEqual([t('SUCCESS').tone, t('FAILURE').tone, t('RUNNING').tone], ['ok', 'fail', 'running']);
  assert.match(t('FAILURE').text, /kırık/);
  assert.match(t('SUCCESS').text, /✓/);
});

test('card shows the run, escapes untrusted text and only offers https links', () => {
  const evil = snapOf({ state: 'FAILURE', sha: 'abc1234', workflow: '<img src=x onerror=1>', title: '"><script>', url: 'javascript:alert(1)' }, nodes);
  const m = mainCICardModel(evil, nodes[1]);
  assert.equal(m.url, '', 'non-https url is dropped');
  const html = mainCICardHtml(m);
  assert.ok(!html.includes('<img') && !html.includes('<script'), 'untrusted text is escaped');
  assert.ok(!html.includes('act-mainci-open'), 'no link button without a safe url');
  const ok = mainCICardHtml(mainCICardModel(snapOf({ state: 'SUCCESS', sha: 'abc1234', url: 'https://github.com/a/b/actions/runs/1' }, nodes), nodes[1]));
  assert.ok(ok.includes('act-mainci-open') && ok.includes('Merge sonrası testler başarılı'));
  assert.equal(mainCICardHtml(null), '');
});

import { isPanExemptTarget } from '../../web/modules/interactions.js';

test('canvas panning must never capture the pointer on the main CI badge (real clicks would be swallowed)', () => {
  const t = (match) => ({ closest: (sel) => (sel === match ? {} : null) });
  assert.equal(isPanExemptTarget(t('.svg-mainci-badge')), true);
  assert.equal(isPanExemptTarget(t('circle')), true);
  assert.equal(isPanExemptTarget(t('.fo-capsule')), true);
  assert.equal(isPanExemptTarget(t('.something-else')), false, 'empty canvas still pans');
  assert.equal(isPanExemptTarget(null), false);
});

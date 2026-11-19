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

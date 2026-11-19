// Merged PR detail: diff target rule, file rows, honest suggested command in the offline engine. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

import { diffArgsFor, mergeCommitOf } from '../../web/modules/utils.js';
import { mergedFilesRowsHtml, mergedFilesCardHtml } from '../../web/modules/inspector-cards.js';

const snap = { nodes: [{ spineIndex: 0, sha: 'aaa0000', shortSha: 'aaa0000' }, { spineIndex: 1, sha: 'bbb1111111', shortSha: 'bbb1111' }] };

test('a merged branch is diffed through its merge commit; others keep the old targets', () => {
  const merged = { name: 'fix/x', state: 'merged', endSpineIndex: 1 };
  assert.deepEqual(diffArgsFor(merged, snap), ['bbb1111111', 'merge']);
  assert.equal(mergeCommitOf(merged, snap).shortSha, 'bbb1111');
  assert.deepEqual(diffArgsFor({ name: 'feat/open', state: 'open' }, snap), ['feat/open', 'branch']);
  assert.deepEqual(diffArgsFor({ id: 'local-wip-working-tree', name: 'wip', state: 'open' }, snap), ['working-tree', 'working-tree']);
  // omurgada karşılığı olmayan birleşmiş dal: dal adına geri dön (eski davranış)
  assert.deepEqual(diffArgsFor({ name: 'old', state: 'merged', endSpineIndex: 99 }, snap), ['old', 'branch']);
  assert.equal(mergeCommitOf({ state: 'open', endSpineIndex: 1 }, snap), null);
});

test('file rows show status, path and +/- and escape hostile paths', () => {
  const html = mergedFilesRowsHtml([
    { path: 'internal/a.go', status: 'M', additions: 3, deletions: 1 },
    { path: 'new.txt', status: 'A', additions: 5 },
    { path: '<img src=x onerror=1>.go', status: 'D', deletions: 2 },
  ]);
  assert.match(html, /chip accent sm mono">M/);
  assert.match(html, /chip emerald sm mono">A/);
  assert.match(html, /chip danger sm mono">D/);
  assert.match(html, /diff-stat-add">\+3/);
  assert.match(html, /diff-stat-del">-1/);
  assert.doesNotMatch(html, /<img/);
  assert.match(mergedFilesRowsHtml([]), /dosya değişikliği bulunamadı/);
  assert.match(mergedFilesCardHtml(), /id="merged-files-list"/);
});

test('offline engine never suggests deleting a copy that is not there', () => {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  const eng = ctx.window.LocalFallbackEngine;
  for (const b of eng.snapshot().branches) {
    if (b.suggestedCmd && /git branch -D/.test(b.suggestedCmd)) assert.ok(b.hasLocal, `${b.name}: "git branch -D" suggested without a local copy`);
    if (/git push origin --delete/.test(b.suggestedCmd || '') && (b.state === 'merged' || b.originGone)) assert.ok(b.hasRemote, `${b.name}: remote delete suggested without a remote copy`);
  }
});

import { diffTabsHtml } from '../../web/modules/studio.js';

test('file rows without stats (offline fallback) show the name but no +0/-0', () => {
  const html = mergedFilesRowsHtml([{ path: 'a.go', status: 'M' }]);
  assert.match(html, /a\.go/);
  assert.doesNotMatch(html, /diff-stat-add/);
});

test('inline diff tabs: one tab per file with +/- and the active one marked; hostile names escaped', () => {
  const html = diffTabsHtml([{ path: 'internal/a.go', additions: 3, deletions: 1 }, { path: '<img src=x>.go' }], 1);
  assert.equal((html.match(/studio-diff-tab /g) || []).length, 2);
  assert.match(html, /data-diff-tab="0"/);
  assert.match(html, /studio-diff-tab active" data-diff-tab="1"/);
  assert.match(html, /diff-stat-add">\+3/);
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /a\.go/, 'shows the file name, not the whole path');
});

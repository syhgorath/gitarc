// The offline engine must flag/clean branches with the same rule as internal/gitengine/prunable.go. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

function loadEngine() {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  return ctx.window.LocalFallbackEngine;
}

test('every branch carries a prunable flag and long-lived branches are never prunable', () => {
  const eng = loadEngine();
  eng.createBranch('develop', 'upper', null);
  const snap = eng.snapshot();
  assert.ok(snap.branches.every((b) => typeof b.prunable === 'boolean'));
  assert.equal(snap.branches.find((b) => b.name === 'develop').prunable, false);
});

test('after pruneGone nothing prunable is left except what the cleanup is not allowed to touch', () => {
  const eng = loadEngine();
  const before = eng.snapshot().branches.filter((b) => b.prunable).length;
  const { snapshot, removed } = eng.pruneGone();
  const after = snapshot.branches.filter((b) => b.prunable);
  assert.equal(typeof removed, 'number');
  assert.equal(after.length, 0, `still prunable after cleanup (was ${before}): ${after.map((b) => b.name)}`);
});

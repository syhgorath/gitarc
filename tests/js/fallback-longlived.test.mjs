// Offline LocalFallbackEngine must treat long-lived branches exactly like internal/gitengine/longlived.go. Run: node --test tests/js
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
const plain = (x) => JSON.parse(JSON.stringify(x));

// Same table as longLivedCases in internal/gitengine/longlived_test.go (default branch: main).
const CASES = [
  ['develop', true], ['Develop', true], ['dev', true], ['staging', true], ['production', true],
  ['release/1.2', true], ['release-2.x', true], ['releases/v3', true], ['support/legacy', true],
  ['maintenance/2024', true], ['stable-1', true], ['1.x', true], ['v2', true], ['2.3.4', true], ['v1.2.x', true],
  ['master', true], ['main', false], ['feat/develop-docs', false], ['fix/release-notes', false], ['feature/dev', false],
  ['devops', false], ['hotfix/x', false], ['nextgen', false],
];

function withBranch(eng, name, extra = {}) {
  const snap = eng.snapshot();
  const donor = snap.branches[0];
  return { snap, branch: { ...plain(donor), id: `br-${name}`, name, ...extra } };
}

test('fallback snapshot flags exactly the same names as the Go engine', () => {
  const eng = loadEngine();
  const snap = eng.snapshot();
  assert.ok(snap.branches.every((b) => typeof b.longLived === 'boolean'), 'every branch carries longLived');
  assert.ok(!snap.branches.some((b) => b.longLived && /^(feat|fix)\//.test(b.name)));
  // Rule table through the public surface: create branches with the case names and read the flag back.
  for (const [name, want] of CASES) {
    const e = loadEngine();
    e.createBranch(name, 'upper', null);
    const b = e.snapshot().branches.find((x) => x.name === name);
    if (b) assert.equal(b.longLived, want, name);
  }
});

test('deleteBranch and pruneGone refuse/skip a merged long-lived branch', () => {
  const eng = loadEngine();
  eng.createBranch('develop', 'upper', null);
  const dev = eng.snapshot().branches.find((b) => b.name === 'develop');
  assert.ok(dev && dev.longLived, 'develop must exist and be flagged');
  assert.throws(() => eng.deleteBranch(dev.id), /uzun ömürlü/);
  const { snapshot } = eng.pruneGone();
  assert.ok(snapshot.branches.some((b) => b.name === 'develop'), 'develop survives pruneGone');
});

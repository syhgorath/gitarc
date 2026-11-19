// Tests for the offline LocalFallbackEngine's AI-proposal flow (web/fallback.js). It must behave like the server's
// Manager.ApproveProposal (tri-mode parity). Run: node --test tests/js
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

// Values created inside the vm context have another realm's prototypes; compare plain JSON copies.
const plain = (x) => JSON.parse(JSON.stringify(x));
const FILES = ['internal/x.go', 'docs/a.md'];
const propose = (eng, branch = 'feat/ai-x') => eng.proposeCommit(branch, 'feat: add x and docs (2 dosya)', FILES, 'AI başlık', 'AI gövde');
const pendingOf = (eng) => eng.snapshot().pendingProposal;
const find = (snap, name) => snap.branches.find((b) => b.name === name);

test('proposeCommit leaves a pending proposal with an id', () => {
  const eng = loadEngine();
  propose(eng);
  const p = pendingOf(eng);
  assert.ok(p && p.id, 'proposal id is required for approval');
  assert.deepEqual(plain(p.files), FILES);
});

test('approveProposal creates the PR branch with ONLY the proposed files and clears the proposal', () => {
  const eng = loadEngine();
  propose(eng);
  const p = pendingOf(eng);
  const snap = eng.approveProposal({ proposalId: p.id, branchName: 'feat/ai-edited', message: 'feat: edited', prTitle: 'Düzenlenmiş', prBody: 'Gövde', files: ['.env', 'smuggled.go'] });
  assert.equal(snap.pendingProposal, null);
  const b = find(snap, 'feat/ai-edited');
  assert.ok(b, 'edited branch name is used');
  assert.equal(b.hemisphere, 'upper');
  assert.equal(b.pr.title, 'Düzenlenmiş');
  assert.equal(b.pr.body, 'Gövde');
  assert.deepEqual(plain(b.changedFiles), FILES, 'client-supplied files are ignored');
  assert.equal(find(snap, 'feat/ai-x'), undefined, 'the originally proposed name is not created when renamed');
  assert.equal(pendingOf(eng), null);
});

test('approveProposal: empty fields fall back to the proposal values', () => {
  const eng = loadEngine();
  propose(eng, 'feat/ai-defaults');
  const p = pendingOf(eng);
  const snap = eng.approveProposal({ proposalId: p.id, branchName: '', message: '', prTitle: '', prBody: '' });
  const b = find(snap, 'feat/ai-defaults');
  assert.ok(b);
  assert.equal(b.pr.title, 'AI başlık');
  assert.equal(b.lastCommitMsg, p.message);
});

test('approveProposal refuses missing, stale and duplicate approvals', () => {
  const eng = loadEngine();
  assert.throws(() => eng.approveProposal({ proposalId: 'x', branchName: 'feat/x', message: 'm' }), /bekleyen AI önerisi yok/);
  propose(eng);
  const p = pendingOf(eng);
  assert.throws(() => eng.approveProposal({ proposalId: 'prop-OTHER', branchName: 'feat/ai-x', message: 'm' }), /öneri değişmiş/);
  assert.throws(() => eng.approveProposal({ branchName: 'feat/ai-x', message: 'm' }), /öneri değişmiş/, 'missing id is refused');
  assert.ok(pendingOf(eng), 'refused approvals leave the proposal pending');
  eng.approveProposal({ proposalId: p.id, branchName: 'feat/ai-x', message: 'm' });
  assert.throws(() => eng.approveProposal({ proposalId: p.id, branchName: 'feat/ai-x', message: 'm' }), /bekleyen AI önerisi yok/);
});

test('approveProposal refuses unsafe or default branch names and keeps the proposal', () => {
  const eng = loadEngine();
  propose(eng);
  const p = pendingOf(eng);
  for (const bad of ['bad name', '../escape', 'main', 'feat/x/', '-x', 'a;rm']) {
    assert.throws(() => eng.approveProposal({ proposalId: p.id, branchName: bad, message: 'm' }), /Geçersiz dal adı|özellik dalı adı/, bad);
  }
  assert.ok(pendingOf(eng));
});

test('approveProposal refuses a merged branch name (commit rules are re-applied)', () => {
  const eng = loadEngine();
  propose(eng);
  const p = pendingOf(eng);
  const merged = eng.snapshot().branches.find((b) => b.state === 'merged');
  assert.ok(merged, 'demo has a merged branch');
  assert.throws(() => eng.approveProposal({ proposalId: p.id, branchName: merged.name, message: 'm' }), /zaten birleşmiş/);
  assert.ok(pendingOf(eng), 'proposal stays pending');
});

test('dismissProposal clears the proposal and approval is then impossible', () => {
  const eng = loadEngine();
  propose(eng);
  const p = pendingOf(eng);
  eng.dismissProposal();
  assert.equal(pendingOf(eng), null);
  assert.throws(() => eng.approveProposal({ proposalId: p.id, branchName: 'feat/x', message: 'm' }), /bekleyen AI önerisi yok/);
});

test('commitWorkingTree uses the given files for the new branch (and the demo default otherwise)', () => {
  const eng = loadEngine();
  const withFiles = eng.commitWorkingTree('feat/with-files', 'm', '', '', false, ['a.go', 'b.go']);
  assert.deepEqual(plain(find(withFiles, 'feat/with-files').changedFiles), ['a.go', 'b.go']);
  const noFiles = eng.commitWorkingTree('feat/no-files', 'm', '', '', false, []);
  assert.deepEqual(plain(find(noFiles, 'feat/no-files').changedFiles), ['CHANGELOG.md']);
});

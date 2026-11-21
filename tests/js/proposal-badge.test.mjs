// Proje seçicideki 📥 rozeti: onay bekleyen öneri kartı olan proje işaretlenir; imza değişince arayüz yenilenir. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { repoOptionLabel } from '../../web/modules/topbar.js';
import { snapshotSignature } from '../../web/modules/utils.js';

test('a project with a pending proposal is marked with 📥 in the selector, others are not', () => {
  assert.equal(repoOptionLabel({ id: 'a', name: 'GitArc (Canlı)', pendingProposal: true }), '📥 GitArc (Canlı)');
  assert.equal(repoOptionLabel({ id: 'b', name: 'sshpocket (Kayıtlı)' }), 'sshpocket (Kayıtlı)');
  assert.equal(repoOptionLabel({ id: 'b', name: 'x', pendingProposal: false }), 'x');
  assert.equal(repoOptionLabel({ id: 'c', name: 'y', queueRunning: true }), '⏳ y', 'arka planda koşusu süren proje ⏳ ile işaretlenir');
  assert.equal(repoOptionLabel({ id: 'c', name: 'y', queueRunning: true, pendingProposal: true }), '⏳ 📥 y');
  assert.equal(repoOptionLabel(undefined), '');
});

const snap = (repos) => ({ repo: { id: 'r', ahead: 0, behind: 0, dirtyCount: 0 }, nodes: [], branches: [], logs: [], repos });

test('a proposal appearing/disappearing in ANOTHER project changes the poll signature (so the badge refreshes)', () => {
  const none = snapshotSignature(snap([{ id: 'a' }, { id: 'b' }]));
  const aHasCard = snapshotSignature(snap([{ id: 'a', pendingProposal: true }, { id: 'b' }]));
  const bHasCard = snapshotSignature(snap([{ id: 'a' }, { id: 'b', pendingProposal: true }]));
  assert.notEqual(aHasCard, none);
  assert.notEqual(bHasCard, none);
  assert.notEqual(aHasCard, bHasCard, 'which project holds the card matters');
});

import { findSelectedBranch, findSelectedNode } from '../../web/modules/inspector-model.js';

test('a fresh repo (server sends null branches/nodes) does not crash the inspector lookups', () => {
  assert.equal(findSelectedBranch({ branches: null }, 'x'), undefined);
  assert.equal(findSelectedBranch(null, 'x'), undefined);
  assert.equal(findSelectedNode({ nodes: null }, 'abc'), undefined);
  assert.equal(findSelectedBranch({ branches: [{ id: 'a' }, { id: 'b' }] }, 'b').id, 'b');
  assert.equal(findSelectedNode({ nodes: [{ shortSha: 'abc' }] }, 'abc').shortSha, 'abc');
});

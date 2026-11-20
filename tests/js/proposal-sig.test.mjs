// Bekleyen öneri kartı (MCP ayrı süreç olarak yazar) yoklama imzasına girmeli; yoksa arayüz kartı hiç çekmez. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { snapshotSignature } from '../../web/modules/utils.js';

const snap = (extra = {}) => ({ repo: { id: 'r', ahead: 0, behind: 0, dirtyCount: 0 }, nodes: [], branches: [], logs: [], ...extra });

test('a new pending proposal changes the poll signature, so the UI refetches and shows the card', () => {
  const without = snapshotSignature(snap());
  const withCard = snapshotSignature(snap({ pendingProposal: { id: 'prop-1' } }));
  assert.notEqual(withCard, without);
});

test('a different proposal id changes it again, and removing the card changes it back', () => {
  const a = snapshotSignature(snap({ pendingProposal: { id: 'prop-1' } }));
  const b = snapshotSignature(snap({ pendingProposal: { id: 'prop-2' } }));
  assert.notEqual(a, b);
  assert.equal(snapshotSignature(snap()), snapshotSignature(snap({ pendingProposal: undefined })));
});

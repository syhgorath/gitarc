// Zero-dependency tests for the left-rail spine card (web/modules/rails.js). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { spineCardModel } from '../../web/modules/rails.js';
import { protectionView } from '../../web/modules/syncsteps.js';

test('spine card name follows the default branch (master/develop), main only as fallback', () => {
  assert.equal(spineCardModel({ defaultBranch: 'master' }).name, 'master');
  assert.equal(spineCardModel({ defaultBranch: 'develop' }).name, 'develop');
  assert.equal(spineCardModel({}).name, 'main');
  assert.equal(spineCardModel(undefined).name, 'main');
});

test('spine card never claims "Korunan" unless protection is really active', () => {
  assert.match(spineCardModel({ protectionState: 'active' }).sub, /Korunan/);
  assert.equal(spineCardModel({ protectionState: 'active' }).tone, 'prot-active');
  const none = spineCardModel({ protectionState: 'none' });
  assert.match(none.sub, /Korumasız/);
  assert.equal(none.tone, 'prot-none');
  const unknown = spineCardModel({ protectionState: 'unknown' });
  assert.match(unknown.sub, /okunamadı/);
  assert.ok(!/Korunan/.test(unknown.sub));
  assert.equal(unknown.tone, 'prot-unknown');
});

test('spine card stays compatible with servers that only send branchProtected', () => {
  assert.equal(spineCardModel({ branchProtected: true }).tone, 'prot-active');
  assert.equal(spineCardModel({ branchProtected: false }).tone, 'prot-none');
});

test('spine card tooltip prefers the server summary', () => {
  assert.equal(spineCardModel({ protectionSummary: 'Ruleset: 2 kural' }).title, 'Ruleset: 2 kural');
  assert.match(spineCardModel({ defaultBranch: 'master', protectionState: 'none' }).title, /^master:/);
});

test('top bar protection button names the real default branch too', () => {
  assert.equal(protectionView({ protectionState: 'active', defaultBranch: 'master' }).button, '🛡️ master Korumalı');
  assert.equal(protectionView({ protectionState: 'active' }).button, '🛡️ main Korumalı');
});

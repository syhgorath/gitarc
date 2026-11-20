// Dal koruması: çevrimdışı motor Go ile aynı kuralları taşır (yalnızca güçlendir, onaysız uygulama yok) ve pencere modeli.
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

import { changeRows, canApplyPlan, applyLabel, PROTECTION_PROFILES } from '../../web/modules/protection.js';

function loadEngine() {
  const ctx = { window: {}, console };
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL('../../web/fallback.js', import.meta.url), 'utf8'), ctx);
  return ctx.window.LocalFallbackEngine;
}

test('profiles match the engine: temel, ci, siki', () => {
  assert.deepEqual(PROTECTION_PROFILES.map((p) => p.id), ['temel', 'ci', 'siki']);
});

test('plan of an unprotected demo repo lists changes and never applies anything', () => {
  const eng = loadEngine();
  const plan = eng.protectionPlan('ci', ['test']);
  assert.equal(plan.state, 'none');
  assert.ok(plan.applicable && plan.fingerprint && plan.changes.length > 0);
  assert.deepEqual([...plan.target.checks], ['test']);
  assert.equal(plan.target.strictUpToDate, true);
  assert.ok(!eng.snapshot().repo.branchProtected, 'plan must not protect');
});

test('apply requires the fingerprint of the shown plan', () => {
  const eng = loadEngine();
  assert.throws(() => eng.applyProtection('ci', ['test'], ''), /onaylanmadı/);
  const plan = eng.protectionPlan('ci', ['test']);
  assert.throws(() => eng.applyProtection('ci', ['lint'], plan.fingerprint), /onaylanmadı/, 'a plan for other checks must not confirm');
  const snap = eng.applyProtection('ci', ['test'], plan.fingerprint);
  assert.equal(snap.repo.branchProtected, true);
  assert.equal(snap.repo.protectionState, 'active');
  assert.match(snap.repo.protectionSummary, /CI: test/);
});

test('a second identical apply has nothing to change', () => {
  const eng = loadEngine();
  eng.applyProtection('temel', [], eng.protectionPlan('temel', []).fingerprint);
  const again = eng.protectionPlan('temel', []);
  assert.equal(again.changes.length, 0);
  assert.throws(() => eng.applyProtection('temel', [], again.fingerprint), /zaten karşılıyor/);
});

test('protection is only ever strengthened: a stricter existing rule survives a weaker profile', () => {
  const eng = loadEngine();
  eng.applyProtection('siki', ['build'], eng.protectionPlan('siki', ['build']).fingerprint);
  const plan = eng.protectionPlan('temel', []);
  assert.equal(plan.target.approvals, 1);
  assert.equal(plan.target.enforceAdmins, true);
  assert.deepEqual([...plan.target.checks], ['build'], 'required checks are never dropped');
  assert.equal(plan.changes.length, 0);
});

test('ci profile without a selected check says so honestly', () => {
  const plan = loadEngine().protectionPlan('ci', []);
  assert.match(plan.notes.join(' '), /seçilmedi/);
});

test('bad profile and bad check names are rejected', () => {
  const eng = loadEngine();
  assert.throws(() => eng.protectionPlan('hepsi', []), /bilinmeyen koruma profili/);
  assert.throws(() => eng.protectionPlan('ci', ['a\nb']), /geçersiz kontrol adı/);
});

test('window model: rows, apply gating and label', () => {
  const plan = { applicable: true, fingerprint: 'x', changes: [{ label: 'PR zorunlu', from: 'hayır', to: 'evet' }] };
  assert.deepEqual(changeRows(plan), [{ label: 'PR zorunlu', from: 'hayır', to: 'evet' }]);
  assert.equal(canApplyPlan(plan), true);
  assert.equal(canApplyPlan({ ...plan, applicable: false }), false);
  assert.equal(canApplyPlan({ ...plan, changes: [] }), false);
  assert.equal(canApplyPlan(null), false);
  assert.equal(applyLabel(plan), '🛡️ 1 Değişikliği Uygula');
  assert.equal(applyLabel({ ...plan, changes: [] }), '🛡️ Değişecek Bir Şey Yok');
});

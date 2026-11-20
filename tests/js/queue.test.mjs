// Merge kuyruğu planı: tarayıcı yedek motoru, internal/gitengine/queue.go ile AYNI mantığı taşır. Run: node --test tests/js
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

const open = (b) => b.state === 'open' && !b.historyOnly;

test('default selection is every open PR, ordered by PR number', () => {
  const eng = loadEngine();
  const expected = eng.snapshot().branches.filter((b) => open(b) && b.pr?.number > 0).map((b) => b.pr.number).sort((a, b) => a - b);
  const plan = eng.queuePlan();
  assert.equal(JSON.stringify(plan.items.map((i) => i.pr)), JSON.stringify(expected));
  assert.equal(plan.mode, 'simulation');
});

test('the queue stops at the first non-clean item; everything after it waits behind the blocker', () => {
  const plan = loadEngine().queuePlan();
  const firstBad = plan.items.findIndex((i) => i.status !== 'clean');
  assert.equal(plan.stopsAt, firstBad === -1 ? 0 : firstBad + 1);
  assert.equal(plan.mergeable, firstBad === -1 ? plan.items.length : firstBad);
  plan.items.forEach((it, idx) => assert.equal(Boolean(it.afterBlocker), firstBad !== -1 && idx > firstBad, `item ${idx}`));
});

test('a branch that already conflicts with main is reported as such; clean items never share files with earlier clean ones', () => {
  const eng = loadEngine();
  const byId = Object.fromEntries(eng.snapshot().branches.map((b) => [b.id, b]));
  const plan = eng.queuePlan();
  const mergedFiles = new Set();
  plan.items.forEach((it) => {
    const b = byId[it.branchId];
    if (b.conflict?.hasConflict) {
      assert.equal(it.status, 'conflict');
      assert.equal(it.conflictsOnMain, true);
      return;
    }
    const shared = (b.changedFiles || []).filter((f) => mergedFiles.has(f));
    if (it.status === 'clean') {
      assert.equal(shared.length, 0, `${it.branch} is clean but shares ${shared}`);
      (b.changedFiles || []).forEach((f) => mergedFiles.add(f));
    } else {
      assert.ok(shared.length > 0, `${it.branch} conflicts without a shared file`);
      assert.ok(it.conflictsWithBranches.length > 0);
    }
  });
});

const qb = (id, pr, ci, files, extra = {}) => ({ id, name: id, state: 'open', changedFiles: files, pr: { number: pr, title: id, ci: ci ? { state: ci } : undefined }, ...extra });

// Go'daki TestQueuePlanFromSnapshotDataUsesQueueOrderAndSharedFiles ile AYNI veri ve AYNI beklenen sonuçlar (eşlik).
test('ORDER matters (same data and expectations as the Go test): a clean, b conflicts with a on y.go, c waits behind the blocker', () => {
  const eng = loadEngine();
  const a = qb('a', 1, 'SUCCESS', ['x.go', 'y.go']);
  const b = qb('b', 2, 'SUCCESS', ['y.go']);
  const c = qb('c', 3, 'RUNNING', ['z.go']);
  const plan = eng.queuePlanFromData([a, b, c], ['a', 'b', 'c'], 'main', true);
  const by = Object.fromEntries(plan.items.map((i) => [i.branch, i]));
  assert.deepEqual([by.a.status, by.b.status, by.c.status], ['clean', 'conflict', 'clean']);
  assert.deepEqual([...by.b.conflictsWith], [1]);
  assert.equal(by.b.conflictFiles[0], 'y.go');
  assert.equal(plan.stopsAt, 2);
  assert.equal(plan.mergeable, 1);
  assert.equal(by.c.afterBlocker, true);
  assert.equal(plan.ciRounds, 3, "'dal güncel olmalı' açıkken her dal için bir CI turu");
  const joined = plan.notes.join('\n');
  assert.ok(joined.includes('CI sürüyor') && joined.includes('ardışık CI turu'));
  // ters sıra: b önce gelirse a çakışır
  const rev = eng.queuePlanFromData([a, b, c], ['b', 'a'], 'main', true);
  assert.equal(JSON.stringify(rev.items.map((i) => [i.branch, i.status])), JSON.stringify([['b', 'clean'], ['a', 'conflict']]));
});

// Go'daki TestQueuePlanFromSnapshotDataHonestAboutUnknownCIAndMainConflicts ile aynı.
test('a branch conflicting with main is flagged; unknown CI and unreadable protection are said honestly', () => {
  const a = qb('a', 1, '', ['x.go'], { conflict: { hasConflict: true, files: ['x.go'] } });
  const plan = loadEngine().queuePlanFromData([a], [], 'main', null);
  assert.equal(plan.items[0].status, 'conflict');
  assert.equal(plan.items[0].conflictsOnMain, true);
  const joined = plan.notes.join('\n');
  assert.ok(joined.includes('bilinmiyor (yeşil varsayılmaz)') && joined.includes('okunamadı'));
});

test('unknown / merged branches are rejected; an empty queue is a note, not an error', () => {
  const eng = loadEngine();
  assert.throws(() => eng.queuePlan(['yok-boyle-bir-dal']), /bulunamadı/);
  const merged = eng.snapshot().branches.find((b) => b.state === 'merged');
  if (merged) assert.throws(() => eng.queuePlan([merged.id]), /açık değil/);
});

test('CI that is not known is never assumed green; strict protection counts CI rounds', () => {
  const eng = loadEngine();
  const plan = eng.queuePlan();
  plan.items.filter((i) => !i.ci).forEach((i) => assert.ok(plan.notes.some((n) => n.includes('bilinmiyor (yeşil varsayılmaz)') && n.includes(i.branch)), i.branch));
  assert.equal(plan.ciRounds, 1, 'unprotected demo: a single CI round');
  eng.applyProtection('ci', ['test'], eng.protectionPlan('ci', ['test']).fingerprint);
  const strict = eng.queuePlan();
  assert.equal(strict.strictUpToDate, true);
  assert.equal(strict.ciRounds, strict.items.length);
});

import { queueCandidates, queueStepModel, queueSummaryText } from '../../web/modules/queue.js';

test('window model: candidates are open PRs by number; step badges, details and the summary sentence', () => {
  const snap = { branches: [
    { id: 'b9', name: 'late', state: 'open', pr: { number: 9 } },
    { id: 'b2', name: 'early', state: 'open', pr: { number: 2 } },
    { id: 'x', name: 'nopr', state: 'open' },
    { id: 'm', name: 'old', state: 'merged', pr: { number: 1 } },
    { id: 'h', name: 'hist', state: 'open', historyOnly: true, pr: { number: 3 } },
  ] };
  assert.deepEqual(queueCandidates(snap).map((b) => b.pr.number), [2, 9]);
  assert.deepEqual(queueCandidates(null), []);

  const conflict = queueStepModel({ status: 'conflict', reason: 'kuyrukta önceki #1 ile aynı dosyaları değiştiriyor', conflictFiles: ['y.go'], afterBlocker: false, behind: 3, ci: '', review: 'APPROVED' });
  assert.equal(conflict.status.text, '⚠ Çakışır');
  assert.ok(conflict.details.includes('Çakışan dosyalar: y.go'));
  assert.ok(conflict.details.includes("main'in 3 commit gerisinde"));
  assert.ok(conflict.details.includes('CI: bilinmiyor (yeşil varsayılmaz)'), 'unknown CI is never shown as green');
  assert.equal(queueStepModel({ status: 'clean', ci: 'SUCCESS' }).status.text, '✓ Temiz');
  assert.equal(queueStepModel({ status: 'unknown' }).status.text, '? Bilinmiyor');
  assert.ok(queueStepModel({ status: 'clean', afterBlocker: true, ci: 'SUCCESS' }).details.some((d) => d.includes('engel aşılana kadar bekler')));

  assert.equal(queueSummaryText({ items: [] }), 'Kuyruğa girecek açık PR/dal yok.');
  assert.equal(queueSummaryText({ items: [{}, {}], stopsAt: 0 }), '✓ 2 dalın tamamı sırayla temiz birleşir.');
  assert.equal(queueSummaryText({ items: [{}, {}, {}], stopsAt: 2, mergeable: 1 }), '1 dal sorunsuz birleşir; kuyruk 2. sırada durur.');
});

import { updatePlanModel } from '../../web/modules/queue.js';

test('update-plan window model: ready can be applied; conflict/up-to-date/blocked cannot and say why', () => {
  const ready = updatePlanModel({ status: 'ready', reason: "origin/main'in 2 yeni commit'i temiz merge edilir", baseRef: 'origin/main', fingerprint: 'abc', notes: ['Not 1'] });
  assert.equal(ready.canApply, true);
  assert.ok(ready.lines.some((l) => l.includes('UZAK PR dalına push edilir')));
  assert.ok(ready.lines.includes('Not 1'));
  assert.equal(updatePlanModel({ status: 'ready', reason: 'x' }).canApply, false, 'no fingerprint, no apply');

  const conflict = updatePlanModel({ status: 'conflict', reason: 'origin/main ile çakışıyor', conflictFiles: ['a.go', 'b.go'] });
  assert.equal(conflict.canApply, false);
  assert.ok(conflict.lines.includes('Çakışan dosyalar: a.go, b.go'));
  assert.ok(conflict.lines.some((l) => l.includes("Çakışma Stüdyosu'ndan çözün")));
  assert.equal(updatePlanModel({ status: 'up_to_date', reason: 'güncel' }).title, '✓ Zaten güncel');
  assert.equal(updatePlanModel({ status: 'blocked', reason: 'dal uzakta yok' }).canApply, false);
});

// Go'daki simBranchUpdatePlanLocked / UpdateBranchFromBase ile AYNI kurallar (yedek motor eşliği).
test('fallback engine: update plan, wrong/missing confirm refused, right confirm updates, conflict refused', () => {
  const eng = loadEngine();
  const snap = eng.snapshot();
  const clean = snap.branches.find((b) => open(b) && b.behindMain > 0 && !b.conflict?.hasConflict);
  assert.ok(clean, 'demo verisinde geride kalmış temiz dal olmalı');
  const plan = eng.updateBranchPlan(clean.id);
  assert.equal(plan.status, 'ready');
  assert.equal(plan.behind, clean.behindMain);
  assert.ok(plan.fingerprint);
  assert.throws(() => eng.updateBranch(clean.id, ''), /onaylanmadı/);
  assert.throws(() => eng.updateBranch(clean.id, 'yanlis'), /onaylanmadı/);
  assert.equal(eng.updateBranchPlan(clean.id).status, 'ready', 'refused attempts change nothing');
  eng.updateBranch(clean.id, plan.fingerprint);
  assert.equal(eng.updateBranchPlan(clean.id).status, 'up_to_date');
  assert.throws(() => eng.updateBranch(clean.id, plan.fingerprint), /zaten ana dala göre güncel/);

  const conflicting = eng.snapshot().branches.find((b) => open(b) && b.conflict?.hasConflict);
  if (conflicting) {
    assert.equal(eng.updateBranchPlan(conflicting.id).status, 'conflict');
    assert.throws(() => eng.updateBranch(conflicting.id, 'x'), /çakışıyor/);
  }
  assert.throws(() => eng.updateBranchPlan('yok-boyle-dal'), /bulunamadı/);
});

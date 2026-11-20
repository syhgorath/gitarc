// "Sistem Logları" tab badge (web/modules/dock.js: logBadgeModel). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { logBadgeModel } from '../../web/modules/dock.js';

test('no errors: plain count, no red', () => {
  const m = logBadgeModel([{ level: 'ok' }, { level: 'info' }, { level: 'warn' }]);
  assert.deepEqual(m, { count: 3, errors: 0, tone: '', title: 'Hata yok' });
});

test('any error turns the badge red and the tooltip says how many; the count stays the total', () => {
  const m = logBadgeModel([{ level: 'error' }, { level: 'ok' }, { level: 'error' }]);
  assert.equal(m.count, 3);
  assert.equal(m.errors, 2);
  assert.equal(m.tone, 'danger');
  assert.equal(m.title, '2 hata içeriyor');
});

test('empty and missing input', () => {
  assert.equal(logBadgeModel([]).count, 0);
  assert.equal(logBadgeModel(undefined).tone, '');
});

// ---- Son İşlemler: günlüğün başarılı satırları ----
import { recentActionsModel } from '../../web/modules/dock.js';

test('recent actions are the successful log lines only; errors, info and automatic checks are left out', () => {
  const logs = [
    { level: 'ok', action: 'Kuyruk Koşusu', message: 'Koşu bitti: 5 PR birleştirildi' },
    { level: 'error', action: 'Merge', message: 'reddedildi' },
    { level: 'info', action: 'Snapshot', message: 'tazelendi' },
    { level: 'ok', action: 'Sürüm Kontrolü', message: 'güncel' },
    { level: 'ok', action: 'Dal Güncelleme', message: 'feat/p güncellendi' },
  ];
  const r = recentActionsModel(logs);
  assert.deepEqual(r.map((l) => l.action), ['Kuyruk Koşusu', 'Dal Güncelleme'], 'order (newest first) is kept');
});

test('recent actions: limit, empty and missing input', () => {
  const many = Array.from({ length: 100 }, (_, i) => ({ level: 'ok', action: `A${i}` }));
  assert.equal(recentActionsModel(many).length, 30);
  assert.equal(recentActionsModel(many, 5).length, 5);
  assert.deepEqual(recentActionsModel([]), []);
  assert.deepEqual(recentActionsModel(undefined), []);
  assert.deepEqual(recentActionsModel([null, { level: 'ok', action: 'X' }]).map((l) => l.action), ['X']);
});

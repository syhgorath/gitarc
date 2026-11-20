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

// ---- Hata Radarı: işlem hataları ----
import { actionFailureCards } from '../../web/modules/dock.js';

const errLog = (id, action, message, extra = {}) => ({ id: `log-${id}`, level: 'error', action, message, timestamp: '21:00:0' + id, ...extra });

test('action failures: only error lines of real actions become cards; background reads and non-errors are left out', () => {
  const logs = [
    errLog(5, 'Kuyruk Koşusu', 'Koşu durdu (blocked): #16 feat/p: CI başarısız'),
    errLog(4, 'Snapshot', 'git fetch zaman aşımı'),
    errLog(3, 'Sürüm Kontrolü', 'ağ yok'),
    { id: 'log-2', level: 'ok', action: 'Merge (merge)', message: 'birleştirildi' },
    { id: 'log-1', level: 'info', action: 'Push & PR Aç', message: 'başladı' },
    errLog(0, 'Dal Güncelleme', 'çakışıyor', { branch: 'feat/x' }),
  ];
  const cards = actionFailureCards(logs);
  assert.deepEqual(cards.map((c) => c.action), ['Kuyruk Koşusu', 'Dal Güncelleme'], 'newest first');
  assert.equal(cards[0].severity, 'critical', 'a stopped queue run is critical');
  assert.equal(cards[0].isQueue, true);
  assert.equal(cards[0].title, 'Kuyruk koşusu durdu');
  assert.equal(cards[1].severity, 'warning');
  assert.equal(cards[1].branch, 'feat/x');
  assert.equal(cards[1].title, 'Dal Güncelleme başarısız');
});

test('the same failure repeating is ONE card with a counter, not many cards', () => {
  const logs = [errLog(3, 'Merge (merge)', 'reddedildi', { branch: 'a' }), errLog(2, 'Merge (merge)', 'reddedildi', { branch: 'a' }), errLog(1, 'Merge (merge)', 'reddedildi', { branch: 'a' }), errLog(0, 'Merge (merge)', 'reddedildi', { branch: 'b' })];
  const cards = actionFailureCards(logs);
  assert.equal(cards.length, 2, 'different branch = different card');
  assert.equal(cards[0].count, 3);
  assert.equal(cards[0].latestId, 'log-3', 'the newest occurrence identifies the card');
  assert.equal(cards[1].count, 1);
});

test('dismissing hides a card until the same failure happens AGAIN', () => {
  const first = [errLog(1, 'Push & PR Aç', 'push reddedildi')];
  const [c] = actionFailureCards(first);
  const dismissed = { [c.key]: c.latestId };
  assert.equal(actionFailureCards(first, dismissed).length, 0, 'dismissed card stays hidden');
  const again = [errLog(2, 'Push & PR Aç', 'push reddedildi'), ...first];
  const back = actionFailureCards(again, dismissed);
  assert.equal(back.length, 1, 'a new occurrence brings it back');
  assert.equal(back[0].count, 2);
  assert.equal(actionFailureCards(first, { x: 'y' }).length, 1, 'unrelated dismissals do nothing');
});

test('action failures: limit and missing input', () => {
  const many = Array.from({ length: 20 }, (_, i) => errLog(i, `Eylem${i}`, 'hata'));
  assert.equal(actionFailureCards(many).length, 8);
  assert.equal(actionFailureCards(many, {}, 3).length, 3);
  assert.deepEqual(actionFailureCards(undefined), []);
  assert.deepEqual(actionFailureCards([null, { level: 'error', action: 'X', message: 'm' }]).map((c) => c.action), ['X']);
});

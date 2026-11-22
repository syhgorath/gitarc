// Toplu Yayınla penceresinin saf modelleri (satır görünümü, düğme metni, sonuç satırları).
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { publishRowModel, publishActionModel, publishResultModel } from '../../web/modules/publish.js';

test('publishRowModel: push_pr / push / blocked satırları dürüst anlatır (mevcut PR güncellenir, yeni PR açılmaz)', () => {
  const pr = publishRowModel({ action: 'push_pr', commits: 3, title: 'fix: x' });
  assert.equal(pr.badge, '🚀 Pushla + PR aç');
  assert.match(pr.text, /3 commit pushlanır, PR açılır: fix: x/);
  const push = publishRowModel({ action: 'push', commits: 2, pr: 158 });
  assert.equal(push.badge, '⬆ Pushla');
  assert.match(push.text, /PR #158 güncellenir \(yeni PR açılmaz\)/);
  const b = publishRowModel({ action: 'blocked', reason: 'ana dalla çakışıyor' });
  assert.deepEqual([b.badge, b.tone, b.text], ['⛔ Yayınlanamaz', 'warn', 'ana dalla çakışıyor']);
});

test('publishActionModel: düğme yalnızca plan yayınlanabilirken etkin; metin ne olacağını sayar; kuyruk seçeneği metne girer', () => {
  const plan = { publishable: true, items: [{ action: 'push_pr' }, { action: 'push' }, { action: 'push_pr' }] };
  const on = publishActionModel(plan, true);
  assert.equal(on.enabled, true);
  assert.equal(on.label, '🚀 Yayınla: 3 dal pushlanır · 2 PR açılır · kuyruk koşusu başlar (ilk sorunda durur)');
  assert.ok(!publishActionModel(plan, false).label.includes('kuyruk'));
  const blocked = publishActionModel({ publishable: false, items: [{}], reason: '1 dal yayınlanamıyor' }, true);
  assert.equal(blocked.enabled, false);
  assert.equal(blocked.hint, '1 dal yayınlanamıyor');
  assert.equal(publishActionModel(null, true).enabled, false);
  assert.equal(publishActionModel({ publishable: true, items: [] }, true).enabled, false);
});

test('publishResultModel: adımlar ✓/✗, PR numarası ve kuyruk notu; yarım kalan yayın tamamlandı sayılmaz', () => {
  const done = publishResultModel({ completed: true, queueStarted: true, queueNote: 'kuyruk koşusu başladı (2 PR)', steps: [{ ok: true, branch: 'feat/a', pr: 7, message: 'pushlandı, PR açıldı' }, { ok: true, branch: 'feat/b', message: 'pushlandı; mevcut PR güncellendi' }] });
  assert.equal(done.completed, true);
  assert.equal(done.lines[0].text, '✓ feat/a (#7): pushlandı, PR açıldı');
  assert.equal(done.lines[2].text, '🚦 kuyruk koşusu başladı (2 PR)');
  const half = publishResultModel({ completed: false, queueNote: 'Kuyruk başlatılmadı: ilk sorunda durdu.', steps: [{ ok: true, branch: 'a', message: 'ok' }, { ok: false, branch: 'b', message: 'push başarısız' }] });
  assert.equal(half.completed, false);
  assert.equal(half.lines[1].ok, false);
  assert.match(half.lines[1].text, /^✗ b: push başarısız/);
  assert.deepEqual(publishResultModel(null), { lines: [], completed: false, queueStarted: false });
});

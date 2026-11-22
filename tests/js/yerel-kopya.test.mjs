// "(yerel)" eşlik kartı: açık PR dalındaki pushlanmamış commit'ler. Kartın adı git dal adı değildir; arayüz onu "Çalışma Alanı" gibi
// göstermemeli, push mevcut PR'ı güncellemeli (yeni PR açmamalı), geç/sil gibi işlemler sunulmamalı.
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isWorkingTreeBranch, diffArgsFor } from '../../web/modules/utils.js';
import { promoteCopy, workingTreeDefaults } from '../../web/modules/inspector-model.js';
import { actionRowHtmlOf } from '../../web/modules/capsule.js';
import { promoteCardHtml } from '../../web/modules/inspector-cards.js';

const comp = { id: 'local-wip-feat-x', name: 'feat/x (yerel)', parentBranch: 'feat/x', hemisphere: 'lower', state: 'open', hasLocal: true, isHead: true, aheadMain: 2, remoteAhead: 2, uncommittedCount: 0 };
const wip = { id: 'local-wip-working-tree', name: 'Çalışma Alanı (Kaydedilmemiş)', hemisphere: 'lower', state: 'open', hasLocal: true, uncommittedCount: 3 };

test('eşlik kartı "çalışma alanı" sayılmaz; gerçek çalışma alanı sayılmaya devam eder', () => {
  assert.equal(isWorkingTreeBranch(comp), false);
  assert.equal(isWorkingTreeBranch(wip), true);
});

test('diff hedefi görünen ad değil gerçek dal adıdır', () => {
  assert.deepEqual(diffArgsFor(comp, null), ['feat/x', 'branch']);
  assert.deepEqual(diffArgsFor(wip, null), ['working-tree', 'working-tree']);
});

test('promoteCopy: eşlik kartında "yeni PR açılmaz" der; normal dalda eski metin', () => {
  const c = promoteCopy(comp);
  assert.equal(c.companion, true);
  assert.match(c.confirm, /"feat\/x" dalının pushlanmamış 2 commit'i/);
  assert.match(c.confirm, /yeni PR açılmaz/);
  const n = promoteCopy({ name: 'feat/y' });
  assert.equal(n.companion, false);
  assert.match(n.confirm, /Pull Request oluşturmak/);
});

test('kapsül: eşlik kartında yalnızca "Pushla (PR güncellenir)" + Diff; "Commit & PR", geç, sil YOK', () => {
  const html = actionRowHtmlOf(comp, { ciPending: false, isQueued: false });
  assert.match(html, /data-act="promote"/);
  assert.match(html, /PR güncellenir/);
  assert.doesNotMatch(html, /Commit &amp; PR|Commit & PR/);
  assert.doesNotMatch(html, /data-act="delete"|data-act="checkout|Push &amp; PR/);
});

test('denetçi: eşlik kartında PR başlık/açıklama formu yok; "mevcut PR güncellenir" der', () => {
  const html = promoteCardHtml(comp, 'x', 'y');
  assert.match(html, /Yerel Commit'leri Pushla/);
  assert.match(html, /mevcut PR güncellenir/);
  assert.doesNotMatch(html, /promote-pr-title/);
  assert.match(promoteCardHtml({ name: 'feat/z' }, 't', 'b'), /promote-pr-title/, 'normal dalda form sürer');
});

test('commit stüdyosunun varsayılan dalı görünen ad değil gerçek dal olur', () => {
  const d = workingTreeDefaults({ ...comp, uncommittedCount: 2 }, ['a.go'], { branches: [] }, false);
  assert.equal(d.defaultWtBranch, 'feat/x');
});

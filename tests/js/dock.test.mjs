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

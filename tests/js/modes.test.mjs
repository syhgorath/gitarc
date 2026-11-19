// Kokpit kipleri: "proje yok" karşılama paneli ve "demo" şeridi (web/modules/topbar.js: modeView, modeLabel). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { modeView, modeLabel } from '../../web/modules/topbar.js';

test('a live repo shows neither the welcome card nor the demo strip', () => {
  assert.deepEqual(modeView({ mode: 'live' }), { noProject: false, demo: false });
});

test('mode none shows the welcome card and the label says so', () => {
  assert.deepEqual(modeView({ mode: 'none' }), { noProject: true, demo: false });
  assert.equal(modeLabel({ mode: 'none' }, false), 'Proje yok');
});

test('the simulation is always marked as a demo (never mistaken for a real repo)', () => {
  assert.deepEqual(modeView({ mode: 'simulation' }), { noProject: false, demo: true });
  // sunucusuz (file://) açılışta da demo şeridi çıkar
  assert.equal(modeView({ mode: 'simulation' }, true).demo, true);
  assert.equal(modeView({ mode: 'live' }, true).demo, false);
});

test('missing repo data never crashes', () => {
  assert.deepEqual(modeView(undefined), { noProject: false, demo: false });
});

test('the page ships the welcome card, its add button and the demo banner', () => {
  const html = readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
  assert.match(html, /id="no-project-card"/);
  assert.match(html, /id="btn-no-project-add"/);
  assert.match(html, /id="demo-banner"/);
  assert.match(html, /gitarc -repo sim/, 'the hint tells how to open the demo on purpose');
});

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

import { loadingHintView } from '../../web/modules/syncsteps.js';

test('the loading hint shows only while GitHub steps are still pending on a live repo', () => {
  const live = (steps) => ({ mode: 'live', syncSteps: steps });
  assert.equal(loadingHintView(live([{ id: 'fetch', state: 'ok' }, { id: 'prs', state: 'ok' }])), null);
  const h = loadingHintView(live([{ id: 'prs', state: 'pending' }, { id: 'merged', state: 'pending' }, { id: 'protection', state: 'ok' }]));
  assert.match(h.text, /GitHub verisi yükleniyor/);
  assert.match(h.title, /PR listesi/);
  assert.match(h.title, /birleşmiş PR/);
  assert.doesNotMatch(h.title, /koruma/);
});

test('the loading hint stays away from stale data, failures, the demo and non-live modes', () => {
  assert.equal(loadingHintView({ mode: 'live', syncSteps: [{ id: 'prs', state: 'stale' }] }), null, 'stale data is already shown');
  assert.equal(loadingHintView({ mode: 'live', syncSteps: [{ id: 'prs', state: 'failed' }] }), null, 'failures have their own badge');
  assert.equal(loadingHintView({ mode: 'simulation', syncSteps: [{ id: 'prs', state: 'pending' }] }), null);
  assert.equal(loadingHintView({ mode: 'none' }), null);
  assert.equal(loadingHintView(undefined), null);
  assert.equal(loadingHintView({ mode: 'live' }), null);
});

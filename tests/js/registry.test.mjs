// Guards the DOM registry in web/modules/state.js against ids that do not exist in index.html.
// A mismatch is silent at runtime (`if (el.foo)` is simply false) — that is how the sync badge
// stayed frozen on "(Synced)" for every repo. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const html = readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
const stateSrc = readFileSync(new URL('../../web/modules/state.js', import.meta.url), 'utf8');

const htmlIds = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((m) => m[1]));
const registryIds = [...stateSrc.match(/const ids = \[([\s\S]*?)\];/)[1].matchAll(/'([^']+)'/g)].map((m) => m[1]);
const aliasIds = [...stateSrc.matchAll(/el\.(\w+) = el\['([^']+)'\]/g)].map((m) => ({ prop: m[1], id: m[2] }));

// Known legacy leftovers from the dock redesign (dead no-ops). Fix or delete them, then remove from this list.
const KNOWN_DEAD = new Set(['radar-count', 'btn-logs-modal', 'log-count', 'engineStatus']);

test('every id in the DOM registry exists in index.html (except the known legacy ones)', () => {
  const missing = registryIds.filter((id) => !htmlIds.has(id) && !KNOWN_DEAD.has(id));
  assert.deepEqual(missing, [], `ids missing from index.html: ${missing.join(', ')}`);
});

test('every el.<alias> = el[id] points at an existing element (except the known legacy ones)', () => {
  const bad = aliasIds.filter((a) => !htmlIds.has(a.id) && !KNOWN_DEAD.has(a.id));
  assert.deepEqual(bad, [], `aliases pointing at missing ids: ${bad.map((b) => `${b.prop}→${b.id}`).join(', ')}`);
});

test('the sync badge alias targets the real button (regression: it pointed at a CSS class name)', () => {
  const badge = aliasIds.find((a) => a.prop === 'spineSyncBadge');
  assert.ok(badge, 'spineSyncBadge alias must exist');
  assert.equal(badge.id, 'btn-canvas-sync');
  assert.ok(htmlIds.has('btn-canvas-sync'));
});

test('the known-dead list does not hide ids that exist (keep it honest)', () => {
  const stale = [...KNOWN_DEAD].filter((id) => htmlIds.has(id));
  assert.deepEqual(stale, [], `remove from KNOWN_DEAD, they exist now: ${stale.join(', ')}`);
});

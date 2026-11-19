// Guards the DOM registry in web/modules/state.js against ids that do not exist in index.html.
// A mismatch is silent at runtime (`if (el.foo)` is simply false) — that is how the sync badge
// stayed frozen on "(Synced)" for every repo. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';

const html = readFileSync(new URL('../../web/index.html', import.meta.url), 'utf8');
const stateSrc = readFileSync(new URL('../../web/modules/state.js', import.meta.url), 'utf8');

const htmlIds = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((m) => m[1]));
const registryIds = [...stateSrc.match(/const ids = \[([\s\S]*?)\];/)[1].matchAll(/'([^']+)'/g)].map((m) => m[1]);
const aliasIds = [...stateSrc.matchAll(/el\.(\w+) = el\['([^']+)'\]/g)].map((m) => ({ prop: m[1], id: m[2] }));

test('every id in the DOM registry exists in index.html', () => {
  const missing = registryIds.filter((id) => !htmlIds.has(id));
  assert.deepEqual(missing, [], `ids missing from index.html: ${missing.join(', ')}`);
});

test('every el.<alias> = el[id] points at an existing element', () => {
  const bad = aliasIds.filter((a) => !htmlIds.has(a.id));
  assert.deepEqual(bad, [], `aliases pointing at missing ids: ${bad.map((b) => `${b.prop}→${b.id}`).join(', ')}`);
});

test('the sync badge alias targets the real button (regression: it pointed at a CSS class name)', () => {
  const badge = aliasIds.find((a) => a.prop === 'spineSyncBadge');
  assert.ok(badge, 'spineSyncBadge alias must exist');
  assert.equal(badge.id, 'btn-canvas-sync');
  assert.ok(htmlIds.has('btn-canvas-sync'));
});

// Every `el.<name>` the code reads must be defined in state.js. A read of an undefined property is silently
// `undefined` and the guarded block (`if (el.logCount) ...`) never runs: that is how the dock's old log badge died.
test('every el.<name> used in the code is defined in the registry', () => {
  const defined = new Set([...stateSrc.matchAll(/\bel\.(\w+)\s*=/g)].map((m) => m[1]));
  const dir = new URL('../../web/modules/', import.meta.url);
  const files = [...readdirSync(dir).filter((f) => f.endsWith('.js')).map((f) => new URL(f, dir)), new URL('../../web/app.js', import.meta.url)];
  const undefinedUses = [];
  for (const f of files) {
    if (f.pathname.endsWith('/state.js')) continue;
    const src = readFileSync(f, 'utf8');
    for (const m of src.matchAll(/\bel\.(\w+)/g)) {
      if (!defined.has(m[1])) undefinedUses.push(`${f.pathname.split('/').pop()}: el.${m[1]}`);
    }
  }
  assert.deepEqual([...new Set(undefinedUses)], [], 'el.* properties used but never defined in state.js');
});

// Regression: the Diff viewer ids (`diff-viewer-modal`, `diff-files-list`, ...) existed in index.html and as aliases but were
// never in the `ids` list, so `el.diffModal` was undefined and `openDiffViewer` returned at once: the modal could not be opened
// from anywhere ("Diff Gör", file rows). An alias to an id that is not registered is always undefined.
test('every el.<alias> = el[id] uses an id that is actually in the registered ids list', () => {
  const registered = new Set(registryIds);
  const orphan = aliasIds.filter((a) => !registered.has(a.id));
  assert.deepEqual(orphan.map((a) => `${a.prop}→${a.id}`), [], 'aliases pointing at ids missing from the ids list are always undefined');
});

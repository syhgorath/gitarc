// Tests for the "going remote" trace (web/modules/trace.js). Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

const calls = { appended: [], removed: 0, timers: [] };
const mkNode = (tag) => ({ tag, attrs: {}, children: [], setAttribute(k, v) { this.attrs[k] = v; }, appendChild(c) { this.children.push(c); return c; }, remove() { calls.removed++; } });
globalThis.document = { createElementNS: (_n, t) => mkNode(t), createElement: mkNode };
const realSetTimeout = globalThis.setTimeout;
globalThis.setTimeout = (fn, ms) => { calls.timers.push(ms); return 0; };

const { el } = await import('../../web/modules/state.js');
const { detectPushes, tracePathD, traceEndpoints, playPushTraces, WIP_ID, TRACE_MS } = await import('../../web/modules/trace.js');

const snap = (branches, repoId = 'r1') => ({ repo: { id: repoId }, branches });
const b = (name, hemisphere, extra = {}) => ({ id: `id-${name}`, name, hemisphere, state: 'open', ...extra });

test('a local branch that becomes an open upper-ray branch is a push', () => {
  const prev = snap([b('feat/x', 'lower')]);
  const next = snap([b('feat/x', 'upper', { id: 'remote-feat/x' })]);
  assert.deepEqual(detectPushes(prev, next), [{ name: 'feat/x', fromId: 'id-feat/x', toId: 'remote-feat/x', kind: 'promote' }]);
});

test('Commit + Push & PR from the working tree: the draft becomes a new upper branch', () => {
  const prev = snap([{ id: WIP_ID, name: 'wip/working-tree', hemisphere: 'lower', state: 'open' }]);
  const next = snap([b('feat/new', 'upper')]);
  assert.deepEqual(detectPushes(prev, next), [{ name: 'feat/new', fromId: WIP_ID, toId: 'id-feat/new', kind: 'wip' }]);
});

test('no trace for things that are not a push', () => {
  assert.deepEqual(detectPushes(null, snap([b('a', 'upper')])), [], 'first load');
  assert.deepEqual(detectPushes(snap([b('a', 'lower')]), snap([b('a', 'lower')])), [], 'unchanged');
  assert.deepEqual(detectPushes(snap([b('a', 'upper')]), snap([b('a', 'upper')])), [], 'already upper');
  assert.deepEqual(detectPushes(snap([b('a', 'lower')], 'r1'), snap([b('a', 'upper')], 'r2')), [], 'switching repos');
  assert.deepEqual(detectPushes(snap([b('a', 'lower')]), snap([b('a', 'upper', { state: 'merged' })])), [], 'a merge is not a push');
  // yeni bir üst ray dalı ama taslak yok (örn. başka biri açtı): iz yok
  assert.deepEqual(detectPushes(snap([b('a', 'upper')]), snap([b('a', 'upper'), b('b', 'upper')])), []);
});

test('one draft produces one trace even if two new upper branches appear', () => {
  const prev = snap([{ id: WIP_ID, name: 'wip', hemisphere: 'lower', state: 'open' }]);
  const next = snap([b('n1', 'upper'), b('n2', 'upper')]);
  assert.equal(detectPushes(prev, next).length, 1);
});

test('tracePathD is a smooth S curve between the two points', () => {
  assert.equal(tracePathD({ x: 100, y: 300 }, { x: 160, y: 100 }), 'M 100 300 C 154 200, 106 200, 160 100');
  assert.match(tracePathD({ x: 50, y: 10 }, { x: 50, y: 90 }), /^M 50 10 C 74 50, 26 50, 50 90$/, 'a vertical hop still bends a little');
});

test('endpoints come from the previous and next apex layouts, and are null when a position is unknown', () => {
  const prevApex = new Map([['a', { apexX: 10, apexY: 300 }]]);
  const nextApex = new Map([['b', { apexX: 20, apexY: 100 }]]);
  assert.deepEqual(traceEndpoints({ fromId: 'a', toId: 'b' }, prevApex, nextApex), { from: { x: 10, y: 300 }, to: { x: 20, y: 100 } });
  assert.equal(traceEndpoints({ fromId: 'zzz', toId: 'b' }, prevApex, nextApex), null);
  assert.equal(traceEndpoints({ fromId: 'a', toId: 'b' }, null, nextApex), null);
});

test('playPushTraces draws a path + moving dot and removes the layer afterwards', () => {
  el.svg = mkNode('svg');
  el.svg.querySelector = () => null;
  calls.timers.length = 0;
  const prevApex = new Map([['a', { apexX: 10, apexY: 300 }]]);
  const nextApex = new Map([['b', { apexX: 20, apexY: 100 }]]);
  const n = playPushTraces([{ fromId: 'a', toId: 'b' }, { fromId: 'missing', toId: 'b' }], prevApex, nextApex);
  assert.equal(n, 1, 'only the trace with known endpoints is drawn');
  const layer = el.svg.children[0];
  assert.equal(layer.attrs.id, 'trace-layer');
  assert.equal(layer.children[0].attrs.class, 'svg-push-trace');
  assert.equal(layer.children[1].attrs.class, 'svg-push-dot');
  assert.equal(layer.children[1].children[0].tag, 'animateMotion');
  assert.deepEqual(calls.timers, [TRACE_MS], 'cleanup is scheduled');
  assert.equal(playPushTraces([], prevApex, nextApex), 0);
  globalThis.setTimeout = realSetTimeout;
});

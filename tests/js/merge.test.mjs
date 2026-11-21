// Merge niyeti (web/modules/merge.js): hangi yol seçilir? app.js'ten çıkarılırken davranışın değişmediğini kilitler. Run: node --test tests/js
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

const toasts = [];
globalThis.document = { getElementById: () => null };
globalThis.window = { location: { protocol: 'http:' }, dispatchEvent: () => true };
globalThis.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
let calls = [];
let respond = () => ({ status: 200, body: {} });
globalThis.fetch = async (path, opts) => {
  calls.push({ path, method: opts?.method || 'GET', body: opts?.body ? JSON.parse(opts.body) : undefined });
  const r = respond(path);
  return { ok: r.status < 400, status: r.status, json: async () => r.body };
};

const { state } = await import('../../web/modules/state.js');
const { mergeBranch } = await import('../../web/modules/merge.js');

const pr = (ci) => ({ number: 7, reviewState: 'OPEN', ...(ci ? { ci: { state: ci } } : {}) });
const branch = (extra = {}) => ({ id: 'a', name: 'feat/a', state: 'open', pr: pr(), ...extra });
let applied;
let rerenders;
const deps = { applySnapshot: (s) => { applied.push(s); }, rerender: () => { rerenders++; } };

beforeEach(() => {
  calls = [];
  toasts.length = 0;
  applied = [];
  rerenders = 0;
  respond = () => ({ status: 200, body: {} });
  state.offlineMode = false;
  state.pendingAutoMerge = {};
  state.mergeStrategy = 'merge';
  state.snapshot = { repo: { id: 'p', requiredChecks: ['web'] }, branches: [branch()] };
});

test('çevrimdışı + CI bekleniyor: bu sekmenin bekçisi açılır, ikinci çağrı kapatır; sunucuya istek gitmez', async () => {
  state.offlineMode = true;
  await mergeBranch('a', null, deps);
  assert.ok(state.pendingAutoMerge.a, 'bekçi açılmalı');
  assert.equal(state.pendingAutoMerge.a.branchName, 'feat/a');
  assert.equal(rerenders, 1);
  await mergeBranch('a', null, deps);
  assert.equal(state.pendingAutoMerge.a, undefined, 'ikinci çağrı iptal eder');
  assert.equal(rerenders, 2);
  assert.equal(calls.length, 0);
});

test('çevrimiçi + CI henüz yok + zorunlu kontrol var: 1 PR\'lık sunucu koşusu planlanır ve başlatılır (düz merge DEĞİL)', async () => {
  respond = (path) => {
    if (path === '/api/queue/run/plan') return { status: 200, body: { startable: true, fingerprint: 'fp1' } };
    return { status: 200, body: {} };
  };
  await mergeBranch('a', null, deps);
  const paths = calls.map((c) => c.path);
  assert.deepEqual(paths.slice(0, 2), ['/api/queue/run/plan', '/api/queue/run/start']);
  assert.equal(calls[1].body.confirm, 'fp1');
  assert.deepEqual(calls[1].body.branchIds, ['a']);
  assert.ok(!paths.includes('/api/branch/merge'), 'CI belirmemişken doğrudan merge denenmemeli');
  assert.ok(paths.includes('/api/snapshot'), 'koşu başlayınca anlık görüntü tazelenir');
  assert.equal(applied.length, 1);
});

test('çevrimiçi + koşu zaten bu PR\'ı işliyor: ikinci tıklama koşuyu durdurur', async () => {
  state.snapshot.repo.queueRun = { total: 1, merged: 0, phase: 'CI bekleniyor', items: [{ branchId: 'a', branch: 'feat/a', state: 'current' }] };
  await mergeBranch('a', null, deps);
  assert.equal(calls[0].path, '/api/queue/run/stop');
  assert.ok(!calls.some((c) => c.path === '/api/branch/merge'));
});

test('CI yeşil: doğrudan birleştirir (strateji gövdede)', async () => {
  state.snapshot.branches = [branch({ pr: pr('SUCCESS') })];
  state.mergeStrategy = 'squash';
  respond = (path) => (path === '/api/branch/merge' ? { status: 200, body: { repo: { id: 'p' } } } : { status: 200, body: {} });
  await mergeBranch('a', null, deps);
  assert.equal(calls[0].path, '/api/branch/merge');
  assert.deepEqual(calls[0].body, { branchId: 'a', strategy: 'squash' });
  assert.equal(applied.length, 1);
});

test('doğrudan merge "koruma kuralı engelliyor" ile reddedilirse çıkmaz sokak yok: CI-bitince koşusu başlatılır', async () => {
  state.snapshot.repo.requiredChecks = []; // arayüz bilmiyordu: düz merge denenir
  respond = (path) => {
    if (path === '/api/branch/merge') return { status: 409, body: { error: 'PR #7 şu an birleştirilemiyor: main dalının koruma kuralı engelliyor (...)' } };
    if (path === '/api/queue/run/plan') return { status: 200, body: { startable: true, fingerprint: 'fp2' } };
    return { status: 200, body: {} };
  };
  await mergeBranch('a', null, deps);
  const paths = calls.map((c) => c.path);
  assert.equal(paths[0], '/api/branch/merge');
  assert.ok(paths.includes('/api/queue/run/start'), 'reddedilince 1 PR\'lık koşu başlatılmalı');
});

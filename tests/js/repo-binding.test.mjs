// Proje bağlama (istemci): her istek sekmenin gösterdiği projenin kimliğini taşır; sunucu uyuşmazlıkta 409 verirse sekme
// sunucunun projesini benimser. Proje değişince projeye özel durum sıfırlanır. Run: node --test tests/js
import { test, beforeEach } from 'node:test';
import assert from 'node:assert/strict';

const events = [];
const store = new Map();
globalThis.window = { location: { protocol: 'http:' }, dispatchEvent: (ev) => { events.push(ev); return true; } };
globalThis.CustomEvent = class { constructor(type, init) { this.type = type; this.detail = init?.detail; } };
globalThis.localStorage = { getItem: (k) => (store.has(k) ? store.get(k) : null), setItem: (k, v) => store.set(k, String(v)), removeItem: (k) => store.delete(k) };

let calls = [];
let nextResponse = { status: 200, body: {} };
globalThis.fetch = async (path, opts) => {
  calls.push({ path, opts });
  return { ok: nextResponse.status < 400, status: nextResponse.status, json: async () => nextResponse.body };
};

const { apiCall, isRepoBindingExempt } = await import('../../web/modules/api.js');
const { state, resetProjectScopedState } = await import('../../web/modules/state.js');

beforeEach(() => {
  calls = [];
  events.length = 0;
  store.clear();
  nextResponse = { status: 200, body: {} };
  state.offlineMode = false;
  state.snapshot = { repo: { id: 'proj-a' } };
});

test('mutating and project-specific requests carry the id of the project the tab shows', async () => {
  await apiCall('/api/branch/delete', 'POST', { branchId: 'x' });
  assert.equal(calls[0].opts.headers['X-GitArc-Repo'], 'proj-a');
  await apiCall('/api/diff?target=x');
  assert.equal(calls[1].opts.headers['X-GitArc-Repo'], 'proj-a');
});

test('project-independent endpoints are exempt (the tab learns / changes the project through them)', async () => {
  for (const p of ['/api/snapshot', '/api/repo/switch', '/api/repo/add', '/api/repo/remove', '/api/system/git-status-ping?x=1', '/api/system/update-check']) {
    assert.equal(isRepoBindingExempt(p), true, p);
  }
  assert.equal(isRepoBindingExempt('/api/branch/delete'), false);
  assert.equal(isRepoBindingExempt('/api/repo/protect'), false, 'a path that merely starts with /api/repo/ is not exempt');
  await apiCall('/api/snapshot');
  assert.equal(calls[0].opts.headers['X-GitArc-Repo'], undefined);
});

test('no header before the first snapshot (nothing to bind to)', async () => {
  state.snapshot = null;
  await apiCall('/api/branch/create', 'POST', {});
  assert.equal(calls[0].opts.headers['X-GitArc-Repo'], undefined);
});

test("a 409 repo_mismatch adopts the server's project, tells the UI, and throws the server's message", async () => {
  nextResponse = { status: 409, body: { error: 'Proje değişti: işlem yapılmadı', code: 'repo_mismatch', activeRepoId: 'proj-b' } };
  await assert.rejects(() => apiCall('/api/branch/delete', 'POST', {}), /Proje değişti/);
  assert.equal(store.get('gitarc_repo_id'), 'proj-b', 'remembered id follows the server so a reload does not flip it back');
  assert.equal(events.length, 1);
  assert.equal(events[0].type, 'gitarc:repo-mismatch');
  assert.equal(events[0].detail.activeRepoId, 'proj-b');
});

test('other 409s (e.g. no project yet) are plain errors, not a project switch', async () => {
  nextResponse = { status: 409, body: { error: 'Önce bir proje ekleyin' } };
  await assert.rejects(() => apiCall('/api/branch/delete', 'POST', {}), /Önce bir proje/);
  assert.equal(events.length, 0);
  assert.equal(store.has('gitarc_repo_id'), false);
});

test('switching project clears selection, drafts and the queued auto-merges of the old project', () => {
  Object.assign(state, {
    selectedBranchId: 'feat/x', selectedNodeSha: 'abc',
    pendingAutoMerge: { 'feat/x': { branchName: 'feat/x' } }, autoMergingInFlight: { 'feat/x': true },
    draftPrEdits: { 'feat/x': { title: 't' } }, proposalDraftEdits: { a: 1 }, localRunOutput: { a: 'x' },
    studio: { open: true }, baseOpen: true, longLivedOpen: true, historyOpen: true,
  });
  resetProjectScopedState();
  assert.equal(state.selectedBranchId, null);
  assert.equal(state.selectedNodeSha, null);
  assert.deepEqual(state.pendingAutoMerge, {}, 'a queued auto-merge must not follow the user into another project');
  assert.deepEqual(state.autoMergingInFlight, {});
  assert.deepEqual(state.draftPrEdits, {});
  assert.deepEqual(state.proposalDraftEdits, {});
  assert.equal(state.studio, null);
  assert.equal(state.baseOpen || state.longLivedOpen || state.historyOpen, false);
});

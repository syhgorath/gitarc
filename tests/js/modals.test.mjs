// Zero-dependency tests for the modal modules split out of modals.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

const { state } = await import('../../web/modules/state.js');
const { studioPreviewForStrategy, getActiveStudioFile } = await import('../../web/modules/conflict-studio.js');
const { TOUR_STEPS } = await import('../../web/modules/tour.js');
const { buildMCPConfigText } = await import('../../web/modules/mcp-modal.js');

test('studioPreviewForStrategy picks the right preview', () => {
  const f = { path: 'a', unionPreview: 'U', oursPreview: 'O', theirsPreview: 'T' };
  assert.equal(studioPreviewForStrategy(f, 'union', {}), 'U');
  assert.equal(studioPreviewForStrategy(f, 'ours', {}), 'O');
  assert.equal(studioPreviewForStrategy(f, 'theirs', {}), 'T');
  assert.equal(studioPreviewForStrategy(f, 'custom', {}), 'U');
  assert.equal(studioPreviewForStrategy(f, 'custom', { a: '' }), '');
  assert.equal(studioPreviewForStrategy(null, 'ours', {}), '');
});

test('getActiveStudioFile falls back to the first file', () => {
  state.studio = null;
  assert.equal(getActiveStudioFile(), null);
  state.studio = { payload: { files: [{ path: 'x' }, { path: 'y' }] }, activeFileIdx: 1 };
  assert.equal(getActiveStudioFile().path, 'y');
  state.studio.activeFileIdx = 9;
  assert.equal(getActiveStudioFile().path, 'x');
  state.studio = null;
});

test('tour steps are numbered consistently', () => {
  TOUR_STEPS.forEach((s, i) => assert.equal(s.badge, `Adım ${i + 1} / ${TOUR_STEPS.length}`));
});

test('MCP config uses the live repo path and never a personal path', () => {
  state.snapshot = { repo: { mode: 'live', path: '/work/r/' } };
  const live = JSON.parse(buildMCPConfigText()).mcpServers.gitarc;
  assert.equal(live.command, '/work/r/gitarc');
  assert.deepEqual(live.args, ['-mcp', '-repo', '/work/r/']);
  state.snapshot = null;
  assert.ok(!buildMCPConfigText().includes('/Users/'));
});

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
  // command = çalışan GitArc ikilisi (snapshot.repo.execPath); açık projenin klasörü DEĞİL (başka projede ikili orada yok).
  state.snapshot = { repo: { mode: 'live', path: '/work/r/', execPath: '/opt/gitarc/gitarc' } };
  const live = JSON.parse(buildMCPConfigText()).mcpServers.gitarc;
  assert.equal(live.command, '/opt/gitarc/gitarc');
  assert.deepEqual(live.args, ['-mcp', '-repo', '/work/r/']);
  state.snapshot = null;
  assert.ok(!buildMCPConfigText().includes('/Users/'));
});


// Çakışma Stüdyosu: strateji düğmeleri / editör (modül bölünmesinde işleyiciler kaybolmuştu; düğmeler hiç tıklanamıyordu).
const { chooseStudioStrategy, editStudioPreview } = await import('../../web/modules/conflict-studio.js');

test('chooseStudioStrategy: seçilen strateji geçer; özel editördeyken yazılan metin dosyaya kaydedilir; bilinmeyen anahtar union olur', () => {
  const file = { path: 'a.go' };
  const st = { strategy: 'ours', customByPath: {} };
  assert.equal(chooseStudioStrategy(st, 'theirs', file, 'x'), true);
  assert.equal(st.strategy, 'theirs');
  assert.deepEqual(st.customByPath, {}, 'özel değilken metin kaydedilmez');
  chooseStudioStrategy(st, 'custom', file, 'x');
  chooseStudioStrategy(st, 'union', file, 'benim düzenlemem');
  assert.equal(st.customByPath['a.go'], 'benim düzenlemem', 'özelden çıkarken yazılan metin kaybolmamalı');
  chooseStudioStrategy(st, 'saçma', file, '');
  assert.equal(st.strategy, 'union');
  assert.equal(chooseStudioStrategy(null, 'ours', file, ''), false);
});

test('editStudioPreview: elle yazınca strateji "özel"e döner (yoksa yazılanlar sessizce uygulanmazdı); zaten özelse değişmez', () => {
  const file = { path: 'a.go' };
  const st = { strategy: 'ours', customByPath: {} };
  assert.equal(editStudioPreview(st, file, 'yeni'), true);
  assert.equal(st.strategy, 'custom');
  assert.equal(st.customByPath['a.go'], 'yeni');
  assert.equal(editStudioPreview(st, file, 'yeni 2'), false);
  assert.equal(st.customByPath['a.go'], 'yeni 2');
  assert.equal(editStudioPreview(null, file, 'x'), false);
});

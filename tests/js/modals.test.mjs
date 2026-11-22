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


// Çakışma Stüdyosu: strateji DOSYA BAŞINA seçilir (eskiden tek strateji tüm dosyalara uygulanıyordu; karışık çakışmalar çözülemiyordu).
const { chooseStudioStrategy, editStudioPreview, studioStrategyOf, studioApplyPayload } = await import('../../web/modules/conflict-studio.js');
const mk = (extra = {}) => ({ strategy: 'union', strategyByPath: {}, customByPath: {}, payload: { files: [{ path: 'a.md' }, { path: 'b.go' }, { path: 'c.lock' }] }, ...extra });

test('chooseStudioStrategy: seçim YALNIZCA etkin dosyayı etkiler; diğerleri varsayılanda kalır', () => {
  const st = mk();
  const [a, b] = st.payload.files;
  assert.equal(chooseStudioStrategy(st, 'ours', b, ''), true);
  assert.equal(studioStrategyOf(st, b), 'ours');
  assert.equal(studioStrategyOf(st, a), 'union', 'başka dosya varsayılanda kalmalı');
  chooseStudioStrategy(st, 'saçma', a, '');
  assert.equal(studioStrategyOf(st, a), 'union', 'bilinmeyen anahtar union olur');
  assert.equal(chooseStudioStrategy(null, 'ours', a, ''), false);
  assert.equal(chooseStudioStrategy(st, 'ours', null, ''), false);
});

test('chooseStudioStrategy: özel dosyadan çıkarken yazılan metin kaybolmaz; "tümüne uygula" her dosyaya yazar', () => {
  const st = mk();
  const [a, b] = st.payload.files;
  chooseStudioStrategy(st, 'custom', a, '');
  chooseStudioStrategy(st, 'theirs', a, 'benim düzenlemem');
  assert.equal(st.customByPath['a.md'], 'benim düzenlemem');
  chooseStudioStrategy(st, 'ours', b, '', true);
  assert.deepEqual(st.payload.files.map((f) => studioStrategyOf(st, f)), ['ours', 'ours', 'ours']);
  assert.equal(st.customByPath['a.md'], 'benim düzenlemem', 'özel metin dosyaya özgü kalır');
});

test('editStudioPreview: elle yazınca YALNIZCA o dosya "özel"e döner (yoksa yazılanlar sessizce uygulanmazdı)', () => {
  const st = mk({ strategy: 'ours' });
  const [a, b] = st.payload.files;
  assert.equal(editStudioPreview(st, a, 'yeni'), true);
  assert.equal(studioStrategyOf(st, a), 'custom');
  assert.equal(studioStrategyOf(st, b), 'ours', 'diğer dosya etkilenmez');
  assert.equal(editStudioPreview(st, a, 'yeni 2'), false);
  assert.equal(st.customByPath['a.md'], 'yeni 2');
  assert.equal(editStudioPreview(null, a, 'x'), false);
});

test('studioApplyPayload: her dosya için strateji AÇIKÇA gider; özel metin yalnızca özel dosyalar için; etiket hepsi aynıysa o, değilse karma', () => {
  const st = mk({ strategy: 'union' });
  const [a, b, c] = st.payload.files;
  chooseStudioStrategy(st, 'union', a, '');
  chooseStudioStrategy(st, 'ours', b, '');
  editStudioPreview(st, c, 'elle yazdım');
  chooseStudioStrategy(st, 'ours', a, '', false); // a: ours
  st.customByPath['b.go'] = 'eski özel metin';      // b özel DEĞİL: gönderilmemeli
  const p = studioApplyPayload(st);
  assert.deepEqual(p.fileStrategies, { 'a.md': 'ours', 'b.go': 'ours', 'c.lock': 'custom' });
  assert.deepEqual(p.customFiles, { 'c.lock': 'elle yazdım' });
  assert.equal(p.label, 'karma');
  const same = studioApplyPayload(mk({ strategy: 'theirs' }));
  assert.equal(same.label, 'theirs');
  assert.equal(same.customFiles, undefined);
});

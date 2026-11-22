// "Şu an hangi daldasın" çipi: ana dalda sakin, başka dalda dikkat çeker, dal bilinmiyorsa gizlenir.
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { headChipModel } from '../../web/modules/topbar.js';

test('headChipModel: canlı depoda sunucunun currentBranch değeri kullanılır; ana dal sakin, başka dal uyarır', () => {
  const on = headChipModel({ mode: 'live', currentBranch: 'main', defaultBranch: 'main', dirtyCount: 0 }, []);
  assert.deepEqual([on.show, on.tone, on.text], [true, 'on-default', '🌿 main']);
  const off = headChipModel({ mode: 'live', currentBranch: 'feat/x', defaultBranch: 'main', dirtyCount: 15 }, []);
  assert.equal(off.tone, 'off-default');
  assert.equal(off.text, '🌿 feat/x · 15 kaydedilmemiş');
  assert.match(off.title, /"feat\/x" dalında \(main değil\); 15 dosya kaydedilmemiş/);
});

test('headChipModel: demo/yedek motorda HEAD dalından türetilir; dal ya da proje yoksa gizli', () => {
  const sim = headChipModel({ mode: 'simulation', defaultBranch: 'main' }, [{ name: 'a', isHead: false }, { name: 'b', isHead: true }]);
  assert.deepEqual([sim.show, sim.text], [true, '🌿 b']);
  assert.equal(headChipModel({ mode: 'live', defaultBranch: 'main' }, []).show, false, 'detached HEAD: dal adı uydurulmaz');
  assert.equal(headChipModel({ mode: 'none', currentBranch: 'main' }, []).show, false);
  assert.equal(headChipModel(null, null).show, false);
});

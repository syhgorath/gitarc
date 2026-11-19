// web/modules/kpi.js - Üstteki KPI çipleri (Tümü, Benim Dallarım, Yerel, ...) ve "Tümü sayacı" ipucu
//
// Sayılar filters.js tablosundan gelir (tuvaldeki süzmeyle aynı kaynak). `buildKpiModel` saf bir
// fonksiyondur ve Node'da test edilir; `renderKPIs` yalnızca DOM'a yazar.
import { state, el } from './state.js';
import { esc } from './utils.js';
import { filterContext, countMatching, dirtyFileCount } from './filters.js';

// Snapshot'tan çip tanımlarını ve sayaçları üretir (DOM yok).
export function buildKpiModel(snapshot) {
  const branches = snapshot?.branches || [];
  const localBranches = branches.filter((b) => b.hasLocal && !b.id.startsWith('gh-merged-') && b.id !== 'local-wip-working-tree');
  const localCount = localBranches.length;
  const mergedPRCount = branches.filter((b) => b.id.startsWith('gh-merged-') || (b.state === 'merged' && !b.hasLocal)).length;
  const dirtyCount = dirtyFileCount(snapshot);

  // Çip sayıları, süzmeyle aynı koşul tablosundan (filters.js) hesaplanır: sayı ile süzme sapamaz.
  const ctx = filterContext(snapshot);
  const upperCount = countMatching('upper', branches, ctx);
  const lowerCount = countMatching('lower', branches, ctx);
  const mineCount = countMatching('mine', branches, ctx);
  const conflictCount = countMatching('issues', branches, ctx);
  const failCount = countMatching('ci_fail', branches, ctx);
  const goneCount = countMatching('gone', branches, ctx);

  const allTitle = `Tümü (${branches.length} düğüm):\n• ${localCount} yerel dal (klasik 'git branch' sayısı)\n• ${mergedPRCount} GitHub birleşmiş PR parabolü\n• ${branches.length - localCount - mergedPRCount} diğer uzak/sanal dal.`;

  const defs = [
    { key: 'all', label: 'Tümü', val: branches.length, tone: '', title: allTitle },
    { key: 'mine', label: '👤 Benim Dallarım (@me)', val: mineCount, tone: mineCount ? 'accent' : '', title: 'Giriş yapan kullanıcıya ait açık dallar' },
    { key: 'lower', label: 'Yerel (Local)', val: lowerCount, tone: lowerCount ? 'purple' : '', title: 'PR açılmamış açık yerel dallar' },
    { key: 'dirty', label: 'Değişiklik (Dirty)', val: dirtyCount, tone: dirtyCount ? 'warn' : '', title: `Çalışma ağacında ${dirtyCount} adet kaydedilmemiş dosya var.` },
    { key: 'issues', label: 'Çakışma / Cross-PR', val: conflictCount, tone: conflictCount ? 'warn' : '', title: 'Çakışan veya aynı dosyaları değiştiren açık dallar' },
    { key: 'ci_fail', label: 'CI Hatası', val: failCount, tone: failCount ? 'danger' : '', title: 'CI testleri başarısız olan dallar' },
    { key: 'gone', label: 'Birleşmiş / Gone', val: goneCount, tone: goneCount ? 'ok' : '', title: 'Main ile birleşmiş veya uzaktaki dalı silinmiş temizleme adayları' },
  ];

  return {
    defs, upperCount, localCount, mergedPRCount, total: branches.length,
    hintHtml: `💡 <strong>Tümü Sayacı (${branches.length}):</strong> ${localCount} yerel dal + ${mergedPRCount} birleşmiş PR parabolüdür.`,
  };
}

export function renderKPIs(onFilterChange = null) {
  const model = buildKpiModel(state.snapshot);

  if (!el.kpiPills) return;
  el.kpiPills.innerHTML = '';
  model.defs.forEach((d) => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = `kpi-pill ${d.tone} ${state.filter === d.key ? 'active' : ''}`;
    if (d.title) btn.title = d.title;
    btn.innerHTML = `<span>${esc(d.label)}</span><strong>${d.val}</strong>`;
    btn.addEventListener('click', () => {
      state.filter = d.key;
      renderKPIs(onFilterChange);
      if (onFilterChange) onFilterChange();
    });
    el.kpiPills.appendChild(btn);
  });

  const prBadge = document.createElement('span');
  prBadge.className = `kpi-pill ${model.upperCount ? 'accent' : ''}`;
  prBadge.title = "GitHub üzerinde açık olan Pull Request'ler";
  prBadge.innerHTML = `<span>Uzak (PR)</span><strong>${model.upperCount}</strong>`;
  el.kpiPills.appendChild(prBadge);

  if (el.dragHint) {
    el.dragHint.innerHTML = model.hintHtml;
  }
}

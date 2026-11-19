// web/modules/filters.js - Dal süzgeçleri ve KPI sayıları için TEK koşul tablosu (DOM yok, Node'da test edilir)
//
// Eskiden `matchesFilter` (tuvaldeki süzme) ve `renderKPIs` (üstteki çip sayıları) aynı koşulları
// ayrı ayrı yazıyordu; biri değişince çipteki sayı ile tuvalde görünen dal sayısı birbirinden sapabiliyordu.
// (Örnek: `upper` süzgeci hiç tanımlı değildi, "Üst Ray" tıklaması tüm dalları gösteriyordu.)
// Artık ikisi de buradaki aynı `FILTERS` tablosundan okur.
import { isMergedOrGone, isOpenPR, isLocalOpen, isMyBranch } from './utils.js';

const hasUnresolvedConflict = (b) => Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
const hasCrossPR = (b) => Boolean(b.crossPr && b.crossPr.length > 0);
const isOpenBranch = (b) => b.state === 'open' && !isMergedOrGone(b);

// Her süzgeç: test(branch, ctx) → boolean. ctx = filterContext(snapshot).
export const FILTERS = {
  all: { test: () => true },
  mine: { test: (b, ctx) => isOpenBranch(b) && isMyBranch(b, ctx.currentUser) },
  lower: { test: (b) => isLocalOpen(b) },
  upper: { test: (b) => isOpenPR(b) },
  // Not: "dirty" süzgeci yalnızca HEAD dalını gösterir; çipin değeri ise dal sayısı değil DOSYA sayısıdır
  // (bkz. dirtyFileCount).
  dirty: { test: (b, ctx) => Boolean(b.isHead && ((b.uncommittedCount || 0) > 0 || ctx.dirtyCount > 0)) },
  issues: { test: (b) => isOpenBranch(b) && (hasUnresolvedConflict(b) || hasCrossPR(b)) },
  ci_fail: { test: (b) => b.state === 'open' && b.pr?.ci?.state === 'FAILURE' },
  gone: { test: (b) => isMergedOrGone(b) },
};

export function filterContext(snapshot) {
  return {
    currentUser: snapshot?.repo?.currentUser,
    dirtyCount: snapshot?.repo?.dirtyCount || 0,
  };
}

// Klasör/dosya merceği: dal adı veya değişen dosyalardan birinde geçiyor mu? Boş sorgu her şeyi eşler.
export function matchesPathLens(b, query) {
  const q = (query || '').trim().toLowerCase();
  if (!q) return true;
  if ((b.name || '').toLowerCase().includes(q)) return true;
  return (b.changedFiles || []).some((f) => f.toLowerCase().includes(q));
}

// Bilinmeyen süzgeç anahtarı her şeyi eşler (eski davranış).
export function filterTest(key) {
  return FILTERS[key]?.test || FILTERS.all.test;
}

export function matchesBranchFilter(b, { filter, pathLens, ctx }) {
  if (!matchesPathLens(b, pathLens)) return false;
  return Boolean(filterTest(filter)(b, ctx || filterContext(null)));
}

// Bir süzgeci sağlayan dal sayısı: KPI çipleri bunu kullanır, böylece sayı süzmeyle her zaman aynıdır.
export function countMatching(key, branches, ctx) {
  const test = filterTest(key);
  return (branches || []).filter((b) => test(b, ctx)).length;
}

// "Değişiklik (Dirty)" çipinin değeri: kirli DOSYA sayısı (depo geneli; yoksa HEAD dalınınki).
export function dirtyFileCount(snapshot) {
  const branches = snapshot?.branches || [];
  return snapshot?.repo?.dirtyCount || branches.find((b) => b.isHead)?.uncommittedCount || 0;
}

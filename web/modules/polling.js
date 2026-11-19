// web/modules/polling.js - Arka plan yoklamasının "şimdi tam snapshot çek" kararı (DOM yok, Node'da test edilir).
//
// Eski kural: diskte git değişikliği yoksa hiç snapshot çekilmez; bir dal/commit seçiliyken yoklama tamamen durur.
// Bu, CI koşan bir PR'da takılıyordu: GitHub verisi yalnızca bir snapshot istendiğinde yenilenir (disk değişmez,
// imza da değişmez), üstüne kullanıcı o dalı incelerken (seçili) yoklama zaten durur. CI bitse de ekranda sonsuza
// dek "Kontroller Devam Ediyor..." kalıyordu. Şimdi CI koşuyorsa seçili olsa bile aralıklı tam snapshot çekilir.

export const CI_REFRESH_MS = 8000;

export function hasPendingCI(snap) {
  return (snap?.branches || []).some((b) => b.state === 'open' && b.pr?.ci?.state === 'RUNNING');
}

// ctx: { pingChanged, snap, now, lastFullAt, selectionActive }
export function pollDecision(ctx) {
  const { pingChanged = false, snap = null, now = 0, lastFullAt = 0, selectionActive = false } = ctx || {};
  if (hasPendingCI(snap) && now - lastFullAt >= CI_REFRESH_MS) return 'fetch';
  if (pingChanged && !selectionActive) return 'fetch'; // seçili öğeyi git değişikliğiyle zıplatma
  return 'skip';
}

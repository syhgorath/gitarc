// web/modules/polling.js - Arka plan yoklamasının "şimdi tam snapshot çek" kararı (DOM yok, Node'da test edilir).
//
// Eski kural: diskte git değişikliği yoksa hiç snapshot çekilmez; bir dal/commit seçiliyken yoklama tamamen durur.
// Bu, CI koşan bir PR'da takılıyordu: GitHub verisi yalnızca bir snapshot istendiğinde yenilenir (disk değişmez,
// imza da değişmez), üstüne kullanıcı o dalı incelerken (seçili) yoklama zaten durur. CI bitse de ekranda sonsuza
// dek "Kontroller Devam Ediyor..." kalıyordu. Şimdi CI koşuyorsa seçili olsa bile aralıklı tam snapshot çekilir.

export const CI_REFRESH_MS = 8000;

export function hasPendingCI(snap) {
  if (snap?.repo?.queueRun) return true; // çalışan kuyruk koşusu: kartlardaki rozet/aşama güncel kalsın
  if (snap?.repo?.mainCI?.state === 'RUNNING') return true; // merge sonrası omurga testleri de bitince görünmeli
  return (snap?.branches || []).some((b) => b.state === 'open' && b.pr?.ci?.state === 'RUNNING');
}

// ctx: { pingChanged, bgChanged, snap, now, lastFullAt, selectionActive }
// bgChanged: arka planda yeni GitHub/fetch verisi geldi (disk değişmeden). Sunucu eldeki bayat veriyi hemen gösterip
// yenisini arka planda topladığı için bunu yakalayıp yenilemek şart; yoksa seçili bir öğe varken ekran bayat kalırdı.
export function pollDecision(ctx) {
  const { pingChanged = false, bgChanged = false, snap = null, now = 0, lastFullAt = 0, selectionActive = false } = ctx || {};
  if (bgChanged) return 'fetch';
  if (hasPendingCI(snap) && now - lastFullAt >= CI_REFRESH_MS) return 'fetch';
  if (pingChanged && !selectionActive) return 'fetch'; // seçili öğeyi git değişikliğiyle zıplatma
  return 'skip';
}

// web/modules/syncsteps.js - Veri toplama adımlarının (git fetch, PR listesi, koruma...) arayüz gösterimi.
// Saf fonksiyonlar: DOM'a dokunmaz, Node'da test edilir.
//
// "main ↔ origin/main (Synced)" rozeti yalnızca YEREL origin/main referansına bakar; fetch hiç
// tamamlanmamış olsa bile "Synced" diyebilir. Bu adımlar hangi verinin eski/eksik olduğunu gösterir.

const ICONS = { ok: '✓', pending: '⏳', stale: '↻', failed: '✗', unknown: '?', skipped: '–' };

export function formatAgo(iso, now = Date.now()) {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const sec = Math.max(0, Math.round((now - t) / 1000));
  if (sec < 5) return 'az önce';
  if (sec < 60) return `${sec} sn önce`;
  if (sec < 3600) return `${Math.floor(sec / 60)} dk önce`;
  if (sec < 86400) return `${Math.floor(sec / 3600)} sa önce`;
  return `${Math.floor(sec / 86400)} gün önce`;
}

export function formatDuration(ms) {
  if (!ms || ms < 0) return '';
  return ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} sn`;
}

export function stepTone(step) {
  switch (step?.state) {
    case 'ok': return 'ok';
    case 'failed': return 'bad';
    // "unknown" (örn. yönetici olmayan kullanıcıda koruma durumu) kalıcı ve zararsız bir bilgidir;
    // rozeti sürekli sarıya boyayıp gerçek sorunları gölgelememesi için sönük gösterilir.
    case 'skipped':
    case 'unknown': return 'muted';
    default: return 'warn'; // pending, stale
  }
}

// Tek adımın rozet metni ve açıklaması.
export function describeStep(step, now = Date.now()) {
  const icon = ICONS[step.state] || '?';
  const label = step.label || step.id;
  let text = `${label} ${icon}`;
  if (step.state === 'ok' && step.count > 0) text += ` ${step.count}`;

  const parts = [];
  switch (step.state) {
    case 'ok': parts.push(step.count > 0 ? `${step.count} kayıt alındı` : 'tamamlandı'); break;
    case 'pending': parts.push('yükleniyor (arka planda sürüyor)'); break;
    case 'stale': parts.push('yenileniyor; önceki sonuç gösteriliyor'); break;
    case 'failed': parts.push('başarısız'); break;
    case 'unknown': parts.push('okunamadı'); break;
    case 'skipped': parts.push('atlandı'); break;
    default: break;
  }
  if (step.durationMs) parts.push(formatDuration(step.durationMs));
  const ago = formatAgo(step.at, now);
  if (ago) parts.push(ago);
  if (step.detail) parts.push(step.detail);
  return { id: step.id, text, tone: stepTone(step), title: `${label}: ${parts.join(' · ')}` };
}

// Tüm adımların özeti: genel ton ve rozet için öğeler.
export function summarizeSync(steps, now = Date.now()) {
  const list = Array.isArray(steps) ? steps : [];
  const items = list.map((s) => describeStep(s, now));
  let tone = 'ok';
  if (items.some((i) => i.tone === 'bad')) tone = 'bad';
  else if (items.some((i) => i.tone === 'warn')) tone = 'warn';
  return { tone, items, allOk: items.length > 0 && tone === 'ok', title: items.map((i) => i.title).join('\n') };
}

// İlk açılışta GitHub verisi (PR listesi, birleşmiş PR, koruma) arka planda toplanırken tuvalde PR görünmez; kullanıcı
// "bu depoda PR yok" sanmasın diye kısa bir ipucu. Veri bir kez gelince (adımlar `ok`/`stale`) ipucu kalkar.
export function loadingHintView(repo) {
  if (!repo || repo.mode !== 'live') return null;
  const steps = repo.syncSteps || [];
  const waiting = steps.filter((s) => ['prs', 'merged', 'protection'].includes(s.id) && s.state === 'pending');
  if (waiting.length === 0) return null;
  const labels = waiting.map((s) => ({ prs: 'PR listesi', merged: 'birleşmiş PR\'lar', protection: 'koruma' }[s.id])).join(', ');
  return { text: '⏳ GitHub verisi yükleniyor…', title: `Henüz gelmeyen: ${labels}. Büyük depolarda birkaç saniye sürebilir; gelince ekran kendiliğinden güncellenir.` };
}

// Merge sonrası omurga testleri (main CI). Çalıştırma yoksa/okunamıyorsa null: "yeşil" asla varsayılmaz.
export function mainCIView(repo) {
  const ci = repo?.mainCI;
  if (!ci || !ci.state) return null;
  const def = ci.branch || repo?.defaultBranch || 'main';
  const detail = [ci.workflow, ci.sha, ci.title].filter(Boolean).join(' · ');
  const safeUrl = typeof ci.url === 'string' && ci.url.startsWith('https://') ? ci.url : '';
  if (ci.state === 'FAILURE') return { state: 'FAILURE', text: `❌ ${def} CI kırık`, btnClass: 'danger', title: `${def} testleri BAŞARISIZ · ${detail}`, url: safeUrl };
  if (ci.state === 'RUNNING') return { state: 'RUNNING', text: `⏳ ${def} testleri koşuyor`, btnClass: 'warn running-pulse', title: `Merge sonrası ${def} testleri sürüyor · ${detail}`, url: safeUrl };
  return { state: 'SUCCESS', text: `✓ ${def} CI`, btnClass: 'ok', title: `${def} testleri başarılı · ${detail}`, url: safeUrl };
}

// Tuvaldeki merge düğümü rozeti: main CI'ı YALNIZCA SHA'sı CI koşusunun SHA'sıyla eşleşen omurga düğümüne bağlar.
// mainCI yalnızca omurganın SON koşusudur; eşleşme yoksa (eski merge düğümleri, SHA okunamadı) rozet YOKTUR ve
// asla "yeşil" varsayılmaz. Dönen nesne: { sha, state, text, tone, detail } ya da null.
export function mainCIBadge(snap) {
  const ci = snap?.repo?.mainCI;
  const sha = typeof ci?.sha === 'string' ? ci.sha.trim() : '';
  if (!ci || !ci.state || sha.length < 7) return null;
  const node = (snap.nodes || []).find((n) => typeof n.shortSha === 'string' && n.shortSha.length >= 7
    && (n.shortSha.startsWith(sha.slice(0, 7)) || sha.startsWith(n.shortSha.slice(0, 7))));
  if (!node) return null;
  const view = mainCIView(snap.repo);
  if (!view) return null;
  const text = { RUNNING: '⏳ testler koşuyor', FAILURE: '❌ main CI kırık', SUCCESS: '✓ main CI' }[ci.state] || null;
  if (!text) return null;
  const tone = { RUNNING: 'running', FAILURE: 'fail', SUCCESS: 'ok' }[ci.state];
  return { sha: node.shortSha, state: ci.state, text, tone, detail: view.title, url: view.url };
}

// Merge düğümüne tıklayınca Inspector'da görünen "main CI" kartının satırları (saf model).
export function mainCICardModel(snap, node) {
  const badge = mainCIBadge(snap);
  if (!badge || !node || badge.sha !== node.shortSha) return null;
  const ci = snap.repo.mainCI;
  const rows = [];
  if (ci.workflow) rows.push(['İş akışı', ci.workflow]);
  rows.push(['Commit', node.shortSha]);
  if (ci.branch || snap.repo.defaultBranch) rows.push(['Dal', ci.branch || snap.repo.defaultBranch]);
  if (ci.title) rows.push(['Başlık', ci.title]);
  if (ci.updatedAt) rows.push(['Son güncelleme', ci.updatedAt]);
  const note = { RUNNING: 'Merge sonrası testler sürüyor; bitince bu rozet güncellenir.', FAILURE: 'Merge sonrası testler BAŞARISIZ: main kırık olabilir, GitHub\'daki koşuyu inceleyin.', SUCCESS: 'Merge sonrası testler başarılı.' }[ci.state];
  return { text: badge.text, tone: badge.tone, rows, note, url: badge.url };
}

// Çalışan otomatik kuyruk koşusundaki PR'ın kart rozeti (saf model). Koşu yoksa/bittiyse ya da bu dal koşuda değilse null.
// Bekleyen PR: "🚦 Kuyrukta 3/5"; şu an işlenen: "⏳ İşleniyor · CI bekleniyor"; bitenler zaten birleşmiş kapsüldür (rozet yok).
export function queueRunBadge(snap, b) {
  const q = snap?.repo?.queueRun;
  if (!q || !Array.isArray(q.items) || !b) return null;
  const i = q.items.findIndex((it) => it.branchId === b.id || it.branch === b.name);
  if (i < 0) return null;
  const it = q.items[i];
  const pos = `${i + 1}/${q.total || q.items.length}`;
  if (it.state === 'current') return { text: `⏳ İşleniyor · ${q.phase || 'planlanıyor'}`, tone: 'current', title: `Kuyruk koşusu ${pos}: bu PR şu an işleniyor (${q.phase || 'planlanıyor'})` };
  if (it.state === 'waiting') return { text: `🚦 Kuyrukta ${pos}`, tone: 'waiting', title: `Kuyruk koşusunda sırada bekliyor (${pos}); önceki PR'lar bitince işlenecek` };
  return null;
}

// Geride kalan PR korumalı depoda DOĞRUDAN birleşemez ("dal güncel olmalı" açıksa). Bu durumda "Merge" yerine "Güncelle & Birleştir"
// (tek onaylı 1 PR'lık kuyruk koşusu) sunulur. Kural bilinmiyorsa (ruleset/okunamadı) sunulmaz: gerçek bir retle karşılaşılırsa
// açıklayıcı hata gösterilir ve aynı plan açılır. Çakışma ayrı bir durumdur (Çakışma Stüdyosu).
export function needsUpdateBeforeMerge(b, snap) {
  return Boolean(b && b.state === 'open' && b.pr && b.pr.number > 0 && (b.behindMain || 0) > 0
    && snap?.repo?.strictUpToDate && !(b.conflict && b.conflict.hasConflict));
}

// Proje değiştirirken uyarı: bu projede KUYRUK KOŞUSU sürüyorsa (RepoMeta.queueRun yalnızca çalışan koşuda vardır) başka projeye geçmek
// koşuyu durdurur (sunucuda tek "etkin proje" var; koşu etkin proje değişince güvenlik için kesilir). İşlenen PR'lar birleşmiş olur,
// kalanlar beklemede kalır: "yarım kalabilir". Koşu yoksa null (uyarı gösterilmez).
export function projectSwitchWarning(snap) {
  const q = snap?.repo?.queueRun;
  if (!q) return null;
  const total = q.total || (Array.isArray(q.items) ? q.items.length : 0);
  const name = snap?.repo?.name || 'Bu proje';
  return `${name}: ${q.merged || 0}/${total} PR'lık kuyruk koşusu sürüyor.\n\nBaşka projeye geçerseniz koşu DURUR ve yarım kalabilir (birleşenler birleşmiş kalır, kalanlar beklemede). Daha sonra projeye dönüp planı yeniden onaylayarak devam edebilirsiniz.\n\nYine de projeyi değiştirmek istiyor musunuz?`;
}

// Birleştirme düğmesi "CI Bitince Merge Et" mi olmalı? PR'ın CI'ı sürüyorsa evet. CI HENÜZ YOKSA (GitHub zorunlu kontrolleri PR açıldıktan
// birkaç saniye sonra oluşturur) ve depoda zorunlu CI kontrolü varsa da evet: düz "Merge" GitHub'ın reddine çarpar. Zorunlu kontrol yoksa
// ya da okunamadıysa (requiredChecks boş) düz Merge kalır: CI'ı hiç olmayan bir depoda boşuna beklenmez. CI biliniyorsa (yeşil/kırmızı) hayır.
export function ciPendingForMerge(b, snap) {
  if (!b || b.state !== 'open' || !b.pr || !(b.pr.number > 0)) return false;
  const st = b.pr.ci?.state;
  if (st === 'RUNNING') return true;
  if (st) return false;
  return Array.isArray(snap?.repo?.requiredChecks) && snap.repo.requiredChecks.length > 0;
}

// BİRLEŞME NİYETİ rozeti (tek dil): bir PR'ın "birleşecek" olduğunu gösteren her durum aynı görünümü alır.
//  - kuyruk koşusunda (sunucu): "🚦 Kuyrukta 3/5" ya da "⏳ İşleniyor · CI bekleniyor" (queueRunBadge),
//  - tekil "CI Bitince Merge Et" (bu sekme, pendingAutoMerge): "⏳ CI bitince birleşecek".
// İkisi de tuvalde aynı köşe etiketini ve işlenen/bekleyen PR için "birleşecek" okunu çizer. Niyet yoksa null: sıradan açık PR'a
// ok çekilmez. Kuyruk koşusu varsa o önceliklidir (aynı PR iki kez gösterilmez).
export function mergeIntentBadge(snap, b, pending = {}) {
  const q = queueRunBadge(snap, b);
  if (q) return q;
  if (!b || !pending || !pending[b.id]) return null;
  return { text: '⏳ CI bitince birleşecek', tone: 'current', title: 'CI bitince otomatik birleştirilecek (çevrimdışı yedekte bu sekmede)' };
}

// Koruma durumu: "Bilinmiyor", "Korumasız"tan ayrı gösterilir (yönetici olmayan kullanıcıya GitHub 404 verir).
export function protectionView(repo) {
  const state = repo?.protectionState || (repo?.branchProtected ? 'active' : 'none');
  const def = repo?.defaultBranch || 'main';
  if (state === 'active') return { state, button: `🛡️ ${def} Korumalı`, btnClass: 'ok', badge: '🛡️ Aktif Korumalı', chip: 'ok', canEnable: false };
  if (state === 'unknown') return { state, button: '❔ Koruma ?', btnClass: 'ghost', badge: '❔ Bilinmiyor', chip: '', canEnable: false };
  return { state: 'none', button: '🔓 Korumasız', btnClass: 'warn', badge: '🔓 Korumasız', chip: 'warn', canEnable: true };
}

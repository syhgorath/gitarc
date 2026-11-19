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

// Koruma durumu: "Bilinmiyor", "Korumasız"tan ayrı gösterilir (yönetici olmayan kullanıcıya GitHub 404 verir).
export function protectionView(repo) {
  const state = repo?.protectionState || (repo?.branchProtected ? 'active' : 'none');
  const def = repo?.defaultBranch || 'main';
  if (state === 'active') return { state, button: `🛡️ ${def} Korumalı`, btnClass: 'ok', badge: '🛡️ Aktif Korumalı', chip: 'ok', canEnable: false };
  if (state === 'unknown') return { state, button: '❔ Koruma ?', btnClass: 'ghost', badge: '❔ Bilinmiyor', chip: '', canEnable: false };
  return { state: 'none', button: '🔓 Korumasız', btnClass: 'warn', badge: '🔓 Korumasız', chip: 'warn', canEnable: true };
}

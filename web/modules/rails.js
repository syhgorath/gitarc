// web/modules/rails.js - Sol raylar: "Üst Ray" / "Alt Ray" sayaçları ve "Geçmiş" çipi (merge noktası bilinmeyen dallar)
//
// Sayaç kuralı tek yerde (`rayCounts`): Daha önce renderSVG ve renderRayPopovers aynı iki rozeti farklı
// kurallarla yazıyordu (biri kirli çalışma ağacını sayıyordu, diğeri saymıyordu); kaydırma/yakınlaştırma
// sonrası "Alt Ray" sayısı bu yüzden bir azalıyordu.
import { state, el } from './state.js';
import { esc, truncate } from './utils.js';
import { splitHistory } from './layout.js';

// Üst ray: açık PR/uzak dallar. Alt ray: açık yerel dallar + kirli çalışma ağacı varsa bir taslak.
export function rayCounts(snapshot) {
  const branches = snapshot?.branches || [];
  const upper = branches.filter((b) => b.hemisphere === 'upper' && b.state === 'open').length;
  const lowerBranches = branches.filter((b) => b.hemisphere === 'lower' && b.state === 'open').length;
  const dirty = snapshot?.repo?.dirtyCount || 0;
  return { upper, lower: lowerBranches + (dirty > 0 ? 1 : 0) };
}

// Omurga kartı (sol ray): ad varsayılan daldan, alt yazı koruma durumundan gelir. Eskiden kart sabit
// "main · Production · Korunan" yazıyordu: `master` varsayılanlı depoda yanlış ad, korumasız/okunamayan
// depoda yanlış "Korunan" iddiası gösteriyordu.
const SPINE_PROTECTION = {
  active: { sub: 'Omurga · Korunan', tone: 'prot-active' },
  none: { sub: 'Omurga · Korumasız', tone: 'prot-none' },
  unknown: { sub: 'Omurga · Koruma okunamadı', tone: 'prot-unknown' },
};

export function spineCardModel(repo) {
  const name = repo?.defaultBranch || 'main';
  const state = repo?.protectionState || (repo?.branchProtected ? 'active' : 'none');
  const p = SPINE_PROTECTION[state] || SPINE_PROTECTION.none;
  return { name, sub: p.sub, tone: p.tone, title: repo?.protectionSummary || `${name}: ${p.sub}` };
}

export function renderSpineCard(snapshot) {
  const card = document.getElementById('btn-main-lane');
  if (!card || !snapshot) return;
  const m = spineCardModel(snapshot.repo);
  const nameEl = document.getElementById('spine-lane-name');
  const subEl = document.getElementById('spine-lane-sub');
  if (nameEl) nameEl.textContent = m.name;
  if (subEl) subEl.textContent = m.sub;
  card.classList.remove('prot-active', 'prot-none', 'prot-unknown');
  card.classList.add(m.tone);
  card.setAttribute('title', m.title);
}

export function renderRayPopovers(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap) return;

  const counts = rayCounts(snap);
  if (el.upperRayCount) el.upperRayCount.textContent = counts.upper > 0 ? String(counts.upper) : '';
  if (el.lowerRayCount) el.lowerRayCount.textContent = counts.lower > 0 ? String(counts.lower) : '';

  renderSpineCard(snap);
  renderHistoryCard(callbacks);
}

// Geçmiş listesindeki bir satırın başlığı ve alt metni (DOM yok).
export function historyItemModel(b) {
  const pr = b.pr?.number ? `#${b.pr.number} · ` : '';
  const who = b.author || b.pr?.author || '';
  return { title: truncate(b.name, 34), sub: pr + (who ? '@' + who : 'birleşmiş') };
}

// Sol raydaki "Geçmiş" çipi: omurgada merge noktası bulunamayan birleşmiş dalları listeler.
export function renderHistoryCard(callbacks = {}) {
  const wrap = document.getElementById('history-lane-wrap');
  const list = document.getElementById('history-list');
  const pop = document.getElementById('history-popover');
  const btn = document.getElementById('btn-history-lane');
  if (!wrap || !list || !pop || !btn) return;

  const { history } = splitHistory(state.snapshot?.branches);
  wrap.hidden = history.length === 0;
  const countEl = document.getElementById('history-count');
  if (countEl) countEl.textContent = history.length > 0 ? String(history.length) : '';
  if (history.length === 0) state.historyOpen = false;

  pop.hidden = !state.historyOpen;
  btn.setAttribute('aria-expanded', String(Boolean(state.historyOpen)));

  list.innerHTML = '';
  history.forEach((b) => {
    const item = document.createElement('div');
    item.className = 'ray-item history-item';
    item.setAttribute('role', 'button');
    item.tabIndex = 0;
    item.dataset.branchId = b.id;
    const m = historyItemModel(b);
    item.innerHTML = `<div><div class="ray-item-title">${esc(m.title)}</div>`
      + `<div class="ray-item-sub">${esc(m.sub)}</div></div>`;
    const choose = () => {
      state.historyOpen = false;
      pop.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
      if (callbacks.onSelectBranch) callbacks.onSelectBranch(b.id);
    };
    item.addEventListener('click', choose);
    item.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); choose(); }
    });
    list.appendChild(item);
  });
}

// "Geçmiş" çipi: aç/kapat, dışarı tıklayınca veya Esc ile kapat.
export function setupHistoryChip() {
  const histBtn = document.getElementById('btn-history-lane');
  const histPop = document.getElementById('history-popover');
  const setHistoryOpen = (open) => {
    state.historyOpen = Boolean(open);
    if (histPop) histPop.hidden = !state.historyOpen;
    histBtn?.setAttribute('aria-expanded', String(state.historyOpen));
  };
  histBtn?.addEventListener('click', (ev) => { ev.stopPropagation(); setHistoryOpen(!state.historyOpen); });
  document.getElementById('history-close')?.addEventListener('click', () => setHistoryOpen(false));
  document.addEventListener('click', (ev) => {
    if (state.historyOpen && !ev.target.closest?.('#history-lane-wrap')) setHistoryOpen(false);
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && state.historyOpen) setHistoryOpen(false);
  });
}

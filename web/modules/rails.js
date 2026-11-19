// web/modules/rails.js - Tuval üstü etiketler ve çipler: "Üst Ray" / "Alt Ray" sayaçları, omurga dalı seçici,
// "Geçmiş" (merge noktası bilinmeyen dallar) ve "Uzun Ömürlü" çipleri (eski tasarımdaki gibi tuvalin köşelerinde)
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

// Üç çip (omurga dalı, Geçmiş, Uzun Ömürlü) artık yan yana duruyor: biri açılınca diğerleri kapanmalı, yoksa
// açılır listeler üst üste biner.
const CHIP_POPOVERS = {
  base: { flag: 'baseOpen', btn: 'btn-base-picker', pop: 'base-popover' },
  history: { flag: 'historyOpen', btn: 'btn-history-lane', pop: 'history-popover' },
  longlived: { flag: 'longLivedOpen', btn: 'btn-longlived-lane', pop: 'longlived-popover' },
};

export function closeChipPopovers(except = '') {
  Object.entries(CHIP_POPOVERS).forEach(([key, c]) => {
    if (key === except) return;
    state[c.flag] = false;
    const pop = document.getElementById(c.pop);
    if (pop) pop.hidden = true;
    document.getElementById(c.btn)?.setAttribute('aria-expanded', 'false');
  });
}

// Omurga dalı seçici: yalnızca canlı depoda ve birden fazla yerel dal varken gösterilir (simülasyonda gerçek
// seçenek yoktur). Sabitlenmişse en üstte "Otomatik tespite dön" satırı çıkar.
export function basePickerModel(repo) {
  const cands = repo?.baseCandidates || [];
  const current = repo?.defaultBranch || 'main';
  const pinned = Boolean(repo?.baseBranchPinned);
  return {
    visible: repo?.mode === 'live' && cands.length > 1,
    pinned,
    current,
    label: `⚙ ${truncate(current, 24)}`,
    title: pinned ? `Omurga dalı: ${current} (elle seçildi) — değiştir` : `Omurga dalı: ${current} (otomatik tespit) — değiştir`,
    items: cands.map((name) => ({ name, current: name === current })),
  };
}

export function renderBasePicker(callbacks = {}) {
  const btn = document.getElementById('btn-base-picker');
  const pop = document.getElementById('base-popover');
  const list = document.getElementById('base-list');
  if (!btn || !pop || !list) return;
  const m = basePickerModel(state.snapshot?.repo);
  btn.hidden = !m.visible;
  btn.textContent = m.label;
  btn.setAttribute('title', m.title);
  if (!m.visible) state.baseOpen = false;
  pop.hidden = !m.visible || !state.baseOpen;
  btn.setAttribute('aria-expanded', String(Boolean(m.visible && state.baseOpen)));

  list.innerHTML = '';
  if (!m.visible) return;
  const rows = [];
  if (m.pinned) rows.push({ name: '', title: '↺ Otomatik tespite dön', sub: 'Elle seçimi kaldır', current: false });
  m.items.forEach((it) => rows.push({ name: it.name, title: truncate(it.name, 34), sub: it.current ? 'şu anki omurga' : 'omurga yap', current: it.current }));
  rows.forEach((r) => {
    const item = document.createElement('div');
    item.className = `ray-item history-item${r.current ? ' current' : ''}`;
    item.setAttribute('role', 'button');
    item.tabIndex = 0;
    item.dataset.baseBranch = r.name;
    item.innerHTML = `<div><div class="ray-item-title">${esc(r.title)}</div><div class="ray-item-sub">${esc(r.sub)}</div></div>`;
    const choose = () => {
      state.baseOpen = false;
      pop.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
      if (!r.current && callbacks.onSetBaseBranch) callbacks.onSetBaseBranch(r.name);
    };
    item.addEventListener('click', choose);
    item.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); choose(); }
    });
    list.appendChild(item);
  });
}

export function setupBasePicker() {
  const btn = document.getElementById('btn-base-picker');
  const pop = document.getElementById('base-popover');
  const setOpen = (open) => {
    if (open) closeChipPopovers('base');
    state.baseOpen = Boolean(open);
    if (pop) pop.hidden = !state.baseOpen;
    btn?.setAttribute('aria-expanded', String(state.baseOpen));
  };
  btn?.addEventListener('click', (ev) => { ev.stopPropagation(); setOpen(!state.baseOpen); });
  document.getElementById('base-close')?.addEventListener('click', () => setOpen(false));
  document.addEventListener('click', (ev) => {
    if (state.baseOpen && !ev.target.closest?.('#base-picker-wrap')) setOpen(false);
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && state.baseOpen) setOpen(false);
  });
}

export function renderRayPopovers(callbacks = {}) {
  const snap = state.snapshot;
  if (!snap) return;

  const counts = rayCounts(snap);
  if (el.upperRayCount) el.upperRayCount.textContent = counts.upper > 0 ? String(counts.upper) : '';
  if (el.lowerRayCount) el.lowerRayCount.textContent = counts.lower > 0 ? String(counts.lower) : '';

  renderBasePicker(callbacks);
  renderHistoryCard(callbacks);
  renderLongLivedCard(callbacks);
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

// Uzun ömürlü dallar (develop, release/*, 1.x): sunucunun `longLived` işaretini listeler. Birleşmiş görünse de
// (develop ana dala birleşir) yerelde veya uzakta gerçekten duruyorsa listelenir; yalnızca kalıntısı kalmamış
// ("gone", yerel ve uzak kopyası yok) dallar düşer.
export function longLivedList(branches) {
  return (branches || []).filter((b) => b.longLived && b.state !== 'gone' && (b.hasLocal || b.hasRemote));
}

export function longLivedItemModel(b) {
  const ahead = b.aheadMain || 0;
  const behind = b.behindMain || 0;
  const where = b.hasLocal && b.hasRemote ? 'yerel + uzak' : (b.hasRemote ? 'uzak' : 'yerel');
  return { title: truncate(b.name, 34), sub: `↑${ahead} ↓${behind} · ${where}` };
}

export function renderLongLivedCard(callbacks = {}) {
  const wrap = document.getElementById('longlived-lane-wrap');
  const list = document.getElementById('longlived-list');
  const pop = document.getElementById('longlived-popover');
  const btn = document.getElementById('btn-longlived-lane');
  if (!wrap || !list || !pop || !btn) return;

  const items = longLivedList(state.snapshot?.branches);
  wrap.hidden = items.length === 0;
  const countEl = document.getElementById('longlived-count');
  if (countEl) countEl.textContent = items.length > 0 ? String(items.length) : '';
  if (items.length === 0) state.longLivedOpen = false;
  pop.hidden = !state.longLivedOpen;
  btn.setAttribute('aria-expanded', String(Boolean(state.longLivedOpen)));

  list.innerHTML = '';
  items.forEach((b) => {
    const item = document.createElement('div');
    item.className = 'ray-item history-item';
    item.setAttribute('role', 'button');
    item.tabIndex = 0;
    item.dataset.branchId = b.id;
    const m = longLivedItemModel(b);
    item.innerHTML = `<div><div class="ray-item-title">${esc(m.title)}</div><div class="ray-item-sub">${esc(m.sub)}</div></div>`;
    const choose = () => {
      state.longLivedOpen = false;
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

// "Uzun Ömürlü" çipi: aç/kapat, dışarı tıklayınca veya Esc ile kapat.
export function setupLongLivedChip() {
  const btn = document.getElementById('btn-longlived-lane');
  const pop = document.getElementById('longlived-popover');
  const setOpen = (open) => {
    if (open) closeChipPopovers('longlived');
    state.longLivedOpen = Boolean(open);
    if (pop) pop.hidden = !state.longLivedOpen;
    btn?.setAttribute('aria-expanded', String(state.longLivedOpen));
  };
  btn?.addEventListener('click', (ev) => { ev.stopPropagation(); setOpen(!state.longLivedOpen); });
  document.getElementById('longlived-close')?.addEventListener('click', () => setOpen(false));
  document.addEventListener('click', (ev) => {
    if (state.longLivedOpen && !ev.target.closest?.('#longlived-lane-wrap')) setOpen(false);
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && state.longLivedOpen) setOpen(false);
  });
}

// "Geçmiş" çipi: aç/kapat, dışarı tıklayınca veya Esc ile kapat.
export function setupHistoryChip() {
  const histBtn = document.getElementById('btn-history-lane');
  const histPop = document.getElementById('history-popover');
  const setHistoryOpen = (open) => {
    if (open) closeChipPopovers('history');
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

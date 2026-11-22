// web/modules/publish.js - "Toplu Yayınla" penceresi: yerel dalları TEK onayla pushla (+PR aç), istenirse kuyruk koşusunu başlat.
// Plan sunucuda (internal/gitengine/publish.go) hesaplanır ve salt okunurdur; burada yalnızca gösterilir. Dal adları, başlıklar ve nedenler
// güvenilmeyen metindir: yalnızca textContent / DOM API ile yazılır.
import { state, el } from './state.js';
import { apiCall } from './api.js';
import { showToast } from './utils.js';

const view = { order: [], included: new Set(), plan: null, running: false, startQueue: true, onApply: null, seq: 0 };

/** Saf: bir plan satırının görünümü. */
export function publishRowModel(it) {
  if (it.action === 'blocked') return { badge: '⛔ Yayınlanamaz', tone: 'warn', text: it.reason || '' };
  if (it.action === 'push') {
    return { badge: '⬆ Pushla', tone: 'ok', text: `${it.commits || '?'} yerel commit pushlanır${it.pr ? `; PR #${it.pr} güncellenir` : ''} (yeni PR açılmaz)` };
  }
  return { badge: '🚀 Pushla + PR aç', tone: 'ok', text: `${it.commits || '?'} commit pushlanır, PR açılır: ${it.title || ''}` };
}

/** Saf: onay düğmesinin metni ve etkin mi. */
export function publishActionModel(plan, startQueue) {
  const n = plan?.items?.length || 0;
  if (!plan || !plan.publishable || n === 0) return { enabled: false, label: '🚀 Yayınla', hint: plan?.reason || 'Yayınlanacak dal seçin.' };
  const prs = plan.items.filter((i) => i.action === 'push_pr').length;
  const parts = [`${n} dal pushlanır`];
  if (prs) parts.push(`${prs} PR açılır`);
  if (startQueue) parts.push('kuyruk koşusu başlar (ilk sorunda durur)');
  return { enabled: true, label: `🚀 Yayınla: ${parts.join(' · ')}`, hint: '' };
}

/** Saf: yürütme sonucunun satırları (adım adım). */
export function publishResultModel(res) {
  const lines = (res?.steps || []).map((s) => ({ ok: s.ok, text: `${s.ok ? '✓' : '✗'} ${s.branch}${s.pr ? ` (#${s.pr})` : ''}: ${s.message}` }));
  const tail = res?.queueNote ? [{ ok: Boolean(res.queueStarted), text: `${res.queueStarted ? '🚦' : 'ℹ'} ${res.queueNote}` }] : [];
  return { lines: [...lines, ...tail], completed: Boolean(res?.completed), queueStarted: Boolean(res?.queueStarted) };
}

function appendText(parent, tag, text, cls) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  n.textContent = text;
  parent.appendChild(n);
  return n;
}

async function loadPlan(ids) {
  const my = ++view.seq;
  try {
    const plan = await apiCall('/api/publish/plan', 'POST', { branchIds: ids });
    if (my !== view.seq) return;
    view.plan = plan;
  } catch (err) {
    if (my !== view.seq) return;
    view.plan = null;
    renderPublish(err?.message === 'offline' ? 'Toplu yayın için çalışan GitArc sunucusu gerekir (çevrimdışı modda kullanılamaz).' : `Plan alınamadı: ${err?.message || err}`);
    return;
  }
  renderPublish();
}

function selectedIds() {
  return view.order.filter((id) => view.included.has(id));
}

function move(id, delta) {
  const i = view.order.indexOf(id);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= view.order.length) return;
  [view.order[i], view.order[j]] = [view.order[j], view.order[i]];
  loadPlan(selectedIds());
  renderPublish();
}

function renderPublish(errorText = '') {
  const box = el.publishBody;
  if (!box) return;
  box.textContent = '';
  if (errorText) { appendText(box, 'div', errorText, 'layer-line'); return; }
  const plan = view.plan;
  if (!plan) { appendText(box, 'div', 'Plan hesaplanıyor…', 'layer-line'); return; }
  if (view.order.length === 0) {
    appendText(box, 'div', 'Yayınlanacak yerel dal yok (pushlanmamış commit\'i olan açık dal bulunamadı).', 'layer-line');
    return;
  }
  const rows = document.createElement('div');
  rows.className = 'publish-rows';
  const byId = new Map((plan.items || []).map((it) => [it.branchId, it]));
  view.order.forEach((id) => {
    const it = byId.get(id) || view.known?.get(id);
    if (!it) return;
    const m = publishRowModel(it);
    const row = document.createElement('div');
    row.className = 'diag-card publish-row';
    const head = document.createElement('label');
    head.className = 'publish-row-head';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = view.included.has(id);
    cb.addEventListener('change', () => {
      if (cb.checked) view.included.add(id); else view.included.delete(id);
      loadPlan(selectedIds());
    });
    appendText(head, 'code', it.branch);
    appendText(head, 'span', m.badge, `chip ${m.tone}`.trim());
    head.prepend(cb);
    const up = document.createElement('button');
    up.type = 'button'; up.className = 'btn ghost sm'; up.textContent = '▲'; up.title = 'Yukarı (daha önce birleşir)';
    up.addEventListener('click', () => move(id, -1));
    const dn = document.createElement('button');
    dn.type = 'button'; dn.className = 'btn ghost sm'; dn.textContent = '▼'; dn.title = 'Aşağı';
    dn.addEventListener('click', () => move(id, 1));
    const top = document.createElement('div');
    top.className = 'publish-row-top';
    top.append(head, up, dn);
    row.appendChild(top);
    appendText(row, 'div', m.text, 'layer-line');
    rows.appendChild(row);
  });
  box.appendChild(rows);

  const qWrap = document.createElement('label');
  qWrap.className = 'publish-queue-opt';
  const qcb = document.createElement('input');
  qcb.type = 'checkbox';
  qcb.checked = view.startQueue;
  qcb.addEventListener('change', () => { view.startQueue = qcb.checked; renderPublish(); });
  qWrap.append(qcb);
  appendText(qWrap, 'span', ' PR\'lar açılınca kuyruk koşusunu da başlat (CI bekler, sırayla birleştirir, ilk sorunda durur)');
  box.appendChild(qWrap);

  const act = publishActionModel(plan, view.startQueue);
  if (act.hint) appendText(box, 'div', act.hint, 'layer-line publish-hint');
  const go = document.createElement('button');
  go.type = 'button';
  go.className = 'btn primary publish-go';
  go.textContent = view.running ? '⏳ Yayınlanıyor…' : act.label;
  go.disabled = !act.enabled || view.running;
  go.addEventListener('click', runPublish);
  box.appendChild(go);
}

async function runPublish() {
  const plan = view.plan;
  if (!plan?.publishable || view.running) return;
  view.running = true;
  renderPublish();
  try {
    const res = await apiCall('/api/publish/run', 'POST', { branchIds: selectedIds(), confirm: plan.fingerprint, startQueue: view.startQueue });
    if (res.snapshot && view.onApply) view.onApply(res.snapshot);
    showResult(res);
  } catch (err) {
    showToast(err?.message || String(err), 'danger');
    view.running = false;
    loadPlan(selectedIds());
  }
}

function showResult(res) {
  view.running = false;
  const box = el.publishBody;
  if (!box) return;
  box.textContent = '';
  const m = publishResultModel(res);
  const card = document.createElement('div');
  card.className = `diag-card publish-result ${m.completed ? 'ok' : 'warn'}`;
  appendText(card, 'h4', m.completed ? '✓ Yayınlandı' : '⚠ Yayın yarım kaldı (ilk sorunda durdu)');
  m.lines.forEach((l) => appendText(card, 'div', l.text, `layer-line ${l.ok ? '' : 'publish-bad'}`.trim()));
  box.appendChild(card);
  showToast(m.completed ? (m.queueStarted ? '✓ Yayınlandı; kuyruk koşusu başladı' : '✓ Yayınlandı') : 'Yayın yarım kaldı; ayrıntı pencerede', m.completed ? 'ok' : 'warn');
}

export async function openPublishModal(onApply = null) {
  view.onApply = onApply;
  view.running = false;
  view.plan = null;
  view.order = [];
  view.included = new Set();
  if (el.publishModal) el.publishModal.hidden = false;
  renderPublish();
  try {
    const plan = await apiCall('/api/publish/plan', 'POST', { branchIds: [] });
    view.order = (plan.items || []).map((it) => it.branchId);
    view.included = new Set(view.order);
    view.known = new Map((plan.items || []).map((it) => [it.branchId, it]));
    view.plan = plan;
    renderPublish();
  } catch (err) {
    renderPublish(err?.message === 'offline' ? 'Toplu yayın için çalışan GitArc sunucusu gerekir (çevrimdışı modda kullanılamaz).' : `Plan alınamadı: ${err?.message || err}`);
  }
}

export function closePublishModal() {
  if (el.publishModal) el.publishModal.hidden = true;
}

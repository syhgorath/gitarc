// web/modules/queue.js - Merge kuyruğu planı penceresi (SALT OKUNUR): PR'ları seç, sırala, planı gör.
// Plan sunucuda (internal/gitengine/queue.go) hesaplanır; burada yalnızca gösterilir. PR başlıkları/dal adları/dosya yolları
// güvenilmeyen metindir: yalnızca textContent / DOM API ile yazılır.
import { state, el } from './state.js';
import { apiCall } from './api.js';

const view = { order: [], included: new Set(), seq: 0 };

/** Kuyruğa girebilecek dallar: açık PR'lar, PR numarasına göre (sunucudaki varsayılanla aynı). */
export function queueCandidates(snap) {
  return (snap?.branches || [])
    .filter((b) => b.state === 'open' && !b.historyOnly && b.pr && b.pr.number > 0)
    .sort((a, b) => a.pr.number - b.pr.number);
}

/** Adımın görünümü: durum rozeti metni/tonu ve ayrıntı satırları (saf model, testlenebilir). */
export function queueStepModel(it) {
  const status = it.status === 'clean'
    ? { text: '✓ Temiz', tone: 'ok' }
    : it.status === 'conflict'
      ? { text: '⚠ Çakışır', tone: 'warn' }
      : { text: '? Bilinmiyor', tone: '' };
  const details = [];
  if (it.status !== 'clean' && it.reason) details.push(it.reason);
  if (it.conflictFiles?.length) details.push(`Çakışan dosyalar: ${it.conflictFiles.join(', ')}`);
  if (it.afterBlocker) details.push('Kuyruk daha önce durduğu için bu dal engel aşılana kadar bekler.');
  if (it.behind > 0) details.push(`main'in ${it.behind} commit gerisinde`);
  details.push(it.ci ? `CI: ${it.ci}` : 'CI: bilinmiyor (yeşil varsayılmaz)');
  if (it.review) details.push(`İnceleme: ${it.review}`);
  return { status, details };
}

/** Plan başlığı: kuyruğun nerede durduğunu tek cümlede söyler. */
export function queueSummaryText(plan) {
  if (!plan || !plan.items?.length) return 'Kuyruğa girecek açık PR/dal yok.';
  if (plan.stopsAt === 0) return `✓ ${plan.items.length} dalın tamamı sırayla temiz birleşir.`;
  return `${plan.mergeable} dal sorunsuz birleşir; kuyruk ${plan.stopsAt}. sırada durur.`;
}

function appendText(parent, tag, text, cls) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  n.textContent = text;
  parent.appendChild(n);
  return n;
}

function renderCandidates() {
  const box = el.queueCandidates;
  if (!box) return;
  box.textContent = '';
  const byId = Object.fromEntries(queueCandidates(state.snapshot).map((b) => [b.id, b]));
  if (!view.order.length) {
    appendText(box, 'div', 'Kuyruğa girecek açık PR yok.', 'prot-note');
    return;
  }
  view.order.forEach((id, idx) => {
    const b = byId[id];
    if (!b) return;
    const row = document.createElement('div');
    row.className = 'queue-row';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = view.included.has(id);
    cb.setAttribute('aria-label', `${b.name} kuyrukta`);
    cb.addEventListener('change', () => { if (cb.checked) view.included.add(id); else view.included.delete(id); refreshQueuePlan(); });
    row.appendChild(cb);
    appendText(row, 'span', `#${b.pr.number}`, 'queue-pr');
    appendText(row, 'span', b.name, 'queue-branch mono');
    appendText(row, 'span', b.pr.title || '', 'queue-title');
    const mk = (label, delta, disabled) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn ghost sm';
      btn.textContent = label;
      btn.disabled = disabled;
      btn.addEventListener('click', () => {
        const j = idx + delta;
        [view.order[idx], view.order[j]] = [view.order[j], view.order[idx]];
        renderCandidates();
        refreshQueuePlan();
      });
      row.appendChild(btn);
    };
    mk('▲', -1, idx === 0);
    mk('▼', 1, idx === view.order.length - 1);
    box.appendChild(row);
  });
}

function renderPlan(plan, errorText) {
  const box = el.queueResult;
  if (!box) return;
  box.textContent = '';
  if (errorText) {
    appendText(box, 'div', errorText, 'prot-note');
    return;
  }
  appendText(box, 'div', queueSummaryText(plan), `queue-summary ${plan.stopsAt === 0 && plan.items.length ? 'ok' : 'warn'}`);
  plan.items.forEach((it) => {
    const m = queueStepModel(it);
    const step = document.createElement('div');
    step.className = `queue-step${it.afterBlocker ? ' after-blocker' : ''}`;
    const head = document.createElement('div');
    head.className = 'queue-step-head';
    appendText(head, 'span', `${it.position}.`, 'queue-pos');
    appendText(head, 'span', it.pr > 0 ? `#${it.pr} ${it.branch}` : it.branch, 'queue-branch mono');
    appendText(head, 'span', m.status.text, `chip ${m.status.tone}`.trim());
    step.appendChild(head);
    m.details.forEach((d) => appendText(step, 'div', d, 'queue-detail'));
    box.appendChild(step);
  });
  (plan.notes || []).forEach((n) => appendText(box, 'div', n, 'prot-note'));
}

async function planCall(ids) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queuePlan(ids);
  return apiCall('/api/queue/plan', 'POST', { branchIds: ids });
}

/** Seçili dalların planını sunucudan ister ve çizer. Eski yanıt (hızlı tıklama) yenisini ezmez. */
export async function refreshQueuePlan() {
  const my = ++view.seq;
  const ids = view.order.filter((id) => view.included.has(id));
  if (!ids.length) {
    renderPlan({ items: [], stopsAt: 0, notes: [] });
    return;
  }
  if (el.queueResult) { el.queueResult.textContent = ''; appendText(el.queueResult, 'div', '⏳ Plan hesaplanıyor...', 'prot-note'); }
  try {
    const plan = await planCall(ids);
    if (my === view.seq) renderPlan(plan);
  } catch (err) {
    if (my === view.seq) renderPlan(null, err.message || String(err));
  }
}

export function openQueueModal() {
  const cands = queueCandidates(state.snapshot);
  view.order = cands.map((b) => b.id);
  view.included = new Set(view.order);
  if (el.queueModal) el.queueModal.hidden = false;
  renderCandidates();
  refreshQueuePlan();
}

export function closeQueueModal() {
  if (el.queueModal) el.queueModal.hidden = true;
}

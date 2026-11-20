// web/modules/queue.js - Merge kuyruğu penceresi: PR'ları seç, sırala, planı gör (salt okunur); sıradaki adımı (güncelle / birleştir) onayla.
// Plan sunucuda (internal/gitengine/queue.go) hesaplanır; burada yalnızca gösterilir. PR başlıkları/dal adları/dosya yolları
// güvenilmeyen metindir: yalnızca textContent / DOM API ile yazılır.
import { state, el } from './state.js';
import { apiCall } from './api.js';
import { showToast } from './utils.js';

const view = { order: [], included: new Set(), seq: 0, onApply: null };

/** Kuyruğa girebilecek dallar: açık PR'lar, PR numarasına göre (sunucudaki varsayılanla aynı). */
export function queueCandidates(snap) {
  return (snap?.branches || [])
    .filter((b) => b.state === 'open' && !b.historyOnly && b.pr && b.pr.number > 0)
    .sort((a, b) => a.pr.number - b.pr.number);
}

/** Birleşen/kapanan PR'ları sıradan ve seçimden çıkarır (aksi hâlde plan isteği artık açık olmayan dalı sorar ve 400 döner). */
export function pruneQueueView(order, included, snap) {
  const open = new Set(queueCandidates(snap).map((b) => b.id));
  return { order: order.filter((id) => open.has(id)), included: new Set([...included].filter((id) => open.has(id))) };
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

/** Kuyruk adımı planının görünümü (saf model): başlık, satırlar, uygulanabilir mi ve onay düğmesi metni. */
export function stepPlanModel(plan) {
  const lines = [];
  if (plan.reason) lines.push(plan.reason);
  lines.push(plan.ci && plan.ci !== 'UNKNOWN' ? `CI: ${plan.ci}` : 'CI: bilinmiyor (yeşil varsayılmaz)');
  if (plan.review) lines.push(`İnceleme: ${plan.review}`);
  if (plan.action === 'merge') lines.push(`PR #${plan.pr} merge commit ile main'e birleştirilir; dal ucu (${(plan.headSha || '').slice(0, 7)}) değişmişse GitHub reddeder. Koruma atlatılmaz.`);
  if (plan.action === 'update') lines.push("Ana dal dalın içine merge edilir ve UZAK PR dalına push edilir (CI yeniden başlar). Birleştirme bir sonraki adımdır.");
  (plan.notes || []).forEach((n) => lines.push(n));
  const title = { update: '↻ Sıradaki adım: dalı güncelle', merge: '✓ Sıradaki adım: birleştir', wait: '⏳ Şimdilik bekle', blocked: '⛔ Kuyruk burada durur' }[plan.action] || 'Sıradaki adım';
  const confirmText = plan.action === 'merge' ? `Onayla: #${plan.pr} birleştir` : 'Onayla: dalı güncelle ve push et';
  return { title, lines, canApply: (plan.action === 'update' || plan.action === 'merge') && Boolean(plan.fingerprint), confirmText };
}

/** Otomatik koşu planının görünümü (saf model): satırlar, başlatılabilir mi ve onay düğmesi metni. */
export function runPlanModel(plan) {
  const lines = [];
  (plan.items || []).forEach((it) => lines.push(`${it.position}. #${it.pr} ${it.branch}`));
  if (!plan.startable && plan.reason) lines.push(`Başlatılamaz: ${plan.reason}`);
  (plan.excluded || []).forEach((it) => lines.push(`✗ koşuya girmez: ${it.pr > 0 ? `#${it.pr} ` : ''}${it.branch}`));
  (plan.notes || []).forEach((n) => lines.push(n));
  const n = (plan.items || []).length;
  return {
    title: plan.startable ? `▶ ${n} PR sırayla işlenecek` : '⛔ Koşu başlatılamaz',
    lines,
    canStart: Boolean(plan.startable && plan.fingerprint),
    confirmText: `Onayla: ${n} PR'ı sırayla güncelle ve birleştir`,
  };
}

const RUN_TONES = { running: 'warn', completed: 'ok', stopped: '', blocked: 'warn', timeout: 'warn', interrupted: 'warn' };
const RUN_TITLES = {
  running: '⏳ Koşu sürüyor', completed: '✓ Koşu bitti', stopped: '■ Koşu durduruldu',
  blocked: '⛔ Koşu ilk sorunda durdu', timeout: '⏱ Koşu zaman aşımında durdu', interrupted: '⚠ Koşu yarım kaldı',
};

/** Koşu durumunun görünümü (saf model). `lines` düz metin özeti, `steps` PR başına adım durumu, `events` zaman çizelgesi. */
export function runStatusModel(run) {
  if (!run) return null;
  const items = run.items || [];
  const total = items.length;
  const merged = Math.min(run.merged || 0, total);
  const lines = [];
  if (run.status === 'running' && total) {
    const idx = Math.min(run.current || 0, total - 1);
    const cur = items[idx];
    lines.push(`İşlenen: ${idx + 1}/${total} · #${cur.pr} ${cur.branch} · ${run.merged || 0} birleşti`);
  } else {
    lines.push(`${run.merged || 0}/${total} PR birleştirildi`);
  }
  if (run.stopReason) lines.push(run.stopReason);
  const events = (run.events || []).slice(-60).map((e) => ({ time: (e.time || '').slice(11, 19), kind: e.kind || 'info', message: e.message || '', branch: e.branch || '' }));
  events.slice(-8).forEach((e) => lines.push(`${e.time} ${e.message}`.trim()));

  // Adım görünümü: sıra sıralı ilerlediği için ilk `merged` PR bitmiştir, `merged`. PR şu anki/duran PR'dır.
  const status = run.status;
  const steps = items.map((it, i) => {
    let st = 'waiting';
    if (i < merged) st = 'done';
    else if (i === merged && status === 'running') st = 'current';
    else if (i === merged && (status === 'blocked' || status === 'timeout')) st = 'failed';
    else if (i === merged && (status === 'stopped' || status === 'interrupted')) st = 'stopped';
    return { label: `${it.pr > 0 ? `#${it.pr} ` : ''}${it.branch}`, branch: it.branch, state: st };
  });
  // Şu anki PR'ın aşaması: son olayına bakılır
  let phase = '';
  if (status === 'running' && steps[merged]) {
    const last = [...events].reverse().find((e) => e.branch === steps[merged].branch && e.kind !== 'info');
    phase = !last ? 'planlanıyor' : (last.kind === 'update' || last.kind === 'wait') ? 'CI bekleniyor' : 'planlanıyor';
  }
  const lastEv = events[events.length - 1];
  return {
    title: RUN_TITLES[status] || status,
    tone: RUN_TONES[status] ?? '',
    status,
    running: status === 'running',
    counter: `${run.merged || 0}/${total}`,
    lines,
    steps,
    phase,
    events,
    stopReason: run.stopReason || '',
    compactLine: `Son koşu: ${(RUN_TITLES[status] || status).replace(/^\S+\s/, '')} · ${run.merged || 0}/${total} PR${lastEv && lastEv.time ? ` · ${lastEv.time}` : ''}`,
    resumable: ['blocked', 'timeout', 'interrupted', 'stopped'].includes(status),
  };
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
    cb.disabled = run.active; // koşu sürerken sıra/seçim değişmez (koşu başladığı plana göre ilerler)
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
    mk('▲', -1, run.active || idx === 0);
    mk('▼', 1, run.active || idx === view.order.length - 1);
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
  if (plan.mergeable > 0) {
    const runBtn = document.createElement('button');
    runBtn.type = 'button';
    runBtn.className = 'btn ghost sm queue-run-btn';
    runBtn.textContent = '▶ Kuyruğu çalıştır (ilk sorunda durur)';
    runBtn.disabled = Boolean(run.active);
    runBtn.addEventListener('click', () => showRunPlan());
    box.appendChild(runBtn);
  }
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
    if (it.position === 1 && it.pr > 0) {
      const stepBtn = document.createElement('button');
      stepBtn.type = 'button';
      stepBtn.className = 'btn ghost sm queue-step-btn';
      stepBtn.textContent = '▶ Sıradaki adımı planla';
      stepBtn.addEventListener('click', () => showStepPlan(it, step));
      step.appendChild(stepBtn);
    }
    box.appendChild(step);
  });
  (plan.notes || []).forEach((n) => appendText(box, 'div', n, 'prot-note'));
}

async function stepPlanCall(branchId) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueStepPlan(branchId);
  return apiCall('/api/queue/step/plan', 'POST', { branchId });
}

async function stepApplyCall(branchId, confirm) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueStep(branchId, confirm);
  return apiCall('/api/queue/step', 'POST', { branchId, confirm });
}

/** Kuyruğun sıradaki dalı için TEK adımı (güncelle / birleştir) planlar; "Onayla" yalnızca gösterilen planın parmak iziyle uygular. */
async function showStepPlan(it, step) {
  step.querySelector('.queue-stepplan')?.remove();
  const panel = document.createElement('div');
  panel.className = 'queue-update queue-stepplan';
  step.appendChild(panel);
  appendText(panel, 'div', '⏳ Adım planı hesaplanıyor (GitHub CI durumu okunuyor)...', 'prot-note');
  try {
    const plan = await stepPlanCall(it.branchId);
    const m = stepPlanModel(plan);
    panel.textContent = '';
    appendText(panel, 'div', m.title, 'queue-update-title');
    m.lines.forEach((l) => appendText(panel, 'div', l, 'queue-detail'));
    if (m.canApply) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn warn sm queue-step-confirm';
      btn.textContent = m.confirmText;
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        try {
          const snap = await stepApplyCall(it.branchId, plan.fingerprint);
          if (view.onApply) view.onApply(snap);
          showToast(plan.action === 'merge' ? `✓ #${plan.pr} ${it.branch} birleştirildi` : `↻ ${it.branch} güncellendi; CI yeniden başladı`, 'ok');
        } catch (err) {
          showToast(err.message, 'danger');
        }
        refreshQueuePlan();
      });
      panel.appendChild(btn);
    } else {
      const again = document.createElement('button');
      again.type = 'button';
      again.className = 'btn ghost sm queue-step-recheck';
      again.textContent = '↻ Yeniden kontrol et';
      again.addEventListener('click', () => showStepPlan(it, step));
      panel.appendChild(again);
    }
  } catch (err) {
    panel.textContent = '';
    appendText(panel, 'div', err.message || String(err), 'prot-note');
  }
}

const run = { timer: null, panel: null, active: false, seq: 0, dismissed: new Set(), expanded: new Set(), shownId: '' };

function ensureRunPanel() {
  if (run.panel && run.panel.isConnected) return run.panel;
  const p = document.createElement('div');
  p.className = 'queue-run';
  p.id = 'queue-run';
  el.queueResult?.parentNode?.insertBefore(p, el.queueResult);
  run.panel = p;
  return p;
}

function selectedIds() { return view.order.filter((id) => view.included.has(id)); }

async function runPlanCall(ids) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueRunPlan(ids);
  return apiCall('/api/queue/run/plan', 'POST', { branchIds: ids });
}
async function runStartCall(ids, confirm) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueRun(ids, confirm);
  return apiCall('/api/queue/run/start', 'POST', { branchIds: ids, confirm });
}
async function runStopCall() {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueRunStop();
  return apiCall('/api/queue/run/stop', 'POST', {});
}
async function runStatusCall() {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queueRunStatus();
  return apiCall('/api/queue/run/status', 'GET');
}

function stopRunPolling() {
  if (run.timer) clearInterval(run.timer);
  run.timer = null;
}

const EVENT_ICONS = { update: '↻', merge: '✓', wait: '⏳', stop: '■', info: '•' };
const STEP_MARKS = { done: '✓', current: '⏳', waiting: '○', failed: '✗', stopped: '■' };

/** Koşu durumunu çizer. Biten koşu (yeni oturumda) tek satıra iner; dikkat isteyen durumlar tam gösterilir. */
function renderRun(runObj, opts = {}) {
  const panel = ensureRunPanel();
  const m = runStatusModel(runObj);
  // Poll her 3 sn yeniden çizer: günlüğün kaydırma konumunu koru (altındaysa altta kal).
  const oldLog = panel.querySelector('.queue-run-log');
  const atBottom = !oldLog || oldLog.scrollTop + oldLog.clientHeight >= oldLog.scrollHeight - 8;
  const oldTop = oldLog ? oldLog.scrollTop : 0;
  panel.textContent = '';
  if (!m || run.dismissed.has(runObj.id)) { const was = run.active; run.active = false; if (was) renderCandidates(); return; }
  const wasActive = run.active;
  run.active = m.running;
  run.shownId = runObj.id;
  if (wasActive !== run.active) renderCandidates();

  if (opts.compact && m.status === 'completed' && !run.expanded.has(runObj.id)) {
    const row = document.createElement('div');
    row.className = 'queue-run-compact';
    appendText(row, 'span', `✓ ${m.compactLine}`, 'queue-run-compact-text');
    const more = document.createElement('button');
    more.type = 'button';
    more.className = 'btn ghost sm';
    more.textContent = 'ayrıntı';
    more.addEventListener('click', () => { run.expanded.add(runObj.id); renderRun(runObj, opts); });
    const x = document.createElement('button');
    x.type = 'button';
    x.className = 'btn ghost sm queue-run-dismiss';
    x.textContent = '×';
    x.setAttribute('aria-label', 'Son koşu bilgisini gizle');
    x.addEventListener('click', () => { run.dismissed.add(runObj.id); renderRun(runObj, opts); });
    row.append(more, x);
    panel.appendChild(row);
    return;
  }

  const card = document.createElement('div');
  card.className = `queue-run-card ${m.tone}`.trim();
  const head = document.createElement('div');
  head.className = 'queue-run-head';
  appendText(head, 'span', m.title, 'queue-update-title');
  appendText(head, 'span', m.counter, `chip ${m.tone}`.trim());
  card.appendChild(head);

  // Adımlar: PR başına belirgin kutucuk (✓ bitti · ⏳ şimdi · ○ sırada · ✗ burada durdu)
  const steps = document.createElement('div');
  steps.className = 'queue-run-steps';
  m.steps.forEach((st) => {
    const pill = document.createElement('span');
    pill.className = `queue-run-step ${st.state}`;
    appendText(pill, 'span', STEP_MARKS[st.state] || '○', 'queue-run-mark');
    appendText(pill, 'span', st.label, 'queue-run-label');
    if (st.state === 'current' && m.phase) appendText(pill, 'span', m.phase, 'queue-run-phase');
    steps.appendChild(pill);
  });
  card.appendChild(steps);
  if (m.stopReason) appendText(card, 'div', m.stopReason, 'queue-run-reason');

  // Olay günlüğü: kendi içinde kaydırılır (modalı büyütmez)
  const log = document.createElement('div');
  log.className = 'queue-run-log';
  log.setAttribute('role', 'log');
  m.events.forEach((e) => {
    const row = document.createElement('div');
    row.className = `queue-run-ev ${e.kind}`;
    appendText(row, 'span', e.time, 'queue-run-time');
    appendText(row, 'span', EVENT_ICONS[e.kind] || '•', 'queue-run-icon');
    appendText(row, 'span', e.message, 'queue-run-msg');
    log.appendChild(row);
  });
  card.appendChild(log);
  log.scrollTop = atBottom ? log.scrollHeight : oldTop;

  const actions = document.createElement('div');
  actions.className = 'queue-run-actions';
  if (m.running) {
    const stop = document.createElement('button');
    stop.type = 'button';
    stop.className = 'btn danger sm queue-run-stop';
    stop.textContent = '■ Durdur';
    stop.addEventListener('click', async () => {
      stop.disabled = true;
      try { renderRun(await runStopCall()); await finishRunUI(); } catch (err) { showToast(err.message, 'danger'); }
    });
    actions.appendChild(stop);
  } else {
    const hide = document.createElement('button');
    hide.type = 'button';
    hide.className = 'btn ghost sm queue-run-dismiss';
    hide.textContent = 'Gizle';
    hide.addEventListener('click', () => { run.dismissed.add(runObj.id); renderRun(runObj, opts); });
    actions.appendChild(hide);
  }
  card.appendChild(actions);
  panel.appendChild(card);
}

/** Koşu bitti/durdu: anlık görüntüyü tazele ve kuyruk planını BİR kez yeniden hesapla (koşu sürerken plan yenilenmez). */
async function finishRunUI() {
  try { const snap = await apiCall('/api/snapshot'); if (view.onApply) view.onApply(snap); } catch { /* çevrimdışı */ }
  refreshQueuePlan();
}

async function pollRun() {
  const my = ++run.seq;
  try {
    const r = await runStatusCall();
    if (my !== run.seq) return;
    const wasActive = run.active;
    renderRun(r);
    if (r && r.status !== 'running') {
      stopRunPolling();
      if (wasActive || r) await finishRunUI();
    }
  } catch { /* geçici hata: sonraki turda yeniden denenir */ }
}

function startRunPolling() {
  stopRunPolling();
  run.timer = setInterval(pollRun, 3000);
}

/** Koşu planını gösterir; "Onayla" yalnızca gösterilen planın parmak iziyle başlatır. */
async function showRunPlan() {
  const panel = ensureRunPanel();
  const ids = selectedIds();
  panel.textContent = '';
  appendText(panel, 'div', '⏳ Koşu planı hesaplanıyor...', 'prot-note');
  try {
    const plan = await runPlanCall(ids);
    const m = runPlanModel(plan);
    panel.textContent = '';
    appendText(panel, 'div', m.title, 'queue-update-title');
    m.lines.forEach((l) => appendText(panel, 'div', l, 'queue-detail'));
    if (m.canStart) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn warn sm queue-run-confirm';
      btn.textContent = m.confirmText;
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        try {
          const started = await runStartCall(ids, plan.fingerprint);
          renderRun(started);
          if (started && started.status === 'running') { startRunPolling(); refreshQueuePlan(); } else await finishRunUI();
          showToast('▶ Kuyruk koşusu başladı; ilk sorunda durur', 'ok');
        } catch (err) {
          showToast(err.message, 'danger');
          btn.disabled = false;
        }
      });
      panel.appendChild(btn);
    }
  } catch (err) {
    panel.textContent = '';
    appendText(panel, 'div', err.message || String(err), 'prot-note');
  }
}

/** Pencere açılınca: sürmekte olan (ya da yeni biten/yarım kalan) bir koşu varsa gösterir ve izler. */
async function restoreRun() {
  try {
    const r = await runStatusCall();
    if (!r) { renderRun(null); return; }
    const recent = r.finishedAt ? (Date.now() - Date.parse(r.finishedAt)) < 30 * 60 * 1000 : true;
    if (r.status === 'running' || recent) renderRun(r, { compact: true }); else renderRun(null);
    if (r.status === 'running') startRunPolling();
  } catch { /* koşu bilgisi yok */ }
}

async function planCall(ids) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.queuePlan(ids);
  return apiCall('/api/queue/plan', 'POST', { branchIds: ids });
}

/** Seçili dalların planını sunucudan ister ve çizer. Eski yanıt (hızlı tıklama) yenisini ezmez. */
/** Koşu sürerken plan alanında yalnızca kısa bir not (eski/bayat plan gösterilmez). */
function renderRunNote() {
  const box = el.queueResult;
  if (!box) return;
  box.textContent = '';
  appendText(box, 'div', 'Koşu sürüyor: PR\'lar sırayla işleniyor. Kuyruk planı koşu bitince ya da durdurulunca yenilenir.', 'prot-note queue-run-note');
}

export async function refreshQueuePlan() {
  const my = ++view.seq;
  const pruned = pruneQueueView(view.order, view.included, state.snapshot);
  view.order = pruned.order;
  view.included = pruned.included;
  renderCandidates();
  // Koşu sürerken plan HESAPLANMAZ: her birleşmeden sonra kuyruğu yeniden simüle etmek gereksiz ve yavaştı ("Plan
  // hesaplanıyor…" altta takılı görünüyordu). Koşu bitince/durunca bir kez yenilenir (finishRunUI).
  if (run.active) { renderRunNote(); return; }
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

export function openQueueModal(onApplySnapshot = null) {
  view.onApply = onApplySnapshot;
  const cands = queueCandidates(state.snapshot);
  view.order = cands.map((b) => b.id);
  view.included = new Set(view.order);
  if (el.queueModal) el.queueModal.hidden = false;
  renderCandidates();
  // Önce süren bir koşu var mı bak: varsa plan hesaplanmaz (renderRunNote), yoksa bir kez hesaplanır.
  restoreRun().then(() => refreshQueuePlan());
}

export function closeQueueModal() {
  stopRunPolling();
  if (el.queueModal) el.queueModal.hidden = true;
}

// web/modules/protection.js - Dal koruması penceresi: profil seç → planı gör → onayla ve uygula.
// Plan sunucuda (internal/gitengine/protection.go) hesaplanır; burada yalnızca gösterilir ve onaylanır.
// GitHub'dan gelen kontrol adları güvenilmeyen metindir: yalnızca textContent / DOM API ile yazılır.
import { state, el } from './state.js';
import { apiCall } from './api.js';
import { showToast } from './utils.js';

export const PROTECTION_PROFILES = [
  { id: 'temel', title: 'Temel', desc: 'Doğrudan push yok, PR zorunlu; force-push ve dal silme kapalı.' },
  { id: 'ci', title: 'CI zorunlu (önerilen)', desc: 'Temel + CI kontrolleri yeşil olmadan ve dal main\'e göre güncel değilken birleştirme yok. AI/ekip PR kuyruğu için uygun.' },
  { id: 'siki', title: 'Sıkı', desc: 'CI zorunlu + 1 onay + kurallar yöneticiler için de geçerli. Tek geliştiricide kendi PR\'ınızı birleştirmenizi engeller.' },
];

/** "Şu an" kontrol listesi: korumanın bugünkü hâli sade satırlar olarak. [{label, on, value}] (on: açık mı). */
export function currentRows(plan) {
  const c = plan?.current;
  if (!c) return [];
  const checks = Array.isArray(c.checks) ? c.checks : [];
  return [
    { label: 'PR olmadan push yok', on: Boolean(c.requirePR), value: c.requirePR ? 'açık' : 'kapalı' },
    { label: 'CI kontrolleri geçmeden merge yok', on: checks.length > 0, value: checks.length ? checks.join(', ') : 'zorunlu kontrol yok' },
    { label: 'Dal main\'e göre güncel olmalı', on: Boolean(c.strictUpToDate), value: c.strictUpToDate ? 'açık' : 'kapalı' },
    { label: 'Gereken onay sayısı', on: (c.approvals || 0) > 0, value: String(c.approvals || 0) },
    { label: 'Force-push engelli', on: Boolean(c.blockForcePush), value: c.blockForcePush ? 'açık' : 'kapalı' },
    { label: 'Dal silme engelli', on: Boolean(c.blockDeletion), value: c.blockDeletion ? 'açık' : 'kapalı' },
    { label: 'Yöneticiler de kurala uyar', on: Boolean(c.enforceAdmins), value: c.enforceAdmins ? 'açık' : 'kapalı' },
  ];
}

const view = { profile: 'ci', checks: new Set(), plan: null, planKey: '', seq: 0 };

// Planın hangi seçim için istendiğini ayırt eden anahtar: bayat bir plan (profil/kontrol sonradan değişti) uygulanmasın.
const selectionKey = () => `${view.profile}|${[...view.checks].sort().join(',')}`;

/** Planın "Değişecekler" satırları: [{label, from, to}]. */
export function changeRows(plan) {
  return (plan?.changes || []).map((c) => ({ label: c.label, from: c.from, to: c.to }));
}

/** Uygula düğmesi: yalnızca uygulanabilir ve değişecek bir şey varsa açık. */
export function canApplyPlan(plan) {
  return Boolean(plan && plan.applicable && plan.fingerprint && (plan.changes || []).length > 0);
}

export function applyLabel(plan) {
  if (!plan) return '🛡️ Planı Uygula';
  const n = (plan.changes || []).length;
  return n > 0 ? `🛡️ ${n} Değişikliği Uygula` : '🛡️ Değişecek Bir Şey Yok';
}

async function planCall(profile, checks) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.protectionPlan(profile, checks);
  return apiCall('/api/repo/protection/plan', 'POST', { profile, checks });
}

async function applyCall(profile, checks, confirm) {
  if (state.offlineMode && window.LocalFallbackEngine) return window.LocalFallbackEngine.applyProtection(profile, checks, confirm);
  return apiCall('/api/repo/protect', 'POST', { profile, checks, confirm });
}

function renderProfiles() {
  const box = el.protProfiles;
  if (!box) return;
  box.textContent = '';
  PROTECTION_PROFILES.forEach((p) => {
    const label = document.createElement('label');
    label.className = 'prot-profile' + (view.profile === p.id ? ' active' : '');
    const input = document.createElement('input');
    input.type = 'radio';
    input.name = 'prot-profile';
    input.value = p.id;
    input.checked = view.profile === p.id;
    input.addEventListener('change', () => { view.profile = p.id; refreshProtectionPlan(); });
    const title = document.createElement('b');
    title.textContent = p.title;
    const desc = document.createElement('span');
    desc.textContent = p.desc;
    label.append(input, title, desc);
    box.appendChild(label);
  });
}

function renderChecks(plan) {
  const box = el.protChecks;
  if (!box) return;
  box.textContent = '';
  const needs = view.profile === 'ci' || view.profile === 'siki';
  box.hidden = !needs || !plan?.applicable;
  if (box.hidden) return;
  const head = document.createElement('div');
  head.className = 'prot-checks-head';
  head.textContent = 'Hangi CI kontrolleri geçmeden birleştirilemesin? (dalın son çalışmasından listelenir)';
  box.appendChild(head);
  const names = plan.availableChecks || [];
  if (!names.length) {
    const none = document.createElement('div');
    none.className = 'prot-note';
    none.textContent = 'Seçilebilir kontrol bulunamadı (henüz CI çalışmamış olabilir). Yalnızca “dal güncel olmalı” eklenebilir.';
    box.appendChild(none);
    return;
  }
  const already = new Set(plan.current?.checks || []);
  names.forEach((name) => {
    const label = document.createElement('label');
    label.className = 'prot-check';
    const input = document.createElement('input');
    input.type = 'checkbox';
    input.checked = view.checks.has(name) || already.has(name);
    input.disabled = already.has(name); // mevcut zorunlu kontrol gevşetilemez
    input.addEventListener('change', () => {
      if (input.checked) view.checks.add(name); else view.checks.delete(name);
      refreshProtectionPlan();
    });
    const text = document.createElement('span');
    text.textContent = already.has(name) ? `${name} (zaten zorunlu)` : name;
    label.append(input, text);
    box.appendChild(label);
  });
}

function renderNow(plan) {
  const box = el.protNow;
  if (!box) return;
  box.textContent = '';
  const rows = currentRows(plan);
  box.hidden = rows.length === 0;
  rows.forEach((r) => {
    const row = document.createElement('div');
    row.className = 'prot-now-row ' + (r.on ? 'on' : 'off');
    const mark = document.createElement('span');
    mark.className = 'prot-now-mark';
    mark.textContent = r.on ? '✓' : '–';
    const label = document.createElement('span');
    label.textContent = r.label;
    const val = document.createElement('span');
    val.className = 'prot-now-val';
    val.textContent = r.value;
    row.append(mark, label, val);
    box.appendChild(row);
  });
}

function renderPlan(plan) {
  renderNow(plan);
  const list = el.protChanges;
  if (list) {
    list.textContent = '';
    changeRows(plan).forEach((r) => {
      const li = document.createElement('li');
      const label = document.createElement('b');
      label.textContent = r.label;
      const arrow = document.createElement('span');
      arrow.className = 'prot-arrow';
      arrow.textContent = `${r.from} → ${r.to}`;
      li.append(label, arrow);
      list.appendChild(li);
    });
  }
  if (el.protNotes) {
    el.protNotes.textContent = '';
    (plan?.notes || []).forEach((n) => {
      const p = document.createElement('div');
      p.className = 'prot-note';
      p.textContent = n;
      el.protNotes.appendChild(p);
    });
  }
  if (el.btnEnableProtection) {
    // Plan gelene kadar ve değişecek bir şey yokken düğme hiç görünmez (turuncu bir düğme belirip solmasın).
    el.btnEnableProtection.hidden = !canApplyPlan(plan);
    el.btnEnableProtection.disabled = false;
    el.btnEnableProtection.textContent = applyLabel(plan);
  }
}

/** Planı sunucudan yeniden ister ve çizer. Eski yanıt (hızlı tıklama) yenisini ezmez. */
export async function refreshProtectionPlan() {
  const my = ++view.seq;
  renderProfiles();
  if (el.btnEnableProtection) el.btnEnableProtection.hidden = true;
  try {
    const key = selectionKey();
    const plan = await planCall(view.profile, [...view.checks]);
    if (my !== view.seq) return;
    view.plan = plan;
    view.planKey = key;
    renderChecks(plan);
    renderPlan(plan);
  } catch (err) {
    if (my !== view.seq) return;
    view.plan = null;
    view.planKey = '';
    if (el.protChanges) el.protChanges.textContent = '';
    if (el.protNotes) {
      el.protNotes.textContent = '';
      const p = document.createElement('div');
      p.className = 'prot-note';
      p.textContent = err.message || String(err);
      el.protNotes.appendChild(p);
    }
    if (el.btnEnableProtection) el.btnEnableProtection.hidden = true;
  }
}

/** "Uygula": yalnızca ekranda gösterilen planı (parmak iziyle) onaylayıp uygular. */
export async function applyShownProtection(applySnapshot) {
  const plan = view.plan;
  // Ekrandaki plan, şu anki seçimin planı değilse (yeni plan yoldayken) hiçbir şey uygulanmaz.
  if (!canApplyPlan(plan) || view.planKey !== selectionKey()) return;
  try {
    const snap = await applyCall(view.profile, [...view.checks], plan.fingerprint);
    applySnapshot(snap);
    showToast(`🛡️ ${plan.branch} dalı korumaya alındı`, 'ok');
  } catch (err) {
    showToast(err.message, 'danger');
  }
  refreshProtectionPlan();
}

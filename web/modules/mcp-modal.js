// web/modules/mcp-modal.js - MCP bağlantı modalı ve yapılandırma metni
import { state, el } from './state.js';
import { apiCall } from './api.js';
import { showToast } from './utils.js';

// Uzağa yazan / geri alınması zor araçlar: varsayılan KAPALI; açan ortam değişkeni yapılandırmaya `env` olarak girer.
export const MCP_GATED_TOOLS = [
  { env: 'GITARC_MCP_ALLOW_QUEUE_RUN', tool: 'gitarc_queue_run', desc: 'PR\'ları sırayla günceller + birleştirir, ilk sorunda durur' },
  { env: 'GITARC_MCP_ALLOW_QUEUE', tool: 'gitarc_queue_step', desc: 'Kuyruğun tek adımı: dalı günceller ya da birleştirir' },
  { env: 'GITARC_MCP_ALLOW_UPDATE', tool: 'gitarc_update_branch', desc: 'PR dalını main ile günceller (uzak dala push)' },
  { env: 'GITARC_MCP_ALLOW_RESOLVE', tool: 'gitarc_resolve_conflict', desc: 'Çakışmayı çözer, dalı uzağa push eder' },
  { env: 'GITARC_MCP_ALLOW_PROTECTION', tool: 'gitarc_protection_apply', desc: 'Dal korumasını güçlendirir (gevşetmez)' },
];

const enabled = new Set(); // işaretli ortam değişkenleri (her açılışta boş: varsayılan hepsi kapalı)

/** Saf: yapılandırma nesnesi. binPath = çalışan GitArc ikilisi (açık projenin klasörü DEĞİL), env = açılan araçlar. */
export function buildMCPConfig({ binPath, repoPath, enabledEnv = [] }) {
  const server = { command: binPath || '/path/to/gitarc', args: ['-mcp', '-repo', repoPath || '.'] };
  const env = {};
  MCP_GATED_TOOLS.forEach((t) => { if (enabledEnv.includes(t.env)) env[t.env] = '1'; });
  if (Object.keys(env).length) server.env = env;
  return { mcpServers: { gitarc: server } };
}

export function buildMCPConfigText() {
  const repo = state.snapshot?.repo;
  const repoPath = repo?.mode === 'live' && repo?.path ? repo.path : '/path/to/project'; // canlı depo yoksa genel yer tutucu
  return JSON.stringify(buildMCPConfig({ binPath: repo?.execPath, repoPath, enabledEnv: [...enabled] }), null, 2);
}

function refreshSnippet() {
  if (el.mcpConfigSnippet) el.mcpConfigSnippet.textContent = buildMCPConfigText();
}

function renderGated() {
  const box = el.mcpGated;
  if (!box) return;
  box.textContent = '';
  MCP_GATED_TOOLS.forEach((t) => {
    const row = document.createElement('label');
    row.className = 'mcp-gated-row';
    const input = document.createElement('input');
    input.type = 'checkbox';
    input.checked = enabled.has(t.env);
    input.addEventListener('change', () => {
      if (input.checked) enabled.add(t.env); else enabled.delete(t.env);
      refreshSnippet();
    });
    const name = document.createElement('code');
    name.textContent = t.tool;
    const desc = document.createElement('span');
    desc.textContent = t.desc;
    row.append(input, name, desc);
    box.appendChild(row);
  });
}

/** Saf: bir skill kartının metinleri ve düğme durumu (sunucu `status`: not_installed | up_to_date | different | unreadable). */
export function skillCardModel(skill, teamMode = false) {
  const st = skill?.status;
  const chips = {
    not_installed: { chip: 'kurulu değil', tone: 'warn' },
    up_to_date: { chip: 'kurulu ✓ güncel', tone: 'ok' },
    different: { chip: 'kurulu · farklı içerik', tone: 'warn' },
    unreadable: { chip: 'okunamadı', tone: 'warn' },
  };
  const c = chips[st] || { chip: 'bilinmiyor', tone: 'warn' };
  let install = { label: "Claude'a kur", disabled: false, confirm: '' };
  if (teamMode) install = { label: "Claude'a kur", disabled: true, confirm: '', why: 'Takım modunda kapalı (dosya sunucu makinesine yazılırdı); elle kurun.' };
  else if (st === 'up_to_date') install = { label: 'Güncel ✓', disabled: true, confirm: '' };
  else if (st === 'unreadable') install = { label: "Claude'a kur", disabled: true, confirm: '', why: skill?.note || 'Hedef dosya okunamadı.' };
  else if (st === 'different') install = { label: 'Güncelle (üzerine yaz)', disabled: false, confirm: `${skill.path} zaten var ve içeriği GitArc'ın sürümünden farklı. Üzerine yazılsın mı? (Kendi düzenlemeniz varsa kaybolur.)` };
  return { chip: c.chip, tone: c.tone, install, manualNote: `Dosyayı şuraya koyun: ${skill?.path || '~/.claude/skills/<ad>/SKILL.md'}` };
}

async function loadSkills() {
  const box = el.mcpSkills;
  if (!box) return;
  box.textContent = '';
  const note = (txt) => { const d = document.createElement('div'); d.className = 'layer-line'; d.textContent = txt; box.appendChild(d); };
  let res;
  try {
    res = await apiCall('/api/system/skills', 'GET');
  } catch (e) {
    // Tarayıcı dosya yazamaz: çevrimdışı/yedek motorda kurulum ve metin sunucudan gelir, bu yüzden yalnızca bilgi verilir.
    note(e?.message === 'offline' ? 'Skill kurulumu için çalışan GitArc sunucusu gerekir (çevrimdışı modda kullanılamaz).' : `Skill listesi alınamadı: ${e?.message || e}`);
    return;
  }
  const list = res?.skills || [];
  if (!list.length) { note('Gömülü skill yok.'); return; }
  list.forEach((sk) => {
    const m = skillCardModel(sk, !!res.teamMode);
    const card = document.createElement('div');
    card.className = 'diag-card skill-card';
    const h = document.createElement('h4');
    const name = document.createElement('span');
    name.textContent = `🧩 ${sk.name}`;
    const chip = document.createElement('span');
    chip.className = `chip ${m.tone}`;
    chip.textContent = m.chip;
    h.append(name, chip);
    const desc = document.createElement('div');
    desc.className = 'layer-line';
    desc.textContent = sk.description || '';
    const path = document.createElement('code');
    path.className = 'skill-path';
    path.textContent = sk.path || '';
    const actions = document.createElement('div');
    actions.className = 'skill-actions';
    const inst = document.createElement('button');
    inst.type = 'button';
    inst.className = 'btn primary sm skill-install';
    inst.textContent = m.install.label;
    inst.disabled = m.install.disabled;
    if (m.install.why) inst.title = m.install.why;
    inst.addEventListener('click', async () => {
      if (m.install.confirm && !window.confirm(m.install.confirm)) return;
      inst.disabled = true;
      try {
        await apiCall('/api/system/skills/install', 'POST', { name: sk.name, overwrite: !!m.install.confirm });
        showToast(`✓ ${sk.name} kuruldu; Claude Code oturumunu yeniden başlatın`, 'ok');
      } catch (e) {
        showToast(`Kurulamadı: ${e?.message || e}`, 'warn');
      }
      loadSkills();
    });
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'btn ghost sm skill-copy';
    copy.textContent = '📋 Elle (kopyala)';
    copy.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(sk.content || '');
        showToast(`✓ ${sk.name} metni kopyalandı. ${m.manualNote}`, 'ok');
      } catch {
        showToast('Pano izni verilemedi', 'warn');
      }
    });
    actions.append(inst, copy);
    card.append(h, desc, path, actions);
    box.appendChild(card);
  });
}

export function openMCPConnectModal() {
  enabled.clear(); // güvenli varsayılan: her açılışta hepsi kapalı
  renderGated();
  refreshSnippet();
  loadSkills();
  if (el.mcpModal) el.mcpModal.hidden = false;
}

export function closeMCPConnectModal() {
  if (el.mcpModal) el.mcpModal.hidden = true;
}

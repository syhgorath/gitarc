// web/modules/mcp-modal.js - MCP bağlantı modalı ve yapılandırma metni
import { state, el } from './state.js';

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

export function openMCPConnectModal() {
  enabled.clear(); // güvenli varsayılan: her açılışta hepsi kapalı
  renderGated();
  refreshSnippet();
  if (el.mcpModal) el.mcpModal.hidden = false;
}

export function closeMCPConnectModal() {
  if (el.mcpModal) el.mcpModal.hidden = true;
}

// web/modules/mcp-modal.js - MCP bağlantı modalı ve yapılandırma metni
import { state, el } from './state.js';

// MCP Connect Modal
export function buildMCPConfigText() {
  const repoPath = state.snapshot?.repo?.mode === 'live' && state.snapshot?.repo?.path
    ? state.snapshot.repo.path
    : '/path/to/GitArc'; // canlı depo yoksa genel yer tutucu
  const binPath = `${repoPath.replace(/\/$/, '')}/gitarc`;
  return JSON.stringify({
    mcpServers: {
      gitarc: {
        command: binPath,
        args: ['-mcp', '-repo', repoPath],
      },
    },
  }, null, 2);
}

export function openMCPConnectModal() {
  if (el.mcpConfigSnippet) {
    el.mcpConfigSnippet.textContent = buildMCPConfigText();
  }
  if (el.mcpModal) el.mcpModal.hidden = false;
}

export function closeMCPConnectModal() {
  if (el.mcpModal) el.mcpModal.hidden = true;
}

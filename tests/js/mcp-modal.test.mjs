// MCP penceresi: yapılandırma metni doğru ikili yolunu kullanır ve yalnızca işaretlenen araçların env'ini ekler.
// Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildMCPConfig, MCP_GATED_TOOLS } from '../../web/modules/mcp-modal.js';

test('command çalışan ikilinin yolu olur, açık projenin klasörü değil', () => {
  const c = buildMCPConfig({ binPath: '/opt/gitarc/gitarc', repoPath: '/Users/x/Developer/baska-proje' });
  assert.equal(c.mcpServers.gitarc.command, '/opt/gitarc/gitarc');
  assert.deepEqual(c.mcpServers.gitarc.args, ['-mcp', '-repo', '/Users/x/Developer/baska-proje']);
});

test('varsayılan: env yok (uzağa yazan araçların hepsi kapalı)', () => {
  const c = buildMCPConfig({ binPath: '/b/gitarc', repoPath: '/r' });
  assert.equal('env' in c.mcpServers.gitarc, false);
});

test('yalnızca işaretlenen araçlar env olarak girer; bilinmeyen anahtar yok sayılır', () => {
  const c = buildMCPConfig({ binPath: '/b/gitarc', repoPath: '/r', enabledEnv: ['GITARC_MCP_ALLOW_QUEUE_RUN', 'GITARC_MCP_ALLOW_BILINMEYEN'] });
  assert.deepEqual(c.mcpServers.gitarc.env, { GITARC_MCP_ALLOW_QUEUE_RUN: '1' });
});

test('ikili yolu bilinmiyorsa yer tutucu kullanılır', () => {
  assert.equal(buildMCPConfig({ repoPath: '/r' }).mcpServers.gitarc.command, '/path/to/gitarc');
});

test('kapalı araç listesi sunucudaki beş ortam değişkeniyle birebir', () => {
  assert.deepEqual(MCP_GATED_TOOLS.map((t) => t.env).sort(), [
    'GITARC_MCP_ALLOW_PROTECTION', 'GITARC_MCP_ALLOW_QUEUE', 'GITARC_MCP_ALLOW_QUEUE_RUN', 'GITARC_MCP_ALLOW_RESOLVE', 'GITARC_MCP_ALLOW_UPDATE',
  ]);
});

import { skillCardModel } from '../../web/modules/mcp-modal.js';

test('skillCardModel: kurulum düğmesi durumu dürüst; farklı dosya onay ister, güncel/okunamayan/takım modunda kapalı', () => {
  const sk = (status, extra = {}) => ({ name: 'ci-fix', path: '/h/.claude/skills/ci-fix/SKILL.md', status, ...extra });
  const fresh = skillCardModel(sk('not_installed'));
  assert.equal(fresh.install.disabled, false);
  assert.equal(fresh.install.confirm, '', 'yeni kurulum onay istemez');
  assert.equal(skillCardModel(sk('up_to_date')).install.disabled, true);
  assert.match(skillCardModel(sk('up_to_date')).chip, /güncel/);
  const diff = skillCardModel(sk('different'));
  assert.equal(diff.install.disabled, false);
  assert.match(diff.install.confirm, /Üzerine yazılsın mı/);
  assert.match(diff.install.confirm, /ci-fix\/SKILL\.md/);
  const bad = skillCardModel(sk('unreadable', { note: 'sembolik bağ' }));
  assert.equal(bad.install.disabled, true);
  assert.match(bad.install.why, /sembolik/);
  const team = skillCardModel(sk('not_installed'), true);
  assert.equal(team.install.disabled, true);
  assert.match(team.install.why, /Takım modunda/);
  assert.match(fresh.manualNote, /\.claude\/skills\/ci-fix\/SKILL\.md/);
});

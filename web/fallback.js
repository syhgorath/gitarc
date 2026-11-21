(() => {
  'use strict';

  // Fallback simulation engine for standalone file:// opening without Go server
  const LocalFallbackEngine = (() => {
    let seq = 12;
    let nextPr = 425;
    let sim = null;

    // Uzun ömürlü dal kuralı: internal/gitengine/longlived.go ile BİREBİR aynı (Go tarafı tek gerçek kaynak).
    const LONG_LIVED_EXACT = ['develop', 'development', 'dev', 'staging', 'stage', 'production', 'prod', 'stable', 'trunk', 'next', 'release', 'releases', 'main', 'master'];
    const LONG_LIVED_PREFIXES = ['release/', 'release-', 'releases/', 'support/', 'maint/', 'maintenance/', 'stable/', 'stable-'];
    function isLongLivedBranch(name, defaultBranch) {
      const n = String(name || '').trim().toLowerCase();
      if (!n || name === defaultBranch) return false;
      if (LONG_LIVED_EXACT.includes(n) || /^v?\d+(\.(\d+|x))*$/.test(n)) return true;
      return LONG_LIVED_PREFIXES.some((p) => n.startsWith(p));
    }

    // "🧹 Temizle" bu dal için bir şey yapar mı: internal/gitengine/prunable.go ile birebir aynı kural.
    function isPrunable(b) {
      if (b.longLived || b.id === 'local-wip-working-tree' || String(b.id).startsWith('local-wip-')) return false;
      if (b.isHead && (b.uncommittedCount || 0) > 0) return false;
      const mergedPR = Boolean(b.pr && b.pr.reviewState === 'MERGED' && !(b.remoteAhead > 0));
      const gone = Boolean(b.originGone || b.state === 'gone');
      const merged = b.state === 'merged' || mergedPR;
      if (b.state === 'gone') return true;
      if (b.hasLocal && (gone || merged)) return true;
      return Boolean(b.hasRemote && merged && !(b.aheadMain > 0));
    }

    function extractCI(rawLog) {
      if (!rawLog) return [];
      const lines = rawLog.split('\n');
      const out = [];
      let lastFile = '';
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i].trim();
        const fm = line.match(/([a-zA-Z0-9_./-]+\.go:\d+)/);
        if (fm) lastFile = fm[1];
        const pm = line.match(/panic:\s+(.+)$/i);
        if (pm) {
          let fl = lastFile;
          for (let k = i + 1; k < lines.length && k <= i + 6; k++) {
            const sf = lines[k].match(/([a-zA-Z0-9_./-]+\.go:\d+)/);
            if (sf) { fl = sf[1]; break; }
          }
          out.push({ kind: 'panic', symbol: 'panic', fileLine: fl, message: pm[1], reproCmd: 'go test -v ./internal/shellgold' });
          continue;
        }
        const tm = line.match(/---\s+FAIL:\s+(\S+)\s+\(([^)]+)\)/);
        if (tm) {
          out.push({
            kind: 'test_fail',
            symbol: tm[1],
            fileLine: lastFile || 'go/internal/shellgold/compare_test.go:118',
            message: `${tm[1]} failed (${tm[2]})`,
            reproCmd: `go test -v ./internal/shellgold -run '^${tm[1]}$'`,
          });
        }
      }
      return out;
    }

    function initData() {
      const ciLog = [
        '2026-11-16T07:12:04.112Z ##[group]Run cd go && go vet ./... && go test ./... -short',
        '2026-11-16T07:12:05.840Z === RUN   TestShellGoldManifestParity',
        '2026-11-16T07:12:05.912Z --- PASS: TestShellGoldManifestParity (0.07s)',
        '2026-11-16T07:12:06.103Z === RUN   TestShellGoldCompare',
        '2026-11-16T07:12:06.489Z     compare_test.go:118: golden output mismatch on key \'hardening_pve_apply\': got "true", want "false"',
        '2026-11-16T07:12:06.520Z --- FAIL: TestShellGoldCompare (0.42s)',
        '2026-11-16T07:12:06.521Z === RUN   TestShellGoldPanicBoundary',
        '2026-11-16T07:12:06.529Z panic: runtime error: index out of range [3] with length 3',
        '2026-11-16T07:12:06.530Z goroutine 42 [running]:',
        '2026-11-16T07:12:06.531Z 	go/internal/shellgold/compare.go:94 +0x1b4',
        '2026-11-16T07:12:06.532Z FAIL	keylab/internal/shellgold	0.614s',
      ].join('\n');

      sim = {
        repo: {
          id: 'sim-sandbox',
          name: 'GitArc Sandbox (Temel Demo)',
          path: '(Bellek İçi Sanal Depo)',
          defaultBranch: 'main',
          remoteName: 'origin',
          ahead: 2,
          behind: 1,
          dirtyCount: 2,
          mode: 'simulation',
          versionTag: 'v4.0.16',
        },
        repos: [
          { id: 'sim-sandbox', name: 'GitArc Sandbox (Temel Demo)', path: '(Bellek İçi Sanal Depo)', mode: 'simulation' },
          { id: 'sim-enterprise', name: 'MegaCorp Platform (50 Geliştirici · Enterprise Demo)', path: '(Enterprise Çoklu Dal Simülasyonu)', mode: 'simulation' },
        ],
        nodes: [
          { sha: '19a4c01', shortSha: '19a4c01', subject: 'chore(release): prepare v4.0.14', author: 'ziya', relativeTime: '6 gün önce', spineIndex: 0, tag: 'v4.0.14' },
          { sha: '2f81d12', shortSha: '2f81d12', subject: 'refactor(runner): sudo askpass', author: 'ziya', relativeTime: '5 gün önce', spineIndex: 1 },
          { sha: '3c19e44', shortSha: '3c19e44', subject: 'Merge PR #412 feat/vault-idle-lock', author: 'ziya', relativeTime: '4 gün önce', spineIndex: 2, isMerge: true },
          { sha: '4b70a89', shortSha: '4b70a89', subject: 'docs(standard): catalog items', author: 'ziya', relativeTime: '3 gün önce', spineIndex: 3, tag: 'v4.0.15' },
          { sha: '5e02b11', shortSha: '5e02b11', subject: 'Merge PR #415 fix/usb-exfat-update', author: 'ziya', relativeTime: '2 gün önce', spineIndex: 4, isMerge: true },
          { sha: '6d44f08', shortSha: '6d44f08', subject: 'feat(components): verify signers', author: 'ziya', relativeTime: '1 gün önce', spineIndex: 5 },
          { sha: '7a91c33', shortSha: '7a91c33', subject: 'Merge PR #418 feat/utility-items', author: 'ziya', relativeTime: '18 saat önce', spineIndex: 6, isMerge: true, tag: 'v4.0.16' },
          { sha: '8e35d02', shortSha: '8e35d02', subject: 'chore(lock): bump utility 0.2.0', author: 'ziya', relativeTime: '6 saat önce', spineIndex: 7 },
          { sha: '9f10b77', shortSha: '9f10b77', subject: 'test(csp): enforce strict csp', author: 'ziya', relativeTime: '2 saat önce', spineIndex: 8, isHead: true },
        ],
        branches: [
          {
            id: 'br-merged-412', name: 'feat/vault-idle-lock', hemisphere: 'upper', state: 'merged',
            startSpineIndex: 0, endSpineIndex: 2, baseSha: '19a4c01', mergeSha: '3c19e44',
            hasRemote: true, remoteRef: 'origin/feat/vault-idle-lock', originGone: false,
            pr: { number: 412, title: 'feat(vault): idle timeout lock', body: '## Neler Değişti?\n- Boşta kalma süresinde vault otomatik kilitlenir.', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 24s' } },
            changedFiles: ['go/internal/vault/vault.go'],
          },
          {
            id: 'br-merged-415', name: 'fix/usb-exfat-update', hemisphere: 'upper', state: 'merged',
            startSpineIndex: 3, endSpineIndex: 4, baseSha: '4b70a89', mergeSha: '5e02b11',
            hasRemote: false, remoteRef: 'origin/fix/usb-exfat-update', originGone: true,
            pr: { number: 415, title: 'fix(selfupdate): exFAT update', body: '## Neler Değişti?\n- exFAT dosya sisteminde güncelleme kilidi düzeltildi.', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 11s' } },
            hasLocal: true, localSha: '5e02b11', changedFiles: ['go/internal/selfupdate/update.go'],
          },
          {
            id: 'br-merged-418', name: 'feat/utility-items-catalog', hemisphere: 'upper', state: 'merged',
            startSpineIndex: 1, endSpineIndex: 6, baseSha: '2f81d12', mergeSha: '7a91c33',
            hasRemote: false, remoteRef: 'origin/feat/utility-items-catalog', originGone: true,
            pr: { number: 418, title: 'feat(manifest): item_var catalog', body: '## Neler Değişti?\n- Dinamik manifest değişken kataloğu eklendi.', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '2m 03s' } },
            hasLocal: true, localSha: '7a91c33', changedFiles: ['go/internal/manifest/items.go'],
          },
          {
            id: 'br-open-421', name: 'feat/terminal-audit-export', hemisphere: 'upper', state: 'open',
            startSpineIndex: 7, endSpineIndex: 8, baseSha: '8e35d02',
            hasRemote: true, remoteRef: 'origin/feat/terminal-audit-export',
            pr: {
              number: 421, title: 'feat(audit): CSV & JSON streaming export', author: 'ziya', reviewState: 'REVIEW_REQUIRED',
              ci: {
                state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 45s',
                checks: [
                  { name: 'Build & Binary Compile', status: 'COMPLETED', conclusion: 'SUCCESS', detailsUrl: 'https://github.com' },
                  { name: 'Unit Tests (go test ./...)', status: 'COMPLETED', conclusion: 'SUCCESS', detailsUrl: 'https://github.com' },
                  { name: 'Static Analysis (go vet & staticcheck)', status: 'COMPLETED', conclusion: 'SUCCESS', detailsUrl: 'https://github.com' },
                  { name: 'Security & Secret Scan', status: 'COMPLETED', conclusion: 'SUCCESS', detailsUrl: 'https://github.com' }
                ]
              }
            },
            hasLocal: true, localSha: '9a81b33', aheadMain: 2, behindMain: 1, uncommittedCount: 0, lastCommitMsg: 'feat(audit): buffer flush on exit',
            changedFiles: ['CHANGELOG.md', 'go/internal/audit/export.go'],
            conflict: {
              hasConflict: true,
              isChangelogOrLock: true,
              files: ['CHANGELOG.md'],
              strategy: 'union-changelog',
              previewUnified: '<<<<<<< origin/main\n## [4.0.16] - 2026-11-15\n- utility 0.2.0 lockfile updated\n=======\n## [Unreleased] - feat/terminal-audit-export\n- terminal-audit.log CSV & JSON export\n>>>>>>> feat/terminal-audit-export',
            },
            crossPr: [
              { otherBranchId: 'br-open-422', otherBranch: 'fix/runner-race-detector', otherPrNumber: 422, sharedFiles: ['CHANGELOG.md'] },
            ],
          },
          {
            id: 'br-open-422', name: 'fix/runner-race-detector', hemisphere: 'upper', state: 'open',
            startSpineIndex: 8, endSpineIndex: 9, baseSha: '9f10b77',
            hasRemote: true, remoteRef: 'origin/fix/runner-race-detector',
            pr: {
              number: 422, title: 'fix(runner): close leak in job loop', author: 'ziya', reviewState: 'CHANGES_REQUESTED',
              ci: {
                state: 'FAILURE', workflow: 'CI / Test & Vet (linux)', duration: '0m 42s', failedStep: 'go test ./...',
                checks: [
                  { name: 'Build & Binary Compile', status: 'COMPLETED', conclusion: 'SUCCESS', detailsUrl: 'https://github.com' },
                  { name: 'Unit Tests (go test ./...)', status: 'COMPLETED', conclusion: 'FAILURE', detailsUrl: 'https://github.com' },
                  { name: 'Race Detector (-race)', status: 'COMPLETED', conclusion: 'FAILURE', detailsUrl: 'https://github.com' },
                  { name: 'Lint & Vet', status: 'COMPLETED', conclusion: 'SKIPPED', detailsUrl: 'https://github.com' }
                ],
                extracts: extractCI(ciLog),
                rawLog: ciLog,
              },
            },
            hasLocal: true, localSha: '8b72c44', aheadMain: 1, behindMain: 0, uncommittedCount: 0, lastCommitMsg: 'fix(runner): atomic load for worker pool',
            changedFiles: ['CHANGELOG.md', 'go/internal/shellgold/compare.go', 'go/internal/shellgold/compare_test.go'],
            crossPr: [
              { otherBranchId: 'br-open-421', otherBranch: 'feat/terminal-audit-export', otherPrNumber: 421, sharedFiles: ['CHANGELOG.md'] },
            ],
          },
          {
            id: 'br-open-424', name: 'chore/bump-hardening', hemisphere: 'upper', state: 'open',
            startSpineIndex: 8, endSpineIndex: 9, baseSha: '9f10b77',
            hasRemote: true, remoteRef: 'origin/chore/bump-hardening',
            pr: {
              number: 424, title: 'chore(lock): upgrade linux-hardening 0.14.0', author: 'ziya', reviewState: 'REVIEW_REQUIRED',
              ci: {
                state: 'RUNNING', workflow: 'Verify Signatures', duration: '0m 27s',
                checks: [
                  { name: 'Verify Signatures', status: 'IN_PROGRESS', conclusion: 'IN_PROGRESS', detailsUrl: 'https://github.com' },
                  { name: 'Dependency Vulnerability Audit', status: 'QUEUED', conclusion: 'QUEUED', detailsUrl: 'https://github.com' }
                ]
              }
            },
            hasLocal: true, localSha: '7c63d55', aheadMain: 1, behindMain: 0, uncommittedCount: 0, lastCommitMsg: 'chore(deps): linux-hardening 0.14.0',
            changedFiles: ['components.lock'],
          },
          {
            id: 'br-gone-409', name: 'feat/legacy-sqlite-sync', hemisphere: 'upper', state: 'gone',
            startSpineIndex: 2, endSpineIndex: 3, baseSha: '3c19e44',
            hasRemote: false, remoteRef: 'origin/feat/legacy-sqlite-sync', originGone: true,
            hasLocal: true, localSha: '6d54e66', aheadMain: 0, behindMain: 6, uncommittedCount: 0, lastCommitMsg: 'chore: obsolete driver migration',
            changedFiles: ['go/internal/db/driver.go'],
          },
          {
            id: 'br-local-wip', name: 'feat/cockpit-live-metrics', hemisphere: 'lower', state: 'open',
            startSpineIndex: 8, endSpineIndex: 9, baseSha: '9f10b77',
            hasRemote: false, remoteRef: '',
            hasLocal: true, localSha: '5e45f77', isHead: false,
            aheadMain: 3, behindMain: 0, uncommittedCount: 2, lastCommitMsg: 'wip(metrics): live memory gauge ring',
            changedFiles: ['web/app.js', 'web/tasarim.css'],
          },
          {
            id: 'br-local-exp', name: 'exp/wasm-terminal-emulator', hemisphere: 'lower', state: 'open',
            startSpineIndex: 7, endSpineIndex: 8, baseSha: '8e35d02',
            hasRemote: false, remoteRef: '',
            hasLocal: true, localSha: '4f36a88', isHead: false,
            aheadMain: 1, behindMain: 1, uncommittedCount: 0, lastCommitMsg: 'poc: vt100 wasm parser bindings',
            changedFiles: ['web/vendor/xterm.wasm'],
          },
        ],
        diagnostics: [],
        logs: [
          { id: 'log-1', timestamp: '14:02:18', level: 'info', action: 'Repo Açıldı', branch: '', message: 'keylab / sshpocket deposu kokpitte açıldı', detail: 'Mod: simulation · 9 düğüm · 9 dal' },
        ],
      };
    }

    function xOfSpine(idx) {
      return SPINE_START_X + idx * SPINE_STEP_X;
    }

    // Dal koruması planı: internal/gitengine/protection.go ile AYNI kurallar (yalnızca güçlendir, planı onaylayarak uygula).
    function protCleanChecks(list) {
      const out = [];
      (list || []).forEach((c) => {
        const v = String(c).trim();
        if (!v) return;
        if (v.length > 100 || /[\r\n\0]/.test(v)) throw new Error('geçersiz kontrol adı');
        if (!out.includes(v)) out.push(v);
      });
      return out.sort();
    }
    function protProfileTarget(profile, checks) {
      const t = { requirePR: true, approvals: 0, dismissStale: false, strictUpToDate: false, checks: [], enforceAdmins: false, blockForcePush: true, blockDeletion: true };
      if (profile === 'temel') return t;
      if (profile === 'ci' || profile === 'siki') {
        t.strictUpToDate = true;
        t.checks = checks;
        if (profile === 'siki') { t.approvals = 1; t.dismissStale = true; t.enforceAdmins = true; }
        return t;
      }
      throw new Error(`bilinmeyen koruma profili "${profile}" (temel, ci veya siki olmalı)`);
    }
    function protStrengthen(cur, want) {
      const out = {
        requirePR: cur.requirePR || want.requirePR,
        approvals: Math.max(cur.approvals, want.approvals),
        dismissStale: cur.dismissStale || want.dismissStale,
        strictUpToDate: cur.strictUpToDate || want.strictUpToDate,
        checks: protCleanChecks([...cur.checks, ...want.checks]),
        enforceAdmins: cur.enforceAdmins || want.enforceAdmins,
        blockForcePush: cur.blockForcePush || want.blockForcePush,
        blockDeletion: cur.blockDeletion || want.blockDeletion,
      };
      if (!out.requirePR) { out.approvals = 0; out.dismissStale = false; }
      return out;
    }
    const PROT_NONE = () => ({ requirePR: false, approvals: 0, dismissStale: false, strictUpToDate: false, checks: [], enforceAdmins: false, blockForcePush: false, blockDeletion: false });
    function protDiff(cur, tgt) {
      const yn = (b) => (b ? 'evet' : 'hayır');
      const ck = (c) => (c.length ? c.join(', ') : 'yok');
      const rows = [
        ['requirePR', 'PR zorunlu', yn(cur.requirePR), yn(tgt.requirePR)],
        ['approvals', 'Gereken onay', String(cur.approvals), String(tgt.approvals)],
        ['dismissStale', 'Yeni commit eski onayı düşürür', yn(cur.dismissStale), yn(tgt.dismissStale)],
        ['strictUpToDate', 'Dal güncel olmalı', yn(cur.strictUpToDate), yn(tgt.strictUpToDate)],
        ['checks', 'Zorunlu CI kontrolleri', ck(cur.checks), ck(tgt.checks)],
        ['enforceAdmins', 'Yöneticiler için de geçerli', yn(cur.enforceAdmins), yn(tgt.enforceAdmins)],
        ['blockForcePush', 'Force-push kapalı', yn(cur.blockForcePush), yn(tgt.blockForcePush)],
        ['blockDeletion', 'Dal silme kapalı', yn(cur.blockDeletion), yn(tgt.blockDeletion)],
      ];
      return rows.filter((r) => r[2] !== r[3]).map(([field, label, from, to]) => ({ field, label, from, to }));
    }
    function protSummary(s) {
      const p = [];
      if (s.requirePR) p.push(s.approvals > 0 ? `PR zorunlu (${s.approvals} onay)` : 'PR zorunlu');
      if (s.strictUpToDate) p.push('dal güncel olmalı');
      if (s.checks.length) p.push('CI: ' + s.checks.join(', '));
      if (s.enforceAdmins) p.push('yöneticiler dahil');
      if (s.blockForcePush) p.push('force-push kapalı');
      if (s.blockDeletion) p.push('silme kapalı');
      return p.length ? p.join(' · ') : 'Koruma var ama kısıtlama yok';
    }
    function protFingerprint(branch, t) {
      const str = branch + JSON.stringify(t);
      let h = 5381;
      for (let i = 0; i < str.length; i += 1) h = ((h * 33) ^ str.charCodeAt(i)) >>> 0;
      return h.toString(16);
    }
    function protPlan(profile, selected) {
      const branch = sim.repo.defaultBranch || 'main';
      const cur = sim.protection ? { ...sim.protection, checks: [...sim.protection.checks] } : PROT_NONE();
      const state = sim.protection ? 'active' : 'none';
      const checks = protCleanChecks(selected);
      const target = protStrengthen(cur, protProfileTarget(profile, checks));
      const changes = protDiff(cur, target);
      const notes = [];
      if ((profile === 'ci' || profile === 'siki') && !target.checks.length) {
        notes.push('Zorunlu CI kontrolü seçilmedi: yalnızca “dal güncel olmalı” eklenir, CI geçmeden merge yine engellenmez. Aşağıdan en az bir kontrol seçin.');
      }
      if (profile === 'siki' && !cur.requirePR) {
        notes.push('1 onay + yöneticiler dahil kuralı, tek geliştiricili depoda kendi PR\'ınızı birleştirmenizi engeller.');
      }
      if (!changes.length) notes.push('Koruma bu profili zaten karşılıyor; değişecek bir şey yok.');
      return { branch, profile, state, applicable: true, current: cur, target, changes, availableChecks: ['build', 'lint', 'test'], notes, fingerprint: protFingerprint(branch, target) };
    }

    // Merge kuyruğu planı: internal/gitengine/queue.go (QueuePlanFromSnapshotData / assembleQueuePlan) ile AYNI mantık ve metinler.
    function queueUniqueSorted(list) {
      return [...new Set(list)].sort();
    }
    function queueLabel(it) {
      return it.pr > 0 ? `#${it.pr} ${it.branch}` : it.branch;
    }
    function queueRefs(list) {
      return list.map((b) => (b.pr && b.pr.number > 0 ? `#${b.pr.number}` : b.name)).join(', ');
    }
    function queueSelect(branches, ids) {
      const open = (b) => b.state === 'open' && !b.historyOnly;
      if (!ids || !ids.length) {
        return branches.filter((b) => open(b) && b.pr && b.pr.number > 0).sort((a, b) => a.pr.number - b.pr.number);
      }
      const out = [];
      const seen = new Set();
      ids.forEach((raw) => {
        const id = String(raw).trim();
        if (!id || seen.has(id)) return;
        seen.add(id);
        let found = null;
        branches.forEach((b) => {
          if ((b.id === id || b.name === id) && (!found || (open(b) && !open(found)))) found = b;
        });
        if (!found) throw new Error(`kuyruğa eklenemeyen dal: "${id}" bulunamadı`);
        if (!open(found)) throw new Error(`kuyruğa eklenemeyen dal: "${found.name}" açık değil (birleşmiş ya da silinmiş)`);
        out.push(found);
      });
      return out;
    }
    function queueAssemble(mode, baseBranch, items, strict) {
      const plan = { mode, baseBranch, items, stopsAt: 0, mergeable: 0, strictUpToDate: strict, ciRounds: 1, notes: [] };
      plan.items.forEach((it) => {
        if (plan.stopsAt === 0 && it.status !== 'clean') plan.stopsAt = it.position;
        else if (plan.stopsAt !== 0) it.afterBlocker = true;
      });
      plan.mergeable = plan.stopsAt !== 0 ? plan.stopsAt - 1 : plan.items.length;
      if (strict === true) plan.ciRounds = plan.items.length;
      if (!plan.items.length) {
        plan.notes.push('Kuyruğa girecek açık PR/dal yok.');
        return plan;
      }
      if (plan.stopsAt !== 0) {
        const it = plan.items[plan.stopsAt - 1];
        plan.notes.push(`Kuyruk ${plan.stopsAt}. sırada (${queueLabel(it)}) durur: ${it.reason}. Ondan sonrakiler, engel aşılana kadar bekler.`);
      }
      if (strict === true && plan.items.length > 1) {
        plan.notes.push(`Korumada “dal güncel olmalı” açık: her birleşmeden sonra sıradaki dal main'e göre güncellenip CI yeniden koşar (≈ ${plan.ciRounds} ardışık CI turu).`);
      }
      if (strict === null || strict === undefined) {
        plan.notes.push('Korumanın “dal güncel olmalı” kuralı okunamadı (yetki yok ya da gh yok); CI tur sayısı tahminidir.');
      }
      plan.items.forEach((it) => {
        if (it.ci === 'FAILURE') plan.notes.push(`${queueLabel(it)}: CI kırmızı; birleşmeden önce düzeltilmeli.`);
        else if (it.ci === 'RUNNING') plan.notes.push(`${queueLabel(it)}: CI sürüyor.`);
        else if (!it.ci) plan.notes.push(`${queueLabel(it)}: CI durumu bilinmiyor (yeşil varsayılmaz).`);
        if (it.behind > 0) plan.notes.push(`${queueLabel(it)}: ${baseBranch}'in ${it.behind} commit gerisinde.`);
      });
      plan.notes.push('Plan metin düzeyindeki çakışmayı gösterir; anlamsal uyumsuzluğu güncellenmiş dalın CI\'ı yakalar. Plan hiçbir şey değiştirmedi.');
      return plan;
    }
    function queuePlanBuild(branches, base, ids, strict) {
      const sel = queueSelect(branches || [], ids);
      const items = [];
      const merged = [];
      sel.forEach((b, i) => {
        const it = {
          position: i + 1, branchId: b.id, branch: b.name, pr: b.pr ? b.pr.number : 0, title: b.pr ? b.pr.title : '', url: b.pr ? b.pr.url : '',
          behind: b.behindMain || 0, ci: b.pr && b.pr.ci ? b.pr.ci.state : '', review: b.pr ? b.pr.reviewState : '',
        };
        if (b.conflict && b.conflict.hasConflict) {
          Object.assign(it, { status: 'conflict', conflictsOnMain: true, conflictFiles: [...(b.conflict.files || [])], reason: `kuyruğa girmeden önce de ${base} ile çakışıyor` });
        } else {
          const with_ = [];
          let shared = [];
          merged.forEach((p) => {
            const set = new Set(p.changedFiles || []);
            const common = (b.changedFiles || []).filter((f) => set.has(f));
            if (common.length) { with_.push(p); shared = shared.concat(common); }
          });
          if (with_.length) {
            Object.assign(it, {
              status: 'conflict', conflictFiles: queueUniqueSorted(shared),
              conflictsWithBranches: with_.map((p) => p.name), conflictsWith: with_.filter((p) => p.pr && p.pr.number > 0).map((p) => p.pr.number),
              reason: `kuyrukta önceki ${queueRefs(with_)} ile aynı dosyaları değiştiriyor`,
            });
          } else {
            Object.assign(it, { status: 'clean', reason: 'önceki kuyruk birleşmiş gibi temiz birleşir' });
            merged.push(b);
          }
        }
        items.push(it);
      });
      return queueAssemble('simulation', base, items, strict);
    }

    // Dalı ana dala göre güncelle: internal/gitengine/branch_update.go (simBranchUpdatePlanLocked) ile AYNI kurallar ve metinler.
    function updatePlanFor(idOrName) {
      const b = (sim.branches || []).find((x) => x.id === idOrName || x.name === idOrName);
      if (!b) throw new Error(`dal bulunamadı: ${idOrName}`);
      const plan = { mode: 'simulation', branch: b.name, branchId: b.id, pr: b.pr ? b.pr.number : 0, baseRef: sim.repo.defaultBranch || 'main', behind: b.behindMain || 0, ahead: b.aheadMain || 0 };
      if (b.state !== 'open') {
        Object.assign(plan, { status: 'blocked', reason: 'dal açık değil (birleşmiş ya da silinmiş)' });
      } else if (b.conflict && b.conflict.hasConflict) {
        Object.assign(plan, { status: 'conflict', reason: 'ana dalla çakışıyor', conflictFiles: [...(b.conflict.files || [])] });
      } else if (!plan.behind) {
        Object.assign(plan, { status: 'up_to_date', reason: 'ana dala göre güncel' });
      } else {
        Object.assign(plan, { status: 'ready', reason: `ana dalın ${plan.behind} yeni commit'i temiz merge edilir`, fingerprint: protFingerprint(b.name, { ahead: plan.ahead, behind: plan.behind }) });
      }
      return plan;
    }

    // Kuyruk adımı (3a): Go'daki decideQueueStep ile AYNI kurallar ve metinler (queue-step.test.mjs).
    function queueStepDecide(plan, input) {
      const up = input.update;
      const out = { ...plan, baseRef: up.baseRef, headSha: up.headSha || '', baseSha: up.baseSha || '', behind: up.behind || 0, ci: input.ci || 'UNKNOWN', review: input.review || '' };
      const block = (reason) => ({ ...out, action: 'blocked', reason, fingerprint: '' });
      if (input.prState && input.prState !== 'OPEN') return block(`PR açık değil (${input.prState})`);
      if (input.draft) return block('PR taslak (draft); önce "Ready for review" yapın');
      if (up.status === 'conflict') {
        out.conflictFiles = [...(up.conflictFiles || [])];
        return block(`ana dalla çakışıyor (${out.conflictFiles.join(', ')}); önce Çakışma Stüdyosu'ndan çözün`);
      }
      if (up.status === 'blocked') return block(up.reason);
      if (input.ghHeadSha && up.headSha && input.ghHeadSha !== up.headSha) return block("GitHub'daki dal ucu yerel uzak kaydından farklı; yenileyip (git fetch) planı yeniden hesaplayın");
      if (input.review === 'CHANGES_REQUESTED') return block('incelemede değişiklik istendi');
      if (input.review === 'REVIEW_REQUIRED') return block('inceleme onayı gerekli');
      if (out.ci === 'FAILURE') return block('CI başarısız; kuyruk durur, önce testleri düzeltin');
      const fp = (action) => protFingerprint(`${action}:${out.branch}`, { head: up.headSha || up.ahead, base: up.baseSha || up.behind, ci: out.ci });
      if (up.status === 'ready') {
        return { ...out, action: 'update', reason: `main'in ${up.behind} yeni commit'i dalın içine merge edilip uzak PR dalına push edilir; CI yeniden başlar`, fingerprint: fp('update') };
      }
      if (up.status !== 'up_to_date') return block(`dal durumu belirlenemedi: ${up.reason}`);
      if (out.ci === 'SUCCESS') return { ...out, action: 'merge', reason: 'dal main\'e göre güncel ve CI yeşil; PR birleştirilebilir (merge commit)', fingerprint: fp('merge') };
      if (out.ci === 'RUNNING') return { ...out, action: 'wait', reason: 'CI sürüyor; bitince bu adım yeniden planlanır' };
      return { ...out, action: 'wait', reason: 'CI durumu bilinmiyor (kontrol yok ya da henüz başlamadı); yeşil varsayılmaz' };
    }

    function queueStepPlanFor(idOrName) {
      const up = updatePlanFor(idOrName);
      const b = sim.branches.find((x) => x.id === up.branchId);
      const plan = { mode: 'simulation', branch: up.branch, branchId: up.branchId, pr: up.pr || 0 };
      if (!plan.pr) return { ...plan, baseRef: up.baseRef, behind: up.behind, action: 'blocked', reason: 'bu dalın açık bir PR\'ı yok', ci: 'UNKNOWN' };
      const st = b.pr && b.pr.ci ? b.pr.ci.state : '';
      const ci = st === 'SUCCESS' || st === 'FAILURE' ? st : (st ? 'RUNNING' : 'UNKNOWN');
      const res = queueStepDecide(plan, { prState: 'OPEN', ci, update: up });
      res.notes = ['Simülasyon: inceleme durumu değerlendirilmez.'];
      return res;
    }

    // Kuyruk otomatik koşusu (3b): çevrimdışı yedek motor ZAMAN geçirmez: adımları art arda uygular; CI "sürüyor/bilinmiyor"
    // ya da herhangi bir engelde durur (gerçek bekleme yalnızca sunucuda). Kurallar Go'daki queue_run.go ile aynıdır.
    let lastQueueRun = null;
    function queueRunPlanFor(ids) {
      const qp = queuePlanBuild(sim.branches, sim.repo.defaultBranch || 'main', ids, sim.protection ? Boolean(sim.protection.strictUpToDate) : false);
      const limit = qp.stopsAt > 0 ? qp.stopsAt - 1 : qp.items.length;
      const items = []; const excluded = [];
      qp.items.forEach((it, i) => {
        const ri = { position: i + 1, branchId: it.branchId, branch: it.branch, pr: it.pr || 0, title: it.title || '' };
        if (i < limit && ri.pr > 0 && excluded.length === 0) items.push(ri); else excluded.push(ri);
      });
      const plan = { mode: 'simulation', baseBranch: qp.baseBranch, baseSha: 'sim', items, excluded, startable: false, pollSeconds: 20, timeoutMinutes: 30, notes: [] };
      if (!items.length) plan.reason = 'kuyruğun ilk PR\'ı sorunsuz birleşemiyor; koşu başlatılamaz';
      else { plan.startable = true; plan.fingerprint = protFingerprint('run', { ids: items.map((x) => x.branchId), n: sim.nodes.length }); }
      if (excluded.length) plan.notes.push(`${excluded.length} PR koşuya girmez (ilk sorunlu PR ve sonrakiler); koşu bitince yeniden planlayın.`);
      plan.notes.push('Çevrimdışı simülasyonda koşu zaman geçirmez: CI sürüyorsa ya da herhangi bir engelde durur.');
      return plan;
    }

    function recompute() {
      const branches = sim.branches;
      const nodes = sim.nodes;

      // Reset lanes
      ['upper', 'lower'].forEach((hemi) => {
        const laneItems = [];
        branches.forEach((b, idx) => {
          if (b.hemisphere !== hemi) return;
          if (b.state === 'merged') {
            const s = Math.min(b.startSpineIndex, b.endSpineIndex);
            const e = Math.max(b.startSpineIndex, b.endSpineIndex);
            laneItems.push({ idx, s, e, span: e - s });
          } else {
            b.lane = 1;
          }
        });
        laneItems.sort((a, b) => a.span - b.span || a.s - b.s);
        const lanes = [];
        laneItems.forEach((it) => {
          let placed = false;
          for (let l = 0; l < lanes.length; l++) {
            const coll = lanes[l].some((occ) => !(it.e <= occ.s || it.s >= occ.e));
            if (!coll) {
              lanes[l].push({ s: it.s, e: it.e });
              branches[it.idx].lane = l + 1;
              placed = true;
              break;
            }
          }
          if (!placed) {
            lanes.push([{ s: it.s, e: it.e }]);
            branches[it.idx].lane = lanes.length;
          }
        });
      });

      const diags = [];
      branches.forEach((b) => {
        b.longLived = isLongLivedBranch(b.name, sim.repo.defaultBranch || 'main');
        b.prunable = isPrunable(b);
        b.canAmendCommit = Boolean(b.isHead && b.state === 'open' && (b.aheadMain || 0) > 0);
        if (b.state === 'merged' || b.originGone || b.state === 'gone') {
          // Sunucudaki BuildSuggestedCommand ile aynı kural: temizlenecek kopya yoksa komut önerme.
          if (!b.hasLocal && !b.hasRemote) b.suggestedCmd = '';
          else if (!b.hasLocal && b.hasRemote) b.suggestedCmd = `git push origin --delete ${b.name} && git fetch --prune origin`;
          else b.suggestedCmd = `git branch -D ${b.name} && git fetch --prune origin`;
        } else if (b.conflict?.hasConflict && !b.conflict.resolved) {
          b.suggestedCmd = `git switch ${b.name} && git merge origin/main`;
        } else if (b.pr?.ci?.state === 'FAILURE') {
          b.suggestedCmd = `git switch ${b.name} && ${b.pr.ci.extracts?.[0]?.reproCmd || 'go test -v ./...'}`;
        } else if (b.pr?.number) {
          b.suggestedCmd = `gh pr merge ${b.pr.number} --merge --delete-branch`;
        } else {
          b.suggestedCmd = `git push -u origin ${b.name} && gh pr create --fill`;
        }
        if (b.pr?.ci?.state === 'FAILURE') {
          diags.push({ id: 'ci-' + b.id, severity: 'critical', kind: 'ci_failure', branchId: b.id, branchName: b.name, title: `PR #${b.pr.number} CI Hatası: ${b.name}`, detail: b.pr.ci.extracts?.[0]?.symbol || b.pr.ci.failedStep });
        }
        if (b.conflict?.hasConflict && !b.conflict.resolved) {
          diags.push({ id: 'conf-' + b.id, severity: 'warning', kind: 'conflict', branchId: b.id, branchName: b.name, title: b.conflict.isChangelogOrLock ? 'CHANGELOG / Lockfile Çakışması' : 'Merge Çakışması', detail: b.conflict.files.join(', ') });
        }
        if (b.crossPr?.length) {
          diags.push({ id: 'cross-' + b.id, severity: 'warning', kind: 'cross_pr', branchId: b.id, branchName: b.name, title: `Cross-PR Çarpışma (${b.crossPr.map((c) => '#' + c.otherPrNumber).join(', ')})`, detail: b.crossPr.map((c) => c.sharedFiles.join(', ')).join(' · ') });
        }
        if ((b.originGone || b.state === 'gone') && !b.longLived) {
          diags.push({ id: 'gone-' + b.id, severity: 'info', kind: 'prune_candidate', branchId: b.id, branchName: b.name, title: 'Uzakta Silinmiş Dal (origin: gone)', detail: `${b.name} temizlenebilir` });
        }
      });
      sim.diagnostics = diags;
      // Demo veri toplama adımları (canlı motordaki SyncSteps ile aynı şekil)
      const nowIso = new Date().toISOString();
      const openPrs = branches.filter((b) => b.state === 'open' && b.pr?.number).length;
      const mergedPrs = branches.filter((b) => b.state === 'merged' && b.pr?.number).length;
      sim.repo.protectionState = sim.repo.branchProtected ? 'active' : 'none';
      if (!sim.repo.mainCI) {
        const tip = sim.nodes[sim.nodes.length - 1];
        sim.repo.mainCI = { state: 'SUCCESS', workflow: 'CI', branch: sim.repo.defaultBranch || 'main', sha: tip ? String(tip.shortSha).slice(0, 7) : '', title: tip ? tip.subject : '' };
      } else if (sim.repo.mainCI.state === 'RUNNING' && Date.now() - Date.parse(sim.repo.mainCI.updatedAt) > 6000) {
        sim.repo.mainCI.state = 'SUCCESS';
      }
      sim.repo.syncSteps = [
        { id: 'fetch', label: 'git fetch', state: 'ok', at: nowIso, durationMs: 410 },
        { id: 'prs', label: 'PR listesi', state: 'ok', at: nowIso, durationMs: 1200, count: openPrs },
        { id: 'merged', label: 'Birleşmiş PR', state: 'ok', at: nowIso, durationMs: 800, count: mergedPrs },
        { id: 'protection', label: 'Koruma', state: 'ok', at: nowIso, durationMs: 300 },
      ];
      return JSON.parse(JSON.stringify(sim));
    }

    initData();

    return {
      snapshot() { return recompute(); },
      reset() { initData(); return recompute(); },
      createBranch(name, hemisphere, fromSha) {
        let startIdx = sim.nodes.length - 1;
        let baseSha = sim.nodes[startIdx].shortSha;
        const found = sim.nodes.find((n) => n.shortSha === fromSha);
        if (found) { startIdx = found.spineIndex; baseSha = found.shortSha; }
        seq++;
        const id = `br-${seq}`;
        sim.branches.forEach((b) => { b.isHead = false; });
        const upper = hemisphere === 'upper';
        sim.branches.push({
          id, name, hemisphere, state: 'open',
          startSpineIndex: startIdx, endSpineIndex: startIdx + 1, baseSha,
          hasRemote: upper, remoteRef: upper ? 'origin/' + name : '',
          pr: upper ? { number: nextPr++, title: 'feat: ' + name, author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'RUNNING', workflow: 'ci.yml', duration: '0m 12s' } } : null,
          hasLocal: true, localSha: (0x90a1000 + seq * 17).toString(16), isHead: true,
          aheadMain: 1, behindMain: 0, uncommittedCount: 0, lastCommitMsg: `wip: ${name} ilk adım`,
          changedFiles: ['go/internal/gitarc/feature.go'],
        });
        return recompute();
      },
      checkoutBranch(branchId) {
        let found = false;
        sim.branches.forEach((b) => {
          if (b.id === branchId) {
            if (b.state !== 'open') throw new Error('Yalnızca açık dallara geçilebilir');
            b.isHead = true;
            found = true;
          } else {
            b.isHead = false;
          }
        });
        if (!found) throw new Error('Dal bulunamadı');
        return recompute();
      },
      updatePRMeta(branchId, title, body) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b || !b.pr) throw new Error('Açık PR bulunamadı');
        b.pr.title = title;
        b.pr.body = body;
        return recompute();
      },
      amendLastCommit(branchId, message) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b || !b.isHead || b.state !== 'open') throw new Error('Yalnızca aktif çalışma dalının (HEAD) son commit mesajı değiştirilebilir');
        b.lastCommitMsg = message;
        return recompute();
      },
      mergeBranch(branchId, strategy = 'merge') {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b) throw new Error('Dal bulunamadı');
        if (b.conflict?.hasConflict && !b.conflict.resolved) {
          throw new Error(`${b.name} dalında çözülmemiş çakışma var; önce Auto-Resolve çalıştırın`);
        }
        if (b.pr?.ci?.state === 'FAILURE') {
          throw new Error(`${b.name} dalında CI testleri başarısız; önce hatayı düzeltin`);
        }
        seq++;
        const idx = sim.nodes.length;
        const sha = (0xa10f000 + seq * 29).toString(16);
        sim.nodes.forEach((n) => { n.isHead = false; });
        sim.nodes.push({
          sha, shortSha: sha,
          subject: b.pr ? `Merge PR #${b.pr.number} ${b.name}` : `Merge branch '${b.name}'`,
          author: 'ziya', relativeTime: 'az önce', spineIndex: idx, isMerge: true, isHead: true,
        });
        b.hemisphere = 'upper';
        b.state = 'merged';
        b.endSpineIndex = idx;
        b.mergeSha = sha;
        b.originGone = true;
        b.isHead = false;
        b.uncommittedCount = 0;
        if (b.pr) { b.pr.reviewState = 'MERGED'; if (b.pr.ci) b.pr.ci.state = 'SUCCESS'; }
        sim.repo.ahead++;
        // Merge sonrası omurgada testler başlar (sunucu simülasyonundaki simMainCI ile aynı): önce RUNNING, ~6 sn sonra SUCCESS.
        sim.repo.mainCI = { state: 'RUNNING', workflow: 'CI', branch: sim.repo.defaultBranch || 'main', sha: sha.slice(0, 7), title: sim.nodes[idx].subject, updatedAt: new Date().toISOString() };
        return recompute();
      },
      fixCI(branchId) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b) throw new Error('Dal bulunamadı');
        if (b.pr?.ci) {
          b.pr.ci.state = 'SUCCESS';
          b.pr.ci.failedStep = '';
          b.pr.ci.rawLog = '';
          b.pr.ci.extracts = [];
          b.pr.ci.duration = '0m 52s';
          b.pr.reviewState = 'APPROVED';
        }
        b.lastCommitMsg = 'fix(shellgold): guard slice upper bound & update golden';
        b.uncommittedCount = 0;
        return recompute();
      },
      promoteBranch(branchId, title = '', body = '') {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b) throw new Error('Dal bulunamadı');
        b.hemisphere = 'upper';
        b.hasRemote = true;
        b.remoteRef = 'origin/' + b.name;
        const cleanTitle = (title || '').trim() || b.draftPrTitle || ('feat: ' + b.name);
        const cleanBody = (body || '').trim() || b.draftPrBody || '';
        if (!b.pr) {
          b.pr = { number: nextPr++, title: cleanTitle, body: cleanBody, author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'RUNNING', workflow: 'ci.yml', duration: '0m 12s' } };
        } else {
          b.pr.title = cleanTitle;
          b.pr.body = cleanBody;
        }
        return recompute();
      },
      commitWorkingTree(branchName, commitMsg, prTitle = '', prBody = '', pushAndPr = false, files = []) {
        const cleanBranch = (branchName || '').trim() || 'feat/working-tree';
        const cleanCommit = (commitMsg || '').trim();
        if (!cleanCommit) throw new Error('Commit mesajı boş olamaz');
        const secretPats = ['img/logoyeni.jpg', 'referans/**', '.mcp.json', '.env', '*.pem', '*.key', 'id_*'];
        for (const f of files) {
          const lower = (f || '').toLowerCase();
          for (const pat of secretPats) {
            if (pat.endsWith('/**') && lower.startsWith(pat.replace('/**', ''))) {
              throw new Error(`GÜVENLİK ENGELİ: Gizli dosya tespit edildi: '${f}' (${pat}). Commit engellendi!`);
            } else if (lower.includes(pat.replace(/\*/g, ''))) {
              throw new Error(`GÜVENLİK ENGELİ: Gizli dosya tespit edildi: '${f}' (${pat}). Commit engellendi!`);
            }
          }
        }
        if (pushAndPr) {
          const existing = sim.branches.find((x) => x.name === cleanBranch);
          if (existing && (existing.state === 'merged' || existing.originGone || existing.state === 'gone')) {
            throw new Error(`'${cleanBranch}' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın üzerine push yapılamaz! Lütfen yeni bir dal adı belirleyin.`);
          }
        }
        const startIdx = Math.max(0, sim.nodes.length - 1);
        const baseSha = sim.nodes[startIdx]?.shortSha || '9f10b77';
        seq++;
        const id = `br-${seq}`;
        sim.repo.dirtyCount = 0;
        sim.branches.forEach((x) => {
          x.isHead = false;
          x.uncommittedCount = 0;
        });
        const cleanTitle = (prTitle || '').trim() || cleanCommit;
        const cleanBody = (prBody || '').trim();
        const pr = pushAndPr
          ? { number: nextPr++, title: cleanTitle, body: cleanBody, author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'RUNNING', workflow: 'ci.yml', duration: '0m 09s' } }
          : null;
        sim.branches.push({
          id,
          name: cleanBranch,
          hemisphere: pushAndPr ? 'upper' : 'lower',
          state: 'open',
          startSpineIndex: startIdx,
          endSpineIndex: startIdx + 1,
          baseSha,
          hasRemote: Boolean(pushAndPr),
          remoteRef: pushAndPr ? 'origin/' + cleanBranch : '',
          pr,
          hasLocal: true,
          localSha: (0x90a1000 + seq * 17).toString(16),
          isHead: true,
          aheadMain: 1,
          behindMain: 0,
          uncommittedCount: 0,
          lastCommitMsg: cleanCommit,
          draftPrTitle: cleanTitle,
          draftPrBody: cleanBody,
          changedFiles: files.length > 0 ? [...files] : ['CHANGELOG.md'],
        });
        return recompute();
      },
      completeRunningCI(branchId) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (b?.pr?.ci && b.pr.ci.state === 'RUNNING') {
          b.pr.ci.state = 'SUCCESS';
          b.pr.ci.duration = '0m 41s';
          b.pr.reviewState = 'APPROVED';
        }
        return recompute();
      },
      dismissProposal() {
        sim.pendingProposal = null;
        return recompute();
      },
      // Bekleyen AI önerisini onaylar (sunucudaki Manager.ApproveProposal ile aynı kurallar): öneri kimliği eşleşmeli,
      // boş alanlar önerinin değerlerini kullanır, dosyalar HER ZAMAN önerinin kendisinden gelir. Hata olursa öneri
      // bekleyen durumda kalır.
      approveProposal(payload = {}) {
        const prop = sim.pendingProposal;
        if (!prop) throw new Error('bekleyen AI önerisi yok (zaten onaylanmış, reddedilmiş veya süresi dolmuş olabilir)');
        if ((payload.proposalId || '').trim() !== prop.id) {
          throw new Error('öneri değişmiş veya artık geçerli değil; sayfayı yenileyip güncel öneriyi onaylayın');
        }
        const branchName = (payload.branchName || '').trim() || prop.branchName;
        const message = (payload.message || '').trim() || prop.message;
        const prTitle = (payload.prTitle || '').trim() || prop.prTitle;
        const prBody = (payload.prBody || '').trim() || prop.prBody;
        if (!/^[A-Za-z0-9][A-Za-z0-9._\/-]*$/.test(branchName) || branchName.includes('..') || branchName.endsWith('/') || branchName.endsWith('.lock')) {
          throw new Error(`Geçersiz dal adı: ${branchName}`);
        }
        if (branchName === (sim.repo.defaultBranch || 'main')) {
          throw new Error(`PR açmak için ${sim.repo.defaultBranch || 'main'} dışında yeni bir özellik dalı adı girin`);
        }
        const result = this.commitWorkingTree(branchName, message, prTitle, prBody, true, [...(prop.files || [])]);
        sim.pendingProposal = null;
        return recompute() || result;
      },
      proposeCommit(branchName, message, files = [], prTitle = '', prBody = '') {
        const cleanBranch = (branchName || '').trim();
        const rawMsg = (message || '').trim();
        if (!cleanBranch || !rawMsg) throw new Error('Dal adı ve mesaj zorunludur');
        if (!files || !Array.isArray(files) || files.length === 0) {
          throw new Error('files parametresi zorunludur ve en az bir geçerli dosya yolu içermelidir; boş bırakılamaz');
        }

        const isMerged = sim.branches.some((b) => b.name.toLowerCase() === cleanBranch.toLowerCase() && (b.state === 'merged' || b.originGone));
        if (isMerged) {
          throw new Error(`'${cleanBranch}' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın adı öneri için tekrar kullanılamaz!`);
        }

        const countMatch = rawMsg.match(/(\d+)\s+dosya/i) || rawMsg.match(/update\s+(\d+)\s+files/i) || rawMsg.match(/(\d+)\s+files/i);
        let countCheckMatch = true;
        let countCheckMessage = `✓ ${files.length} dosya seçildi (mesajda sayı belirtilmedi)`;
        if (countMatch) {
          const num = parseInt(countMatch[1], 10);
          countCheckMatch = num === files.length;
          countCheckMessage = countCheckMatch
            ? `✓ Dosya sayısı commit mesajıyla tam eşleşiyor (${files.length} dosya)`
            : `⚠ UYARI: Commit mesajında '${num}' dosya belirtilmiş, ancak ${files.length} dosya seçildi!`;
        }

        sim.pendingProposal = {
          id: 'prop-fallback',
          branchName: cleanBranch,
          baseBranch: 'origin/main',
          message: message,
          files: files,
          prTitle: prTitle || rawMsg,
          prBody: prBody || 'Claude önerisi',
          diffStat: `${files[0]} | 2 +-\n ${files.length} file(s) changed`,
          filesChanged: files.length,
          totalLines: 2,
          isLargeDiff: files.length > 10,
          createdAt: new Date().toLocaleTimeString(),
          shieldResult: `✓ Güvenlik Kalkanı: 0 secret, 0 özel anahtar, 0 .gitignore ihlali (${files.length} dosya onaylandı)`,
          countCheckMatch,
          countCheckMessage,
        };
        return recompute();
      },
      inspectConflict(branchId) {
        const b = sim.branches.find((x) => x.id === branchId || x.name === branchId);
        if (!b || !b.conflict || !b.conflict.hasConflict) {
          throw new Error('Aktif çakışma bulunamadı');
        }
        const files = (b.conflict.files || []).map((f) => {
          if (f.toLowerCase().includes('changelog')) {
            return {
              path: f,
              startLine: 1,
              endLine: 10,
              conflictHunks: `<<<<<<< origin/main\n## [4.0.16] - 2026-11-15\n- utility 0.2.0 lockfile updated\n=======\n## [Unreleased] - ${b.name}\n- terminal-audit.log CSV & JSON export\n>>>>>>> ${b.name}`,
              unionPreview: '## [4.0.16] - 2026-11-15\n- utility 0.2.0 lockfile updated\n## [Unreleased]\n- terminal-audit.log CSV & JSON export',
              oursPreview: '## [4.0.16] - 2026-11-15\n- utility 0.2.0 lockfile updated',
              theirsPreview: `## [Unreleased] - ${b.name}\n- terminal-audit.log CSV & JSON export`,
            };
          }
          return {
            path: f,
            startLine: 1,
            endLine: 6,
            conflictHunks: `<<<<<<< origin/main\nutility 0.2.0 sha256:9f82c10d\n=======\naudit-exporter 0.1.0 sha256:44a1e8b2\n>>>>>>> ${b.name}`,
            unionPreview: 'utility 0.2.0 sha256:9f82c10d\naudit-exporter 0.1.0 sha256:44a1e8b2',
            oursPreview: 'utility 0.2.0 sha256:9f82c10d',
            theirsPreview: 'audit-exporter 0.1.0 sha256:44a1e8b2',
          };
        });
        const isOnlyChangelog = (b.conflict.files || []).every((f) => f.toLowerCase().includes('changelog'));
        const hasLockOrVersion = (b.conflict.files || []).some((f) => {
          const l = f.toLowerCase();
          return l.includes('lock') || l.includes('version') || l.endsWith('.sum');
        });

        let recommended = 'ours';
        let recommendationReason = 'Çakışan dosya kaynak kodudur. İki bloğu alt alta eklemek derleme hatası üretebilir; origin/main kodu veya özel AI düzenleme önerilir.';

        if (isOnlyChangelog) {
          recommended = 'union';
          recommendationReason = "Çakışan dosya değişiklik günlüğüdür (CHANGELOG.md). '1. İkisini Alt Alta Koru (Union)' seçeneği her iki tarafın sürüm notlarını kayıpsız alt alta birleştirir.";
        } else if (hasLockOrVersion) {
          recommended = 'custom';
          recommendationReason = 'Çakışan dosyalarda VERSION veya kilit dosyası (components.lock) bulunmaktadır. Sürüm veya kilit dosyalarına otomatik union uygulanamaz; insan onayı ve manuel/AI seçimi gereklidir.';
        }

        const strategies = isOnlyChangelog ? [
          { strategy: 'union', badge: '★ ÖNERİLEN · Veri Kaybı Yok', tone: 'ok' },
          { strategy: 'ours', badge: 'Daldaki Notları Atlar', tone: 'warn' },
          { strategy: 'theirs', badge: 'Main Notlarını Atlar', tone: 'warn' },
          { strategy: 'custom', badge: 'Tam Kontrol', tone: 'accent' },
        ] : hasLockOrVersion ? [
          { strategy: 'custom', badge: '★ İNSAN ONAYI GEREKLİ', tone: 'ok' },
          { strategy: 'ours', badge: 'Main Kilidini Koru', tone: 'warn' },
          { strategy: 'theirs', badge: 'Dal Kilidini Koru', tone: 'accent' },
          { strategy: 'union', badge: '⛔ Formatı Bozar', tone: 'danger' },
        ] : [
          { strategy: 'union', badge: '⚠ Kodda Çift Tanım Riski', tone: 'warn' },
          { strategy: 'ours', badge: '★ ÖNERİLEN · Main Güncel Kodu', tone: 'ok' },
          { strategy: 'theirs', badge: '✓ Sadece Dalın Kodu', tone: 'accent' },
          { strategy: 'custom', badge: '✓ Özgün Birleştirme', tone: 'accent' },
        ];
        return {
          branchId: b.id,
          branchName: b.name,
          baseRef: 'origin/main',
          recommended,
          recommendationReason,
          strategies,
          aiPrompt: `GitArc Çakışma Çözümü İsteği (${b.name} ↔ origin/main)\n${files.map((x) => `${x.path}:\n${x.conflictHunks}`).join('\n\n')}`,
          files,
        };
      },
      resolveConflict(branchId, strategy = 'union') {
        const b = sim.branches.find((x) => x.id === branchId || x.name === branchId);
        if (b?.conflict) {
          b.conflict.hasConflict = false;
          b.conflict.resolved = true;
          b.conflict.strategy = strategy || 'union';
          b.conflict.resolvedNote = `Çakışma (${strategy}) ile çözüldü; çalışma dizini temiz.`;
          b.conflict.previewUnified = `✓ Çakışma Çözüm Stüdyosu (${strategy}) uygulandı.\n✓ git merge-tree --write-tree: exit 0`;
          b.behindMain = 0;
        }
        return recompute();
      },
      deleteBranch(branchId) {
        const next = [];
        sim.branches.forEach((b) => {
          if (b.id === branchId) {
            if (isLongLivedBranch(b.name, sim.repo.defaultBranch || 'main')) {
              throw new Error(`${b.name} uzun ömürlü bir dal (develop/release); GitArc bunu silmez`);
            }
            if (b.state === 'merged' && (b.hasLocal || b.originGone)) {
              next.push({ ...b, hasLocal: false, originGone: false, isHead: false });
            }
            return;
          }
          next.push(b);
        });
        sim.branches = next;
        return recompute();
      },
      pruneGone() {
        let removed = 0;
        const next = [];
        sim.branches.forEach((b) => {
          if (isLongLivedBranch(b.name, sim.repo.defaultBranch || 'main')) {
            next.push(b);
            return;
          }
          if (b.state === 'merged') {
            if (b.hasLocal || b.originGone || (b.hasRemote && !(b.aheadMain > 0))) {
              if (isPrunable(b)) removed++;
              // Canlı motor gibi: birleşmiş dalın uzak kopyası da tek tıkta silinir.
              next.push({ ...b, hasLocal: false, originGone: false, isHead: false, hasRemote: (b.aheadMain > 0) ? b.hasRemote : false });
            } else {
              next.push(b);
            }
            return;
          }
          if (b.originGone || b.state === 'gone') {
            removed++;
            return;
          }
          next.push(b);
        });
        sim.branches = next;
        return { snapshot: recompute(), removed };
      },
      protectionPlan(profile, checks) {
        return protPlan(profile, checks);
      },
      applyProtection(profile, checks, confirm) {
        const plan = protPlan(profile, checks);
        if (!plan.changes.length) throw new Error('koruma bu profili zaten karşılıyor; değişecek bir şey yok');
        if (!confirm || confirm !== plan.fingerprint) throw new Error('koruma planı değişti veya onaylanmadı; planı yeniden inceleyip onaylayın');
        sim.protection = plan.target;
        sim.repo.strictUpToDate = Boolean(plan.target.strictUpToDate);
        sim.repo.branchProtected = true;
        sim.repo.protectionSummary = protSummary(plan.target);
        return recompute();
      },
      updateBranchPlan(idOrName) {
        recompute();
        return updatePlanFor(idOrName);
      },
      updateBranch(idOrName, confirm) {
        recompute();
        const plan = updatePlanFor(idOrName);
        if (plan.status === 'up_to_date') throw new Error(`${plan.branch} zaten ana dala göre güncel; yapılacak bir şey yok`);
        if (plan.status === 'conflict') throw new Error(`${plan.branch} ana dalla çakışıyor (${plan.conflictFiles.join(', ')}); güncelleme yapılmadı, önce çakışmayı Çakışma Stüdyosu'ndan çözün`);
        if (plan.status !== 'ready') throw new Error(`${plan.branch} güncellenemez: ${plan.reason}`);
        if (!confirm || confirm !== plan.fingerprint) throw new Error('güncelleme planı değişti veya onaylanmadı (dal ya da ana dal ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey değiştirilmedi');
        const b = sim.branches.find((x) => x.id === plan.branchId);
        b.behindMain = 0;
        return recompute();
      },
      queueStepPlan(idOrName) {
        recompute();
        return queueStepPlanFor(idOrName);
      },
      queueStep(idOrName, confirm) {
        recompute();
        const plan = queueStepPlanFor(idOrName);
        if (plan.action === 'wait') throw new Error(`${plan.branch} için şimdilik yapılacak bir şey yok: ${plan.reason}`);
        if (plan.action !== 'update' && plan.action !== 'merge') throw new Error(`${plan.branch} için adım uygulanamaz: ${plan.reason}`);
        if (!confirm || confirm !== plan.fingerprint) throw new Error('kuyruk adımı planı değişti veya onaylanmadı (dal, main ya da CI durumu ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey değiştirilmedi');
        if (plan.action === 'update') return this.updateBranch(plan.branchId, updatePlanFor(plan.branchId).fingerprint);
        const snap = this.mergeBranch(plan.branchId, 'merge');
        sim.branches.forEach((o) => { if (o.state === 'open' && o.id !== plan.branchId) o.behindMain = (o.behindMain || 0) + 1; });
        return recompute();
      },
      queueRunPlan(ids) {
        recompute();
        return queueRunPlanFor(ids);
      },
      queueRun(ids, confirm) {
        recompute();
        const plan = queueRunPlanFor(ids);
        if (!plan.startable) throw new Error(`koşu başlatılamaz: ${plan.reason}`);
        if (!confirm || confirm !== plan.fingerprint) throw new Error('koşu planı değişti veya onaylanmadı; planı yeniden inceleyip onaylayın. Hiçbir şey başlatılmadı');
        const now = () => new Date().toISOString();
        const run = { id: `run-${Date.now()}`, status: 'completed', mode: 'simulation', items: plan.items, current: 0, merged: 0, stopReason: '', message: '', startedAt: now(), updatedAt: now(), timeoutMinutes: 30, events: [{ time: now(), kind: 'info', message: `Koşu başladı: ${plan.items.length} PR sırayla işlenecek` }] };
        const stop = (status, reason) => { run.status = status; run.stopReason = reason; };
        outer: for (let i = 0; i < plan.items.length; i += 1) {
          const it = plan.items[i];
          run.current = i;
          for (let guard = 0; guard < 10; guard += 1) {
            const p = queueStepPlanFor(it.branchId);
            if (p.action === 'blocked') { stop('blocked', `#${it.pr} ${it.branch}: ${p.reason}`); break outer; }
            if (p.action === 'wait') { stop('timeout', `#${it.pr} ${it.branch}: ${p.reason} (çevrimdışı simülasyonda beklenmez)`); break outer; }
            this.queueStep(it.branchId, p.fingerprint);
            if (p.action === 'merge') { run.merged += 1; run.events.push({ time: now(), kind: 'merge', message: `#${it.pr} ${it.branch} birleştirildi` }); continue outer; }
            run.events.push({ time: now(), kind: 'update', message: `#${it.pr} ${it.branch} main'e göre güncellendi` });
          }
          stop('blocked', `#${it.pr} ${it.branch}: adım sınırı aşıldı`);
          break;
        }
        run.current = Math.min(run.current, plan.items.length);
        run.finishedAt = now();
        run.message = run.status === 'completed' ? `Koşu bitti: ${run.merged} PR birleştirildi` : `Koşu durdu (${run.status}): ${run.stopReason}`;
        run.events.push({ time: run.finishedAt, kind: 'stop', message: run.message });
        lastQueueRun = run;
        return run;
      },
      queueRunStatus() {
        return lastQueueRun;
      },
      queueRunStop() {
        throw new Error('bu projede çalışan bir kuyruk koşusu yok');
      },
      queuePlan(ids) {
        recompute();
        return queuePlanBuild(sim.branches, sim.repo.defaultBranch || 'main', ids, sim.protection ? Boolean(sim.protection.strictUpToDate) : false);
      },
      // Saf mantık (git/sim durumu olmadan): Go'daki QueuePlanFromSnapshotData ile birebir; testler bunu doğrudan çağırır.
      // Saf karar mantığı (Go'daki decideQueueStep ile birebir); testler doğrudan çağırır.
      queueStepDecide(plan, input) {
        return queueStepDecide(plan, input);
      },
      queuePlanFromData(branches, ids, baseBranch, strict) {
        return queuePlanBuild(branches, baseBranch || 'main', ids, strict);
      },
      checkUpdates() {
        return {
          currentVersion: sim.repo.appVersion || '0.10.0',
          latestVersion: sim.repo.appVersion || '0.10.0',
          updateAvailable: false,
          updateCommand: 'git pull --ff-only origin main && go build -o gitarc .',
          teamMode: Boolean(sim.repo.teamMode),
          checkError: 'çevrimdışı demo: sürüm denetlenemez', // demo "güncel" demez
        };
      },
      syncSpine() {
        if (sim.repo.behind > 0) {
          seq++;
          const sha = (0xb220000 + seq * 13).toString(16);
          sim.nodes.push({ sha, shortSha: sha, subject: 'chore(index): sync origin/main', author: 'github-actions', relativeTime: 'az önce', spineIndex: sim.nodes.length });
        }
        sim.repo.ahead = 0;
        sim.repo.behind = 0;
        return recompute();
      },
      switchRepo(repoId) {
        const found = (sim.repos || []).find((r) => r.id === repoId);
        if (found) {
          sim.repo.id = found.id;
          sim.repo.name = found.name;
          sim.repo.path = found.path;
          sim.repo.mode = found.mode;
        }
        return recompute();
      },
      addProject(repoPath, customName) {
        const cleanPath = (repoPath || '').trim();
        if (!cleanPath) throw new Error('Depo yolu boş olamaz');
        const name = (customName || '').trim() || cleanPath.split(/[/\\]/).filter(Boolean).pop() || 'Yeni Proje';
        const id = `live-proj-${Date.now()}`;
        const newEntry = { id, name: `${name} (Canlı)`, path: cleanPath, mode: 'live' };
        sim.repos = sim.repos || [];
        sim.repos.push(newEntry);
        sim.repo.id = newEntry.id;
        sim.repo.name = newEntry.name;
        sim.repo.path = newEntry.path;
        sim.repo.mode = 'live';
        return recompute();
      },
      setBaseBranch() {
        // Demo deposunda gerçek dal seçeneği yok; sunucudaki SetBaseBranch ile aynı ret.
        throw new Error('Simülasyon deposunda omurga dalı değiştirilemez');
      },
      removeProject(repoId) {
        if (String(repoId).startsWith('sim-')) {
          throw new Error('Simülasyon deposu silinemez');
        }
        sim.repos = (sim.repos || []).filter((r) => r.id !== repoId);
        if (sim.repo.id === repoId) {
          sim.repo.id = sim.repos[0]?.id || 'sim-sandbox';
          sim.repo.name = sim.repos[0]?.name || 'GitArc Sandbox';
        }
        return recompute();
      },
      discardWorkingTree() {
        sim.repo.dirtyCount = 0;
        (sim.branches || []).forEach((b) => {
          b.uncommittedCount = 0;
        });
        sim.branches = (sim.branches || []).filter((b) => !b.id.startsWith('local-wip'));
        return recompute();
      },
      getDiff(target, type) {
        const rawDiff = `diff --git a/internal/gitengine/engine.go b/internal/gitengine/engine.go
index 1a2b3c4..5d6e7f8 100644
--- a/internal/gitengine/engine.go
+++ b/internal/gitengine/engine.go
@@ -42,6 +42,8 @@ func Example() {
+	// Gitarc Tri-Mode Diff Engine
+	log.Println("Diff ready")
 }
`;
        return {
          target: target || 'working-tree',
          type: type || 'working-tree',
          title: target ? `Diff: ${target}` : 'Çalışma Alanı (Taslak)',
          rawDiff,
          files: [
            { path: 'internal/gitengine/engine.go', oldPath: 'internal/gitengine/engine.go', status: 'M', additions: 2, deletions: 0 },
          ],
          filesChanged: 1,
          totalAdds: 2,
          totalDels: 0,
        };
      },
    };
  })();

  window.LocalFallbackEngine = LocalFallbackEngine;
})();

(() => {
  'use strict';

  // Fallback simulation engine for standalone file:// opening without Go server
  const LocalFallbackEngine = (() => {
    let seq = 12;
    let nextPr = 425;
    let sim = null;

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
            pr: { number: 412, title: 'feat(vault): idle timeout lock', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 24s' } },
            changedFiles: ['go/internal/vault/vault.go'],
          },
          {
            id: 'br-merged-415', name: 'fix/usb-exfat-update', hemisphere: 'upper', state: 'merged',
            startSpineIndex: 3, endSpineIndex: 4, baseSha: '4b70a89', mergeSha: '5e02b11',
            hasRemote: false, remoteRef: 'origin/fix/usb-exfat-update', originGone: true,
            pr: { number: 415, title: 'fix(selfupdate): exFAT update', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 11s' } },
            hasLocal: true, localSha: '5e02b11', changedFiles: ['go/internal/selfupdate/update.go'],
          },
          {
            id: 'br-merged-418', name: 'feat/utility-items-catalog', hemisphere: 'upper', state: 'merged',
            startSpineIndex: 1, endSpineIndex: 6, baseSha: '2f81d12', mergeSha: '7a91c33',
            hasRemote: false, remoteRef: 'origin/feat/utility-items-catalog', originGone: true,
            pr: { number: 418, title: 'feat(manifest): item_var catalog', author: 'ziya', reviewState: 'MERGED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '2m 03s' } },
            hasLocal: true, localSha: '7a91c33', changedFiles: ['go/internal/manifest/items.go'],
          },
          {
            id: 'br-open-421', name: 'feat/terminal-audit-export', hemisphere: 'upper', state: 'open',
            startSpineIndex: 7, endSpineIndex: 8, baseSha: '8e35d02',
            hasRemote: true, remoteRef: 'origin/feat/terminal-audit-export',
            pr: { number: 421, title: 'feat(audit): CSV & JSON streaming export', author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 45s' } },
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
            pr: { number: 424, title: 'chore(lock): upgrade linux-hardening 0.14.0', author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'RUNNING', workflow: 'Verify Signatures', duration: '0m 27s' } },
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
        b.canAmendCommit = Boolean(b.isHead && b.state === 'open' && (b.aheadMain || 0) > 0);
        if (b.state === 'merged' || b.originGone || b.state === 'gone') {
          b.suggestedCmd = `git branch -D ${b.name} && git fetch --prune origin`;
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
        if (b.originGone || b.state === 'gone') {
          diags.push({ id: 'gone-' + b.id, severity: 'info', kind: 'prune_candidate', branchId: b.id, branchName: b.name, title: 'Uzakta Silinmiş Dal (origin: gone)', detail: `${b.name} temizlenebilir` });
        }
      });
      sim.diagnostics = diags;
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
          changedFiles: ['CHANGELOG.md'],
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
          if (b.state === 'merged') {
            if (b.hasLocal || b.originGone) {
              next.push({ ...b, hasLocal: false, originGone: false, isHead: false });
              removed++;
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
      protectBranch() {
        sim.repo.branchProtected = true;
        sim.repo.protectionSummary = 'PR Zorunlu + CI Status Checks + Force-Push Kilitli';
        return recompute();
      },
      checkUpdates() {
        return {
          currentVersion: sim.repo.appVersion || '0.7.0',
          latestVersion: sim.repo.appVersion || '0.7.0',
          updateAvailable: false,
          updateCommand: 'git pull --ff-only origin main && go build -o gitarc .',
          teamMode: Boolean(sim.repo.teamMode),
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
    };
  })();

  window.LocalFallbackEngine = LocalFallbackEngine;
})();

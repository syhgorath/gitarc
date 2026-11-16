(() => {
  'use strict';

  const SVG_NS = 'http://www.w3.org/2000/svg';
  const XHTML_NS = 'http://www.w3.org/1999/xhtml';
  const MAIN_Y = 300;
  const SPINE_START_X = 140;
  const SPINE_STEP_X = 168;
  const LANE_BASE_OFFSET = 108;
  const LANE_STEP_Y = 106;
  const FO_W = 244;
  const FO_H = 122;
  const MERGED_FO_W = 204;
  const MERGED_FO_H = 38;
  const OPEN_CAPSULE_MIN_GAP_X = 264;
  const REPO_STORAGE_KEY = 'gitarc_repo_id';

  const state = {
    snapshot: null,
    selectedBranchId: null,
    selectedNodeSha: null,
    filter: 'all', // 'all' | 'mine' | 'issues' | 'ci_fail' | 'lower' | 'gone'
    pathLens: '',
    showMergedArches: true,
    mergeStrategy: 'merge',
    offlineMode: false,
    lastSig: '',
    zoom: 1,
    panX: 0,
    panY: 0,
    panning: null,
    drag: null,
    localRunOutput: {},
    pendingAutoMerge: {},
    autoMergingInFlight: {},
    draftPrEdits: {},
    studio: null,
    clientLogs: [],
  };

  const el = {
    svg: document.getElementById('rail-svg'),
    viewport: document.getElementById('svg-viewport'),
    repoSelect: document.getElementById('repo-select'),
    modeBadge: document.getElementById('mode-badge'),
    kpiPills: document.getElementById('kpi-pills'),
    radarList: document.getElementById('radar-list'),
    radarCount: document.getElementById('radar-count'),
    detail: document.getElementById('inspector-detail'),
    pruneCount: document.getElementById('prune-count'),
    dragHint: document.getElementById('drag-hint'),
    spineSyncBadge: document.getElementById('spine-sync-badge'),
    engineStatus: document.getElementById('engine-status'),
    zoomReset: document.getElementById('zoom-reset'),
    toggleMerged: document.getElementById('toggle-merged-arches'),
    modal: document.getElementById('create-branch-modal'),
    newBranchForm: document.getElementById('new-branch-form'),
    newBranchName: document.getElementById('new-branch-name'),
    newBranchBase: document.getElementById('new-branch-base'),
    toastStack: document.getElementById('toast-stack'),
    conflictStudioModal: document.getElementById('conflict-studio-modal'),
    studioBranchBadge: document.getElementById('studio-branch-badge'),
    studioFileTabs: document.getElementById('studio-file-tabs'),
    studioGuideBox: document.getElementById('studio-guide-box'),
    studioGuideText: document.getElementById('studio-guide-text'),
    studioStrategyBar: document.getElementById('studio-strategy-bar'),
    studioLineBadge: document.getElementById('studio-line-badge'),
    studioModeBadge: document.getElementById('studio-mode-badge'),
    studioLeftConflict: document.getElementById('studio-left-conflict'),
    studioRightPreview: document.getElementById('studio-right-preview'),
    studioErrorBanner: document.getElementById('studio-error-banner'),
    studioErrorText: document.getElementById('studio-error-text'),
    studioRunVet: document.getElementById('studio-run-vet'),
    studioApplyBtn: document.getElementById('studio-apply'),
    mcpModal: document.getElementById('mcp-connect-modal'),
    mcpConfigSnippet: document.getElementById('mcp-config-snippet'),
    btnLogsModal: document.getElementById('btn-logs-modal'),
    logCount: document.getElementById('log-count'),
    logsModal: document.getElementById('logs-modal'),
    logsList: document.getElementById('logs-list'),
    btnProtectBranch: document.getElementById('btn-protect-branch'),
    btnUpdateCheck: document.getElementById('btn-update-check'),
    lensPathInput: document.getElementById('lens-path-input'),
    perfCacheBadge: document.getElementById('perf-cache-badge'),
    securityUpdateModal: document.getElementById('security-update-modal'),
    secProtBadge: document.getElementById('sec-prot-badge'),
    secProtSummary: document.getElementById('sec-prot-summary'),
    btnEnableProtection: document.getElementById('btn-enable-protection'),
    secTeamChip: document.getElementById('sec-team-chip'),
    secUpdateStatus: document.getElementById('sec-update-status'),
    secUpdateCmd: document.getElementById('sec-update-cmd'),
  };

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
          id: 'sim-keylab',
          name: 'keylab / sshpocket',
          path: '/Users/ziyayesilbahce/Developer/sshpocket',
          defaultBranch: 'main',
          remoteName: 'origin',
          ahead: 2,
          behind: 1,
          dirtyCount: 2,
          mode: 'simulation',
          versionTag: 'v4.0.16',
        },
        repos: [
          { id: 'sim-keylab', name: 'keylab / sshpocket (Offline Simülasyon)', path: '/Users/ziyayesilbahce/Developer/sshpocket', mode: 'simulation' },
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
            startSpineIndex: 4, endSpineIndex: 7, baseSha: '5e02b11',
            hasRemote: true, remoteRef: 'origin/feat/terminal-audit-export',
            pr: { number: 421, title: 'feat(report): export terminal-audit.log', author: 'ziya', reviewState: 'APPROVED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 38s' } },
            hasLocal: true, localSha: 'c18e420', aheadMain: 3, behindMain: 2, uncommittedCount: 0,
            lastCommitMsg: 'report: mask token columns in terminal audit CSV',
            changedFiles: ['CHANGELOG.md', 'components.lock', 'go/internal/report/audit.go'],
            conflict: {
              hasConflict: true,
              files: ['CHANGELOG.md', 'components.lock'],
              isChangelogOrLock: true,
              strategy: 'union-changelog',
              previewUnified: '<<<<<< HEAD (main)\n## [4.0.16] - 2026-11-15\n- utility 0.2.0 lockfile updated\n======\n## [Unreleased] - feat/terminal-audit-export\n- terminal-audit.log CSV & JSON export\n>>>>>> origin/feat/terminal-audit-export',
            },
          },
          {
            id: 'br-open-422', name: 'fix/shellgold-diff-parser', hemisphere: 'upper', state: 'open',
            startSpineIndex: 5, endSpineIndex: 8, baseSha: '6d44f08',
            hasRemote: true, remoteRef: 'origin/fix/shellgold-diff-parser',
            pr: {
              number: 422, title: 'fix(shellgold): parse 3-column diff', author: 'ziya', reviewState: 'CHANGES_REQUESTED',
              ci: { state: 'FAILURE', workflow: 'Go Vet & ShellGold CI', duration: '0m 49s', failedStep: 'Run go test ./internal/shellgold', rawLog: ciLog, extracts: extractCI(ciLog) },
            },
            hasLocal: true, localSha: 'd42b991', isHead: true, aheadMain: 2, behindMain: 1, remoteAhead: 1, uncommittedCount: 2,
            lastCommitMsg: 'shellgold: guard empty column split in compare.go',
            changedFiles: ['CHANGELOG.md', 'go/internal/shellgold/compare.go', 'go/internal/shellgold/compare_test.go'],
          },
          {
            id: 'br-open-424', name: 'chore/hardening-0.14-lock', hemisphere: 'upper', state: 'open',
            startSpineIndex: 7, endSpineIndex: 9, baseSha: '8e35d02',
            hasRemote: true, remoteRef: 'origin/chore/hardening-0.14-lock',
            pr: { number: 424, title: 'chore(lock): upgrade linux-hardening 0.14.0', author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'RUNNING', workflow: 'Verify Signatures', duration: '0m 27s' } },
            hasLocal: true, localSha: 'e99a014', aheadMain: 1, behindMain: 0, uncommittedCount: 0,
            lastCommitMsg: 'components.lock: bump linux-hardening sha256',
            changedFiles: ['components.lock', 'VERSION'],
          },
          {
            id: 'br-local-wip', name: 'wip/keychain-probe', hemisphere: 'lower', state: 'open',
            startSpineIndex: 6, endSpineIndex: 8, baseSha: '7a91c33',
            hasRemote: false, hasLocal: true, localSha: 'f03c812', aheadMain: 3, behindMain: 1, uncommittedCount: 4,
            lastCommitMsg: 'wip(doctor): probe macOS Keychain ACL before ssh-add',
            changedFiles: ['go/internal/doctor/keychain.go', 'go/main.go'],
          },
          {
            id: 'br-local-exp', name: 'exp/parabolic-export', hemisphere: 'lower', state: 'open',
            startSpineIndex: 7, endSpineIndex: 9, baseSha: '8e35d02',
            hasRemote: false, hasLocal: true, localSha: 'a17c504', aheadMain: 2, behindMain: 0, uncommittedCount: 1,
            lastCommitMsg: 'exp(report): export SVG fleet dependency arc',
            changedFiles: ['go/web/js/reports.js'],
          },
        ],
      };
    }

    function recompute() {
      const branches = sim.branches;
      branches.forEach((b) => { b.crossPr = []; });
      for (let i = 0; i < branches.length; i++) {
        if (branches[i].state !== 'open' || !branches[i].changedFiles?.length) continue;
        const setA = new Set(branches[i].changedFiles);
        for (let j = i + 1; j < branches.length; j++) {
          if (branches[j].state !== 'open' || !branches[j].changedFiles?.length) continue;
          const shared = branches[j].changedFiles.filter((f) => setA.has(f));
          if (!shared.length) continue;
          branches[i].crossPr.push({ otherBranchId: branches[j].id, otherBranch: branches[j].name, otherPrNumber: branches[j].pr?.number || 0, sharedFiles: shared });
          branches[j].crossPr.push({ otherBranchId: branches[i].id, otherBranch: branches[i].name, otherPrNumber: branches[i].pr?.number || 0, sharedFiles: shared });
        }
      }
      ['upper', 'lower'].forEach((hemi) => {
        const items = branches
          .map((b, idx) => ({ idx, s: b.startSpineIndex, e: b.state === 'merged' ? Math.max(b.startSpineIndex + 1, b.endSpineIndex) : Math.max(b.startSpineIndex + 1, b.endSpineIndex) + 1 }))
          .filter((it) => branches[it.idx].hemisphere === hemi)
          .sort((a, b) => (a.e - a.s) - (b.e - b.s) || a.s - b.s);
        const lanes = [];
        items.forEach((it) => {
          let placed = false;
          for (let li = 0; li < lanes.length; li++) {
            const conflict = lanes[li].some((r) => it.s < r.e && it.e > r.s);
            if (!conflict) {
              lanes[li].push({ s: it.s, e: it.e });
              branches[it.idx].lane = li + 1;
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
      commitWorkingTree(branchName, commitMsg, prTitle = '', prBody = '', pushAndPr = false) {
        const cleanBranch = (branchName || '').trim() || 'feat/working-tree';
        const cleanCommit = (commitMsg || '').trim();
        if (!cleanCommit) throw new Error('Commit mesajı boş olamaz');
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
        const recommended = b.conflict.isChangelogOrLock ? 'union' : 'ours';
        const strategies = b.conflict.isChangelogOrLock ? [
          { strategy: 'union', badge: '★ ÖNERİLEN · Veri Kaybı Yok', tone: 'ok' },
          { strategy: 'ours', badge: 'Daldaki Eklemeyi Atlar', tone: 'warn' },
          { strategy: 'theirs', badge: 'Main Eklemeyi Atlar', tone: 'warn' },
          { strategy: 'custom', badge: 'Tam Kontrol', tone: 'accent' },
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
          recommendationReason: b.conflict.isChangelogOrLock
            ? "Çakışan dosya değişiklik günlüğü / kilit dosyasıdır. '1. İkisini Alt Alta Koru' seçeneği her iki tarafı da kayıpsız birleştirir."
            : "Çakışan dosya kaynak kodudur. 'İkisini Alt Alta Koru' derleme hatası üretebilir; '2. Maindeki Güncel Kodu Kullan' önerilir.",
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
    };
  })();

  async function apiCall(path, method = 'GET', body = null) {
    if (state.offlineMode || window.location.protocol === 'file:') {
      throw new Error('offline');
    }
    const opts = {
      method,
      headers: {
        'X-Requested-With': 'gitarc',
      },
      credentials: 'same-origin',
    };
    if (body !== null) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const resp = await fetch(path, opts);
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return data;
  }

  function getMergedLogs() {
    const serverLogs = state.snapshot?.logs || [];
    const seenMsgs = new Set(serverLogs.map((l) => `${l.level}:${l.message}`));
    const extra = state.clientLogs.filter((l) => !seenMsgs.has(`${l.level}:${l.message}`));
    return [...extra, ...serverLogs].slice(0, 60);
  }

  function renderLogBadge() {
    const logs = getMergedLogs();
    const errCount = logs.filter((l) => l.level === 'error').length;
    el.logCount.textContent = String(errCount > 0 ? errCount : logs.length);
    el.btnLogsModal.classList.toggle('danger', errCount > 0);
    if (!el.logsModal.hidden) {
      renderLogsModal();
    }
  }

  function recordClientLog(level, action, message, detail = '') {
    const now = new Date();
    const ts = now.toTimeString().slice(0, 8);
    state.clientLogs.unshift({
      id: `cli-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      timestamp: ts,
      level,
      action,
      message: String(message || ''),
      detail: String(detail || ''),
    });
    if (state.clientLogs.length > 40) {
      state.clientLogs.length = 40;
    }
    renderLogBadge();
  }

  function showToast(msg, tone = 'info') {
    if (tone === 'danger') {
      recordClientLog('error', 'Arayüz / API Hatası', msg);
    }
    const t = document.createElement('div');
    t.className = `toast ${tone}`;
    t.title = 'Metni panoya kopyalamak için tıklayın';
    t.textContent = msg;
    t.addEventListener('click', () => {
      navigator.clipboard?.writeText(msg).catch(() => {});
      t.classList.add('fade-out');
      setTimeout(() => t.remove(), 180);
    });
    el.toastStack.appendChild(t);
    const timeoutMs = tone === 'danger' ? 7500 : 3200;
    setTimeout(() => {
      t.classList.add('fade-out');
      setTimeout(() => t.remove(), 260);
    }, timeoutMs);
  }

  function svgEl(tag, attrs = {}, text = null) {
    const node = document.createElementNS(SVG_NS, tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (v !== undefined && v !== null) {
        node.setAttribute(k, String(v));
      }
    }
    if (text !== null) {
      node.textContent = text;
    }
    return node;
  }

  function esc(str) {
    return String(str ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function truncate(str, max) {
    if (!str) return '';
    return str.length > max ? str.slice(0, max - 1) + '…' : str;
  }

  function snapshotSignature(snap) {
    if (!snap) return '';
    const r = snap.repo || {};
    const lastNode = snap.nodes?.[snap.nodes.length - 1]?.shortSha || '';
    const bSig = (snap.branches || []).map((b) => [
      b.id, b.state, b.hemisphere, b.isHead ? 1 : 0, b.hasLocal ? 1 : 0, b.aheadMain, b.behindMain,
      b.uncommittedCount, b.lastCommitMsg, b.draftPrTitle || '', b.draftPrBody || '',
      (b.changedFiles || []).length, b.originGone ? 1 : 0,
      b.pr?.title || '', b.pr?.body || '', b.pr?.ci?.state || '', b.pr?.ci?.duration || '',
      b.conflict?.hasConflict ? 1 : 0,
    ].join('|')).join(';');
    const logSig = `${snap.logs?.length || 0}:${snap.logs?.[0]?.id || ''}`;
    return `${r.id}:${r.ahead}:${r.behind}:${r.dirtyCount}:${snap.nodes?.length || 0}:${lastNode}:${bSig}:${logSig}`;
  }

  async function bootstrap() {
    try {
      let snap = await apiCall('/api/snapshot');
      const savedId = localStorage.getItem(REPO_STORAGE_KEY);
      if (savedId && savedId !== snap.repo.id && (snap.repos || []).some((r) => r.id === savedId)) {
        try {
          snap = await apiCall('/api/repo/switch', 'POST', { repoId: savedId });
        } catch {
          // Keep initial snapshot if switch fails
        }
      }
      localStorage.setItem(REPO_STORAGE_KEY, snap.repo.id);
      applySnapshot(snap);
    } catch {
      state.offlineMode = true;
      const snap = LocalFallbackEngine.snapshot();
      applySnapshot(snap);
      showToast('Sunucusuz tarayıcı simülasyonu aktif (Offline Mode)', 'info');
    }
  }

  // 4-second silent live polling when user is idle (not dragging or typing)
  setInterval(async () => {
    if (state.offlineMode || state.drag || state.panning || !el.modal.hidden || !el.conflictStudioModal.hidden || !el.logsModal.hidden || !el.mcpModal.hidden || !el.securityUpdateModal.hidden) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    try {
      const nextSnap = await apiCall('/api/snapshot');
      const sig = snapshotSignature(nextSnap);
      if (sig && sig !== state.lastSig) {
        applySnapshot(nextSnap);
      }
    } catch {
      // Ignore transient polling errors
    }
  }, 4000);

  function applySnapshot(snap) {
    state.snapshot = snap;
    state.lastSig = snapshotSignature(snap);

    const queuedIds = Object.keys(state.pendingAutoMerge || {});
    queuedIds.forEach((branchId) => {
      const queued = state.pendingAutoMerge[branchId];
      if (!queued) return;
      const target = (snap.branches || []).find((x) => x.id === branchId || (queued.branchName && x.name === queued.branchName));
      if (!target || target.state === 'merged') {
        delete state.pendingAutoMerge[branchId];
        return;
      }
      const ci = target.pr?.ci?.state;
      if (ci === 'FAILURE') {
        delete state.pendingAutoMerge[branchId];
        showToast(`❌ ${target.name} dalında CI başarısız oldu; otomatik birleştirme iptal edildi`, 'danger');
        return;
      }
      if (target.conflict?.hasConflict && !target.conflict.resolved) {
        delete state.pendingAutoMerge[branchId];
        showToast(`⚠ ${target.name} dalında çakışma tespit edildi; otomatik birleştirme durduruldu`, 'danger');
        return;
      }
      if (ci === 'SUCCESS' && !state.autoMergingInFlight[branchId]) {
        state.autoMergingInFlight[branchId] = true;
        delete state.pendingAutoMerge[branchId];
        executeMergeNow(target.id, queued.strategy || 'merge', true).finally(() => {
          delete state.autoMergingInFlight[branchId];
        });
      }
    });

    if (!state.selectedBranchId && snap.branches?.length) {
      const priority = snap.branches.find((b) => b.pr?.ci?.state === 'FAILURE')
        || snap.branches.find((b) => b.conflict?.hasConflict)
        || snap.branches.find((b) => b.hemisphere === 'lower')
        || snap.branches.find((b) => b.state === 'open')
        || snap.branches[0];
      state.selectedBranchId = priority.id;
    } else if (state.selectedBranchId && !snap.branches.some((b) => b.id === state.selectedBranchId)) {
      state.selectedBranchId = snap.branches[0]?.id || null;
    }

    renderAll();
  }

  function renderAll() {
    renderTopbar();
    renderKPIs();
    renderRadar();
    renderSVG();
    renderInspector();
    populateBaseCommitOptions();
  }

  function renderTopbar() {
    const snap = state.snapshot;
    if (!snap) return;

    const liveLabel = state.offlineMode ? 'Offline Simülasyon' : (snap.repo.mode === 'live' ? 'Canlı Git · Aktif' : 'Simülasyon · Etkileşimli');
    el.modeBadge.textContent = `${snap.repo.name} · ${liveLabel}`;
    el.engineStatus.textContent = snap.repo.mode === 'live' ? '● Canlı Repo Bağlı' : '● Simülasyon Hazır';

    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = r.name;
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });

    const cleanableCount = (snap.branches || []).filter((b) => b.originGone || b.state === 'gone' || (b.state === 'merged' && b.hasLocal)).length;
    el.pruneCount.textContent = String(cleanableCount);
    document.getElementById('btn-prune-gone').disabled = cleanableCount === 0 && snap.repo.mode !== 'live';
    document.getElementById('btn-new-branch').disabled = false;
    document.getElementById('btn-sync-spine').disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;
    document.getElementById('btn-reset-sim').hidden = snap.repo.mode === 'live';
    renderLogBadge();

    const isProtected = Boolean(snap.repo.branchProtected);
    el.btnProtectBranch.textContent = isProtected ? '🛡️ main Korumalı' : '🔓 Korumasız';
    el.btnProtectBranch.className = `btn ${isProtected ? 'ok' : 'warn'}`;

    const verTxt = `v${snap.repo.appVersion || '0.7.0'}`;
    const teamTag = snap.repo.teamMode ? ' · TEAM' : '';
    const updTag = snap.repo.updateAvailable ? ' ⬆ Güncelle' : '';
    el.btnUpdateCheck.textContent = verTxt + teamTag + updTag;
    el.btnUpdateCheck.className = `btn mono ${snap.repo.updateAvailable ? 'warn' : 'ghost'}`;

    el.perfCacheBadge.textContent = `⚡ ${snap.repo.cacheHits || 0} SHA hit`;

    if (!el.securityUpdateModal.hidden) {
      renderSecurityUpdateModal();
    }

    const ahead = snap.repo.ahead || 0;
    const behind = snap.repo.behind || 0;
    const remoteName = snap.repo.remoteName || 'origin';
    const defBranch = snap.repo.defaultBranch || 'main';
    if (ahead === 0 && behind === 0) {
      el.spineSyncBadge.textContent = `${defBranch} ↔ ${remoteName}/${defBranch} (Synced)`;
      el.spineSyncBadge.classList.remove('skewed');
    } else {
      el.spineSyncBadge.textContent = `${defBranch} ↔ ${remoteName}/${defBranch}  ↑${ahead} · ↓${behind}`;
      el.spineSyncBadge.classList.add('skewed');
    }
  }

  function isMyBranch(b) {
    const me = (state.snapshot?.repo?.currentUser || '').trim().toLowerCase();
    if (!me) return Boolean(b.isHead || b.hasLocal);
    const bAuth = (b.author || '').trim().toLowerCase();
    const prAuth = (b.pr?.author || '').trim().toLowerCase();
    return bAuth === me || prAuth === me || Boolean(b.isHead);
  }

  function renderKPIs() {
    const branches = state.snapshot?.branches || [];
    const upperCount = branches.filter((b) => b.hemisphere === 'upper' && b.state !== 'merged').length;
    const lowerCount = branches.filter((b) => (b.hemisphere === 'lower' && b.state !== 'merged') || (b.state === 'merged' && (b.hasLocal || b.originGone))).length;
    const mineCount = branches.filter((b) => b.state !== 'merged' && isMyBranch(b)).length;
    const conflictCount = branches.filter((b) => (b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length > 0)).length;
    const failCount = branches.filter((b) => b.pr?.ci?.state === 'FAILURE').length;
    const goneCount = branches.filter((b) => b.originGone || b.state === 'gone' || b.state === 'merged').length;

    const defs = [
      { key: 'all', label: 'Tümü', val: branches.length, tone: '' },
      { key: 'mine', label: '👤 Benim Dallarım (@me)', val: mineCount, tone: mineCount ? 'accent' : '' },
      { key: 'lower', label: 'Alt Ray (Local)', val: lowerCount, tone: lowerCount ? 'purple' : '' },
      { key: 'issues', label: 'Çakışma / Cross-PR', val: conflictCount, tone: conflictCount ? 'warn' : '' },
      { key: 'ci_fail', label: 'CI Hatası', val: failCount, tone: failCount ? 'danger' : '' },
      { key: 'gone', label: 'Birleşmiş / Gone', val: goneCount, tone: goneCount ? 'ok' : '' },
    ];

    el.kpiPills.innerHTML = '';
    defs.forEach((d) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `kpi-pill ${d.tone} ${state.filter === d.key ? 'active' : ''}`;
      btn.innerHTML = `<span>${esc(d.label)}</span><strong>${d.val}</strong>`;
      btn.addEventListener('click', () => {
        state.filter = d.key;
        renderKPIs();
        renderSVG();
      });
      el.kpiPills.appendChild(btn);
    });

    const prBadge = document.createElement('span');
    prBadge.className = 'kpi-pill';
    prBadge.innerHTML = `<span>Üst Ray (PR)</span><strong>${upperCount}</strong>`;
    el.kpiPills.appendChild(prBadge);
  }

  function matchesFilter(b) {
    const q = (state.pathLens || '').trim().toLowerCase();
    if (q) {
      const nameHit = (b.name || '').toLowerCase().includes(q);
      const fileHit = (b.changedFiles || []).some((f) => f.toLowerCase().includes(q));
      if (!nameHit && !fileHit) return false;
    }
    switch (state.filter) {
      case 'mine':
        return isMyBranch(b);
      case 'lower':
        return b.hemisphere === 'lower' || (b.state === 'merged' && Boolean(b.hasLocal || b.originGone));
      case 'issues':
        return Boolean((b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length));
      case 'ci_fail':
        return b.pr?.ci?.state === 'FAILURE';
      case 'gone':
        return Boolean(b.originGone || b.state === 'gone' || b.state === 'merged');
      default:
        return true;
    }
  }

  function renderRadar() {
    const diags = state.snapshot?.diagnostics || [];
    el.radarCount.textContent = `${diags.length} uyarı`;
    el.radarList.innerHTML = '';

    if (diags.length === 0) {
      const clean = document.createElement('div');
      clean.className = 'empty-detail';
      clean.textContent = 'Radar temiz — çakışma, Cross-PR veya CI hatası yok.';
      el.radarList.appendChild(clean);
      return;
    }

    diags.forEach((d) => {
      const item = document.createElement('button');
      item.type = 'button';
      item.className = `radar-item ${d.severity}`;
      item.innerHTML = `<span class="radar-title">${esc(d.title)}</span><span class="radar-sub">${esc(d.branchName)} · ${esc(d.detail)}</span>`;
      item.addEventListener('click', () => {
        selectBranch(d.branchId);
      });
      el.radarList.appendChild(item);
    });
  }

  function xOfSpine(idx) {
    return SPINE_START_X + idx * SPINE_STEP_X;
  }

  function yOfLane(hemisphere, lane, isMerged = false) {
    const l = Math.max(1, lane || 1);
    const offset = isMerged ? (84 + (l - 1) * 56) : (LANE_BASE_OFFSET + (l - 1) * LANE_STEP_Y);
    return hemisphere === 'lower' ? MAIN_Y + offset : MAIN_Y - offset;
  }

  function layoutOpenApexPositions(branches) {
    const map = new Map();
    const lastMergedEndX = branches
      .filter((b) => b.state === 'merged')
      .reduce((acc, b) => Math.max(acc, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex))), 0);

    ['upper', 'lower'].forEach((hemi) => {
      const openList = branches
        .filter((b) => b.state !== 'merged' && b.hemisphere === hemi)
        .sort((a, b) => a.startSpineIndex - b.startSpineIndex || a.endSpineIndex - b.endSpineIndex || a.name.localeCompare(b.name));

      let prevX = -Infinity;
      openList.forEach((b) => {
        const startX = xOfSpine(b.startSpineIndex);
        const baseX = Math.max(startX + 150, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex)) - 10);
        // Keep open upper capsules clear of merged arch crests when nearby
        const minClearX = hemi === 'upper' && lastMergedEndX > 0 ? Math.max(baseX, lastMergedEndX + 20) : baseX;
        const apexX = prevX === -Infinity ? minClearX : Math.max(minClearX, prevX + OPEN_CAPSULE_MIN_GAP_X);
        const apexY = hemi === 'lower' ? MAIN_Y + LANE_BASE_OFFSET : MAIN_Y - LANE_BASE_OFFSET;
        prevX = apexX;
        map.set(b.id, { apexX, apexY });
      });
    });
    return map;
  }

  function computeBranchGeometry(b, tipXOverride = null, tipYOverride = null, apexLayout = null) {
    const startX = xOfSpine(b.startSpineIndex);
    const isMerged = b.state === 'merged';
    const laneY = yOfLane(b.hemisphere, b.lane, isMerged);

    if (isMerged && tipXOverride === null) {
      const endX = xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex));
      const apexX = (startX + endX) / 2;
      const apexY = laneY;
      const cp1X = startX + (apexX - startX) * 0.55;
      const cp2X = apexX + (endX - apexX) * 0.45;
      const openD = `M ${startX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
      const closeD = `M ${apexX} ${apexY} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y}`;
      const areaD = `${openD} C ${cp2X} ${apexY}, ${cp2X} ${MAIN_Y}, ${endX} ${MAIN_Y} Z`;
      return { startX, endX, apexX, apexY, openD, closeD, areaD };
    }

    const laid = apexLayout?.get(b.id);
    const defaultApexX = laid ? laid.apexX : Math.max(startX + 150, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex)) - 10);
    const defaultApexY = laid ? laid.apexY : laneY;
    const apexX = tipXOverride !== null ? tipXOverride : defaultApexX;
    const apexY = tipYOverride !== null ? tipYOverride : defaultApexY;
    const cp1X = startX + (apexX - startX) * 0.55;
    const openD = `M ${startX} ${MAIN_Y} C ${cp1X} ${MAIN_Y}, ${cp1X} ${apexY}, ${apexX} ${apexY}`;
    return { startX, endX: null, apexX, apexY, openD, closeD: null, areaD: null };
  }

  function branchToneClass(b) {
    if (b.pr?.ci?.state === 'FAILURE') return 'ci-fail';
    if (b.conflict?.hasConflict && !b.conflict.resolved) return 'conflict';
    if (b.pr?.ci?.state === 'RUNNING') return 'ci-running';
    if (b.state === 'merged') return 'merged';
    if (b.originGone || b.state === 'gone') return 'gone';
    return b.hemisphere === 'lower' ? 'lower-open' : 'upper-open';
  }

  function branchStrokeColor(b) {
    if (b.pr?.ci?.state === 'FAILURE') return '#f85149';
    if (b.conflict?.hasConflict && !b.conflict.resolved) return '#f97316';
    if (b.pr?.ci?.state === 'RUNNING') return '#d29922';
    if (b.state === 'merged') return '#3fb950';
    if (b.originGone || b.state === 'gone') return '#d29922';
    return b.hemisphere === 'lower' ? '#a371f7' : '#58a6ff';
  }

  function renderSVG() {
    const snap = state.snapshot;
    if (!snap) return;

    const nodes = snap.nodes || [];
    const branches = snap.branches || [];
    const visibleBranches = branches.filter((b) => state.showMergedArches || b.state !== 'merged');
    const apexLayout = layoutOpenApexPositions(visibleBranches);
    state.apexLayout = apexLayout;

    const maxSpineIdx = Math.max(nodes.length - 1, ...visibleBranches.map((b) => b.endSpineIndex || 0), 6);
    const maxMergedUpperLane = Math.max(1, ...visibleBranches.filter((b) => b.hemisphere === 'upper' && b.state === 'merged').map((b) => b.lane || 1));
    const maxApexX = Math.max(0, ...Array.from(apexLayout.values()).map((v) => v.apexX));

    const minY = Math.min(0, MAIN_Y - LANE_BASE_OFFSET - (maxMergedUpperLane - 1) * LANE_STEP_Y - 130);
    const maxY = Math.max(600, MAIN_Y + LANE_BASE_OFFSET + 155);
    const sceneW = Math.max(1280, SPINE_START_X + (maxSpineIdx + 2) * SPINE_STEP_X + 140, maxApexX + 200);
    const sceneH = maxY - minY;

    const viewW = sceneW / state.zoom;
    const viewH = sceneH / state.zoom;
    el.svg.setAttribute('viewBox', `${state.panX} ${minY + state.panY} ${viewW} ${viewH}`);
    el.zoomReset.textContent = `${Math.round(state.zoom * 100)}%`;

    el.svg.innerHTML = '';

    // 1. SVG <defs> (glow filters + main gradient from parabolic.html)
    const defs = svgEl('defs');
    defs.innerHTML = `
      <filter id="glow-green" x="-20%" y="-20%" width="140%" height="140%">
        <feGaussianBlur stdDeviation="4" result="blur"/>
        <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
      </filter>
      <filter id="glow-red" x="-30%" y="-30%" width="160%" height="160%">
        <feGaussianBlur stdDeviation="4.5" result="blur"/>
        <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
      </filter>
      <linearGradient id="main-rail-grad" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" stop-color="#238636" stop-opacity="0.45"/>
        <stop offset="20%" stop-color="#3fb950" stop-opacity="0.95"/>
        <stop offset="85%" stop-color="#3fb950" stop-opacity="1"/>
        <stop offset="100%" stop-color="#58a6ff" stop-opacity="0.45"/>
      </linearGradient>
    `;
    el.svg.appendChild(defs);

    // 2. Background horizontal dashed guide rails (Upper Remote + Lower Local)
    el.svg.appendChild(svgEl('line', { x1: 40, y1: MAIN_Y - LANE_BASE_OFFSET, x2: sceneW - 40, y2: MAIN_Y - LANE_BASE_OFFSET, class: 'svg-guide-line' }));
    el.svg.appendChild(svgEl('line', { x1: 40, y1: MAIN_Y + LANE_BASE_OFFSET, x2: sceneW - 40, y2: MAIN_Y + LANE_BASE_OFFSET, class: 'svg-guide-line' }));

    // 3. Parabolic Curves Layer (Merged arches behind, open half-parabolas in front)
    const arcLayer = svgEl('g', { id: 'arc-layer' });
    const capsuleLayer = svgEl('g', { id: 'capsule-layer' });

    const ordered = [...visibleBranches].sort((a, b) => (a.state === 'merged' ? 0 : 1) - (b.state === 'merged' ? 0 : 1));
    ordered.forEach((b) => {
      const geom = computeBranchGeometry(b, null, null, apexLayout);
      const visible = matchesFilter(b);
      const isSelected = state.selectedBranchId === b.id;
      const tone = branchToneClass(b);
      const strokeColor = branchStrokeColor(b);

      if (b.state === 'merged' && geom.areaD) {
        arcLayer.appendChild(svgEl('path', {
          d: geom.areaD,
          fill: strokeColor,
          class: `svg-arc-area ${isSelected ? 'selected' : ''} ${visible ? '' : 'dimmed'}`,
        }));
      }

      const openCls = ['svg-arc-path', tone];
      if (isSelected) openCls.push('selected');
      if (!visible) openCls.push('dimmed');
      const openAttrs = { d: geom.openD, class: openCls.join(' ') };
      if (b.pr?.ci?.state === 'FAILURE') openAttrs.filter = 'url(#glow-red)';
      const openPath = svgEl('path', openAttrs);
      openPath.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      arcLayer.appendChild(openPath);

      if (b.state === 'merged' && geom.closeD) {
        const closePath = svgEl('path', { d: geom.closeD, class: openCls.join(' ') });
        closePath.addEventListener('click', (ev) => {
          ev.stopPropagation();
          selectBranch(b.id);
        });
        arcLayer.appendChild(closePath);

        if (b.hasLocal || b.originGone) {
          const mergeX = geom.endX || xOfSpine(b.endSpineIndex || b.startSpineIndex || 0);
          const remY = MAIN_Y + 56 + (((b.endSpineIndex || 0) % 2) * 30);
          const remX = mergeX + 18;
          const remPath = svgEl('path', {
            d: `M ${mergeX} ${MAIN_Y} C ${mergeX} ${MAIN_Y + 30}, ${remX - 16} ${remY}, ${remX} ${remY}`,
            class: `svg-local-remnant-path ${isSelected ? 'selected' : ''} ${visible ? '' : 'dimmed'}`,
          });
          remPath.addEventListener('click', (ev) => {
            ev.stopPropagation();
            selectBranch(b.id);
          });
          arcLayer.appendChild(remPath);

          const remFo = svgEl('foreignObject', {
            x: remX,
            y: remY - 12,
            width: 150,
            height: 26,
            class: `fo-capsule ${visible ? '' : 'dimmed'}`,
            'data-remnant-branch-id': b.id,
          });
          const remPill = document.createElementNS(XHTML_NS, 'div');
          remPill.className = `cap-local-remnant ${isSelected ? 'selected' : ''}`;
          remPill.innerHTML = `
            <span class="remnant-txt" title="${esc(b.name)}">💻 ${b.isHead ? 'HEAD·' : ''}${esc(truncate(b.name, 13))}</span>
            <button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>
          `;
          remPill.querySelector('button[data-act="delete"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
          remPill.querySelector('button[data-act="delete"]')?.addEventListener('click', (ev) => {
            ev.stopPropagation();
            handleDeleteBranch(b.id);
          });
          remPill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
          remPill.addEventListener('click', (ev) => {
            ev.stopPropagation();
            selectBranch(b.id);
          });
          remFo.appendChild(remPill);
          capsuleLayer.appendChild(remFo);
        }
      } else {
        // Open half-parabola dashed tail extending past the apex
        arcLayer.appendChild(svgEl('line', {
          x1: geom.apexX,
          y1: geom.apexY,
          x2: geom.apexX + 95,
          y2: geom.apexY,
          class: `svg-open-tail ${tone} ${visible ? '' : 'dimmed'}`,
        }));
      }

      // Apex commit dot
      const dot = svgEl('circle', {
        cx: geom.apexX,
        cy: geom.apexY,
        r: isSelected ? 6 : 4.5,
        class: `svg-apex-dot ${tone}`,
      });
      dot.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      arcLayer.appendChild(dot);

      // Apex Capsule Card via <foreignObject>
      const fo = buildCapsuleForeignObject(b, geom, isSelected, visible, tone);
      capsuleLayer.appendChild(fo);
    });

    el.svg.appendChild(arcLayer);

    // 4. Central Main Spine Drop Zone + Glowing Gradient Line
    const spineEndX = Math.max(xOfSpine(maxSpineIdx) + 140, 1140);
    el.svg.appendChild(svgEl('rect', {
      id: 'svg-main-drop-rect',
      x: 50,
      y: MAIN_Y - 24,
      width: spineEndX - 40,
      height: 48,
      rx: 24,
      class: 'svg-main-drop-rect',
    }));

    el.svg.appendChild(svgEl('line', {
      x1: 72,
      y1: MAIN_Y,
      x2: spineEndX - 15,
      y2: MAIN_Y,
      stroke: 'url(#main-rail-grad)',
      filter: 'url(#glow-green)',
      class: 'svg-spine-line',
    }));

    const defBranch = snap.repo.defaultBranch || 'main';
    const mainTagW = Math.max(58, defBranch.length * 8 + 22);
    el.svg.appendChild(svgEl('rect', { x: 22, y: MAIN_Y - 13, width: mainTagW, height: 26, rx: 8, class: 'svg-main-tag-bg' }));
    el.svg.appendChild(svgEl('text', { x: 22 + mainTagW / 2, y: MAIN_Y + 4, class: 'svg-main-tag-txt' }, defBranch));

    // 5. Commit Nodes on the Central Spine
    const nodeLayer = svgEl('g', { id: 'node-layer' });
    nodes.forEach((n, idx) => {
      const cx = xOfSpine(n.spineIndex);
      const isLatest = idx === nodes.length - 1;
      const cls = ['svg-node-circle'];
      if (n.isMerge) cls.push('merge-node');
      if (n.isHead || isLatest) cls.push('head-node');

      const circle = svgEl('circle', {
        cx,
        cy: MAIN_Y,
        r: isLatest ? 8.5 : (n.isMerge ? 7 : 5.5),
        class: cls.join(' '),
      });
      circle.addEventListener('click', (ev) => {
        ev.stopPropagation();
        state.selectedNodeSha = n.shortSha;
        state.selectedBranchId = null;
        renderInspector();
      });
      nodeLayer.appendChild(circle);

      nodeLayer.appendChild(svgEl('text', {
        x: cx,
        y: MAIN_Y - 14,
        class: `svg-node-sha ${isLatest ? 'latest' : ''}`,
      }, n.shortSha));

      nodeLayer.appendChild(svgEl('text', {
        x: cx,
        y: MAIN_Y + 23,
        class: 'svg-node-label',
      }, truncate(n.subject, 19)));

      if (n.tag) {
        const tw = n.tag.length * 6.8 + 14;
        nodeLayer.appendChild(svgEl('rect', { x: cx - tw / 2, y: MAIN_Y - 40, width: tw, height: 17, rx: 5, class: 'svg-node-tag-bg' }));
        nodeLayer.appendChild(svgEl('text', { x: cx, y: MAIN_Y - 28, class: 'svg-node-tag-txt' }, n.tag));
      }
    });

    el.svg.appendChild(nodeLayer);
    el.svg.appendChild(capsuleLayer);
    el.svg.appendChild(svgEl('path', { id: 'drag-preview-path', class: 'svg-drag-preview', d: '' }));
  }

  function capsulePositionForApex(b, apexX, apexY) {
    if (b.state === 'merged') {
      return {
        x: apexX - MERGED_FO_W / 2,
        y: apexY - MERGED_FO_H - 6,
      };
    }
    const x = apexX - FO_W / 2;
    const y = b.hemisphere === 'lower' ? apexY + 14 : apexY - FO_H + 12;
    return { x, y };
  }

  function buildCapsuleForeignObject(b, geom, isSelected, visible, tone) {
    const isMerged = b.state === 'merged';
    const pos = capsulePositionForApex(b, geom.apexX, geom.apexY);
    const fo = svgEl('foreignObject', {
      x: pos.x,
      y: pos.y,
      width: isMerged ? MERGED_FO_W : FO_W,
      height: isMerged ? MERGED_FO_H : FO_H,
      class: `fo-capsule ${visible ? '' : 'dimmed'}`,
      'data-branch-id': b.id,
    });

    if (isMerged) {
      const pill = document.createElementNS(XHTML_NS, 'div');
      const hasLocalLeftover = Boolean(b.originGone || b.hasLocal);
      pill.className = `cap-merged-pill ${isSelected ? 'selected' : ''} ${hasLocalLeftover ? 'gone' : ''}`;
      const prTxt = b.pr ? `#${b.pr.number} ` : '';
      const localTagHtml = hasLocalLeftover
        ? `<span class="cap-merged-local-tag" title="Yerelde dal kopyası duruyor">${b.isHead ? 'HEAD·Local' : '💻 Local'}</span>`
        : '';
      const delBtnHtml = hasLocalLeftover
        ? '<button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>'
        : '';
      pill.innerHTML = `
        <span class="cap-merged-txt">✓ ${esc(prTxt + truncate(b.name, hasLocalLeftover ? 11 : 19))}</span>
        ${localTagHtml}
        ${delBtnHtml}
      `;
      pill.querySelector('button[data-act="delete"]')?.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      pill.querySelector('button[data-act="delete"]')?.addEventListener('click', (ev) => {
        ev.stopPropagation();
        handleDeleteBranch(b.id);
      });
      pill.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      pill.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      fo.appendChild(pill);
      return fo;
    }

    const isLower = b.hemisphere === 'lower';
    const isFailed = b.pr?.ci?.state === 'FAILURE';
    const isRunning = b.pr?.ci?.state === 'RUNNING';
    const isQueued = Boolean(state.pendingAutoMerge[b.id]);
    const hasConflict = Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
    const isRemoteOnly = !b.hasLocal && b.hasRemote && b.state === 'open';
    const delTitle = isRemoteOnly ? 'PR Kapat &amp; Uzak Dalı Sil' : 'Dalı Sil';

    let badgeTone = 'sky';
    let badgePulse = false;
    let badgeText = b.pr ? `PR #${b.pr.number} · ${b.pr.reviewState}` : 'Remote Ray';
    if (isFailed) {
      badgeTone = 'danger';
      badgeText = `PR #${b.pr.number} · CI Failed`;
    } else if (hasConflict) {
      badgeTone = 'warn';
      badgeText = b.pr ? `PR #${b.pr.number} · Çakışma` : 'Merge Çakışması';
    } else if (isRunning) {
      badgeTone = 'warn';
      badgePulse = true;
      badgeText = `PR #${b.pr.number} · CI Çalışıyor`;
    } else if (b.originGone || b.state === 'gone') {
      badgeTone = 'warn';
      badgeText = 'origin: gone';
    } else if (isLower) {
      badgeTone = 'purple';
      badgeText = 'Yarım Parabol · Local';
    }

    const subText = b.lastCommitMsg
      ? b.lastCommitMsg
      : (isLower ? `↑${b.aheadMain || 0} yerel · ${b.uncommittedCount || 0} dirty` : `↑${b.aheadMain || 0} ↓${b.behindMain || 0}`);

    const card = document.createElementNS(XHTML_NS, 'div');
    card.className = `cap-card ${tone} ${isSelected ? 'selected' : ''}`;

    let alertsHtml = '';
    if (isRemoteOnly) {
      alertsHtml += '<span class="cap-alert-pill cross">☁ Sadece Origin</span>';
    }
    if (hasConflict) {
      alertsHtml += `<span class="cap-alert-pill conflict">${b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lock' : '⚠ Conflict'}</span>`;
    }
    if (b.crossPr?.length) {
      alertsHtml += `<span class="cap-alert-pill cross">⇄ Cross-PR ${b.crossPr.map((c) => '#' + (c.otherPrNumber || '?')).join(',')}</span>`;
    }

    const isWorkingTreeWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-');
    const checkoutTitle = isRemoteOnly ? 'Yerelde İndir &amp; Geç (git switch --track)' : 'Bu dala geç (git switch)';
    const checkoutBtn = (!b.isHead && !isWorkingTreeWip)
      ? `<button type="button" class="cap-btn-ico" data-act="checkout" title="${checkoutTitle}">🎯</button>`
      : '';

    let actionRowHtml = '';
    if (isFailed) {
      actionRowHtml = `
        <button type="button" class="cap-btn danger" data-act="inspect">🔍 Hatayı Gör</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else if (hasConflict) {
      actionRowHtml = `
        <button type="button" class="cap-btn amber" data-act="resolve">⚡ Çakışma Stüdyosu</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else if (isLower) {
      if (isWorkingTreeWip) {
        actionRowHtml = `<button type="button" class="cap-btn sky" data-act="inspect">🚀 Commit &amp; PR Stüdyosu</button>`;
      } else {
        actionRowHtml = `
          <button type="button" class="cap-btn sky" data-act="promote">🚀 Push &amp; PR</button>
          <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge</button>
          ${checkoutBtn}
          <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
        `;
      }
    } else if (isRunning) {
      actionRowHtml = `
        <button type="button" class="cap-btn amber ${isQueued ? 'running-pulse' : ''}" data-act="merge">${isQueued ? '⏳ CI Bekleniyor (İptal)' : '⏳ CI Bitince Merge Et'}</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    } else {
      actionRowHtml = `
        <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge (Parabolü Kapat)</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="${delTitle}">🗑️</button>
      `;
    }

    card.innerHTML = `
      <div class="cap-top-row">
        <span class="cap-badge ${badgeTone} ${badgePulse ? 'running-pulse' : ''}">${esc(badgeText)}</span>
        ${b.isHead ? '<span class="cap-head-pill">HEAD</span>' : ''}
      </div>
      <div class="cap-name">${esc(b.name)}</div>
      <div class="cap-sub">${esc(truncate(subText, 34))}${b.uncommittedCount > 0 ? ` · ●${b.uncommittedCount} dirty` : ''}</div>
      ${alertsHtml ? `<div class="cap-alerts">${alertsHtml}</div>` : ''}
      <div class="cap-actions">${actionRowHtml}</div>
    `;

    card.querySelectorAll('button[data-act]').forEach((btn) => {
      btn.addEventListener('pointerdown', (ev) => ev.stopPropagation());
      btn.addEventListener('click', (ev) => {
        ev.stopPropagation();
        const act = btn.getAttribute('data-act');
        if (act === 'inspect') selectBranch(b.id);
        else if (act === 'checkout') handleCheckoutBranch(b.id);
        else if (act === 'merge') handleMergeBranch(b.id);
        else if (act === 'promote') handlePromoteBranch(b.id);
        else if (act === 'resolve') openConflictStudio(b.id);
        else if (act === 'delete') handleDeleteBranch(b.id);
      });
    });

    card.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0 || ev.target.closest('button')) return;
      ev.stopPropagation();
      startCapsulePointer(ev, b, fo, card);
    });

    fo.appendChild(card);
    return fo;
  }

  function clientToSvg(clientX, clientY) {
    const pt = el.svg.createSVGPoint();
    pt.x = clientX;
    pt.y = clientY;
    const ctm = el.svg.getScreenCTM();
    if (!ctm) return { x: 0, y: 0 };
    return pt.matrixTransform(ctm.inverse());
  }

  function startCapsulePointer(ev, branch, foEl, cardEl) {
    const startPt = clientToSvg(ev.clientX, ev.clientY);
    const geom = computeBranchGeometry(branch, null, null, state.apexLayout);
    const isVirtualWip = branch.id === 'local-wip-working-tree' || branch.id.startsWith('local-wip-');
    const canDragMerge = branch.state === 'open' && !isVirtualWip && branch.pr?.ci?.state !== 'FAILURE';

    state.drag = {
      branch,
      foEl,
      cardEl,
      startClientX: ev.clientX,
      startClientY: ev.clientY,
      startSvgX: startPt.x,
      startSvgY: startPt.y,
      baseApexX: geom.apexX,
      baseApexY: geom.apexY,
      active: false,
      canDragMerge,
    };
  }

  window.addEventListener('pointermove', (ev) => {
    if (state.panning) {
      const dx = (ev.clientX - state.panning.x) / state.zoom;
      const dy = (ev.clientY - state.panning.y) / state.zoom;
      state.panX = state.panning.panX - dx;
      state.panY = state.panning.panY - dy;
      renderSVG();
      return;
    }
    if (!state.drag) return;

    const dist = Math.hypot(ev.clientX - state.drag.startClientX, ev.clientY - state.drag.startClientY);
    if (!state.drag.active && dist >= 7 && state.drag.canDragMerge) {
      state.drag.active = true;
      state.drag.cardEl.classList.add('dragging');
      document.getElementById('svg-main-drop-rect')?.classList.add('visible');
      el.dragHint.classList.add('active-drop');
    }

    if (state.drag.active) {
      const pt = clientToSvg(ev.clientX, ev.clientY);
      const dx = pt.x - state.drag.startSvgX;
      const dy = pt.y - state.drag.startSvgY;
      const curApexX = state.drag.baseApexX + dx;
      const curApexY = state.drag.baseApexY + dy;

      const pos = capsulePositionForApex(state.drag.branch, curApexX, curApexY);
      state.drag.foEl.setAttribute('x', String(pos.x));
      state.drag.foEl.setAttribute('y', String(pos.y));

      const preview = computeBranchGeometry(state.drag.branch, curApexX, curApexY);
      document.getElementById('drag-preview-path')?.setAttribute('d', preview.openD);

      const overSpine = Math.abs(curApexY - MAIN_Y) < 68;
      state.drag.overSpine = overSpine;
      document.getElementById('svg-main-drop-rect')?.classList.toggle('hot', overSpine);
    }
  });

  window.addEventListener('pointerup', async () => {
    if (state.panning) {
      state.panning = null;
      el.viewport.classList.remove('panning');
    }
    if (!state.drag) return;

    const d = state.drag;
    state.drag = null;

    document.getElementById('svg-main-drop-rect')?.classList.remove('visible', 'hot');
    document.getElementById('drag-preview-path')?.setAttribute('d', '');
    el.dragHint.classList.remove('active-drop');
    d.cardEl.classList.remove('dragging');

    if (!d.active) {
      selectBranch(d.branch.id);
      return;
    }

    if (d.overSpine) {
      await handleMergeBranch(d.branch.id);
    } else {
      renderSVG();
    }
  });

  function selectBranch(branchId) {
    state.selectedBranchId = branchId;
    state.selectedNodeSha = null;
    renderSVG();
    renderInspector();
  }

  function suggestUniqueBranchName(b, snap) {
    const files = b.changedFiles || [];
    let prefix = 'feat';
    let slug = 'cockpit-update';
    if (files.length > 0) {
      const first = files[0].toLowerCase();
      if (first.endsWith('.md') || first.startsWith('docs/')) {
        prefix = 'docs';
      } else if (first.includes('test')) {
        prefix = 'test';
      } else if (first.endsWith('.css') || first.endsWith('.html')) {
        prefix = 'style';
      }
      const base = first.split('/').pop().replace(/\.[^.]+$/, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
      if (base && base !== 'index' && base !== 'main') {
        slug = `${base}-update`;
      }
    }
    const candidateBase = `${prefix}/${slug}`.slice(0, 40);
    const existing = new Set((snap.branches || []).map((x) => (x.name || '').toLowerCase()));
    if (!existing.has(candidateBase)) {
      return candidateBase;
    }
    const shortSha = (snap.nodes?.[snap.nodes.length - 1]?.shortSha || '').slice(0, 5);
    if (shortSha && !existing.has(`${candidateBase}-${shortSha}`.toLowerCase())) {
      return `${candidateBase}-${shortSha}`;
    }
    let n = 2;
    while (existing.has(`${candidateBase}-${n}`)) {
      n++;
    }
    return `${candidateBase}-${n}`;
  }

  function renderInspector() {
    const snap = state.snapshot;
    if (!snap) return;

    if (state.selectedNodeSha) {
      const node = snap.nodes.find((n) => n.shortSha === state.selectedNodeSha);
      if (node) {
        el.detail.innerHTML = `
          <div class="det-header">
            <div class="det-title-row">
              <h3 class="det-branch-name">Commit ${esc(node.shortSha)}</h3>
              ${node.tag ? `<span class="chip accent">${esc(node.tag)}</span>` : ''}
            </div>
            <div class="det-badges">
              <span class="chip">${esc(node.author)}</span>
              <span class="chip">${esc(node.relativeTime)}</span>
              ${node.isMerge ? '<span class="chip ok">Merge Node</span>' : ''}
            </div>
          </div>
          <div class="diag-card">
            <h4>Commit Mesajı</h4>
            <div class="layer-line">${esc(node.subject)}</div>
          </div>
          <div class="lock-note">🔒 <strong>main</strong> omurgasındaki commit mesajları tarihçe güvenliği için kilitlidir.</div>
        `;
        return;
      }
    }

    const b = snap.branches.find((x) => x.id === state.selectedBranchId);
    if (!b) {
      el.detail.innerHTML = '<div class="empty-detail">İncelemek için soldaki parabolik grafikten bir dal kapsülü veya düğüm seçin.</div>';
      return;
    }

    const isLive = snap.repo.mode === 'live';
    const hasConflict = b.conflict?.hasConflict && !b.conflict.resolved;
    const isOpen = b.state === 'open';
    const isRemoteOnly = !b.hasLocal && b.hasRemote;
    const isVirtualWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-');
    const ciState = b.pr?.ci?.state;
    const isRunning = ciState === 'RUNNING';
    const isQueued = Boolean(state.pendingAutoMerge[b.id]);
    const canCommitWorkingTree = isOpen && (isVirtualWip || (b.uncommittedCount || 0) > 0);
    const canPromote = isOpen && b.hemisphere === 'lower' && !isVirtualWip;
    const canMerge = isOpen && !hasConflict && !isVirtualWip && ciState !== 'FAILURE';

    const ciChip = ciState === 'FAILURE' ? '<span class="chip danger">CI: FAILURE</span>'
      : ciState === 'RUNNING' ? '<span class="chip warn running-pulse">CI: RUNNING</span>'
        : ciState === 'SUCCESS' ? '<span class="chip ok">CI: PASS</span>' : '';

    const queuedChip = isQueued ? '<span class="chip warn running-pulse">⏳ Auto-Merge Kuyruğunda</span>' : '';

    let html = `
      <div class="det-header">
        <div class="det-title-row">
          <h3 class="det-branch-name">${esc(b.name)}</h3>
          <span class="chip ${b.state === 'merged' ? 'ok' : (b.hemisphere === 'lower' ? 'purple' : 'accent')}">${b.state === 'merged' ? 'Tam Parabol' : b.hemisphere === 'lower' ? 'Alt Ray · Local' : 'Üst Ray · PR'}</span>
        </div>
        <div class="det-badges">
          ${b.pr ? `<span class="chip accent">PR #${b.pr.number}</span>` : ''}
          ${ciChip}
          ${queuedChip}
          ${b.isHead ? '<span class="chip ok">HEAD</span>' : ''}
          ${isRemoteOnly ? '<span class="chip warn">☁ Yerelde Yok (Sadece Origin)</span>' : ''}
          ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
        </div>
        <div class="det-actions">
          ${!b.isHead && isOpen && !isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-checkout" ${snap.repo.teamMode ? 'disabled title="Team Server modunda paylaşılan HEAD kilitlidir"' : ''}>${isRemoteOnly ? '🎯 Yerelde İndir &amp; Geç' : '🎯 Dala Geç'}</button>` : ''}
          ${canPromote ? `<button type="button" class="btn primary sm" id="act-promote">🚀 Push &amp; PR Aç</button>` : ''}
          ${isOpen && !isVirtualWip ? `
            <div class="merge-group">
              <select id="merge-strategy" class="select-inline" aria-label="Merge stratejisi">
                <option value="merge" ${state.mergeStrategy === 'merge' ? 'selected' : ''}>Merge</option>
                <option value="squash" ${state.mergeStrategy === 'squash' ? 'selected' : ''}>Squash</option>
                <option value="rebase" ${state.mergeStrategy === 'rebase' ? 'selected' : ''}>Rebase</option>
                <option value="auto" ${state.mergeStrategy === 'auto' ? 'selected' : ''}>Auto-Merge (Queue)</option>
              </select>
              ${isRunning
                ? `<button type="button" class="btn warn sm ${isQueued ? 'running-pulse' : ''}" id="act-merge">${isQueued ? '⏳ CI Bekleniyor (İptal Et)' : '⏳ CI Bitince Otomatik Merge Et'}</button>`
                : `<button type="button" class="btn ok sm" id="act-merge" ${canMerge ? '' : 'disabled'}>⚡ Birleştir</button>`}
            </div>
          ` : ''}
          ${hasConflict ? `<button type="button" class="btn warn sm" id="act-resolve">⚡ Çakışma Stüdyosunu Aç</button>` : ''}
          ${!isVirtualWip && (b.state !== 'merged' || b.hasLocal || b.originGone || b.hasRemote) ? `<button type="button" class="btn ghost sm" id="act-delete">${isRemoteOnly ? '🗑️ PR Kapat &amp; Uzak Dalı Sil' : '🧹 Sil / Temizle'}</button>` : ''}
        </div>
      </div>

      <div class="det-grid">
        <div class="layer-box">
          <h3>Üst Ray · Remote / PR</h3>
          ${b.pr ? `
            <div class="layer-line">PR #${b.pr.number} · ${esc(b.pr.reviewState)}</div>
            <div class="layer-line">${esc(truncate(b.pr.title, 48))}</div>
            <div class="layer-line">CI: ${esc(b.pr.ci?.workflow || 'ci.yml')} (${esc(b.pr.ci?.duration || '—')})</div>
          ` : `
            <div class="layer-line">${isRemoteOnly ? `Remote aktif: origin/${esc(b.name)}` : (b.originGone ? 'Remote silindi (origin: gone)' : 'Henüz PR / Remote yok')}</div>
            <div class="layer-line">${isRemoteOnly ? 'Yerelde kopyası silinmiş uzak dal' : 'Alt rayda yerel çalışma'}</div>
          `}
        </div>
        <div class="layer-box">
          <h3>Alt Ray · Local</h3>
          ${!b.hasLocal ? `
            <div class="layer-line">${b.state === 'merged' && !b.originGone ? '✓ Yerel kalıntı dal temizlendi' : `Yerelde dal yok (origin/${esc(b.name)})`}</div>
            <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
            <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
          ` : `
            ${b.state === 'merged' ? '<div class="layer-line">⚠ Birleşmiş yerel kalıntı (tek tıkla temizlenebilir)</div>' : ''}
            <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
            <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
            <div class="layer-line">Uncommitted: ${b.uncommittedCount || 0} dosya</div>
          `}
        </div>
      </div>
    `;

    if (b.conflict) {
      const resolved = b.conflict.resolved;
      html += `
        <div class="diag-card ${resolved ? '' : 'conflict'}">
          <h4>
            <span>${resolved ? '✓ Çakışma Çözüldü' : (b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lockfile Çakışması' : '⚠ Merge Çakışması')}</span>
            <span class="chip">${esc(b.conflict.strategy || 'merge-tree')}</span>
          </h4>
          <div class="layer-line">${resolved ? esc(b.conflict.resolvedNote) : `git merge-tree --write-tree: ${esc((b.conflict.files || []).join(', '))}`}</div>
          <pre class="terminal-box">${esc(b.conflict.previewUnified || '')}</pre>
          ${!resolved ? `<div class="det-actions"><button type="button" class="btn warn sm" id="act-resolve-card">⚡ Çakışma Stüdyosunu Aç (Sol: Olan / Sağ: Olacak)</button></div>` : ''}
        </div>
      `;
    }

    if (b.crossPr?.length) {
      html += `
        <div class="diag-card cross">
          <h4><span>⇄ Cross-PR Dosya Çarpışması</span></h4>
          ${b.crossPr.map((c) => `
            <div class="layer-line"><strong>${c.otherPrNumber ? 'PR #' + c.otherPrNumber : ''} (${esc(c.otherBranch)})</strong>: ${esc(c.sharedFiles.join(', '))}</div>
          `).join('')}
        </div>
      `;
    }

    if (b.pr?.ci?.state === 'FAILURE') {
      const extracts = b.pr.ci.extracts || [];
      const localRunOut = state.localRunOutput[b.id] || '';
      html += `
        <div class="diag-card ci">
          <h4>
            <span>Gürültüsüz CI Hata Ayıklayıcı</span>
            <span class="chip danger">${extracts.length} kritik bulgu</span>
          </h4>
          <div class="extract-list">
            ${extracts.map((ex) => `
              <div class="extract-item">
                <div class="extract-head">
                  <span>${esc(ex.kind.toUpperCase())} · ${esc(ex.symbol)}</span>
                  <span>${esc(ex.fileLine || '')}</span>
                </div>
                <div class="extract-msg">${esc(ex.message)}</div>
                ${ex.reproCmd ? `<div class="extract-cmd">$ ${esc(ex.reproCmd)}</div>` : ''}
              </div>
            `).join('')}
          </div>
          <div class="det-actions">
            ${!isLive ? '<button type="button" class="btn emerald sm" id="act-fix-ci">🛠️ Hatayı Düzelt &amp; CI Yeniden Çalıştır</button>' : ''}
            <button type="button" class="btn sm" id="act-copy-log">Logu Kopyala</button>
            <button type="button" class="btn primary sm" id="act-run-local">▶ Yerelde Testi Çalıştır</button>
          </div>
          <pre class="terminal-box" id="ci-raw-log">${esc(localRunOut || b.pr.ci.rawLog || 'Ham log bulunamadı.')}</pre>
        </div>
      `;
    }

    if (canCommitWorkingTree) {
      const defaultWtBranch = isVirtualWip ? suggestUniqueBranchName(b, snap) : b.name;
      const defaultCommitMsg = b.lastCommitMsg || b.draftPrTitle || 'feat: cockpit workflow updates';
      const defaultPrTitle = b.draftPrTitle || b.lastCommitMsg || defaultCommitMsg;
      const defaultPrBody = b.draftPrBody || '';
      html += `
        <div class="diag-card">
          <h4>
            <span>🚀 Konsolsuz Dal Aç, Commit &amp; PR Oluştur</span>
            <span class="chip accent">${b.uncommittedCount || 0} dosya</span>
          </h4>
          <div class="wt-note">${isVirtualWip ? 'Şu an <strong>main</strong> üzerindeki kaydedilmemiş değişiklikleri görüyorsunuz. Terminal açmadan yeni dal açabilir, commit atabilir ve doğrudan push + PR oluşturabilirsiniz.' : 'Bu daldaki kaydedilmemiş değişiklikleri konsol kullanmadan yerelde commitleyin veya tek tıkla pushlayıp PR açın.'}</div>
          <form id="form-commit-wt" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="wt-branch-name">Özellik Dalı Adı</label>
              <input type="text" id="wt-branch-name" class="input-control mono" value="${esc(defaultWtBranch)}" placeholder="feat/cockpit-updates" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-commit-msg">Commit Mesajı</label>
              <input type="text" id="wt-commit-msg" class="input-control mono" value="${esc(defaultCommitMsg)}" placeholder="feat: ..." required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-title">PR Başlığı (Push &amp; PR için)</label>
              <input type="text" id="wt-pr-title" class="input-control mono" value="${esc(defaultPrTitle)}" placeholder="PR başlığı">
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-body">PR Açıklaması</label>
              <textarea id="wt-pr-body" class="textarea-control" placeholder="Neler değişti ve nasıl doğrulandı?">${esc(defaultPrBody)}</textarea>
            </div>
            <div class="det-actions">
              <button type="submit" class="btn emerald sm" id="act-wt-commit-only" ${snap.repo.teamMode ? 'disabled' : ''}>💾 Sadece Dal Aç &amp; Commit At</button>
              <button type="button" class="btn primary sm" id="act-wt-commit-push" ${snap.repo.teamMode ? 'disabled' : ''}>🚀 Commit + Push &amp; PR Oluştur</button>
            </div>
          </form>
        </div>
      `;
    }

    if (canPromote) {
      const savedDraft = state.draftPrEdits[b.name];
      const promoteTitle = savedDraft?.title ?? b.draftPrTitle ?? b.lastCommitMsg ?? ('feat: ' + b.name);
      const promoteBody = savedDraft?.body ?? b.draftPrBody ?? '';
      html += `
        <div class="diag-card">
          <h4>
            <span>📝 Push Öncesi PR Başlık &amp; Açıklama</span>
            <span class="chip purple">Local → Remote</span>
          </h4>
          <div class="wt-note">Dalı origin’e pushlamadan önce PR başlığını ve açıklamasını burada hazırlayıp kaydedebilirsiniz.</div>
          <form id="form-promote-pr" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="promote-pr-title">PR Başlığı</label>
              <input type="text" id="promote-pr-title" class="input-control mono" value="${esc(promoteTitle)}" placeholder="Örn: feat(ui): Add cockpit workflow" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="promote-pr-body">PR Açıklaması</label>
              <textarea id="promote-pr-body" class="textarea-control" placeholder="Neler değişti ve doğrulama adımları...">${esc(promoteBody)}</textarea>
            </div>
            <div class="det-actions">
              <button type="submit" class="btn primary sm" id="act-promote-submit">🚀 Pushla &amp; Bu Bilgilerle PR Aç</button>
            </div>
          </form>
        </div>
      `;
    }

    if (b.changedFiles?.length) {
      html += `
        <div class="diag-card">
          <h4>Değişen Dosyalar (${b.changedFiles.length})</h4>
          <div class="file-list">
            ${b.changedFiles.map((f) => {
              const special = /changelog|components\.lock|go\.mod|go\.sum|version/i.test(f);
              return `<span class="file-tag ${special ? 'special' : ''}">${esc(f)}</span>`;
            }).join('')}
          </div>
        </div>
      `;
    }

    if (b.pr && isOpen) {
      html += `
        <div class="diag-card">
          <h4>✏️ PR #${b.pr.number} Başlık &amp; Açıklama Düzenle</h4>
          <form id="form-edit-pr" class="edit-form">
            <input type="text" id="pr-edit-title" class="input-control mono" value="${esc(b.pr.title)}" placeholder="PR başlığı" required>
            <textarea id="pr-edit-body" class="textarea-control" placeholder="PR açıklaması (opsiyonel)">${esc(b.pr.body || '')}</textarea>
            <div class="det-actions">
              <button type="submit" class="btn primary sm">PR Bilgisini Güncelle</button>
            </div>
          </form>
        </div>
      `;
    }

    if (b.canAmendCommit) {
      html += `
        <div class="diag-card">
          <h4>✏️ Son Commit Mesajını Değiştir (--amend --only)</h4>
          <form id="form-amend-commit" class="edit-form">
            <input type="text" id="commit-amend-msg" class="input-control mono" value="${esc(b.lastCommitMsg || '')}" placeholder="Yeni commit mesajı" required>
            <div class="det-actions">
              <button type="submit" class="btn sm">Commit Mesajını Güncelle</button>
            </div>
          </form>
        </div>
      `;
    } else if (isOpen && !isVirtualWip && !b.isHead) {
      html += `<div class="lock-note">🔒 Commit mesajı yalnızca aktif çalışma dalı (HEAD) iken değiştirilebilir. Önce <strong>🎯 Dala Geç</strong> ile geçiş yapın.</div>`;
    }

    if (b.suggestedCmd) {
      html += `
        <div class="diag-card">
          <h4>
            <span>📋 Tek Tıkla Terminal Komutu</span>
            <button type="button" class="btn sm" id="act-copy-cmd">Kopyala</button>
          </h4>
          <div class="cmd-box"><code class="cmd-code">${esc(b.suggestedCmd)}</code></div>
        </div>
      `;
    }

    el.detail.innerHTML = html;

    document.getElementById('act-checkout')?.addEventListener('click', () => handleCheckoutBranch(b.id));
    document.getElementById('act-promote')?.addEventListener('click', () => handlePromoteBranch(b.id));
    document.getElementById('form-commit-wt')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const bName = document.getElementById('wt-branch-name').value;
      const cMsg = document.getElementById('wt-commit-msg').value;
      const pTitle = document.getElementById('wt-pr-title').value;
      const pBody = document.getElementById('wt-pr-body').value;
      const btn = document.getElementById('act-wt-commit-only');
      handleCommitWorkingTree(bName, cMsg, pTitle, pBody, false, btn);
    });
    document.getElementById('act-wt-commit-push')?.addEventListener('click', () => {
      const form = document.getElementById('form-commit-wt');
      if (form && form.reportValidity && !form.reportValidity()) return;
      const bName = document.getElementById('wt-branch-name').value;
      const cMsg = document.getElementById('wt-commit-msg').value;
      const pTitle = document.getElementById('wt-pr-title').value;
      const pBody = document.getElementById('wt-pr-body').value;
      const btn = document.getElementById('act-wt-commit-push');
      handleCommitWorkingTree(bName, cMsg, pTitle, pBody, true, btn);
    });
    document.getElementById('promote-pr-title')?.addEventListener('input', (ev) => {
      state.draftPrEdits[b.name] = {
        title: ev.target.value,
        body: document.getElementById('promote-pr-body')?.value || '',
      };
    });
    document.getElementById('promote-pr-body')?.addEventListener('input', (ev) => {
      state.draftPrEdits[b.name] = {
        title: document.getElementById('promote-pr-title')?.value || '',
        body: ev.target.value,
      };
    });
    document.getElementById('form-promote-pr')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const pTitle = document.getElementById('promote-pr-title').value;
      const pBody = document.getElementById('promote-pr-body').value;
      const btn = document.getElementById('act-promote-submit');
      handlePromoteBranch(b.id, pTitle, pBody, btn);
    });
    document.getElementById('merge-strategy')?.addEventListener('change', (ev) => {
      state.mergeStrategy = ev.target.value;
    });
    document.getElementById('act-merge')?.addEventListener('click', () => handleMergeBranch(b.id));
    document.getElementById('act-resolve')?.addEventListener('click', () => openConflictStudio(b.id));
    document.getElementById('act-resolve-card')?.addEventListener('click', () => openConflictStudio(b.id));
    document.getElementById('act-fix-ci')?.addEventListener('click', () => handleFixCI(b.id));
    document.getElementById('act-delete')?.addEventListener('click', () => handleDeleteBranch(b.id));
    document.getElementById('form-edit-pr')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const title = document.getElementById('pr-edit-title').value;
      const body = document.getElementById('pr-edit-body').value;
      handleEditPR(b.id, title, body);
    });
    document.getElementById('form-amend-commit')?.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const msg = document.getElementById('commit-amend-msg').value;
      handleAmendCommit(b.id, msg);
    });
    document.getElementById('act-copy-cmd')?.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.suggestedCmd);
        showToast('Terminal komutu panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi', 'warn');
      }
    });
    document.getElementById('act-copy-log')?.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.pr.ci.rawLog || '');
        showToast('CI hata logu panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi; logu seçip kopyalayabilirsiniz', 'warn');
      }
    });
    document.getElementById('act-run-local')?.addEventListener('click', () => {
      const cmd = b.pr.ci.extracts?.[0]?.reproCmd || 'go test -v ./...';
      handleLocalTestRun(b.id, cmd);
    });
  }

  function populateBaseCommitOptions() {
    const nodes = state.snapshot?.nodes || [];
    el.newBranchBase.innerHTML = '';
    [...nodes].reverse().forEach((n) => {
      const opt = document.createElement('option');
      opt.value = n.shortSha;
      opt.textContent = `${n.shortSha} — ${truncate(n.subject, 36)}`;
      el.newBranchBase.appendChild(opt);
    });
  }

  // Action Handlers
  async function handleCheckoutBranch(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.checkoutBranch(branchId)
        : await apiCall('/api/branch/checkout', 'POST', { branchId });
      applySnapshot(snap);
      showToast('Çalışma dalı (HEAD) değiştirildi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleEditPR(branchId, title, body) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.updatePRMeta(branchId, title, body)
        : await apiCall('/api/pr/edit', 'POST', { branchId, title, body });
      applySnapshot(snap);
      showToast('PR başlık ve açıklaması güncellendi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleAmendCommit(branchId, message) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.amendLastCommit(branchId, message)
        : await apiCall('/api/commit/amend', 'POST', { branchId, message });
      applySnapshot(snap);
      showToast('Son commit mesajı (--amend --only) güncellendi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function executeMergeNow(branchId, strategy = 'merge', fromQueue = false) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.mergeBranch(branchId, strategy)
        : await apiCall('/api/branch/merge', 'POST', { branchId, strategy });
      applySnapshot(snap);
      showToast(
        fromQueue
          ? `✅ CI başarılı! Dal otomatik olarak (${strategy}) main'e birleştirildi.`
          : `Dal merkez main omurgasına (${strategy}) birleşti — Tam parabol tamamlandı!`,
        'ok',
      );
    } catch (err) {
      showToast(err.message, 'warn');
      renderSVG();
    }
  }

  async function handleMergeBranch(branchId) {
    const b = state.snapshot?.branches?.find((x) => x.id === branchId);
    const strategy = state.mergeStrategy || 'merge';
    if (b?.pr?.ci?.state === 'RUNNING') {
      if (state.pendingAutoMerge[branchId]) {
        delete state.pendingAutoMerge[branchId];
        recordClientLog('info', 'Auto-Merge İptal', b.name, 'CI bekleme kuyruğundan çıkarıldı');
        showToast(`${b.name} için otomatik birleştirme kuyruğu iptal edildi`, 'info');
      } else {
        state.pendingAutoMerge[branchId] = { strategy, branchName: b.name };
        recordClientLog('ok', 'Auto-Merge Kuyruğu', b.name, `CI geçince (${strategy}) otomatik birleştirilecek`);
        showToast(`⏳ ${b.name} kuyruğa alındı: CI geçince otomatik merge edilecek!`, 'info');
        if (state.offlineMode) {
          setTimeout(() => {
            if (!state.pendingAutoMerge[branchId]) return;
            try {
              const nextSnap = LocalFallbackEngine.completeRunningCI(branchId);
              applySnapshot(nextSnap);
            } catch {
              // ignore
            }
          }, 2600);
        }
      }
      renderSVG();
      renderInspector();
      return;
    }
    await executeMergeNow(branchId, strategy, false);
  }

  async function handleCommitWorkingTree(branchName, commitMsg, prTitle, prBody, pushAndPr, btnEl = null) {
    const cleanBranch = (branchName || '').trim();
    const cleanCommit = (commitMsg || '').trim();
    if (!cleanBranch || !cleanCommit) {
      showToast('Özellik dalı adı ve commit mesajı zorunludur', 'warn');
      return;
    }
    if (!pushAndPr) {
      state.draftPrEdits[cleanBranch] = {
        title: (prTitle || '').trim() || cleanCommit,
        body: (prBody || '').trim(),
      };
    } else {
      delete state.draftPrEdits[cleanBranch];
    }
    if (btnEl) btnEl.disabled = true;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.commitWorkingTree(cleanBranch, cleanCommit, prTitle || '', prBody || '', Boolean(pushAndPr))
        : await apiCall('/api/working-tree/commit', 'POST', {
          branchName: cleanBranch,
          commitMsg: cleanCommit,
          prTitle: (prTitle || '').trim(),
          prBody: (prBody || '').trim(),
          pushAndPr: Boolean(pushAndPr),
        });
      const created = (snap.branches || []).find((x) => x.name === cleanBranch);
      if (created) {
        state.selectedBranchId = created.id;
      }
      applySnapshot(snap);
      showToast(
        pushAndPr
          ? `Dal açıldı, commitlendi, pushlandı ve PR oluşturuldu (${cleanBranch})!`
          : `Değişiklikler '${cleanBranch}' dalına commitlendi!`,
        'ok',
      );
    } catch (err) {
      showToast(err.message, 'danger');
    } finally {
      if (btnEl) btnEl.disabled = false;
    }
  }

  async function handlePromoteBranch(branchId, explicitTitle, explicitBody, btnEl = null) {
    const b = state.snapshot?.branches?.find((x) => x.id === branchId);
    let title = explicitTitle;
    let body = explicitBody;
    if (title === undefined && state.selectedBranchId === branchId) {
      title = document.getElementById('promote-pr-title')?.value ?? state.draftPrEdits[b?.name]?.title ?? b?.draftPrTitle ?? '';
      body = document.getElementById('promote-pr-body')?.value ?? state.draftPrEdits[b?.name]?.body ?? b?.draftPrBody ?? '';
    }
    if (btnEl) btnEl.disabled = true;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.promoteBranch(branchId, title || '', body || '')
        : await apiCall('/api/branch/promote', 'POST', {
          branchId,
          title: (title || '').trim(),
          body: (body || '').trim(),
        });
      if (b?.name) delete state.draftPrEdits[b.name];
      applySnapshot(snap);
      showToast('Dal origin’e pushlandı ve PR açılarak üst raya taşındı!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    } finally {
      if (btnEl) btnEl.disabled = false;
    }
  }

  const STUDIO_MODE_LABELS = {
    union: '1. Union (Her İkisini Koru)',
    ours: '2. Ours (Main Bazlı)',
    theirs: '3. Theirs (Dal Bazlı)',
    custom: '4. AI / Elle Düzenlendi',
  };

  async function openConflictStudio(branchId) {
    try {
      const payload = state.offlineMode
        ? LocalFallbackEngine.inspectConflict(branchId)
        : await apiCall('/api/conflict/inspect', 'POST', { branchId });
      if (!payload.files || payload.files.length === 0) {
        showToast('Bu dalda incelenecek aktif çakışma dosyası bulunamadı', 'warn');
        return;
      }
      state.studio = {
        payload,
        activeFileIdx: 0,
        strategy: payload.recommended || 'union',
        customByPath: {},
      };
      el.studioRunVet.checked = true;
      el.studioErrorBanner.hidden = true;
      el.studioErrorText.textContent = '';
      renderConflictStudio();
      el.conflictStudioModal.hidden = false;
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  function closeConflictStudio() {
    el.conflictStudioModal.hidden = true;
    state.studio = null;
  }

  function getActiveStudioFile() {
    if (!state.studio) return null;
    const files = state.studio.payload.files || [];
    return files[state.studio.activeFileIdx] || files[0] || null;
  }

  function studioPreviewForStrategy(file, strat, customByPath) {
    if (!file) return '';
    if (strat === 'custom') {
      return customByPath[file.path] !== undefined ? customByPath[file.path] : (file.unionPreview || '');
    }
    if (strat === 'ours') return file.oursPreview || '';
    if (strat === 'theirs') return file.theirsPreview || '';
    return file.unionPreview || '';
  }

  function renderConflictStudio() {
    const st = state.studio;
    if (!st) return;
    const { payload, activeFileIdx, strategy, customByPath } = st;
    const files = payload.files || [];
    const curFile = files[activeFileIdx] || files[0];

    el.studioBranchBadge.textContent = `${payload.branchName} ↔ ${payload.baseRef}`;

    el.studioFileTabs.innerHTML = '';
    files.forEach((f, idx) => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = `studio-file-tab ${idx === activeFileIdx ? 'active' : ''}`;
      btn.textContent = `📄 ${f.path}`;
      btn.addEventListener('click', () => {
        if (st.strategy === 'custom' && curFile) {
          st.customByPath[curFile.path] = el.studioRightPreview.value;
        }
        st.activeFileIdx = idx;
        renderConflictStudio();
      });
      el.studioFileTabs.appendChild(btn);
    });

    el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
      const stratKey = btn.getAttribute('data-strat');
      btn.classList.toggle('active', stratKey === strategy);
      const badgeEl = btn.querySelector('.strat-badge');
      if (badgeEl && payload.strategies) {
        const adv = payload.strategies.find((s) => s.strategy === stratKey);
        if (adv) {
          badgeEl.textContent = adv.badge;
          badgeEl.className = `strat-badge ${adv.tone || ''}`;
        }
      }
    });

    if (payload.recommendationReason) {
      el.studioGuideText.textContent = payload.recommendationReason;
      el.studioGuideBox.hidden = false;
    } else {
      el.studioGuideBox.hidden = true;
    }

    if (curFile) {
      el.studioLineBadge.textContent = `Satır ${curFile.startLine || 1}-${curFile.endLine || 1}`;
      el.studioLeftConflict.textContent = curFile.conflictHunks || '';
      el.studioRightPreview.value = studioPreviewForStrategy(curFile, strategy, customByPath);
    }
    el.studioModeBadge.textContent = STUDIO_MODE_LABELS[strategy] || strategy;
  }

  async function applyConflictStudio() {
    const st = state.studio;
    if (!st) return;
    const curFile = getActiveStudioFile();
    if (st.strategy === 'custom' && curFile) {
      st.customByPath[curFile.path] = el.studioRightPreview.value;
    }

    const customFiles = st.strategy === 'custom' ? st.customByPath : undefined;
    const runVet = Boolean(el.studioRunVet.checked);

    el.studioErrorBanner.hidden = true;
    el.studioErrorText.textContent = '';
    el.studioApplyBtn.disabled = true;
    el.studioApplyBtn.textContent = 'Uygulanıyor...';
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.resolveConflict(st.payload.branchId, st.strategy)
        : await apiCall('/api/branch/resolve', 'POST', {
          branchId: st.payload.branchId,
          strategy: st.strategy,
          customFiles,
          runVet,
        });
      closeConflictStudio();
      applySnapshot(snap);
      showToast(`Çakışma (${st.strategy}) ile çözüldü ve güncellendi!`, 'ok');
    } catch (err) {
      el.studioErrorText.textContent = err.message || String(err);
      el.studioErrorBanner.hidden = false;
      showToast(err.message, 'danger');
      if (!state.offlineMode) {
        apiCall('/api/snapshot').then(applySnapshot).catch(() => {});
      }
    } finally {
      el.studioApplyBtn.disabled = false;
      el.studioApplyBtn.textContent = '✅ Çözümü Uygula & Pushla';
    }
  }

  function renderLogsModal() {
    const logs = getMergedLogs();
    el.logsList.innerHTML = '';
    if (logs.length === 0) {
      el.logsList.innerHTML = '<div class="empty-detail">Henüz kaydedilmiş hata veya işlem günlüğü yok.</div>';
      return;
    }
    logs.forEach((item) => {
      const card = document.createElement('div');
      const tone = item.level === 'error' ? 'error' : (item.level === 'ok' ? 'ok' : '');
      card.className = `log-item ${tone}`;
      card.innerHTML = `
        <div class="log-item-head">
          <span>${item.level === 'error' ? '❌' : (item.level === 'ok' ? '✅' : 'ℹ️')} ${esc(item.action)}${item.branch ? ` · ${esc(item.branch)}` : ''}</span>
          <span class="log-item-meta">${esc(item.timestamp || '')}</span>
        </div>
        <div class="log-item-msg">${esc(item.message)}</div>
        ${item.detail ? `<pre class="log-item-detail">${esc(item.detail)}</pre>` : ''}
      `;
      el.logsList.appendChild(card);
    });
  }

  function openLogsModal() {
    renderLogsModal();
    el.logsModal.hidden = false;
  }

  function closeLogsModal() {
    el.logsModal.hidden = true;
  }

  function renderSecurityUpdateModal() {
    const r = state.snapshot?.repo || {};
    const isProt = Boolean(r.branchProtected);
    el.secProtBadge.textContent = isProt ? '🛡️ Korumalı' : '🔓 Korumasız';
    el.secProtBadge.className = `chip ${isProt ? 'ok' : 'warn'}`;
    el.secProtSummary.textContent = r.protectionSummary || (isProt ? 'Ana dal korumalı' : 'Doğrudan push/force-push açık');
    el.btnEnableProtection.disabled = isProt;
    el.secTeamChip.textContent = r.teamMode ? '🏢 Team Server (-team)' : '💻 Bireysel Masaüstü';
    el.secTeamChip.className = `chip ${r.teamMode ? 'ok' : 'accent'} mono`;

    const cur = r.appVersion || '0.7.0';
    const lat = r.latestVersion || cur;
    el.secUpdateStatus.textContent = r.updateAvailable
      ? `Yeni sürüm bulundu: v${lat} (Mevcut: v${cur})`
      : `Mevcut sürüm güncel: v${cur}${r.latestVersion ? ` (Uzak etiket: v${lat})` : ''}`;
  }

  function openSecurityUpdateModal() {
    renderSecurityUpdateModal();
    el.securityUpdateModal.hidden = false;
  }

  function closeSecurityUpdateModal() {
    el.securityUpdateModal.hidden = true;
  }

  async function handleProtectBranch() {
    el.btnEnableProtection.disabled = true;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.protectBranch()
        : await apiCall('/api/repo/protect', 'POST', {});
      applySnapshot(snap);
      renderSecurityUpdateModal();
      showToast('Ana dal koruması (Branch Protection) etkinleştirildi!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    } finally {
      if (state.snapshot?.repo) {
        el.btnEnableProtection.disabled = Boolean(state.snapshot.repo.branchProtected);
      }
    }
  }

  async function handleCheckUpdates() {
    try {
      const res = state.offlineMode
        ? LocalFallbackEngine.checkUpdates()
        : await apiCall('/api/system/update-check', 'POST', {});
      if (state.snapshot?.repo) {
        state.snapshot.repo.latestVersion = res.latestVersion;
        state.snapshot.repo.updateAvailable = res.updateAvailable;
      }
      if (res.updateCommand) {
        el.secUpdateCmd.textContent = res.updateCommand;
      }
      renderTopbar();
      showToast(res.updateAvailable ? `Yeni sürüm mevcut: v${res.latestVersion}` : `GitArc güncel (v${res.currentVersion})`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  function buildMCPConfigText() {
    const repoPath = state.snapshot?.repo?.mode === 'live' && state.snapshot?.repo?.path
      ? state.snapshot.repo.path
      : '/Users/ziyayesilbahce/Developer/GitArc';
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

  function openMCPConnectModal() {
    el.mcpConfigSnippet.textContent = buildMCPConfigText();
    el.mcpModal.hidden = false;
  }

  function closeMCPConnectModal() {
    el.mcpModal.hidden = true;
  }

  async function handleFixCI(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.fixCI(branchId)
        : await apiCall('/api/branch/fix-ci', 'POST', { branchId });
      delete state.localRunOutput[branchId];
      applySnapshot(snap);
      showToast('CI testleri yeşile döndü — PR artık merge edilebilir!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleDeleteBranch(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.deleteBranch(branchId)
        : await apiCall('/api/branch/delete', 'POST', { branchId });
      applySnapshot(snap);
      showToast('Dal kaldırıldı', 'warn');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handlePruneGone() {
    try {
      let snap;
      let removed = 0;
      if (state.offlineMode) {
        const res = LocalFallbackEngine.pruneGone();
        snap = res.snapshot;
        removed = res.removed;
      } else {
        const res = await apiCall('/api/branch/prune', 'POST', {});
        snap = res.snapshot;
        removed = res.removed;
      }
      applySnapshot(snap);
      showToast(`${removed} adet silinmiş/birleşmiş dal temizlendi`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleSyncSpine() {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.syncSpine()
        : await apiCall('/api/spine/sync', 'POST', {});
      applySnapshot(snap);
      showToast('main ile origin/main senkronize edildi', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleLocalTestRun(branchId, cmd) {
    const repoDir = state.snapshot?.repo?.path || '.';
    state.localRunOutput[branchId] = `$ cd ${repoDir} && ${cmd}\nÇalıştırılıyor...\n`;
    renderInspector();
    try {
      if (!state.offlineMode) {
        const res = await apiCall('/api/test/run', 'POST', { cmd });
        state.localRunOutput[branchId] = res.output;
      } else {
        await new Promise((r) => setTimeout(r, 350));
        state.localRunOutput[branchId] = [
          `$ ${cmd}`,
          '=== RUN   TestShellGoldCompare',
          '    compare_test.go:118: golden output mismatch on key \'hardening_pve_apply\': got "true", want "false"',
          '--- FAIL: TestShellGoldCompare (0.39s)',
          'FAIL\tkeylab/internal/shellgold\t0.412s',
        ].join('\n');
      }
      renderInspector();
    } catch (err) {
      state.localRunOutput[branchId] += `\nHata: ${err.message}`;
      renderInspector();
    }
  }

  // Toolbar, Modal & Viewport Events
  el.repoSelect.addEventListener('change', async () => {
    const repoId = el.repoSelect.value;
    if (state.offlineMode) return;
    try {
      const snap = await apiCall('/api/repo/switch', 'POST', { repoId });
      localStorage.setItem(REPO_STORAGE_KEY, repoId);
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      applySnapshot(snap);
      showToast(`Aktif depo: ${snap.repo.name}`, 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  el.toggleMerged.addEventListener('change', () => {
    state.showMergedArches = el.toggleMerged.checked;
    renderSVG();
  });

  el.spineSyncBadge.addEventListener('click', handleSyncSpine);

  function openNewBranchModal() {
    el.modal.hidden = false;
    el.newBranchName.focus();
  }

  function closeNewBranchModal() {
    el.modal.hidden = true;
  }

  document.getElementById('btn-new-branch').addEventListener('click', openNewBranchModal);
  document.getElementById('modal-close').addEventListener('click', closeNewBranchModal);
  document.getElementById('modal-cancel').addEventListener('click', closeNewBranchModal);

  el.newBranchForm.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const name = el.newBranchName.value.trim();
    const checkedRadio = document.querySelector('input[name="branch-mode"]:checked');
    const hemisphere = checkedRadio ? checkedRadio.value : 'lower';
    const fromSha = el.newBranchBase.value;
    if (!name) return;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.createBranch(name, hemisphere, fromSha)
        : await apiCall('/api/branch/create', 'POST', { name, hemisphere, fromSha });
      closeNewBranchModal();
      el.newBranchName.value = '';
      const created = snap.branches.find((b) => b.name === name);
      if (created) state.selectedBranchId = created.id;
      applySnapshot(snap);
      showToast(`'${name}' dalı ${hemisphere === 'upper' ? 'üst rayda (PR)' : 'alt rayda (yerel kesikli parabol)'} açıldı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('btn-prune-gone').addEventListener('click', handlePruneGone);
  el.btnProtectBranch.addEventListener('click', openSecurityUpdateModal);
  el.btnUpdateCheck.addEventListener('click', openSecurityUpdateModal);
  el.lensPathInput.addEventListener('input', () => {
    state.pathLens = el.lensPathInput.value;
    renderSVG();
  });
  el.btnLogsModal.addEventListener('click', openLogsModal);
  document.getElementById('btn-mcp-connect').addEventListener('click', openMCPConnectModal);
  document.getElementById('btn-sync-spine').addEventListener('click', handleSyncSpine);

  document.getElementById('sec-close').addEventListener('click', closeSecurityUpdateModal);
  document.getElementById('sec-close-btn').addEventListener('click', closeSecurityUpdateModal);
  el.btnEnableProtection.addEventListener('click', handleProtectBranch);
  document.getElementById('btn-run-update-check').addEventListener('click', handleCheckUpdates);
  document.getElementById('sec-copy-update-cmd').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(el.secUpdateCmd.textContent || '');
      showToast('Güncelleme komutu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  document.getElementById('studio-close').addEventListener('click', closeConflictStudio);
  document.getElementById('studio-cancel').addEventListener('click', closeConflictStudio);
  document.getElementById('studio-apply').addEventListener('click', applyConflictStudio);
  document.getElementById('studio-copy-error').addEventListener('click', async () => {
    const txt = el.studioErrorText.textContent;
    if (!txt) return;
    try {
      await navigator.clipboard.writeText(txt);
      showToast('Hata metni panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  document.getElementById('logs-close').addEventListener('click', closeLogsModal);
  document.getElementById('logs-close-btn').addEventListener('click', closeLogsModal);
  document.getElementById('logs-copy-all').addEventListener('click', async () => {
    const text = getMergedLogs().map((l) => `[${l.timestamp}] [${l.level.toUpperCase()}] ${l.action}${l.branch ? ' (' + l.branch + ')' : ''}: ${l.message}${l.detail ? '\n  ' + l.detail : ''}`).join('\n\n');
    try {
      await navigator.clipboard.writeText(text || 'Log kaydı yok.');
      showToast('Tüm loglar panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  document.getElementById('logs-clear').addEventListener('click', async () => {
    state.clientLogs = [];
    if (!state.offlineMode) {
      try {
        const snap = await apiCall('/api/logs/clear', 'POST', {});
        applySnapshot(snap);
      } catch (err) {
        showToast(err.message, 'danger');
      }
    }
    renderLogBadge();
    renderLogsModal();
    showToast('Log kayıtları temizlendi', 'info');
  });
  document.getElementById('studio-copy-ai').addEventListener('click', async () => {
    const promptTxt = state.studio?.payload?.aiPrompt;
    if (!promptTxt) return;
    try {
      await navigator.clipboard.writeText(promptTxt);
      showToast('AI / MCP çözüm istemi panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });
  el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      if (!state.studio) return;
      const curFile = getActiveStudioFile();
      if (state.studio.strategy === 'custom' && curFile) {
        state.studio.customByPath[curFile.path] = el.studioRightPreview.value;
      }
      state.studio.strategy = btn.getAttribute('data-strat') || 'union';
      renderConflictStudio();
      if (state.studio.strategy === 'custom') {
        el.studioRightPreview.focus();
      }
    });
  });
  el.studioRightPreview.addEventListener('input', () => {
    if (!state.studio) return;
    const curFile = getActiveStudioFile();
    if (curFile) {
      state.studio.customByPath[curFile.path] = el.studioRightPreview.value;
    }
    if (state.studio.strategy !== 'custom') {
      state.studio.strategy = 'custom';
      el.studioStrategyBar.querySelectorAll('.strat-btn').forEach((btn) => {
        btn.classList.toggle('active', btn.getAttribute('data-strat') === 'custom');
      });
      el.studioModeBadge.textContent = STUDIO_MODE_LABELS.custom;
    }
  });

  document.getElementById('mcp-close').addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-close-btn').addEventListener('click', closeMCPConnectModal);
  document.getElementById('mcp-copy-json').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(buildMCPConfigText());
      showToast('MCP konfigürasyonu panoya kopyalandı', 'ok');
    } catch {
      showToast('Pano izni verilemedi', 'warn');
    }
  });

  document.getElementById('btn-reset-sim').addEventListener('click', async () => {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.reset()
        : await apiCall('/api/simulation/reset', 'POST', {});
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      state.localRunOutput = {};
      applySnapshot(snap);
      showToast('Kokpit simülasyonu başlangıç durumuna döndürüldü', 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('btn-theme').addEventListener('click', () => {
    const root = document.documentElement;
    const next = root.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
    if (next === 'dark') {
      root.removeAttribute('data-theme');
    } else {
      root.setAttribute('data-theme', 'light');
    }
  });

  // Pan & Zoom
  document.getElementById('zoom-in').addEventListener('click', () => {
    state.zoom = Math.min(2.2, +(state.zoom + 0.15).toFixed(2));
    renderSVG();
  });
  document.getElementById('zoom-out').addEventListener('click', () => {
    state.zoom = Math.max(0.55, +(state.zoom - 0.15).toFixed(2));
    renderSVG();
  });
  document.getElementById('zoom-reset').addEventListener('click', () => {
    state.zoom = 1;
    state.panX = 0;
    state.panY = 0;
    renderSVG();
  });

  el.viewport.addEventListener('wheel', (ev) => {
    ev.preventDefault();
    const delta = ev.deltaY < 0 ? 0.08 : -0.08;
    state.zoom = Math.min(2.2, Math.max(0.55, +(state.zoom + delta).toFixed(2)));
    renderSVG();
  }, { passive: false });

  el.viewport.addEventListener('pointerdown', (ev) => {
    if (ev.target.closest('.fo-capsule') || ev.target.closest('.svg-node-circle')) return;
    state.panning = { x: ev.clientX, y: ev.clientY, panX: state.panX, panY: state.panY };
    el.viewport.classList.add('panning');
  });

  bootstrap();
})();

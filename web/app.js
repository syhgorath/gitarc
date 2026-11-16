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
    filter: 'all', // 'all' | 'issues' | 'ci_fail' | 'lower' | 'gone'
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
      promoteBranch(branchId) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b) throw new Error('Dal bulunamadı');
        b.hemisphere = 'upper';
        b.hasRemote = true;
        b.remoteRef = 'origin/' + b.name;
        if (!b.pr) {
          b.pr = { number: nextPr++, title: 'feat: ' + b.name, author: 'ziya', reviewState: 'REVIEW_REQUIRED', ci: { state: 'SUCCESS', workflow: 'ci.yml', duration: '1m 14s' } };
        }
        return recompute();
      },
      resolveConflict(branchId) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (b?.conflict) {
          b.conflict.hasConflict = false;
          b.conflict.resolved = true;
          b.conflict.resolvedNote = 'Union merge uygulandı; her iki tarafın girdileri korundu.';
          b.conflict.previewUnified = '✓ CHANGELOG.md & components.lock birleştirildi.\n✓ git merge-tree --write-tree: exit 0';
          b.behindMain = 0;
        }
        return recompute();
      },
      deleteBranch(branchId) {
        sim.branches = sim.branches.filter((b) => b.id !== branchId);
        return recompute();
      },
      pruneGone() {
        const before = sim.branches.length;
        sim.branches = sim.branches.filter((b) => !b.originGone && b.state !== 'gone' && b.state !== 'merged');
        return { snapshot: recompute(), removed: before - sim.branches.length };
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

  function showToast(msg, tone = 'info') {
    const t = document.createElement('div');
    t.className = `toast ${tone}`;
    t.textContent = msg;
    el.toastStack.appendChild(t);
    setTimeout(() => {
      t.classList.add('fade-out');
      setTimeout(() => t.remove(), 260);
    }, 3100);
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
      b.id, b.state, b.hemisphere, b.isHead ? 1 : 0, b.aheadMain, b.behindMain,
      b.uncommittedCount, b.lastCommitMsg, b.originGone ? 1 : 0,
      b.pr?.title || '', b.pr?.body || '', b.pr?.ci?.state || '',
      b.conflict?.hasConflict ? 1 : 0,
    ].join('|')).join(';');
    return `${r.id}:${r.ahead}:${r.behind}:${r.dirtyCount}:${snap.nodes?.length || 0}:${lastNode}:${bSig}`;
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
    if (state.offlineMode || state.drag || state.panning || !el.modal.hidden) return;
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

    const cleanableCount = (snap.branches || []).filter((b) => b.originGone || b.state === 'gone' || (snap.repo.mode !== 'live' && b.state === 'merged')).length;
    el.pruneCount.textContent = String(cleanableCount);
    document.getElementById('btn-prune-gone').disabled = cleanableCount === 0 && snap.repo.mode !== 'live';
    document.getElementById('btn-new-branch').disabled = false;
    document.getElementById('btn-sync-spine').disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;
    document.getElementById('btn-reset-sim').hidden = snap.repo.mode === 'live';

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

  function renderKPIs() {
    const branches = state.snapshot?.branches || [];
    const upperCount = branches.filter((b) => b.hemisphere === 'upper' && b.state !== 'merged').length;
    const lowerCount = branches.filter((b) => b.hemisphere === 'lower' && b.state !== 'merged').length;
    const conflictCount = branches.filter((b) => (b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length > 0)).length;
    const failCount = branches.filter((b) => b.pr?.ci?.state === 'FAILURE').length;
    const goneCount = branches.filter((b) => b.originGone || b.state === 'gone' || b.state === 'merged').length;

    const defs = [
      { key: 'all', label: 'Tümü', val: branches.length, tone: '' },
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
    switch (state.filter) {
      case 'lower':
        return b.hemisphere === 'lower';
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
    if (b.state === 'merged') return 'merged';
    if (b.originGone || b.state === 'gone') return 'gone';
    return b.hemisphere === 'lower' ? 'lower-open' : 'upper-open';
  }

  function branchStrokeColor(b) {
    if (b.pr?.ci?.state === 'FAILURE') return '#f85149';
    if (b.conflict?.hasConflict && !b.conflict.resolved) return '#f97316';
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
      const gone = Boolean(b.originGone || b.hasLocal);
      pill.className = `cap-merged-pill ${isSelected ? 'selected' : ''} ${gone ? 'gone' : ''}`;
      const prTxt = b.pr ? `#${b.pr.number} ` : '';
      pill.innerHTML = `
        <span class="cap-merged-txt">✓ ${esc(prTxt + truncate(b.name, 18))}</span>
        <button type="button" class="cap-merged-del" data-act="delete" title="Kalıntı yerel dalı temizle">🧹</button>
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
    const hasConflict = Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
    const isLive = state.snapshot?.repo?.mode === 'live';

    let badgeTone = 'sky';
    let badgeText = b.pr ? `PR #${b.pr.number} · ${b.pr.reviewState}` : 'Remote Ray';
    if (isFailed) {
      badgeTone = 'danger';
      badgeText = `PR #${b.pr.number} · CI Failed`;
    } else if (hasConflict) {
      badgeTone = 'warn';
      badgeText = b.pr ? `PR #${b.pr.number} · Çakışma` : 'Merge Çakışması';
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
    if (hasConflict) {
      alertsHtml += `<span class="cap-alert-pill conflict">${b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lock' : '⚠ Conflict'}</span>`;
    }
    if (b.crossPr?.length) {
      alertsHtml += `<span class="cap-alert-pill cross">⇄ Cross-PR ${b.crossPr.map((c) => '#' + (c.otherPrNumber || '?')).join(',')}</span>`;
    }

    const isWorkingTreeWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-');
    const checkoutBtn = (!b.isHead && !isWorkingTreeWip)
      ? `<button type="button" class="cap-btn-ico" data-act="checkout" title="Bu dala geç (git switch)">🎯</button>`
      : '';

    let actionRowHtml = '';
    if (isFailed) {
      actionRowHtml = `
        <button type="button" class="cap-btn danger" data-act="inspect">🔍 Hatayı Gör</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="Dalı Sil">🗑️</button>
      `;
    } else if (hasConflict && !isLive) {
      actionRowHtml = `
        <button type="button" class="cap-btn amber" data-act="resolve">✦ Auto-Resolve</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="Dalı Sil">🗑️</button>
      `;
    } else if (isLower) {
      if (isWorkingTreeWip) {
        actionRowHtml = `<button type="button" class="cap-btn sky" data-act="inspect">🔍 Yerel Farkı Gör</button>`;
      } else {
        actionRowHtml = `
          <button type="button" class="cap-btn sky" data-act="promote">🚀 Push &amp; PR</button>
          <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge</button>
          ${checkoutBtn}
          <button type="button" class="cap-btn-del" data-act="delete" title="Dalı Sil">🗑️</button>
        `;
      }
    } else {
      actionRowHtml = `
        <button type="button" class="cap-btn emerald" data-act="merge">⚡ Merge (Parabolü Kapat)</button>
        ${checkoutBtn}
        <button type="button" class="cap-btn-del" data-act="delete" title="Dalı Sil">🗑️</button>
      `;
    }

    card.innerHTML = `
      <div class="cap-top-row">
        <span class="cap-badge ${badgeTone}">${esc(badgeText)}</span>
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
        else if (act === 'resolve') handleResolveConflict(b.id);
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
    const isVirtualWip = b.id === 'local-wip-working-tree' || b.id.startsWith('local-wip-');
    const ciState = b.pr?.ci?.state;
    const canPromote = isOpen && b.hemisphere === 'lower' && !isVirtualWip;
    const canMerge = isOpen && !hasConflict && !isVirtualWip && ciState !== 'FAILURE';

    const ciChip = ciState === 'FAILURE' ? '<span class="chip danger">CI: FAILURE</span>'
      : ciState === 'RUNNING' ? '<span class="chip accent">CI: RUNNING</span>'
        : ciState === 'SUCCESS' ? '<span class="chip ok">CI: PASS</span>' : '';

    let html = `
      <div class="det-header">
        <div class="det-title-row">
          <h3 class="det-branch-name">${esc(b.name)}</h3>
          <span class="chip ${b.state === 'merged' ? 'ok' : (b.hemisphere === 'lower' ? 'purple' : 'accent')}">${b.state === 'merged' ? 'Tam Parabol' : b.hemisphere === 'lower' ? 'Alt Ray · Local' : 'Üst Ray · PR'}</span>
        </div>
        <div class="det-badges">
          ${b.pr ? `<span class="chip accent">PR #${b.pr.number}</span>` : ''}
          ${ciChip}
          ${b.isHead ? '<span class="chip ok">HEAD</span>' : ''}
          ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
        </div>
        <div class="det-actions">
          ${!b.isHead && isOpen && !isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-checkout">🎯 Dala Geç</button>` : ''}
          ${canPromote ? `<button type="button" class="btn primary sm" id="act-promote">🚀 Push &amp; PR Aç</button>` : ''}
          ${isOpen && !isVirtualWip ? `
            <div class="merge-group">
              <select id="merge-strategy" class="select-inline" aria-label="Merge stratejisi">
                <option value="merge" ${state.mergeStrategy === 'merge' ? 'selected' : ''}>Merge</option>
                <option value="squash" ${state.mergeStrategy === 'squash' ? 'selected' : ''}>Squash</option>
                <option value="rebase" ${state.mergeStrategy === 'rebase' ? 'selected' : ''}>Rebase</option>
              </select>
              <button type="button" class="btn ok sm" id="act-merge" ${canMerge ? '' : 'disabled'}>⚡ Birleştir</button>
            </div>
          ` : ''}
          ${!isLive && hasConflict ? `<button type="button" class="btn warn sm" id="act-resolve">✦ Auto-Resolve</button>` : ''}
          ${!isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-delete">🧹 Sil / Temizle</button>` : ''}
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
            <div class="layer-line">${b.originGone ? 'Remote silindi (origin: gone)' : 'Henüz PR / Remote yok'}</div>
            <div class="layer-line">Alt rayda yerel çalışma</div>
          `}
        </div>
        <div class="layer-box">
          <h3>Alt Ray · Local</h3>
          <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
          <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
          <div class="layer-line">Uncommitted: ${b.uncommittedCount || 0} dosya</div>
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
          ${!resolved && !isLive ? `<div class="det-actions"><button type="button" class="btn warn sm" id="act-resolve-card">✦ Her İki Tarafı Koru (Auto-Resolve)</button></div>` : ''}
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
    document.getElementById('merge-strategy')?.addEventListener('change', (ev) => {
      state.mergeStrategy = ev.target.value;
    });
    document.getElementById('act-merge')?.addEventListener('click', () => handleMergeBranch(b.id));
    document.getElementById('act-resolve')?.addEventListener('click', () => handleResolveConflict(b.id));
    document.getElementById('act-resolve-card')?.addEventListener('click', () => handleResolveConflict(b.id));
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

  async function handleMergeBranch(branchId) {
    const strategy = state.mergeStrategy || 'merge';
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.mergeBranch(branchId, strategy)
        : await apiCall('/api/branch/merge', 'POST', { branchId, strategy });
      applySnapshot(snap);
      showToast(`Dal merkez main omurgasına (${strategy}) birleşti — Tam parabol tamamlandı!`, 'ok');
    } catch (err) {
      showToast(err.message, 'warn');
      renderSVG();
    }
  }

  async function handlePromoteBranch(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.promoteBranch(branchId)
        : await apiCall('/api/branch/promote', 'POST', { branchId });
      applySnapshot(snap);
      showToast('Dal origin’e pushlandı ve PR açılarak üst raya taşındı!', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  }

  async function handleResolveConflict(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.resolveConflict(branchId)
        : await apiCall('/api/branch/resolve', 'POST', { branchId });
      applySnapshot(snap);
      showToast('CHANGELOG / Lockfile çakışması otomatik çözüldü; artık merge edilebilir', 'ok');
    } catch (err) {
      showToast(err.message, 'danger');
    }
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
  document.getElementById('btn-sync-spine').addEventListener('click', handleSyncSpine);

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

(() => {
  'use strict';

  const SVG_NS = 'http://www.w3.org/2000/svg';
  const SPINE_Y = 340;
  const SPINE_START_X = 120;
  const SPINE_STEP_X = 148;
  const LANE_BASE_OFFSET = 98;
  const LANE_STEP_Y = 96;
  const CAPSULE_W = 224;
  const CAPSULE_H = 66;

  const state = {
    snapshot: null,
    selectedBranchId: null,
    selectedNodeSha: null,
    filter: 'all', // 'all' | 'issues' | 'ci_fail' | 'gone'
    geomMode: 'viaduct', // 'viaduct' | 'arch'
    offlineMode: false,
    zoom: 1,
    panX: 0,
    panY: 0,
    panning: null,
    drag: null,
    localRunOutput: {},
  };

  // DOM Elements
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
    zoomReset: document.getElementById('zoom-reset'),
    popover: document.getElementById('new-branch-popover'),
    newBranchForm: document.getElementById('new-branch-form'),
    newBranchName: document.getElementById('new-branch-name'),
    newBranchHemi: document.getElementById('new-branch-hemi'),
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
          { sha: '19a4c01', shortSha: '19a4c01', subject: 'chore(release): prepare v4.0.14 signed archive', author: 'ziya', relativeTime: '6 gün önce', spineIndex: 0, tag: 'v4.0.14' },
          { sha: '2f81d12', shortSha: '2f81d12', subject: 'refactor(runner): unify sudo askpass gate', author: 'ziya', relativeTime: '5 gün önce', spineIndex: 1 },
          { sha: '3c19e44', shortSha: '3c19e44', subject: 'Merge PR #412 feat/vault-idle-lock', author: 'ziya', relativeTime: '4 gün önce', spineIndex: 2, isMerge: true },
          { sha: '4b70a89', shortSha: '4b70a89', subject: 'docs(standard): document catalog items contract', author: 'ziya', relativeTime: '3 gün önce', spineIndex: 3, tag: 'v4.0.15' },
          { sha: '5e02b11', shortSha: '5e02b11', subject: 'Merge PR #415 fix/usb-exfat-update', author: 'ziya', relativeTime: '2 gün önce', spineIndex: 4, isMerge: true },
          { sha: '6d44f08', shortSha: '6d44f08', subject: 'feat(components): verify index_signers timestamp', author: 'ziya', relativeTime: '1 gün önce', spineIndex: 5 },
          { sha: '7a91c33', shortSha: '7a91c33', subject: 'Merge PR #418 feat/utility-items-catalog', author: 'ziya', relativeTime: '18 saat önce', spineIndex: 6, isMerge: true, tag: 'v4.0.16' },
          { sha: '8e35d02', shortSha: '8e35d02', subject: 'chore(lock): bump utility 0.2.0 sha256', author: 'ziya', relativeTime: '6 saat önce', spineIndex: 7 },
          { sha: '9f10b77', shortSha: '9f10b77', subject: 'test(csp): enforce style-src-attr none', author: 'ziya', relativeTime: '2 saat önce', spineIndex: 8, isHead: true },
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
      mergeBranch(branchId) {
        const b = sim.branches.find((x) => x.id === branchId);
        if (!b) throw new Error('Dal bulunamadı');
        if (b.conflict?.hasConflict && !b.conflict.resolved) {
          throw new Error(`${b.name} dalında çözülmemiş çakışma var; önce Auto-Resolve çalıştırın`);
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
      pruneGone() {
        const before = sim.branches.length;
        sim.branches = sim.branches.filter((b) => !b.originGone && b.state !== 'gone');
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

  // Network / API helper with graceful fallback
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
      t.remove();
    }, 3400);
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

  async function bootstrap() {
    try {
      const snap = await apiCall('/api/snapshot');
      applySnapshot(snap);
    } catch {
      state.offlineMode = true;
      const snap = LocalFallbackEngine.snapshot();
      applySnapshot(snap);
      showToast('Sunucusuz tarayıcı simülasyonu aktif (Offline Mode)', 'info');
    }
  }

  function applySnapshot(snap) {
    state.snapshot = snap;
    if (!state.selectedBranchId && snap.branches?.length) {
      const priority = snap.branches.find((b) => b.pr?.ci?.state === 'FAILURE')
        || snap.branches.find((b) => b.conflict?.hasConflict)
        || snap.branches.find((b) => b.state === 'open')
        || snap.branches[0];
      state.selectedBranchId = priority.id;
    } else if (state.selectedBranchId && !snap.branches.some((b) => b.id === state.selectedBranchId)) {
      state.selectedBranchId = snap.branches[0]?.id || null;
    }

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

    el.repoSelect.innerHTML = '';
    (snap.repos || []).forEach((r) => {
      const opt = document.createElement('option');
      opt.value = r.id;
      opt.textContent = r.name;
      if (r.id === snap.repo.id) opt.selected = true;
      el.repoSelect.appendChild(opt);
    });

    const goneCount = (snap.branches || []).filter((b) => b.originGone || b.state === 'gone').length;
    el.pruneCount.textContent = String(goneCount);
    document.getElementById('btn-prune-gone').disabled = goneCount === 0 && snap.repo.mode !== 'live';
    document.getElementById('btn-new-branch').disabled = false;
    document.getElementById('btn-sync-spine').disabled = snap.repo.mode !== 'live' && snap.repo.ahead === 0 && snap.repo.behind === 0;
    document.getElementById('btn-reset-sim').hidden = snap.repo.mode === 'live';
  }

  function renderKPIs() {
    const branches = state.snapshot?.branches || [];
    const openCount = branches.filter((b) => b.state === 'open').length;
    const conflictCount = branches.filter((b) => (b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length > 0)).length;
    const failCount = branches.filter((b) => b.pr?.ci?.state === 'FAILURE').length;
    const goneCount = branches.filter((b) => b.originGone || b.state === 'gone').length;

    const defs = [
      { key: 'all', label: 'Tümü', val: branches.length, tone: '' },
      { key: 'issues', label: 'Çakışma / Cross-PR', val: conflictCount, tone: conflictCount ? 'warn' : '' },
      { key: 'ci_fail', label: 'CI Hatası', val: failCount, tone: failCount ? 'danger' : '' },
      { key: 'gone', label: 'Temizlenebilir (gone)', val: goneCount, tone: goneCount ? 'ok' : '' },
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

    const openBadge = document.createElement('span');
    openBadge.className = 'kpi-pill';
    openBadge.innerHTML = `<span>Açık Ray</span><strong>${openCount}</strong>`;
    el.kpiPills.appendChild(openBadge);
  }

  function matchesFilter(b) {
    switch (state.filter) {
      case 'issues':
        return Boolean((b.conflict?.hasConflict && !b.conflict.resolved) || (b.crossPr && b.crossPr.length));
      case 'ci_fail':
        return b.pr?.ci?.state === 'FAILURE';
      case 'gone':
        return Boolean(b.originGone || b.state === 'gone');
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
        state.selectedBranchId = d.branchId;
        state.selectedNodeSha = null;
        renderSVG();
        renderInspector();
      });
      el.radarList.appendChild(item);
    });
  }

  function xOfSpine(idx) {
    return SPINE_START_X + idx * SPINE_STEP_X;
  }

  function yOfLane(hemisphere, lane) {
    const l = Math.max(1, lane || 1);
    const offset = LANE_BASE_OFFSET + (l - 1) * LANE_STEP_Y;
    return hemisphere === 'lower' ? SPINE_Y + offset : SPINE_Y - offset;
  }

  function buildArcPath(b, tipXOverride = null, tipYOverride = null) {
    const x0 = xOfSpine(b.startSpineIndex);
    const y0 = SPINE_Y;
    const yLane = yOfLane(b.hemisphere, b.lane);

    if (b.state === 'merged' && tipXOverride === null) {
      const xEnd = xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex));
      if (state.geomMode === 'arch') {
        const span = xEnd - x0;
        const yc = y0 + (yLane - y0) * 1.333;
        return {
          d: `M ${x0},${y0} C ${x0 + span * 0.22},${yc} ${xEnd - span * 0.22},${yc} ${xEnd},${y0}`,
          peakX: (x0 + xEnd) / 2,
          peakY: yLane,
          tipX: xEnd,
          tipY: y0,
        };
      }
      const xRise = Math.min(x0 + 62, (x0 + xEnd) / 2);
      const xFall = Math.max(xEnd - 62, (x0 + xEnd) / 2);
      return {
        d: `M ${x0},${y0} C ${x0 + 34},${y0} ${xRise - 28},${yLane} ${xRise},${yLane} L ${xFall},${yLane} C ${xFall + 28},${yLane} ${xEnd - 34},${y0} ${xEnd},${y0}`,
        peakX: (xRise + xFall) / 2,
        peakY: yLane,
        tipX: xEnd,
        tipY: y0,
      };
    }

    const defaultTipX = Math.max(x0 + 115, xOfSpine(Math.max(b.startSpineIndex + 1, b.endSpineIndex)) - 20);
    const tipX = tipXOverride !== null ? tipXOverride : defaultTipX;
    const tipY = tipYOverride !== null ? tipYOverride : yLane;

    if (state.geomMode === 'arch' || tipXOverride !== null) {
      const dx = tipX - x0;
      return {
        d: `M ${x0},${y0} C ${x0 + dx * 0.42},${y0} ${tipX - dx * 0.42},${tipY} ${tipX},${tipY}`,
        peakX: x0 + dx * 0.55,
        peakY: (y0 + tipY) / 2,
        tipX,
        tipY,
      };
    }

    const xRise = Math.min(x0 + 66, tipX - 18);
    return {
      d: `M ${x0},${y0} C ${x0 + 36},${y0} ${xRise - 28},${tipY} ${xRise},${tipY} L ${tipX},${tipY}`,
      peakX: (xRise + tipX) / 2,
      peakY: tipY,
      tipX,
      tipY,
    };
  }

  function renderSVG() {
    const snap = state.snapshot;
    if (!snap) return;

    const nodes = snap.nodes || [];
    const branches = snap.branches || [];
    const maxSpineIdx = Math.max(nodes.length - 1, ...branches.map((b) => b.endSpineIndex || 0), 6);
    const sceneW = Math.max(1240, SPINE_START_X + (maxSpineIdx + 2) * SPINE_STEP_X + 120);
    const sceneH = 680;

    const viewW = sceneW / state.zoom;
    const viewH = sceneH / state.zoom;
    el.svg.setAttribute('viewBox', `${state.panX} ${state.panY} ${viewW} ${viewH}`);
    el.zoomReset.textContent = `${Math.round(state.zoom * 100)}%`;

    el.svg.innerHTML = '';

    // 1. Hemisphere background bands
    el.svg.appendChild(svgEl('rect', { x: -400, y: -200, width: sceneW + 800, height: SPINE_Y + 200, class: 'svg-hemisphere-band-upper' }));
    el.svg.appendChild(svgEl('rect', { x: -400, y: SPINE_Y, width: sceneW + 800, height: sceneH - SPINE_Y + 400, class: 'svg-hemisphere-band-lower' }));

    // 2. Central Spine Drop Zone (shown while dragging a capsule)
    const dropZone = svgEl('rect', {
      id: 'spine-dropzone',
      x: 40,
      y: SPINE_Y - 38,
      width: sceneW - 80,
      height: 76,
      rx: 18,
      class: 'svg-spine-dropzone',
    });
    el.svg.appendChild(dropZone);

    const dropLabel = svgEl('text', {
      id: 'spine-drop-label',
      x: sceneW / 2,
      y: SPINE_Y - 46,
      class: 'svg-spine-drop-label',
    }, '');
    el.svg.appendChild(dropLabel);

    // 3. Central Main Spine Line + Sync Skew Capsule
    const spineStartX = 64;
    const spineEndX = xOfSpine(maxSpineIdx) + 90;
    el.svg.appendChild(svgEl('line', {
      x1: spineStartX,
      y1: SPINE_Y,
      x2: spineEndX,
      y2: SPINE_Y,
      class: 'svg-spine-line',
    }));

    const syncX = spineEndX - 18;
    const ahead = snap.repo.ahead || 0;
    const behind = snap.repo.behind || 0;
    const remoteName = snap.repo.remoteName || 'origin';
    const defBranch = snap.repo.defaultBranch || 'main';
    const syncText = (ahead === 0 && behind === 0)
      ? `${remoteName}/${defBranch} ✓ eşit`
      : `${remoteName}/${defBranch}  ↑${ahead} · ↓${behind}`;
    const syncW = Math.max(152, syncText.length * 7.2 + 20);
    const syncGroup = svgEl('g', {});
    const syncRect = svgEl('rect', {
      x: syncX,
      y: SPINE_Y - 14,
      width: syncW,
      height: 28,
      rx: 14,
      class: `svg-sync-pill-bg ${(ahead > 0 || behind > 0) ? 'skewed' : ''}`,
    });
    syncRect.addEventListener('click', () => handleSyncSpine());
    syncGroup.appendChild(syncRect);
    syncGroup.appendChild(svgEl('text', { x: syncX + syncW / 2, y: SPINE_Y + 4, class: 'svg-sync-pill-txt' }, syncText));
    el.svg.appendChild(syncGroup);

    // 4. Merged Arches first (background layer), then Open Parabolas (foreground layer)
    const arcLayer = svgEl('g', { id: 'arc-layer' });
    const capsuleLayer = svgEl('g', { id: 'capsule-layer' });
    const badgeLayer = svgEl('g', { id: 'badge-layer' });

    const ordered = [...branches].sort((a, b) => (a.state === 'merged' ? 0 : 1) - (b.state === 'merged' ? 0 : 1));
    ordered.forEach((b) => {
      const geom = buildArcPath(b);
      const visible = matchesFilter(b);
      const isSelected = state.selectedBranchId === b.id;
      const isFail = b.pr?.ci?.state === 'FAILURE';
      const hasConf = b.conflict?.hasConflict && !b.conflict.resolved;

      const classes = ['svg-arc-path'];
      if (b.state === 'merged') classes.push('merged');
      else if (b.originGone || b.state === 'gone') classes.push('gone');
      else if (b.hemisphere === 'lower') classes.push('lower-open');
      else classes.push('upper-open');

      if (hasConf) classes.push('conflict');
      if (isFail) classes.push('ci-fail');
      if (isSelected) classes.push('selected');
      if (!visible) classes.push('dimmed');

      const pathEl = svgEl('path', { d: geom.d, class: classes.join(' ') });
      pathEl.addEventListener('click', (ev) => {
        ev.stopPropagation();
        selectBranch(b.id);
      });
      arcLayer.appendChild(pathEl);

      // Merged arch crest label
      if (b.state === 'merged') {
        const labelTxt = `${b.pr ? '#' + b.pr.number + ' ' : ''}${truncate(b.name, 22)}${b.originGone ? ' · gone' : ''}`;
        const lw = Math.max(120, labelTxt.length * 6.6 + 18);
        const crestG = svgEl('g', { class: visible ? '' : 'dimmed' });
        const crestRect = svgEl('rect', {
          x: geom.peakX - lw / 2,
          y: geom.peakY - 12,
          width: lw,
          height: 22,
          rx: 11,
          class: `svg-arch-label-bg ${b.originGone ? 'gone' : ''}`,
        });
        crestRect.addEventListener('click', (ev) => {
          ev.stopPropagation();
          selectBranch(b.id);
        });
        crestG.appendChild(crestRect);
        crestG.appendChild(svgEl('text', { x: geom.peakX, y: geom.peakY + 2.5, class: 'svg-arch-label-txt' }, labelTxt));
        badgeLayer.appendChild(crestG);
        return;
      }

      // Floating conflict & Cross-PR badges along the parabola
      let badgeOffsetY = b.hemisphere === 'lower' ? 22 : -22;
      if (hasConf) {
        const txt = b.conflict.isChangelogOrLock ? '⚠ CHANGELOG / Lockfile Çakışması' : '⚠ Merge Çakışması';
        const bw = txt.length * 6.6 + 18;
        const bx = geom.peakX - 18;
        const by = geom.peakY + badgeOffsetY;
        const bg = svgEl('rect', { x: bx - bw / 2, y: by - 11, width: bw, height: 20, rx: 10, class: 'arc-warn-badge-bg' });
        bg.addEventListener('click', (ev) => { ev.stopPropagation(); selectBranch(b.id); });
        badgeLayer.appendChild(bg);
        badgeLayer.appendChild(svgEl('text', { x: bx, y: by + 2.5, class: 'arc-warn-badge-txt' }, txt));
        badgeOffsetY += b.hemisphere === 'lower' ? 22 : -22;
      }
      if (b.crossPr?.length) {
        const peers = b.crossPr.map((c) => c.otherPrNumber ? '#' + c.otherPrNumber : c.otherBranch).join(', ');
        const txt = `⇄ Cross-PR ${peers}`;
        const bw = txt.length * 6.6 + 18;
        const bx = geom.peakX + 26;
        const by = geom.peakY + badgeOffsetY;
        const bg = svgEl('rect', { x: bx - bw / 2, y: by - 11, width: bw, height: 20, rx: 10, class: 'arc-cross-badge-bg' });
        bg.addEventListener('click', (ev) => { ev.stopPropagation(); selectBranch(b.id); });
        badgeLayer.appendChild(bg);
        badgeLayer.appendChild(svgEl('text', { x: bx, y: by + 2.5, class: 'arc-cross-badge-txt' }, txt));
      }

      // Unified Branch Pair Capsule at tip
      const capX = geom.tipX;
      const capY = geom.tipY - CAPSULE_H / 2;
      const capG = buildBranchPairCapsule(b, capX, capY, isSelected, visible, isFail, hasConf);
      capsuleLayer.appendChild(capG);
    });

    el.svg.appendChild(arcLayer);

    // 5. Commit Nodes on the Central Spine
    const nodeLayer = svgEl('g', { id: 'node-layer' });
    nodes.forEach((n) => {
      const cx = xOfSpine(n.spineIndex);
      const cy = SPINE_Y;
      const cls = ['svg-node-circle'];
      if (n.isMerge) cls.push('merge-node');
      if (n.isHead) cls.push('head-node');

      const circle = svgEl('circle', {
        cx,
        cy,
        r: n.isMerge ? 8 : 6.5,
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
        y: cy + 23,
        class: 'svg-node-sha',
      }, n.shortSha));

      if (n.tag) {
        const tw = n.tag.length * 6.8 + 14;
        nodeLayer.appendChild(svgEl('rect', {
          x: cx - tw / 2,
          y: cy - 31,
          width: tw,
          height: 17,
          rx: 5,
          class: 'svg-node-tag-bg',
        }));
        nodeLayer.appendChild(svgEl('text', {
          x: cx,
          y: cy - 19,
          class: 'svg-node-tag-txt',
        }, n.tag));
      }
    });

    el.svg.appendChild(nodeLayer);
    el.svg.appendChild(capsuleLayer);
    el.svg.appendChild(badgeLayer);

    // Drag preview curve layer
    el.svg.appendChild(svgEl('path', { id: 'drag-preview-path', class: 'svg-drag-preview', d: '' }));
  }

  function buildBranchPairCapsule(b, x, y, isSelected, visible, isFail, hasConf) {
    const g = svgEl('g', {
      class: `capsule-group ${visible ? '' : 'dimmed'}`,
      transform: `translate(${x}, ${y})`,
      'data-branch-id': b.id,
    });

    const cardCls = ['capsule-card'];
    if (isSelected) cardCls.push('selected');
    if (isFail) cardCls.push('ci-fail');
    if (hasConf) cardCls.push('conflict');

    // Outer card
    g.appendChild(svgEl('rect', {
      x: 0,
      y: 0,
      width: CAPSULE_W,
      height: CAPSULE_H,
      rx: 10,
      class: cardCls.join(' '),
    }));

    // Upper background band (Remote / PR / CI)
    g.appendChild(svgEl('rect', {
      x: 1,
      y: 1,
      width: CAPSULE_W - 2,
      height: 31,
      rx: 9,
      class: 'cap-upper-bg',
    }));

    // Divider line between Remote (top) and Local (bottom)
    g.appendChild(svgEl('line', {
      x1: 0,
      y1: 33,
      x2: CAPSULE_W,
      y2: 33,
      class: 'capsule-divider',
    }));

    // Upper Layer Content: PR & CI
    if (b.pr) {
      g.appendChild(svgEl('text', { x: 10, y: 21, class: 'cap-txt-pr' }, `PR #${b.pr.number}`));

      // Review pill
      const revLabel = b.pr.reviewState === 'APPROVED' ? '✓ OK'
        : b.pr.reviewState === 'CHANGES_REQUESTED' ? '✎ Fix'
          : '◔ Rev';
      const revCls = b.pr.reviewState === 'APPROVED' ? 'cap-pill-ok'
        : b.pr.reviewState === 'CHANGES_REQUESTED' ? 'cap-pill-warn'
          : 'cap-pill-run';
      g.appendChild(svgEl('rect', { x: 64, y: 8, width: 46, height: 18, rx: 9, class: revCls }));
      g.appendChild(svgEl('text', { x: 87, y: 20, class: `${revCls}-txt` }, revLabel));

      // CI pill
      const ciState = b.pr.ci?.state || 'SUCCESS';
      const ciLabel = ciState === 'FAILURE' ? '● CI FAIL' : ciState === 'RUNNING' ? '◐ CI RUN' : '● CI PASS';
      const ciCls = ciState === 'FAILURE' ? 'cap-pill-fail' : ciState === 'RUNNING' ? 'cap-pill-run' : 'cap-pill-ok';
      g.appendChild(svgEl('rect', { x: 118, y: 8, width: 94, height: 18, rx: 9, class: ciCls }));
      g.appendChild(svgEl('text', { x: 165, y: 20, class: `${ciCls}-txt` }, ciLabel));
    } else if (b.originGone) {
      g.appendChild(svgEl('text', { x: 10, y: 21, class: 'cap-txt-muted' }, 'origin: gone (silinmiş)'));
    } else {
      g.appendChild(svgEl('text', { x: 10, y: 21, class: 'cap-txt-muted' }, 'local-only · PR açılmadı'));
    }

    // Lower Layer Content: Local branch, HEAD, dirty files
    g.appendChild(svgEl('text', { x: 10, y: 53, class: 'cap-txt-branch' }, truncate(b.name, 20)));

    let badgeRightX = CAPSULE_W - 8;
    if (b.uncommittedCount > 0) {
      const dt = `●${b.uncommittedCount} dirty`;
      const dw = 50;
      badgeRightX -= dw;
      g.appendChild(svgEl('rect', { x: badgeRightX, y: 40, width: dw, height: 18, rx: 9, class: 'cap-pill-dirty' }));
      g.appendChild(svgEl('text', { x: badgeRightX + dw / 2, y: 52, class: 'cap-pill-dirty-txt' }, dt));
      badgeRightX -= 5;
    }

    if (b.isHead) {
      const hw = 36;
      badgeRightX -= hw;
      g.appendChild(svgEl('rect', { x: badgeRightX, y: 40, width: hw, height: 18, rx: 9, class: 'cap-pill-head' }));
      g.appendChild(svgEl('text', { x: badgeRightX + hw / 2, y: 52, class: 'cap-pill-head-txt' }, 'HEAD'));
    } else if ((b.aheadMain || 0) > 0) {
      g.appendChild(svgEl('text', { x: badgeRightX - 24, y: 53, class: 'cap-txt-meta' }, `↑${b.aheadMain}`));
    }

    // Drag & Select Pointer Handling
    g.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      ev.stopPropagation();
      startCapsulePointer(ev, b, g);
    });

    return g;
  }

  function clientToSvg(clientX, clientY) {
    const pt = el.svg.createSVGPoint();
    pt.x = clientX;
    pt.y = clientY;
    const ctm = el.svg.getScreenCTM();
    if (!ctm) return { x: 0, y: 0 };
    return pt.matrixTransform(ctm.inverse());
  }

  function startCapsulePointer(ev, branch, groupEl) {
    const startPt = clientToSvg(ev.clientX, ev.clientY);
    const geom = buildArcPath(branch);
    const canDragMerge = branch.state === 'open';

    state.drag = {
      branch,
      groupEl,
      startClientX: ev.clientX,
      startClientY: ev.clientY,
      startSvgX: startPt.x,
      startSvgY: startPt.y,
      baseTipX: geom.tipX,
      baseTipY: geom.tipY,
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
      state.drag.groupEl.classList.add('dragging');
      document.getElementById('spine-dropzone')?.classList.add('visible');
      const lbl = document.getElementById('spine-drop-label');
      if (lbl) lbl.textContent = `MERGE ETMEK İÇİN MAIN OMURGASINA BIRAK: ${state.drag.branch.name}`;
      el.dragHint.classList.add('active-drop');
    }

    if (state.drag.active) {
      const pt = clientToSvg(ev.clientX, ev.clientY);
      const dx = pt.x - state.drag.startSvgX;
      const dy = pt.y - state.drag.startSvgY;
      const curTipX = state.drag.baseTipX + dx;
      const curTipY = state.drag.baseTipY + dy;

      state.drag.groupEl.setAttribute('transform', `translate(${curTipX}, ${curTipY - CAPSULE_H / 2})`);
      const preview = buildArcPath(state.drag.branch, curTipX, curTipY);
      document.getElementById('drag-preview-path')?.setAttribute('d', preview.d);

      const overSpine = Math.abs(curTipY - SPINE_Y) < 60;
      state.drag.overSpine = overSpine;
      document.getElementById('spine-dropzone')?.classList.toggle('hot', overSpine);
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

    document.getElementById('spine-dropzone')?.classList.remove('visible', 'hot');
    const lbl = document.getElementById('spine-drop-label');
    if (lbl) lbl.textContent = '';
    document.getElementById('drag-preview-path')?.setAttribute('d', '');
    el.dragHint.classList.remove('active-drop');
    d.groupEl.classList.remove('dragging');

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
        `;
        return;
      }
    }

    const b = snap.branches.find((x) => x.id === state.selectedBranchId);
    if (!b) {
      el.detail.innerHTML = '<div class="empty-detail">Bir dal kapsülüne, parabol kemere veya radar uyarısına tıklayın.</div>';
      return;
    }

    const isLive = snap.repo.mode === 'live';
    const hasConflict = b.conflict?.hasConflict && !b.conflict.resolved;
    const isOpen = b.state === 'open';
    const canPromote = isOpen && b.hemisphere === 'lower';
    const canMerge = isOpen && !hasConflict;

    const ciState = b.pr?.ci?.state;
    const ciChip = ciState === 'FAILURE' ? '<span class="chip danger">CI: FAILURE</span>'
      : ciState === 'RUNNING' ? '<span class="chip accent">CI: RUNNING</span>'
        : ciState === 'SUCCESS' ? '<span class="chip ok">CI: PASS</span>' : '';

    let html = `
      <div class="det-header">
        <div class="det-title-row">
          <h3 class="det-branch-name">${esc(b.name)}</h3>
          <span class="chip ${b.state === 'merged' ? 'ok' : 'accent'}">${b.state === 'merged' ? 'Tam Kemer' : b.hemisphere === 'lower' ? 'Yerel Ray' : 'PR Rayı'}</span>
        </div>
        <div class="det-badges">
          ${b.pr ? `<span class="chip accent">PR #${b.pr.number}</span>` : ''}
          ${ciChip}
          ${b.isHead ? '<span class="chip ok">HEAD</span>' : ''}
          ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
        </div>
        <div class="det-actions">
          ${canPromote ? `<button type="button" class="btn primary sm" id="act-promote">↑ Push &amp; PR Aç</button>` : ''}
          ${isOpen ? `<button type="button" class="btn ok sm" id="act-merge" ${canMerge ? '' : 'disabled'}>⇄ Main'e Merge Et</button>` : ''}
          ${!isLive && hasConflict ? `<button type="button" class="btn warn sm" id="act-resolve">✦ Auto-Resolve</button>` : ''}
        </div>
      </div>

      <div class="det-grid">
        <div class="layer-box">
          <h3>Üst Katman · Remote / PR</h3>
          ${b.pr ? `
            <div class="layer-line">PR #${b.pr.number} · ${esc(b.pr.reviewState)}</div>
            <div class="layer-line">${esc(truncate(b.pr.title, 48))}</div>
            <div class="layer-line">CI: ${esc(b.pr.ci?.workflow || 'ci.yml')} (${esc(b.pr.ci?.duration || '—')})</div>
          ` : `
            <div class="layer-line">${b.originGone ? 'Remote silindi (origin: gone)' : 'Henüz pushlanmadı'}</div>
            <div class="layer-line">Alt yarı-kürede izole çalışma</div>
          `}
        </div>
        <div class="layer-box">
          <h3>Alt Katman · Local</h3>
          <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
          <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
          <div class="layer-line">Uncommitted: ${b.uncommittedCount || 0} dosya</div>
        </div>
      </div>
    `;

    // Conflict Card (git merge-tree --write-tree)
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

    // Cross-PR Collision Card
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

    // Noise-Free CI Failure Log Extractor
    if (b.pr?.ci?.state === 'FAILURE') {
      const extracts = b.pr.ci.extracts || [];
      const firstRepro = extracts.find((x) => x.reproCmd)?.reproCmd || 'go test -v ./...';
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
            <button type="button" class="btn sm" id="act-copy-log">Logu Kopyala</button>
            <button type="button" class="btn primary sm" id="act-run-local">▶ Yerelde Testi Çalıştır</button>
          </div>
          <pre class="terminal-box" id="ci-raw-log">${esc(localRunOut || b.pr.ci.rawLog || 'Ham log bulunamadı.')}</pre>
        </div>
      `;
    }

    // Changed Files
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

    el.detail.innerHTML = html;

    // Bind detail action buttons
    document.getElementById('act-promote')?.addEventListener('click', () => handlePromoteBranch(b.id));
    document.getElementById('act-merge')?.addEventListener('click', () => handleMergeBranch(b.id));
    document.getElementById('act-resolve')?.addEventListener('click', () => handleResolveConflict(b.id));
    document.getElementById('act-resolve-card')?.addEventListener('click', () => handleResolveConflict(b.id));
    document.getElementById('act-copy-log')?.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(b.pr.ci.rawLog || '');
        showToast('CI hata logu panoya kopyalandı', 'ok');
      } catch {
        showToast('Pano izni verilemedi; logu seçip kopyalayabilirsiniz', 'warn');
      }
    });
    document.getElementById('act-run-local')?.addEventListener('click', () => {
      const cmd = b.pr.ci.extracts?.[0]?.reproCmd || 'go test -v ./internal/shellgold';
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
  async function handleMergeBranch(branchId) {
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.mergeBranch(branchId)
        : await apiCall('/api/branch/merge', 'POST', { branchId });
      applySnapshot(snap);
      showToast('Dal merkez main omurgasına merge edildi (tam parabol kemer!)', 'ok');
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
      showToast('Dal origin’e pushlandı ve PR açılarak üst yarı-küreye taşındı', 'ok');
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

  // Toolbar & Viewport Events
  el.repoSelect.addEventListener('change', async () => {
    const repoId = el.repoSelect.value;
    if (state.offlineMode) return;
    try {
      const snap = await apiCall('/api/repo/switch', 'POST', { repoId });
      state.selectedBranchId = null;
      state.selectedNodeSha = null;
      applySnapshot(snap);
      showToast(`Aktif depo: ${snap.repo.name}`, 'info');
    } catch (err) {
      showToast(err.message, 'danger');
    }
  });

  document.getElementById('geom-viaduct').addEventListener('click', (ev) => {
    state.geomMode = 'viaduct';
    ev.currentTarget.classList.add('active');
    document.getElementById('geom-arch').classList.remove('active');
    renderSVG();
  });

  document.getElementById('geom-arch').addEventListener('click', (ev) => {
    state.geomMode = 'arch';
    ev.currentTarget.classList.add('active');
    document.getElementById('geom-viaduct').classList.remove('active');
    renderSVG();
  });

  document.getElementById('btn-new-branch').addEventListener('click', () => {
    el.popover.hidden = !el.popover.hidden;
    if (!el.popover.hidden) {
      el.newBranchName.focus();
    }
  });

  document.getElementById('pop-close').addEventListener('click', () => { el.popover.hidden = true; });
  document.getElementById('pop-cancel').addEventListener('click', () => { el.popover.hidden = true; });

  el.newBranchForm.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const name = el.newBranchName.value.trim();
    const hemisphere = el.newBranchHemi.value;
    const fromSha = el.newBranchBase.value;
    if (!name) return;
    try {
      const snap = state.offlineMode
        ? LocalFallbackEngine.createBranch(name, hemisphere, fromSha)
        : await apiCall('/api/branch/create', 'POST', { name, hemisphere, fromSha });
      el.popover.hidden = true;
      el.newBranchName.value = '';
      const created = snap.branches.find((b) => b.name === name);
      if (created) state.selectedBranchId = created.id;
      applySnapshot(snap);
      showToast(`${name} dalı ${hemisphere === 'upper' ? 'üst' : 'alt'} yarı-kürede açıldı`, 'ok');
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
    if (ev.target.closest('.capsule-group') || ev.target.closest('.svg-node-circle') || ev.target.closest('.svg-sync-pill-bg')) return;
    state.panning = { x: ev.clientX, y: ev.clientY, panX: state.panX, panY: state.panY };
    el.viewport.classList.add('panning');
  });

  bootstrap();
})();

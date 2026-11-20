// web/modules/utils.js - Shared formatting, DOM, security and naming helpers
import { SVG_NS } from './constants.js';

// PR başlığı: "PR #12 · APPROVED". İnceleme kararı belirtilmemişse (GitHub boş döner: inceleme zorunlu değil / henüz karar yok)
// yalnızca "PR #12": "inceleme gerekli" uydurulmaz.
export function prHeadline(pr) {
  return `PR #${pr.number}${pr.reviewState ? ` · ${pr.reviewState}` : ''}`;
}

export function esc(str) {
  return String(str ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

export function truncate(str, max) {
  if (!str) return '';
  return str.length > max ? str.slice(0, max - 1) + '…' : str;
}

export function timeAgo(isoOrDate) {
  if (!isoOrDate) return '—';
  const d = new Date(isoOrDate);
  if (isNaN(d.getTime())) return String(isoOrDate);
  const sec = Math.floor((Date.now() - d.getTime()) / 1000);
  if (sec < 5) return 'az önce';
  if (sec < 60) return `${sec} sn önce`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} dk önce`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} sa önce`;
  const day = Math.floor(hr / 24);
  return `${day} gün önce`;
}

export function showToast(msg, tone = 'ok') {
  const stack = document.getElementById('toast-stack');
  if (!stack) return;
  const t = document.createElement('div');
  t.className = `toast ${tone}`;
  t.title = 'Metni panoya kopyalamak için tıklayın';
  t.textContent = msg;
  t.addEventListener('click', () => {
    navigator.clipboard?.writeText(msg).catch(() => {});
    t.classList.add('fade-out');
    setTimeout(() => t.remove(), 180);
  });
  stack.appendChild(t);
  const timeoutMs = tone === 'danger' ? 7500 : 3200;
  setTimeout(() => {
    t.classList.add('fade-out');
    setTimeout(() => t.remove(), 260);
  }, timeoutMs);
}

export async function runWithBtnBusy(btnEl, busyText, fn) {
  if (!btnEl) return await fn();
  // Düğmenin çocuk DÜĞÜMLERİNİ sakla (innerHTML değil): yazıyı geri yüklerken aynı düğümler dönmeli, yoksa
  // el kaydındaki rozet/sayaç elemanları (ör. #prune-count) DOM'dan kopar ve bir daha güncellenmez.
  const origNodes = Array.from(btnEl.childNodes);
  btnEl.disabled = true;
  btnEl.classList.add('btn-busy');
  btnEl.textContent = busyText || '⏳ İşleniyor...';

  const statusEl = document.getElementById('engine-status');
  const origStatus = statusEl ? statusEl.textContent : '';
  if (statusEl) {
    statusEl.textContent = '● Git İşlemi Yürütülüyor...';
    statusEl.classList.add('running-pulse');
  }

  try {
    return await fn();
  } finally {
    btnEl.disabled = false;
    btnEl.classList.remove('btn-busy');
    btnEl.replaceChildren(...origNodes);
    if (statusEl) {
      statusEl.textContent = origStatus || '● Hazır';
      statusEl.classList.remove('running-pulse');
    }
  }
}

export function svgEl(tag, attrs = {}, text = null) {
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

export function isMergedOrGone(b) {
  return Boolean(b.state === 'merged' || b.state === 'gone' || b.originGone);
}

export function isOpenPR(b) {
  return Boolean(b.pr && b.pr.number > 0 && b.state === 'open' && b.pr.reviewState !== 'MERGED');
}

export function isLocalOpen(b) {
  return Boolean(b.hasLocal && b.state === 'open' && !isMergedOrGone(b));
}

export function isMyBranch(b, currentUser = '') {
  const me = (currentUser || '').trim().toLowerCase();
  if (!me) return Boolean(b.isHead);
  const bAuth = (b.author || '').trim().toLowerCase();
  const prAuth = (b.pr?.author || '').trim().toLowerCase();
  return bAuth === me || prAuth === me || Boolean(b.isHead);
}

export function isSecretFile(f, snap = null) {
  if (!f) return false;
  const patterns = snap?.repo?.secretPatterns || [
    'img/logoyeni.jpg',
    'referans/**',
    'referans/*',
    'referans',
    '.mcp.json',
    '.env*',
    '*.env',
    '*.pem',
    '*.key',
    'id_*',
    'id_rsa*',
    'id_ed25519*',
  ];
  const clean = f.replace(/\\/g, '/').replace(/^\.\//, '').toLowerCase();
  const base = clean.split('/').pop();

  for (const p of patterns) {
    const pClean = p.replace(/\\/g, '/').replace(/^\.\//, '').toLowerCase();
    if (pClean.endsWith('/**') || pClean.endsWith('/*')) {
      const prefix = pClean.replace(/\/\*\*?$/, '');
      if (clean === prefix || clean.startsWith(prefix + '/')) return true;
    } else if (pClean.endsWith('/')) {
      if (clean.startsWith(pClean)) return true;
    } else if (clean === pClean) {
      return true;
    }
    if (!pClean.includes('/')) {
      if (pClean.includes('*')) {
        const reg = new RegExp('^' + pClean.replace(/\./g, '\\.').replace(/\*/g, '.*') + '$');
        if (reg.test(base)) return true;
      } else if (base === pClean) {
        return true;
      }
    }
    if (pClean.includes('*')) {
      const reg = new RegExp('^' + pClean.replace(/\./g, '\\.').replace(/\*/g, '.*') + '$');
      if (reg.test(clean)) return true;
    }
  }
  return false;
}

export function suggestUniqueBranchName(arg1, arg2, arg3) {
  let prefix = 'feat';
  let files = [];
  let snap = null;
  if (typeof arg1 === 'string') {
    prefix = arg1;
    files = Array.isArray(arg2) ? arg2 : [];
    snap = arg3;
  } else if (arg1 && typeof arg1 === 'object') {
    files = arg1.changedFiles || [];
    snap = arg2;
  }
  let slug = 'new-feature';
  if (files.length > 0) {
    const first = (files[0] || '').toLowerCase();
    if (first.endsWith('.md') || first.startsWith('docs/')) {
      prefix = 'docs';
    } else if (first.includes('test')) {
      prefix = 'test';
    } else if (first.endsWith('.css') || first.endsWith('.html')) {
      prefix = 'style';
    } else if (first.includes('gitignore') || first.endsWith('.json') || first.endsWith('.lock') || first.endsWith('.yml') || first.endsWith('.yaml')) {
      prefix = 'chore';
    }
    const rawBase = first.split('/').pop().replace(/^\./, '').replace(/\.[^.]+$/, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
    if (rawBase && rawBase !== 'index' && rawBase !== 'main') {
      slug = `${rawBase}-guncel`;
    }
  }
  const candidateBase = `${prefix}/${slug}`.slice(0, 40);
  const existing = new Set((snap?.branches || []).map((x) => (x.name || '').toLowerCase()));
  const matchesPrefixOrExact = (cand) => {
    const cLow = cand.toLowerCase();
    return Array.from(existing).some((ex) => ex === cLow || ex.startsWith(cLow + '-'));
  };

  if (!matchesPrefixOrExact(candidateBase)) {
    return candidateBase;
  }
  const shortSha = (snap?.nodes?.[snap.nodes.length - 1]?.shortSha || '').slice(0, 5);
  if (shortSha && !existing.has(`${candidateBase}-${shortSha}`.toLowerCase())) {
    return `${candidateBase}-${shortSha}`;
  }
  let n = 2;
  while (existing.has(`${candidateBase}-${n}`)) {
    n++;
  }
  return `${candidateBase}-${n}`;
}

export function snapshotSignature(snap) {
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
  // Veri toplama adımları ve koruma durumu değişince arayüz yenilensin (zaman damgası dışarıda: her anketi tetiklemesin).
  const syncSig = (r.syncSteps || []).map((s) => `${s.id}=${s.state}/${s.count || 0}`).join(',');
  return `${r.id}:${r.ahead}:${r.behind}:${r.dirtyCount}:${snap.nodes?.length || 0}:${lastNode}:${bSig}:${logSig}:${r.protectionState || ''}:${syncSig}:${r.mainCI?.state || ''}:${r.mainCI?.sha || ''}:${r.queueRun ? `${r.queueRun.merged}/${r.queueRun.current}/${r.queueRun.phase}` : ''}:${snap.pendingProposal?.id || ''}:${(snap.repos || []).map((x) => (x.pendingProposal ? 1 : 0)).join('')}`;
}

// Birleşmiş dalın birleşme (merge) commit'i: omurgada EndSpineIndex konumundaki düğüm.
export function mergeCommitOf(b, snap) {
  if (!b || b.state !== 'merged') return null;
  const nodes = snap?.nodes || [];
  return nodes.find((n) => n.spineIndex === b.endSpineIndex) || null;
}

// "Diff Gör" düğmelerinin tek kuralı: [hedef, tür]. Birleşmiş dalın diff'i dal adından (çoğu zaman uzakta/yerelde
// silinmiştir) değil, birleşme commit'inin getirdiği değişikliklerden alınır; çalışma alanı kendi türünü kullanır.
export function diffArgsFor(b, snap = null) {
  if (isWorkingTreeBranch(b)) return ['working-tree', 'working-tree'];
  const mc = mergeCommitOf(b, snap);
  if (mc && (mc.sha || mc.shortSha)) return [mc.sha || mc.shortSha, 'merge'];
  return [b?.name, 'branch'];
}

// Sanal "çalışma alanı" dalı mı? (kaydedilmemiş değişiklikleri temsil eden `local-wip-working-tree` ve ona benzer
// `local-wip-*` taslak dalları). Kapsül, stüdyo ve diff bu tek tanımı kullanır. Not: eskiden stüdyo, hiçbir yerde
// üretilmeyen `b.isWorkingTreeWip` alanına bakıyordu; bu yüzden çalışma alanı seçiliyken stüdyoda "Dala Aktar /
// Commit" düğmesi hiç görünmüyordu.
export function isWorkingTreeBranch(b) {
  return Boolean(b && (b.id === 'local-wip-working-tree' || (b.id || '').startsWith('local-wip-') || b.name === 'wip/working-tree'));
}

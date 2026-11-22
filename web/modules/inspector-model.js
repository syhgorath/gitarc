// web/modules/inspector-model.js - Sağ Inspector ve Teşhis Stüdyosu için saf kararlar (DOM yok, Node'da test edilir)
//
// Hangi kartın/butonun/metnin gösterileceği buradaki fonksiyonlardan gelir; HTML basımı inspector-cards.js ve
// studio.js'te, olay bağlama inspector.js / studio.js / proposal.js'te yapılır.
import { truncate, isMergedOrGone, suggestUniqueBranchName, prHeadline } from './utils.js';
import { needsUpdateBeforeMerge, queueRunBadge, ciPendingForMerge } from './syncsteps.js';

// ---------------------------------------------------------------------------
// CI kontrolleri: iki panelde (stüdyo ve inspector) tek sınıflandırma
// ---------------------------------------------------------------------------
// Eskiden iki ayrı if-zinciri vardı ve tutarsızdı: `PENDING` durumu inspector'da 🟡 (devam ediyor), stüdyoda ise
// "Başarılı" görünüyordu. Artık tek kural: ok | fail | running | skip.
// Hiç dalı/commit'i olmayan taze bir depoda sunucu dizi yerine null gönderebilir; arayüz bu yüzden çökmemeli
// (eskiden renderInspector `snap.branches.find` ile çöküp arayüzü sessizce Offline Simülasyon kipine düşürüyordu).
export function findSelectedBranch(snap, branchId) {
  return (snap?.branches || []).find((x) => x.id === branchId);
}
export function findSelectedNode(snap, shortSha) {
  return (snap?.nodes || []).find((n) => n.shortSha === shortSha);
}

export function checkConclusion(chk) {
  return (chk.conclusion || chk.status || '').toUpperCase();
}

export function classifyCheck(chk) {
  const conc = checkConclusion(chk);
  if (conc === 'FAILURE' || conc === 'TIMED_OUT' || conc === 'CANCELLED') return 'fail';
  if (conc === 'IN_PROGRESS' || conc === 'QUEUED' || conc === 'PENDING') return 'running';
  if (conc === 'SKIPPED') return 'skip';
  return 'ok';
}

// Stüdyo sol kolonundaki adım rozeti
export const CHECK_STUDIO_VIEW = {
  ok: { cls: 'ok', text: 'Başarılı' },
  fail: { cls: 'fail', text: 'Hatalı' },
  running: { cls: 'running', text: 'Çalışıyor' },
  skip: { cls: 'skip', text: 'Atlandı' },
};

// Inspector CI kartındaki kontrol satırı
export const CHECK_INSPECTOR_VIEW = {
  ok: { icon: '✅', chip: 'ok' },
  fail: { icon: '❌', chip: 'danger' },
  running: { icon: '🟡', chip: 'warn' },
  skip: { icon: '⚪', chip: 'ghost' },
};

// ---------------------------------------------------------------------------
// Dal inspector'u: durum bayrakları ve özetler
// ---------------------------------------------------------------------------

// st: { pendingAutoMerge } (UI durumu). snap: o anki snapshot.
export function branchViewModel(b, snap, st = {}) {
  const isOpen = b.state === 'open';
  const isVirtualWip = b.id === 'local-wip-working-tree';
  const hasConflict = Boolean(b.conflict?.hasConflict && !b.conflict.resolved);
  const isRemoteOnly = !b.hasLocal && b.hasRemote && isOpen;
  const isLive = snap.repo.mode === 'live';
  const ciState = b.pr?.ci?.state || '';
  // "CI Bitince Merge Et" gerekir mi: CI sürüyor ya da (zorunlu kontrol varken) henüz hiç belirmedi. Rozet/yeşil-kırmızı metinleri ciState'e bakar.
  const isRunning = ciState === 'RUNNING';
  const mergeWaitsForCI = ciPendingForMerge(b, snap);
  const isQueued = Boolean(st.pendingAutoMerge?.[b.id] || queueRunBadge(snap, b));
  const needsUpdate = needsUpdateBeforeMerge(b, snap);
  const canCommitWorkingTree = Boolean(isVirtualWip || (b.uncommittedCount || 0) > 0 || (b.isHead && (snap?.repo?.dirtyCount || 0) > 0));
  const isAlreadySynced = b.aheadMain === 0 && (b.uncommittedCount || 0) === 0 && !isVirtualWip;
  const canPromote = isOpen && b.hemisphere === 'lower' && !isVirtualWip && !canCommitWorkingTree && !isAlreadySynced;
  const hasOpenPR = Boolean(b.pr && b.pr.number > 0 && b.pr.reviewState !== 'MERGED');
  const canMerge = isOpen && hasOpenPR && !hasConflict && !isVirtualWip && ciState !== 'FAILURE';

  const authorName = (b.author || '').trim();
  const currentUser = (snap.repo?.currentUser || '').trim().toLowerCase();
  const isMe = authorName && currentUser && authorName.toLowerCase() === currentUser;

  const { upperSummary, lowerSummary } = layerSummaries(b, snap, { isVirtualWip, isRemoteOnly });

  return {
    b, snap, isOpen, isVirtualWip, hasConflict, isRemoteOnly, isLive, ciState, isRunning, mergeWaitsForCI, isQueued, needsUpdate,
    canCommitWorkingTree, isAlreadySynced, canPromote, hasOpenPR, canMerge, authorName, isMe,
    upperSummary, lowerSummary,
    title: isVirtualWip ? '✏️ Çalışma Alanı (Taslak)' : b.name,
  };
}

// Üst ray (Remote/PR) ve alt ray (Local) akordeon başlıklarındaki özet metinler.
export function layerSummaries(b, snap, { isVirtualWip, isRemoteOnly }) {
  const upperSummary = b.pr
    ? prHeadline(b.pr)
    : (isRemoteOnly ? `origin/${truncate(b.name, 16)}` : (b.originGone ? 'origin: gone' : 'PR Yok'));

  const lowerSummary = isVirtualWip
    ? `${b.uncommittedCount || snap.repo?.dirtyCount || 0} dosya bekliyor`
    : (!b.hasLocal ? 'Yerelde Yok' : (b.isHead ? 'HEAD (Aktif)' : `↑${b.aheadMain || 0} / ↓${b.behindMain || 0}`));
  return { upperSummary, lowerSummary };
}

// Birleşmiş/silinmiş (kalıntı) dal mı? Üzerine yeni commit/PR açılamaz.
export function isMergedRemnant(b) {
  return isMergedOrGone(b) || b.state === 'merged' || b.pr?.reviewState === 'MERGED';
}

// ---------------------------------------------------------------------------
// Konsolsuz Commit & PR Stüdyosu varsayılanları
// ---------------------------------------------------------------------------

// İlk değişen dosyadan Conventional Commit türü ve kapsamı.
export function commitScope(firstFile) {
  let prefix = 'feat';
  let scope = '';
  const first = (firstFile || '').toLowerCase();
  if (first.startsWith('internal/mcp')) { scope = '(mcp)'; prefix = 'feat'; }
  else if (first.startsWith('web/')) { scope = '(ui)'; prefix = 'feat'; }
  else if (first.startsWith('docs/') || first.endsWith('.md')) { scope = '(docs)'; prefix = 'docs'; }
  else if (first.includes('test')) { scope = '(test)'; prefix = 'test'; }
  else if (first.startsWith('.github/')) { scope = '(ci)'; prefix = 'ci'; }
  return { prefix, scope };
}

// Form alanlarının varsayılan değerleri. Sanal çalışma alanı (wip) için dosyalardan dal adı, commit mesajı
// ve PR gövdesi türetilir; gerçek dallarda dalın kendi bilgileri kullanılır.
export function workingTreeDefaults(b, candidateFiles, snap, isVirtualWip) {
  let defaultWtBranch = b.parentBranch || b.name;
  let branchPlaceholder = 'feat/ozellik-adi';
  let defaultCommitMsg = b.lastCommitMsg || '';
  if (/update\s+\d+\s+files/i.test(defaultCommitMsg) || defaultCommitMsg.startsWith('Pushlanmamış yerel çalışma')) {
    defaultCommitMsg = '';
  }
  let defaultPrTitle = b.draftPrTitle || defaultCommitMsg;
  if (/update\s+\d+\s+files/i.test(defaultPrTitle)) {
    defaultPrTitle = '';
  }
  let defaultPrBody = b.draftPrBody || '';

  if (isVirtualWip) {
    defaultWtBranch = suggestUniqueBranchName('feat', candidateFiles, snap);
    branchPlaceholder = 'Örn: ' + defaultWtBranch;

    // Sunucu, değişikliğin KENDİ bilgisinden (kaydedilmemiş CHANGELOG eklemesi, dosyalar) bir öneri verir: o öncelikli. Yalnızca sunucu bir şey
    // vermediyse dosya listesinden GERÇEK bilgi türetilir. Eskiden burada uydurma metin vardı ("üzerinde geliştirmeler", ve doğrulanmamış bir
    // "## Doğrulama - Yerel testler ve kontroller tamamlandı." iddiası): hiçbir şey çalıştırılmadan test geçmiş gibi yazılıyordu.
    if (!defaultCommitMsg || !defaultPrTitle || !defaultPrBody) {
      const { prefix, scope } = commitScope(candidateFiles[0]);
      const count = candidateFiles.length;
      const fileSummary = candidateFiles.slice(0, 2).map((x) => x.split('/').pop()).join(', ');
      const desc = count > 0 ? `${count === 1 ? '1 dosya' : `${count} dosya`} güncellendi (${fileSummary}${count > 2 ? ', …' : ''})` : 'değişiklikleri kaydet';
      const fallbackMsg = `${prefix}${scope}: ${desc}`;
      if (!defaultCommitMsg) defaultCommitMsg = fallbackMsg;
      if (!defaultPrTitle) defaultPrTitle = defaultCommitMsg;
      if (!defaultPrBody) defaultPrBody = `## Neler Değişti?\n${candidateFiles.slice(0, 8).map((f) => `- \`${f}\``).join('\n')}`;
    }
  }
  return { defaultWtBranch, branchPlaceholder, defaultCommitMsg, defaultPrTitle, defaultPrBody };
}

// "Push Öncesi PR" formunun başlangıç değerleri (kullanıcı taslağı varsa o).
/** Push düğmesinin metinleri. "(yerel)" eşlik kartı mevcut PR'ın dalındaki pushlanmamış commit'leri temsil eder: yeni PR açılmaz. */
export function promoteCopy(b) {
  if (b?.parentBranch) {
    const n = Number(b.remoteAhead || b.aheadMain) || 0;
    return {
      companion: true,
      confirm: `"${b.parentBranch}" dalının pushlanmamış ${n > 0 ? `${n} commit'i` : 'commit\'leri'} origin'e pushlansın mı? Mevcut PR güncellenir; yeni PR açılmaz.`,
      busy: '⏳ Pushlanıyor...',
      done: 'Yerel commit\'ler origin\'e pushlandı; mevcut PR güncellendi.',
    };
  }
  return {
    companion: false,
    confirm: `"${b?.name || ''}" dalını origin'e pushlamak ve GitHub üzerinde Pull Request oluşturmak istiyor musunuz?`,
    busy: '⏳ Push & PR Açılıyor...',
    done: 'Dal origin’e pushlandı ve PR açılarak uzağa (GitHub) taşındı!',
  };
}

export function promoteDefaults(b, savedDraft) {
  return {
    title: savedDraft?.title ?? b.draftPrTitle ?? b.lastCommitMsg ?? ('feat: ' + b.name),
    body: savedDraft?.body ?? b.draftPrBody ?? '',
  };
}

// ---------------------------------------------------------------------------
// AI commit/PR önerisi
// ---------------------------------------------------------------------------

// Öneri formunun değerleri: aynı öneriye ait kullanıcı taslağı varsa o, yoksa önerinin kendisi.
export function proposalFormValues(prop, draft) {
  const edits = (draft && draft.id === prop.id) ? draft : null;
  return {
    branch: edits ? edits.branch : prop.branchName,
    message: edits ? edits.message : prop.message,
    title: edits ? edits.title : (prop.prTitle || ''),
    body: edits ? edits.body : (prop.prBody || ''),
  };
}

// Onay yükünü ve eksik alan uyarısını üretir. { error } doluysa gönderilmez.
export function buildApprovePayload(prop, fields) {
  const branchName = fields.branch;
  const message = fields.message;
  if (!branchName) return { error: 'Lütfen geçerli bir dal adı girin' };
  if (!message) return { error: 'Commit mesajı boş olamaz' };
  return { payload: { proposalId: prop.id, branchName, message, prTitle: fields.title, prBody: fields.body } };
}

// web/modules/inspector-cards.js - Sağ Inspector'un HTML kartları (saf: veri alır, HTML dizgesi döndürür)
//
// Şablon metinleri (boşluklar dahil) orijinal inspector.js'ten aynen taşınmıştır. Kararlar
// inspector-model.js'te verilir; burada yalnızca basılır. Kullanıcı verisi HER ZAMAN esc() ile basılır.
import { esc, truncate, isSecretFile, prHeadline } from './utils.js';
import { CHECK_INSPECTOR_VIEW, checkConclusion, classifyCheck } from './inspector-model.js';

/** Merge düğümünde "main CI" kartı (mainCICardModel çıktısı). Kullanıcı verisi esc() ile basılır; bağlantı düğmesi yalnızca https. */
export function mainCICardHtml(m) {
  if (!m) return '';
  const rows = m.rows.map(([k, v]) => `<div class="layer-line"><strong>${esc(k)}:</strong> ${esc(v)}</div>`).join('');
  const open = m.url ? `<button type="button" class="btn ghost sm" id="act-mainci-open" data-url="${esc(m.url)}">↗ GitHub'da koşuyu aç</button>` : '';
  return `
        <div class="diag-card mainci-card ${esc(m.tone)}">
          <h4>main CI · <span class="chip ${m.tone === 'ok' ? 'ok' : (m.tone === 'fail' ? 'danger' : 'warn')}">${esc(m.text)}</span></h4>
          ${rows}
          <div class="layer-line">${esc(m.note || '')}</div>
          ${open}
        </div>`;
}

export function nodeInspectorHtml(node, mainCI = null) {
      return `
        <div class="det-header">
          <div class="det-title-row">
            <h3 class="det-branch-name">Commit ${esc(node.shortSha)}</h3>
            ${node.tag ? `<span class="chip accent">${esc(node.tag)}</span>` : ''}
          </div>
          <div class="det-badges">
            <span class="chip">👤 ${esc(node.author)}</span>
            <span class="chip">${esc(node.relativeTime)}</span>
            ${node.isMerge ? '<span class="chip ok">Merge Node</span>' : ''}
          </div>
          <div class="det-actions">
            <button type="button" class="btn primary sm" id="act-node-diff">🔍 Commit Değişikliklerini Gör (Diff)</button>
          </div>
        </div>
        <div class="diag-card">
          <h4>Commit Mesajı</h4>
          <div class="layer-line">${esc(node.subject)}</div>
        </div>
        ${mainCICardHtml(mainCI)}
        <div class="lock-note">🔒 <strong>main</strong> omurgasındaki commit mesajları tarihçe güvenliği için kilitlidir.</div>
      `;
}

export function nodeStudioHtml(node) {
        return `
          <div class="studio-detail-card">
            <div class="studio-card-row">
              <span class="studio-branch-name">Commit ${esc(node.shortSha)}</span>
              ${node.tag ? `<span class="chip sm accent">${esc(node.tag)}</span>` : '<span class="chip sm">Spine</span>'}
            </div>
            <div class="studio-card-row">
              <span class="studio-author-pill">👤 ${esc(node.author)}</span>
              <span class="studio-sha-badge">${esc(node.relativeTime)}</span>
            </div>
            <div class="layer-line">${esc(node.subject)}</div>
            <div class="studio-actions-grid">
              <button type="button" class="btn primary sm" id="studio-act-node-diff">🔍 Diff Gör</button>
            </div>
          </div>
        `;
}

// draft: kullanıcının düzenleme taslağı (state.proposalDraftEdits); yalnızca aynı öneriye aitse kullanılır.
export function proposalCardHtml(prop, draft) {
  const edits = (draft && draft.id === prop.id)
    ? draft
    : null;
    const branchVal = edits ? edits.branch : prop.branchName;
    const msgVal = edits ? edits.message : prop.message;
    const titleVal = edits ? edits.title : (prop.prTitle || '');
    const bodyVal = edits ? edits.body : (prop.prBody || '');

    return `
      <div class="diag-card proposal-card">
        <div class="proposal-head">
          <h4>
            <span>🤖 Claude / AI Commit &amp; PR Önerisi</span>
            <span class="chip accent">Kullanıcı Onayı Bekliyor</span>
          </h4>
          <span class="proposal-time">${esc(prop.createdAt || '')}</span>
        </div>
        ${prop.repoName ? `<div class="proposal-project">📁 Proje: <strong>${esc(prop.repoName)}</strong> — bu öneri yalnızca bu projede onaylanır</div>` : ''}
        <div class="wt-note">
          Claude bir commit ve PR önerisi hazırladı. Bu öneri <strong>${esc(prop.baseBranch || 'origin/main')}</strong> tabanlı yeni bir dal açacak, yalnız aşağıdaki açık dosyaları stage edecek ve PR oluşturacaktır. İnceleyip onaylayabilir veya başlık/açıklama alanlarını doğrudan düzenleyebilirsiniz.
        </div>

        <div class="wt-field">
          <label class="wt-label" for="prop-edit-branch">Önerilen Dal Adı <span class="chip ok">${esc(prop.baseBranch || 'origin/main')} tabanlı</span></label>
          <input type="text" class="input-control mono" id="prop-edit-branch" value="${esc(branchVal)}" />
        </div>

        <div class="wt-field">
          <label class="wt-label">Commitlenecek Açık Dosyalar (${prop.files?.length || 0} adet)</label>
          <div class="file-list">
            ${(prop.files || []).map((f) => `<span class="file-tag special">${esc(f)}</span>`).join('')}
          </div>
        </div>

        ${prop.diffStat ? `
          <div class="wt-field">
            <label class="wt-label">Değişiklik Özeti (git diff --stat)</label>
            <pre class="terminal-box">${esc(prop.diffStat)}</pre>
          </div>
        ` : ''}

        <div class="wt-field">
          <label class="wt-label">🛡️ Güvenlik Kalkanı Kontrolü</label>
          <div class="layer-line ok"><strong>${esc(prop.shieldResult || '✓ 0 secret, 0 özel anahtar, 0 .gitignore ihlali (Tüm dosyalar güvenli)')}</strong></div>
        </div>

        <div class="wt-field">
          <label class="wt-label">📊 Dosya Sayısı &amp; Mesaj Doğrulaması</label>
          <div class="layer-line ${prop.countCheckMatch !== false ? 'ok' : 'warn'}">
            <strong>${esc(prop.countCheckMessage || `✓ ${prop.files?.length || 0} dosya seçildi`)}</strong>
          </div>
        </div>

        <div class="wt-field">
          <label class="wt-label" for="prop-edit-message">Commit Mesajı &amp; Co-Authors (Düzenlenebilir)</label>
          <textarea class="textarea-control commit-msg-box" id="prop-edit-message" rows="4">${esc(msgVal)}</textarea>
        </div>

        <div class="wt-field">
          <label class="wt-label" for="prop-edit-title">PR Başlığı (Düzenlenebilir)</label>
          <input type="text" class="input-control" id="prop-edit-title" value="${esc(titleVal)}" placeholder="GitHub PR başlığı belirleyin" />
        </div>

        <div class="wt-field">
          <label class="wt-label" for="prop-edit-body">PR Açıklaması (Düzenlenebilir)</label>
          <textarea class="textarea-control" id="prop-edit-body" rows="4" placeholder="PR açıklaması...">${esc(bodyVal)}</textarea>
        </div>

        <div class="det-actions">
          <button type="button" class="btn primary sm" id="act-proposal-approve">✓ Onayla: Commit At &amp; PR Aç</button>
          <button type="button" class="btn ghost sm" id="act-proposal-dismiss">✕ Öneriyi Reddet (Temizle)</button>
        </div>
      </div>
    `;
}

export function branchHeaderHtml(v) {
  const { proposalHtml, b, snap, isVirtualWip, isAlreadySynced, isOpen, isRemoteOnly, canPromote, hasOpenPR, mergeStrategy, isRunning, mergeWaitsForCI, isQueued, needsUpdate, canMerge, hasConflict, authorName, isMe } = v;
  return `
    ${proposalHtml}
    <div class="det-header">
      <div class="det-title-row">
        <h3 class="det-branch-name">${isVirtualWip ? '✏️ Çalışma Alanı (Kaydedilmemiş Değişiklikler)' : esc(b.name)}</h3>
        <span class="chip ${b.state === 'merged' ? 'ok' : (isVirtualWip ? 'purple' : (b.hemisphere === 'lower' ? 'purple' : 'accent'))}">
          ${isVirtualWip ? 'Yerel Taslak · Uncommitted' : (b.state === 'merged' ? 'Tam Parabol' : (isAlreadySynced ? '✓ main ile Eşit (Kalıntı Dal)' : (b.hemisphere === 'lower' && b.aheadMain === 0 ? '🌱 Yeni Yerel Dal' : b.hemisphere === 'lower' ? 'Yerel · Local' : 'Uzak · PR')))}
        </span>
      </div>
      <div class="det-badges">
        ${b.isHead ? '<span class="chip ok">HEAD (Aktif Dal)</span>' : ''}
        ${b.worktreePath ? `<span class="chip warn" title="${esc(b.worktreePath)}">📁 Başka klasörde açık</span>` : ''}
        ${authorName ? `<span class="chip ${isMe ? 'accent' : 'purple'}" title="Dal/PR Sahibi">👤 @${esc(authorName)} ${isMe ? '(Ben)' : ''}</span>` : ''}
        ${b.pr?.ci ? `<span class="chip ${b.pr.ci.state === 'SUCCESS' ? 'ok' : (b.pr.ci.state === 'FAILURE' ? 'danger' : 'warn')}">CI: ${esc(b.pr.ci.state)}</span>` : ''}
        ${b.originGone ? '<span class="chip warn">origin: gone</span>' : ''}
      </div>
      <div class="det-actions">
        <button type="button" class="btn sm" id="act-branch-diff" title="Kod değişikliklerini (diff) görüntüle">🔍 Diff Gör</button>
        ${!b.isHead && isOpen && !isVirtualWip ? `<button type="button" class="btn ghost sm" id="act-checkout" ${snap.repo.teamMode ? 'disabled title="Team Server modunda paylaşılan HEAD kilitlidir"' : (b.worktreePath ? `disabled title="Bu dal başka bir klasörde açık (${esc(b.worktreePath)}); git burada geçmeyi reddeder"` : '')}>${isRemoteOnly ? '🎯 Yerelde İndir &amp; Geç' : '🎯 Dala Geç'}</button>` : ''}
        ${b.isHead && isAlreadySynced && !isVirtualWip ? `<button type="button" class="btn primary sm" id="act-checkout-main">🎯 main Omurgasına Geç</button>` : ''}
        ${canPromote && !hasOpenPR ? `<button type="button" class="btn primary sm" id="act-promote">🚀 Push &amp; PR Aç</button>` : ''}
        ${hasOpenPR && !isVirtualWip ? `
          <div class="merge-group">
            <select id="merge-strategy" class="select-inline" aria-label="Merge stratejisi">
              <option value="merge" ${mergeStrategy === 'merge' ? 'selected' : ''}>Merge</option>
              <option value="squash" ${mergeStrategy === 'squash' ? 'selected' : ''}>Squash</option>
              <option value="rebase" ${mergeStrategy === 'rebase' ? 'selected' : ''}>Rebase</option>
              <option value="auto" ${mergeStrategy === 'auto' ? 'selected' : ''}>Auto-Merge (Queue)</option>
            </select>
            ${needsUpdate
              ? `<button type="button" class="btn warn sm" id="act-merge" title="Dal main'in gerisinde: güncelle, CI'ı bekle, birleştir">↻ Güncelle &amp; Birleştir</button>`
              : (isRunning || mergeWaitsForCI)
              ? `<button type="button" class="btn warn sm ${isQueued ? 'running-pulse' : ''}" id="act-merge">${isQueued ? '⏳ CI Bekleniyor (İptal Et)' : '⏳ CI Bitince Otomatik Merge Et'}</button>`
              : `<button type="button" class="btn ok sm" id="act-merge" ${canMerge ? '' : 'disabled'}>⚡ Birleştir</button>`}
          </div>
          ${!b.pr?.ci ? `
            <div class="ci-merge-gate">${mergeWaitsForCI
              ? '⏳ Zorunlu CI kontrolleri henüz belirmedi (GitHub PR açıldıktan birkaç saniye sonra başlatır); bitince otomatik birleşir. Yeşil varsayılmaz.'
              : 'CI durumu bilinmiyor (bu PR için kontrol bulunamadı); yeşil varsayılmaz. GitHub birleştirmeye izin veriyorsa birleştirilebilir.'}</div>
          ` : ''}
          ${b.pr?.ci ? `
            <div class="ci-merge-gate ${isRunning ? 'running' : (b.pr.ci.state === 'FAILURE' ? 'fail' : 'ok')}">
              ${b.pr.ci.state === 'FAILURE' ? '❌ CI kontrolleri başarısız; merge kilidi aktif' : (isRunning ? '⏳ CI kontrolleri sürüyor; otomatik merge kuyrukta' : '✓ Tüm CI kontrolleri başarılı; güvenle birleştirilebilir')}
            </div>
          ` : ''}
        ` : ''}
        ${hasConflict ? `<button type="button" class="btn warn sm" id="act-resolve">⚡ Çakışma Stüdyosunu Aç</button>` : ''}
        ${!isVirtualWip && !b.longLived && (b.state !== 'merged' || b.hasLocal || b.originGone || b.hasRemote) ? `<button type="button" class="btn ghost sm" id="act-delete">${isRemoteOnly ? '🗑️ PR Kapat &amp; Uzak Dalı Sil' : '🧹 Sil / Temizle'}</button>` : ''}
      </div>
    </div>
  `;
}

export function layersHtml(v) {
  const { b, snap, isVirtualWip, isRemoteOnly, upperSummary, lowerSummary } = v;
  return `
    <div class="layer-accordion-container">
      <details class="layer-accordion">
        <summary class="layer-summary">
          <span class="layer-summary-title">☁️ Uzak · Remote / PR</span>
          <span class="layer-summary-badge">${esc(upperSummary)}</span>
        </summary>
        <div class="layer-scroll-body">
          ${b.pr ? `
            <div class="layer-line">${esc(prHeadline(b.pr))}</div>
            <div class="layer-line">${esc(truncate(b.pr.title, 56))}</div>
            ${b.pr.ci ? `<div class="layer-line">CI: ${esc(b.pr.ci.workflow || 'CI')} (${esc(b.pr.ci.duration || '—')})</div>` : (b.state === 'open' && b.pr.reviewState !== 'MERGED' ? '<div class="layer-line">CI: bilinmiyor (bu PR için kontrol bulunamadı)</div>' : '')}
          ` : `
            <div class="layer-line">${isRemoteOnly ? `Remote aktif: origin/${esc(b.name)}` : (b.originGone ? 'Remote silindi (origin: gone)' : 'Henüz PR / Remote yok')}</div>
            <div class="layer-line">${isRemoteOnly ? 'Yerelde kopyası silinmiş uzak dal' : 'Yerel çalışma (henüz uzakta yok)'}</div>
          `}
        </div>
      </details>

      <details class="layer-accordion">
        <summary class="layer-summary">
          <span class="layer-summary-title">💻 Yerel · Local</span>
          <span class="layer-summary-badge">${esc(lowerSummary)}</span>
        </summary>
        <div class="layer-scroll-body">
          ${isVirtualWip ? `
            <div class="layer-line">Konum: main omurgası (Kaydedilmemiş Çalışma)</div>
            <div class="layer-line">Durum: Henüz bir dala veya commite aktarılmamış</div>
            <div class="layer-line">Değişen Dosyalar: ${b.uncommittedCount || snap?.repo?.dirtyCount || 0} adet dosya</div>
          ` : (!b.hasLocal ? `
            <div class="layer-line">${b.state === 'merged' && !b.originGone ? '✓ Yerel kalıntı dal temizlendi' : `Yerelde dal yok (origin/${esc(b.name)})`}</div>
            <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
            <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
          ` : `
            ${b.state === 'merged' ? '<div class="layer-line">⚠ Birleşmiş yerel kalıntı (tek tıkla temizlenebilir)</div>' : (b.aheadMain === 0 && !isVirtualWip ? '<div class="layer-line">🌱 main omurgasında taze dal (ilk commitinizi bekliyor)</div>' : '')}
            <div class="layer-line">SHA: ${esc(b.localSha || b.mergeSha || b.baseSha)}</div>
            <div class="layer-line">main farkı: ↑${b.aheadMain || 0} / ↓${b.behindMain || 0}</div>
            <div class="layer-line">Uncommitted (Staged + Untracked): ${b.uncommittedCount || (b.isHead ? snap?.repo?.dirtyCount : 0) || 0} dosya</div>
          `)}
        </div>
      </details>
    </div>
  `;
}

export function conflictCardHtml(b) {
    const resolved = b.conflict.resolved;
    return `
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

export function crossPrCardHtml(b) {
    return `
      <div class="diag-card cross">
        <h4><span>⇄ Cross-PR Dosya Çarpışması</span></h4>
        ${b.crossPr.map((c) => `
          <div class="layer-line"><strong>${c.otherPrNumber ? 'PR #' + c.otherPrNumber : ''} (${esc(c.otherBranch)})</strong>: ${esc(c.sharedFiles.join(', '))}</div>
        `).join('')}
      </div>
    `;
}

// ctx: { isLive, localRunOut }
export function ciCardHtml(b, ctx) {
  const { isLive, localRunOut } = ctx;
    const ci = b.pr.ci;
    const ciState = ci.state || '';
    const isFailed = ciState === 'FAILURE';
    const isRunningCI = ciState === 'RUNNING';
    const isUnknownCI = !ciState; // durum bilinmiyorsa yeşil gösterilmez
    const checks = ci.checks || [];
    const extracts = ci.extracts || [];
    

    let cardModifier = 'ci-ok';
    let statusText = '✓ Tüm Kontroller Başarılı';
    let chipTone = 'ok';
    if (isFailed) {
      cardModifier = '';
      statusText = '❌ Başarısız Kontrol';
      chipTone = 'danger';
    } else if (isRunningCI) {
      cardModifier = 'ci-running';
      statusText = '⏳ Kontroller Devam Ediyor...';
      chipTone = 'warn';
    } else if (isUnknownCI) {
      cardModifier = '';
      statusText = '? CI Durumu Bilinmiyor';
      chipTone = '';
    }

    return `
      <div class="diag-card ci ${cardModifier}">
        <h4>
          <span>⚙️ GitHub Actions CI Adımları ${checks.length > 0 ? `(${checks.length} Kontrol)` : ''}</span>
          <span class="chip ${chipTone}">${statusText}</span>
        </h4>
        <div class="wt-note">
          <strong>İş Akışı:</strong> ${esc(ci.workflow || 'CI')} ${ci.duration ? `· Süre: ${esc(ci.duration)}` : ''}
          ${ci.failedStep ? `<br><span class="ci-failed-tag">Patlayan Adım: ${esc(ci.failedStep)}</span>` : ''}
        </div>

        ${checks.length > 0 ? `
          <div class="ci-checks-list">
            ${checks.map((chk) => {
              const conc = checkConclusion(chk);
              const view = CHECK_INSPECTOR_VIEW[classifyCheck(chk)];
              const icon = view.icon;
              const chipClass = view.chip;
              return `
                <div class="ci-check-row">
                  <div class="ci-check-info">
                    <span class="ci-check-icon">${icon}</span>
                    <span class="ci-check-name">${esc(chk.name || 'check')}</span>
                  </div>
                  <div class="ci-check-status">
                    <span class="chip ${chipClass} sm mono">${esc(conc || 'COMPLETED')}</span>
                    ${chk.detailsUrl ? `<a href="${esc(chk.detailsUrl)}" target="_blank" rel="noopener" class="btn ghost sm ci-link-btn" title="GitHub Actions üzerinde incele">🔗 Detay</a>` : ''}
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        ` : ''}

        ${isFailed ? `
          <div class="ci-fail-section">
            <div class="layer-line"><strong>Gürültüsüz CI Hata Ayıklayıcı (${extracts.length} kritik bulgu):</strong></div>
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
            <pre class="terminal-box" id="ci-raw-log">${esc(localRunOut || ci.rawLog || 'Ham log bulunamadı.')}</pre>
          </div>
        ` : ''}
      </div>
    `;
}

export function mergedRemnantCardHtml(b, candidateFiles) {
      return `
        <div class="diag-card working-tree-card">
          <h4>
            <span>⚠️ Birleşmiş Kalıntı Dalda Açık Dosyalar</span>
            <span class="chip warn">${candidateFiles.length} Dosya Bekliyor</span>
          </h4>
          <div class="wt-note">
            Aktif dalınız (<strong>${esc(b.name)}</strong>) GitHub üzerinde birleşmiş ve silinmiştir (origin: gone). Birleşmiş bir dalın üzerine yeni commit veya PR açılamaz.<br><br>
            Açık <strong>${candidateFiles.length}</strong> dosyanız güvenle korunmaktadır. Bu kalıntıyı temizleyip ana omurgaya geçebilir veya dosyaları yeni bir dala aktarabilirsiniz.
          </div>
          <div class="det-actions">
            ${b.hasLocal ? '<button type="button" class="btn primary sm" id="act-clean-and-switch-main">🧹 Kalıntıyı Temizle ve main\'e Geç</button>' : ''}
            <button type="button" class="btn emerald sm" id="act-open-new-branch-modal">+ Yeni Dala Aktar</button>
            <button type="button" class="btn warn sm" id="act-wt-discard">🗑️ Değişiklikleri Geri Al (Discard)</button>
          </div>
        </div>
      `;
}

// d: workingTreeDefaults() sonucu + { secretFile, candidateFiles }
export function commitStudioCardHtml(b, snap, d) {
  const { secretFile, candidateFiles, defaultWtBranch, branchPlaceholder, defaultCommitMsg, defaultPrTitle, defaultPrBody } = d;
      return `
        <div class="diag-card working-tree-card">
          <h4>
            <span>🚀 Konsolsuz Commit &amp; PR Stüdyosu</span>
            <span class="chip ${secretFile ? 'danger' : 'purple'}">${secretFile ? '🛡️ Güvenlik Engeli' : (candidateFiles.length + ' Dosya Bekliyor')}</span>
          </h4>
          <div class="wt-note">
            Terminal açmadan çalışma alanındaki açık dosyaları yeni bir dala aktarabilir, commit atabilir veya doğrudan origin'e pushlayıp GitHub PR açabilirsiniz.
          </div>

          ${secretFile ? `
            <div class="wt-alert danger">
              <strong>GÜVENLİK ENGELİ:</strong> '${esc(secretFile)}' dosyası gizli/hassas bilgi (secret/private key/env) olarak tespit edildi! Bu dosya güvenliğiniz için commitlenemez.
            </div>
          ` : ''}

          <form id="form-wt-commit" class="edit-form">
            <div class="wt-field">
              <label class="wt-label" for="wt-branch-name">Hedef Dal Adı</label>
              <input type="text" id="wt-branch-name" class="input-control mono" value="${esc(defaultWtBranch)}" placeholder="${esc(branchPlaceholder)}" required ${snap.repo.teamMode ? 'disabled' : ''}>
            </div>
            <div class="wt-field">
              <label class="wt-label">Kaydedilecek Açık Dosyalar (${candidateFiles.length} dosya)</label>
              <div class="file-list">
                ${candidateFiles.map((f) => {
                  const isSec = isSecretFile(f, snap);
                  return `<span class="file-tag ${isSec ? 'special danger-file' : ''}" title="${isSec ? 'Güvenlik kalkanı tarafından engellendi' : ''}">${isSec ? '🛡️ ' : ''}${esc(f)}</span>`;
                }).join('')}
              </div>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-commit-msg">Commit Mesajı</label>
              <input type="text" id="wt-commit-msg" class="input-control mono" value="${esc(defaultCommitMsg)}" placeholder="Örn: feat(ui): add new interactive panel" required>
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-title">PR Başlığı (Opsiyonel)</label>
              <input type="text" id="wt-pr-title" class="input-control mono" value="${esc(defaultPrTitle)}" placeholder="Örn: feat: add new interactive panel">
            </div>
            <div class="wt-field">
              <label class="wt-label" for="wt-pr-body">PR Açıklaması</label>
              <textarea id="wt-pr-body" class="textarea-control" placeholder="Neler değişti ve nasıl doğrulandı?">${esc(defaultPrBody)}</textarea>
            </div>
            <div class="det-actions">
              <button type="submit" class="btn emerald sm" id="act-wt-commit-only" ${snap.repo.teamMode || secretFile ? 'disabled' : ''}>💾 Sadece Dal Aç &amp; Commit At</button>
              <button type="button" class="btn primary sm" id="act-wt-commit-push" ${snap.repo.teamMode || secretFile ? 'disabled' : ''}>${b.pr && b.pr.number > 0 ? `🚀 Commit &amp; PR #${b.pr.number}'a Pushla` : '🚀 Commit + Push &amp; PR Oluştur'}</button>
              <button type="button" class="btn warn sm" id="act-wt-discard" ${snap.repo.teamMode ? 'disabled' : ''}>🗑️ Değişiklikleri Geri Al (Discard)</button>
            </div>
          </form>
        </div>
      `;
}

export function promoteCardHtml(b, promoteTitle, promoteBody) {
    return `
      <div class="diag-card">
        <h4>
          <span>📝 Push Öncesi PR Başlık &amp; Açıklama</span>
          <span class="chip purple">Local → Remote</span>
        </h4>
        <div class="wt-note">Dalı origin’e pushlamadan önce PR başlığını ve açıklamasını burada hazırlayıp kaydedebilirsiniz.</div>
        <form id="form-promote-pr" class="edit-form">
          <div class="wt-field">
            <label class="wt-label" for="promote-pr-title">PR Başlığı</label>
            <input type="text" id="promote-pr-title" class="input-control mono" value="${esc(promoteTitle)}" placeholder="Örn: feat(ui): add new feature" required>
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

export function syncedCardHtml(b) {
    return `
      <div class="diag-card">
        <h4>
          <span>✓ main Omurgası ile Senkronize</span>
          <span class="chip ok">Kalıntı Dal</span>
        </h4>
        <div class="wt-note">
          Bu dal (<strong>${esc(b.name)}</strong>) GitHub <strong>main</strong> omurgasıyla tamamen eşitlendi. Push edilecek yeni bir commit veya kaydedilmemiş açık değişiklik bulunmuyor.
        </div>
        <div class="det-actions">
          ${b.isHead ? '<button type="button" class="btn primary sm" id="act-checkout-main-synced">🎯 main Omurgasına Geç</button>' : ''}
          ${b.longLived ? '' : '<button type="button" class="btn warn sm" id="act-delete-synced">🧹 Kalıntı Dalı Temizle</button>'}
        </div>
      </div>
    `;
}

export function changedFilesCardHtml(b) {
    return `
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

export function prEditCardHtml(b) {
    return `
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

export function prDetailCardHtml(b) {
    return `
      <div class="diag-card">
        <h4>
          <span>📜 PR #${b.pr.number} Detayı</span>
          <span class="chip ok">${esc(b.pr.reviewState || 'MERGED')}</span>
        </h4>
        <div class="layer-line"><strong>Başlık:</strong> ${esc(b.pr.title)}</div>
        ${b.pr.author ? `<div class="layer-line"><strong>Yazar:</strong> @${esc(b.pr.author)}</div>` : ''}
        ${b.pr.ci ? `
          <div class="layer-line"><strong>CI Durumu:</strong> <span class="chip ${b.pr.ci.state === 'SUCCESS' ? 'ok' : 'danger'} sm mono">CI: ${esc(b.pr.ci.state)}</span> (${esc(b.pr.ci.workflow || 'CI')})</div>
        ` : ''}
        ${b.pr.body ? `
          <div class="wt-label">PR Açıklaması</div>
          <pre class="pr-body-box">${esc(b.pr.body)}</pre>
        ` : '<div class="wt-note">Bu PR için GitHub açıklaması kaydedilmemiş veya gövde boş.</div>'}
      </div>
    `;
}

export function amendCardHtml(b) {
    return `
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
}

export function suggestedCmdCardHtml(b) {
    return `
      <div class="diag-card">
        <h4>
          <span>Tek Tıkla Terminal Komutu</span>
          <button type="button" class="btn ghost sm" id="act-copy-cmd">Kopyala</button>
        </h4>
        <pre class="terminal-box">${esc(b.suggestedCmd)}</pre>
      </div>
    `;
}

// Birleşmiş dalın "Değişen Dosyalar" kartı: dosyalar sunucudan (birleşme commit'inin diff'i) yüklenince doldurulur.
export function mergedFilesCardHtml() {
  return `
    <div class="diag-card" id="merged-files-card">
      <h4><span>📂 Değişen Dosyalar</span><span class="muted mono" id="merged-files-count">yükleniyor…</span></h4>
      <div class="merged-files-list" id="merged-files-list"></div>
    </div>
  `;
}

// Dosya satırları: durum rozeti, yol, +eklenen / -silinen (istatistik yoksa gösterilmez). Tümü kaçışlanır.
export function mergedFilesRowsHtml(files) {
  const list = files || [];
  if (list.length === 0) return '<div class="muted">Bu birleşmede dosya değişikliği bulunamadı.</div>';
  return list.map((f, idx) => {
    const st = f.status || 'M';
    const cls = st === 'A' ? 'emerald' : (st === 'D' ? 'danger' : 'accent');
    return `
      <button type="button" class="merged-file-row" data-file-idx="${idx}" title="${esc(f.path)}">
        <span class="chip ${cls} sm mono">${esc(st)}</span>
        <span class="merged-file-path">${esc(f.path)}</span>
        <span class="merged-file-stat">${f.additions === undefined && f.deletions === undefined ? '' : `<span class="diff-stat-add">+${f.additions || 0}</span> <span class="diff-stat-del">-${f.deletions || 0}</span>`}</span>
      </button>
    `;
  }).join('');
}

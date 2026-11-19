// web/modules/diff-viewer.js - Diff görüntüleyici modalı
import { state, el } from './state.js';
import { esc, showToast } from './utils.js';
import { apiCall } from './api.js';

let currentDiffResult = null;

export async function openDiffViewer(target, type = '') {
  if (!el.diffModal) return;
  el.diffModal.hidden = false;
  currentDiffResult = null;

  if (el.diffTargetLabel) el.diffTargetLabel.textContent = target || 'Çalışma Alanı';
  if (el.diffStatsBadge) el.diffStatsBadge.textContent = 'Yükleniyor...';
  if (el.diffFilesCount) el.diffFilesCount.textContent = 'Dosyalar taranıyor...';
  if (el.diffFilesList) el.diffFilesList.innerHTML = '';
  if (el.diffViewerContainer) {
    el.diffViewerContainer.innerHTML = '<div class="diff-loading-msg">Değişiklikler yükleniyor...</div>';
  }

  try {
    let res;
    if (state.offlineMode || window.location.protocol === 'file:') {
      if (window.LocalFallbackEngine?.getDiff) {
        res = window.LocalFallbackEngine.getDiff(target, type);
      } else {
        throw new Error('Offline diff motoru kullanılamıyor');
      }
    } else {
      res = await apiCall(`/api/diff?target=${encodeURIComponent(target || '')}&type=${encodeURIComponent(type || '')}`);
    }
    renderDiffViewer(res);
  } catch (err) {
    if (el.diffViewerContainer) {
      el.diffViewerContainer.innerHTML = `<div class="diff-empty-msg">⚠️ Diff yüklenirken hata oluştu: ${esc(err.message)}</div>`;
    }
    if (el.diffStatsBadge) el.diffStatsBadge.textContent = 'Hata';
  }
}

export function closeDiffViewer() {
  if (el.diffModal) el.diffModal.hidden = true;
  currentDiffResult = null;
}

export function renderDiffViewer(diffResult) {
  currentDiffResult = diffResult;
  if (!diffResult) return;

  if (el.diffTargetLabel) {
    el.diffTargetLabel.textContent = diffResult.title || diffResult.target || 'Diff';
  }

  const files = diffResult.files || [];
  const totalAdds = diffResult.totalAdds || 0;
  const totalDels = diffResult.totalDels || 0;

  if (el.diffStatsBadge) {
    el.diffStatsBadge.innerHTML = `<span class="diff-stat-add">+${totalAdds}</span> / <span class="diff-stat-del">-${totalDels}</span>`;
  }
  if (el.diffFilesCount) {
    el.diffFilesCount.textContent = `${files.length} Dosya Değişti`;
  }

  // Populate files list
  if (el.diffFilesList) {
    if (files.length === 0) {
      el.diffFilesList.innerHTML = '<div class="diff-empty-msg">Değişen dosya yok</div>';
    } else {
      el.diffFilesList.innerHTML = files.map((f, idx) => {
        const statusClass = f.status === 'A' ? 'emerald' : (f.status === 'D' ? 'danger' : 'accent');
        const shortName = f.path.split('/').pop();
        return `
          <button type="button" class="diff-file-btn ${idx === 0 ? 'active' : ''}" data-file-idx="${idx}" title="${esc(f.path)}">
            <span class="chip ${statusClass} sm mono">${esc(f.status || 'M')}</span>
            <span class="diff-file-name">${esc(shortName)}</span>
            <span class="diff-file-stat">
              <span class="diff-stat-add">+${f.additions || 0}</span>
              <span class="diff-stat-del">-${f.deletions || 0}</span>
            </span>
          </button>
        `;
      }).join('');

      el.diffFilesList.querySelectorAll('.diff-file-btn').forEach((btn) => {
        btn.addEventListener('click', () => {
          el.diffFilesList.querySelectorAll('.diff-file-btn').forEach((b) => b.classList.remove('active'));
          btn.classList.add('active');
          const idx = btn.getAttribute('data-file-idx');
          const targetCard = document.getElementById(`diff-card-${idx}`);
          if (targetCard) {
            targetCard.scrollIntoView({ behavior: 'smooth', block: 'start' });
          }
        });
      });
    }
  }

  // Populate diff content container
  if (el.diffViewerContainer) {
    const raw = diffResult.rawDiff || '';
    if (!raw.trim()) {
      el.diffViewerContainer.innerHTML = '<div class="diff-empty-msg">Bu hedef için gösterilecek kod farkı bulunamadı (temiz/senkron).</div>';
      return;
    }

    // Split diff into per-file chunks
    const rawFiles = raw.split(/^diff --git /m).filter(Boolean);
    if (rawFiles.length === 0) {
      el.diffViewerContainer.innerHTML = `<pre class="terminal-box">${esc(raw)}</pre>`;
      return;
    }

    el.diffViewerContainer.innerHTML = rawFiles.map((chunk, idx) => {
      const lines = chunk.split('\n');
      const firstLine = lines[0] || '';
      const parts = firstLine.split(' ');
      const filePath = parts[1] ? parts[1].replace(/^[b]/, '').replace(/^\//, '') : (files[idx]?.path || `dosya-${idx + 1}`);

      const renderedLines = lines.slice(1).map((line) => {
        if (line.startsWith('@@')) {
          return `<div class="diff-line diff-hunk">${esc(line)}</div>`;
        }
        if (line.startsWith('+') && !line.startsWith('+++')) {
          return `<div class="diff-line diff-add">${esc(line)}</div>`;
        }
        if (line.startsWith('-') && !line.startsWith('---')) {
          return `<div class="diff-line diff-del">${esc(line)}</div>`;
        }
        return `<div class="diff-line diff-ctx">${esc(line)}</div>`;
      }).join('');

      return `
        <div class="diff-file-card" id="diff-card-${idx}">
          <div class="diff-file-card-head">
            <span class="diff-card-path">📄 ${esc(filePath)}</span>
            <span class="chip ghost sm mono">${lines.length} satır</span>
          </div>
          <div class="diff-file-card-body">${renderedLines}</div>
        </div>
      `;
    }).join('');
  }
}

export function initDiffViewerListeners() {
  el.diffModalClose?.addEventListener('click', closeDiffViewer);
  el.diffModalFootClose?.addEventListener('click', closeDiffViewer);
  el.btnCopyDiff?.addEventListener('click', () => {
    if (!currentDiffResult?.rawDiff) {
      showToast('Kopyalanacak diff bulunamadı', 'warn');
      return;
    }
    navigator.clipboard?.writeText(currentDiffResult.rawDiff).then(() => {
      showToast('Diff panoya kopyalandı! 📋', 'ok');
    }).catch(() => {
      showToast('Panoya kopyalanamadı', 'danger');
    });
  });
}

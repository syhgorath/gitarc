// web/modules/api.js - Unified REST API client with offline LocalFallbackEngine support
import { state } from './state.js';
import { REPO_STORAGE_KEY } from './constants.js';

// Projeden bağımsız uçlar: istek, proje kimliğine bağlanmaz (sunucudaki repoBindingExempt ile AYNI liste). Anlık görüntü ve
// proje seçme/ekleme/kaldırma, sekmenin etkin projeyi öğrenmesi/değiştirmesi içindir.
export function isRepoBindingExempt(path) {
  const p = String(path).split('?')[0];
  return ['/api/snapshot', '/api/repo/switch', '/api/repo/add', '/api/repo/remove'].includes(p) || p.startsWith('/api/system/');
}

export async function apiCall(path, method = 'GET', body = null) {
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
  // Sunucuda "etkin proje" tek ve geneldir; başka bir sekme projeyi değiştirdiyse bu sekmedeki işlem ÖTEKİ projeye giderdi.
  // Sekmenin gösterdiği projenin kimliği her istekle gider; uyuşmazsa sunucu işlemi yapmaz (409).
  const repoId = state.snapshot?.repo?.id;
  if (repoId && !isRepoBindingExempt(path)) {
    opts.headers['X-GitArc-Repo'] = repoId;
  }
  if (body !== null) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(path, opts);
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    if (resp.status === 409 && data.code === 'repo_mismatch') {
      // Bu sekme eski bir projeyi gösteriyor: sunucunun etkin projesini benimse (sayfa yeniden yüklenince başka sekmelerle
      // projeyi birbirine geri çevirmesin diye hatırlanan kimlik de güncellenir) ve arayüze haber ver.
      try {
        if (data.activeRepoId) localStorage.setItem(REPO_STORAGE_KEY, data.activeRepoId);
        else localStorage.removeItem(REPO_STORAGE_KEY);
      } catch { /* localStorage yoksa yoksay */ }
      try {
        window.dispatchEvent(new CustomEvent('gitarc:repo-mismatch', { detail: { activeRepoId: data.activeRepoId || '' } }));
      } catch { /* olay desteklenmiyorsa yoksay */ }
    }
    throw new Error(data.error || `HTTP ${resp.status}`);
  }
  return data;
}

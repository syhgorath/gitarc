// web/modules/api.js - Unified REST API client with offline LocalFallbackEngine support
import { state } from './state.js';

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

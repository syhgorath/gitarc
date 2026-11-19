// web/modules/theme.js - Koyu / Açık tema seçimi (CSP uyumlu: yalnızca data-theme özniteliği)
//
// Renkler web/tasarim.css içinde `:root` (koyu) ve `:root[data-theme="light"]` (açık)
// değişken bloklarında tanımlıdır; burada yalnızca özniteliği değiştirir ve seçimi saklarız.

export const THEME_KEY = 'gitarc_theme';

export function normalizeTheme(value) {
  return value === 'light' ? 'light' : 'dark';
}

export function toggleTheme(current) {
  return normalizeTheme(current) === 'light' ? 'dark' : 'light';
}

// Kayıtlı temayı okur; depolama kapalı/erişilemezse (gizli pencere vb.) koyu temaya düşer.
export function loadTheme(storage = globalThis.localStorage) {
  try {
    return normalizeTheme(storage?.getItem(THEME_KEY));
  } catch {
    return 'dark';
  }
}

export function saveTheme(theme, storage = globalThis.localStorage) {
  try {
    storage?.setItem(THEME_KEY, normalizeTheme(theme));
  } catch {
    // Depolama yoksa tema yalnızca bu oturumda geçerli olur.
  }
}

// Temayı kök öğeye uygular ve düğmenin erişilebilirlik durumunu günceller.
export function applyTheme(theme, root = globalThis.document?.documentElement, btn = null) {
  const t = normalizeTheme(theme);
  if (root) root.dataset.theme = t;
  if (btn) {
    btn.setAttribute('aria-pressed', String(t === 'light'));
    btn.title = t === 'light' ? 'Koyu temaya geç' : 'Açık temaya geç';
  }
  return t;
}

// Başlangıçta kayıtlı temayı uygular ve düğmeyi bağlar.
export function initTheme(btn, { root, storage } = {}) {
  let current = applyTheme(loadTheme(storage), root, btn);
  btn?.addEventListener('click', () => {
    current = applyTheme(toggleTheme(current), root, btn);
    saveTheme(current, storage);
  });
  return () => current;
}

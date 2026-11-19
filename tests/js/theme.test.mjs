// Zero-dependency tests for web/modules/theme.js. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  THEME_KEY, normalizeTheme, toggleTheme, loadTheme, saveTheme, applyTheme, initTheme,
} from '../../web/modules/theme.js';

const memStorage = (init = {}) => {
  const data = { ...init };
  return { getItem: (k) => (k in data ? data[k] : null), setItem: (k, v) => { data[k] = v; }, data };
};
const brokenStorage = () => ({
  getItem() { throw new Error('denied'); },
  setItem() { throw new Error('denied'); },
});
const fakeBtn = () => {
  const handlers = {};
  return {
    attrs: {}, title: '',
    setAttribute(k, v) { this.attrs[k] = v; },
    addEventListener(ev, fn) { handlers[ev] = fn; },
    click() { handlers.click(); },
  };
};

test('normalizeTheme accepts only light; everything else is dark', () => {
  assert.equal(normalizeTheme('light'), 'light');
  for (const v of ['dark', '', null, undefined, 'LIGHT', 'blue', 42]) assert.equal(normalizeTheme(v), 'dark');
});

test('toggleTheme flips between dark and light', () => {
  assert.equal(toggleTheme('dark'), 'light');
  assert.equal(toggleTheme('light'), 'dark');
  assert.equal(toggleTheme(undefined), 'light');
});

test('loadTheme / saveTheme round-trip and ignore invalid stored values', () => {
  const s = memStorage();
  assert.equal(loadTheme(s), 'dark');
  saveTheme('light', s);
  assert.equal(s.data[THEME_KEY], 'light');
  assert.equal(loadTheme(s), 'light');
  s.data[THEME_KEY] = 'garbage';
  assert.equal(loadTheme(s), 'dark');
  saveTheme('nonsense', s);
  assert.equal(s.data[THEME_KEY], 'dark');
});

test('storage failures never throw (private windows / blocked site data)', () => {
  assert.equal(loadTheme(brokenStorage()), 'dark');
  assert.doesNotThrow(() => saveTheme('light', brokenStorage()));
  assert.equal(loadTheme(null), 'dark');
});

test('applyTheme sets data-theme and the button accessibility state', () => {
  const root = { dataset: {} };
  const btn = fakeBtn();
  assert.equal(applyTheme('light', root, btn), 'light');
  assert.equal(root.dataset.theme, 'light');
  assert.equal(btn.attrs['aria-pressed'], 'true');
  assert.match(btn.title, /Koyu/);
  assert.equal(applyTheme('whatever', root, btn), 'dark');
  assert.equal(root.dataset.theme, 'dark');
  assert.equal(btn.attrs['aria-pressed'], 'false');
});

test('applyTheme tolerates a missing root or button', () => {
  assert.equal(applyTheme('light', null, null), 'light');
});

test('initTheme restores the saved theme, toggles on click and persists', () => {
  const root = { dataset: {} };
  const btn = fakeBtn();
  const storage = memStorage({ [THEME_KEY]: 'light' });
  const get = initTheme(btn, { root, storage });
  assert.equal(root.dataset.theme, 'light');
  assert.equal(get(), 'light');

  btn.click();
  assert.equal(root.dataset.theme, 'dark');
  assert.equal(storage.data[THEME_KEY], 'dark');
  btn.click();
  assert.equal(root.dataset.theme, 'light');
  assert.equal(storage.data[THEME_KEY], 'light');
});

test('initTheme without a button still applies the stored theme', () => {
  const root = { dataset: {} };
  initTheme(null, { root, storage: memStorage({ [THEME_KEY]: 'light' }) });
  assert.equal(root.dataset.theme, 'light');
});

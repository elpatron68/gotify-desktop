/**
 * Shell i18n: catalog comes from Go (internal/i18n) so tray/menu and UI stay in sync.
 * HTML nodes use data-i18n="<key>" and optional data-i18n-attr="placeholder|title|…".
 */

/** @type {Record<string, string>} */
let catalog = {};

/** @type {string} */
let locale = 'en';

export function getLocale() {
  return locale;
}

export function t(key, vars) {
  let s = catalog[key] ?? key;
  if (vars && typeof vars === 'object') {
    for (const [k, v] of Object.entries(vars)) {
      s = s.replaceAll(`{{.${k}}}`, String(v));
    }
  }
  return s;
}

export function applyDom(root = document) {
  root.querySelectorAll('[data-i18n]').forEach((el) => {
    const key = el.getAttribute('data-i18n');
    if (!key) return;
    const attr = el.getAttribute('data-i18n-attr');
    const value = t(key);
    if (attr) {
      el.setAttribute(attr, value);
    } else {
      el.textContent = value;
    }
  });
  const title = t('setup.title');
  if (title && title !== 'setup.title') {
    document.title = title;
  }
  if (document.documentElement) {
    document.documentElement.lang = locale;
  }
}

/**
 * @param {{locale: string, catalog: Record<string, string>}} payload
 */
export function loadCatalog({locale: loc, catalog: cat}) {
  locale = loc || 'en';
  catalog = cat && typeof cat === 'object' ? cat : {};
  applyDom();
}

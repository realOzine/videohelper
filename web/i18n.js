// Chinese / English for every page. Static text carries data-i18n="key" (and data-i18n-attr="attr:key;…");
// scripts call I18N.t(key, vars) and re-render on I18N.on(fn). The choice is remembered per browser.
window.I18N = (() => {
  const dict = { zh: {}, en: {} }, subs = [];
  // a link ending in #en or #zh wins, then the visitor's last choice, then the browser's language
  let saved = null;
  try { saved = localStorage.getItem('vh-lang'); } catch { /* storage blocked: fall back to the browser */ }
  const hash = location.hash.slice(1);
  let lang = hash === 'en' || hash === 'zh' ? hash : saved || ((navigator.language || 'zh').toLowerCase().startsWith('zh') ? 'zh' : 'en');
  if (lang !== 'en') lang = 'zh';
  const t = (key, vars = {}) => {
    const s = dict[lang][key] ?? dict.zh[key] ?? key;
    return String(s).replace(/\{(\w+)\}/g, (_, k) => (vars[k] ?? ''));
  };
  function apply(root = document) {
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
    root.querySelectorAll('[data-i18n]').forEach((el) => { el.textContent = t(el.dataset.i18n); });
    root.querySelectorAll('[data-i18n-html]').forEach((el) => { el.innerHTML = t(el.dataset.i18nHtml); });
    root.querySelectorAll('[data-i18n-attr]').forEach((el) => el.dataset.i18nAttr.split(';').forEach((pair) => { const [a, k] = pair.split(':'); el.setAttribute(a, t(k)); }));
    root.querySelectorAll('[data-lang]').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.lang === lang)));
    if (dict[lang]['page.title']) document.title = t('page.title');
  }
  function set(next) {
    if (next === lang) return;
    lang = next;
    try { localStorage.setItem('vh-lang', lang); } catch { /* not remembered, still switched */ }
    apply(); subs.forEach((fn) => fn(lang));
  }
  document.addEventListener('click', (e) => { const b = e.target.closest('[data-lang]'); if (b) set(b.dataset.lang); });
  return { t, apply, set, on: (fn) => subs.push(fn), add: (zh, en) => { Object.assign(dict.zh, zh); Object.assign(dict.en, en); }, get lang() { return lang; } };
})();

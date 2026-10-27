// Chinese / English. A link ending in #en or #zh wins, then the visitor's last choice, then the browser's language.
// Components call useLang() so a switch re-renders them in place.
import { useSyncExternalStore } from 'react';

export type Lang = 'zh' | 'en';
type Dict = Record<string, string>;
const dict: Record<Lang, Dict> = { zh: {}, en: {} };
const subs = new Set<() => void>();

function initial(): Lang {
  const hash = location.hash.slice(1);
  if (hash === 'en' || hash === 'zh') return hash;
  let saved: string | null = null;
  try { saved = localStorage.getItem('vh-lang'); } catch { /* storage blocked: fall back to the browser */ }
  if (saved === 'en' || saved === 'zh') return saved;
  return (navigator.language || 'zh').toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

let lang: Lang = initial();
document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';

export const addStrings = (zh: Dict, en: Dict) => { Object.assign(dict.zh, zh); Object.assign(dict.en, en); };

export const t = (key: string, vars: Record<string, string | number> = {}) =>
  String(dict[lang][key] ?? dict.zh[key] ?? key).replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? ''));

export function setLang(next: Lang) {
  if (next === lang) return;
  lang = next;
  try { localStorage.setItem('vh-lang', lang); } catch { /* not remembered, still switched */ }
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
  // Notifications can synchronously unsubscribe and resubscribe during a React render.
  for (const fn of [...subs]) fn();
}

export function subscribeLanguage(fn: () => void) {
  subs.add(fn);
  return () => { subs.delete(fn); };
}

export const useLang = () => useSyncExternalStore(subscribeLanguage, () => lang);

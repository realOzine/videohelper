import { useSyncExternalStore, type MouseEvent } from 'react';

const changed = 'videohelper:query';
export function subscribeQuery(listener: () => void) {
  window.addEventListener('popstate', listener);
  window.addEventListener(changed, listener);
  return () => {
    window.removeEventListener('popstate', listener);
    window.removeEventListener(changed, listener);
  };
}

export function setPageQuery(params: URLSearchParams) {
  const search = params.size ? `?${params}` : '';
  if (search === location.search) return;
  history.pushState(null, '', location.pathname + search + location.hash);
  window.dispatchEvent(new Event(changed));
}

// Opt in only on pages whose filters react to query changes; other page links still navigate normally.
export function followQueryLink(e: MouseEvent<HTMLElement>) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const link = (e.target as Element).closest('a');
  if (!link || link.target || link.hasAttribute('download')) return;
  const url = new URL(link.href);
  if (url.protocol !== location.protocol || url.host !== location.host || url.pathname !== location.pathname || (url.hash && url.hash !== location.hash)) return;
  e.preventDefault();
  setPageQuery(url.searchParams);
}

export function usePageQuery() {
  return new URLSearchParams(useSyncExternalStore(subscribeQuery, () => location.search));
}

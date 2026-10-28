// Search selection shares the current entry with the detail view; Back still returns to its gallery.
export function searchURL(extra: Record<string, string | null>): string {
  const url = new URL(location.href);
  for (const [key, value] of Object.entries(extra)) {
    if (value === null) url.searchParams.delete(key);
    else url.searchParams.set(key, value);
  }
  return url.pathname + url.search + url.hash;
}

export function searchMoment(count: number): number {
  if (!count) return -1;
  const index = Number(new URLSearchParams(location.search).get('m'));
  return Number.isInteger(index) && index >= 0 ? Math.min(index, count - 1) : 0;
}

export function selectSearchMoment(index: number) {
  history.replaceState({ ...history.state, m: index }, '', searchURL({ view: 'detail', m: String(index) }));
}

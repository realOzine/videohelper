import assert from 'node:assert/strict';
import test from 'node:test';
import { searchMoment, searchURL, selectSearchMoment } from './searchHistory.ts';

test('changing a hit replaces its detail URL and state while retaining the gallery return entry', () => {
  const gallery = { url: new URL('mygo://localhost/search.html?q=coffee&project=9&view=dir#en'), state: { scroll: 840, marker: 'gallery' } };
  const entries: { url: URL; state: Record<string, unknown> }[] = [gallery];
  let current = 0;
  Object.defineProperty(globalThis, 'location', { configurable: true, get: () => entries[current].url });
  Object.assign(globalThis, { history: {
    get state() { return entries[current].state; },
    replaceState(state: Record<string, unknown>, _: string, url?: string) { entries[current] = { state, url: url ? new URL(url, location.href) : entries[current].url }; },
  } });
  // Opening a gallery card pushes one detail entry. Subsequent list, J/K and lane choices share it.
  entries.push({ url: new URL(searchURL({ view: 'detail', m: '0' }), location.href), state: { m: 0, fromDir: true, scroll: 0, marker: 'detail' } });
  current = 1;
  for (const index of [1, 4, 2]) {
    selectSearchMoment(index);
    assert.equal(searchMoment(7), index, 'reload reads the current URL selection');
    assert.equal(history.state.m, index);
    assert.equal(history.state.fromDir, true);
    assert.equal(history.state.marker, 'detail');
    assert.equal(history.state.scroll, 0);
  }
  assert.equal(entries.length, 2, 'stepping through hits must not add Back entries');
  assert.equal(location.search, '?q=coffee&project=9&view=detail&m=2');
  assert.equal(location.hash, '#en');
  current = 0;
  assert.equal(history.state.scroll, 840, 'Back retains the originating gallery scroll');
  assert.equal(location.search, '?q=coffee&project=9&view=dir');
  current = 1;
  assert.equal(searchMoment(7), 2, 'Forward restores the last detail selection');
  assert.equal(searchURL({ view: 'dir', m: null }), '/search.html?q=coffee&project=9&view=dir#en');
});

test('URL selection is bounded and rejects malformed or negative hit indexes', () => {
  let current = new URL('mygo://localhost/search.html?m=3');
  Object.defineProperty(globalThis, 'location', { configurable: true, get: () => current });
  assert.equal(searchMoment(2), 1);
  assert.equal(searchMoment(0), -1);
  for (const index of ['-1', '1.5', 'NaN', '2tail', 'Infinity', '']) {
    current.searchParams.set('m', index);
    assert.equal(searchMoment(7), 0);
  }
});

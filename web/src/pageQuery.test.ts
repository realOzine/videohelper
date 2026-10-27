import assert from 'node:assert/strict';
import test from 'node:test';
import type { MouseEvent } from 'react';
import { followQueryLink, setPageQuery, subscribeQuery } from './pageQuery.ts';

test('library filters update history without navigating, retain the language hash and follow Back', () => {
  const browser = new EventTarget();
  let current = new URL('mygo://localhost/library.html?project=9#zh');
  const entries = [current.href];
  Object.assign(globalThis, {
    window: browser,
    history: { pushState: (_: unknown, __: string, url: string) => { current = new URL(url, current); entries.push(current.href); } },
  });
  Object.defineProperty(globalThis, 'location', { configurable: true, get: () => current });
  const observed: string[] = [];
  const stop = subscribeQuery(() => observed.push(location.search));
  const click = (href: string, options: Record<string, unknown> = {}, target = '', download = false) => {
    let prevented = false;
    const e = { button: 0, target: { closest: () => ({ href, target, hasAttribute: () => download }) }, preventDefault: () => { prevented = true; }, ...options };
    followQueryLink(e as unknown as MouseEvent<HTMLElement>);
    return prevented;
  };
  try {
    assert(click('mygo://localhost/library.html?project=9&status=importing'));
    assert.equal(current.href, 'mygo://localhost/library.html?project=9&status=importing#zh');
    assert.deepEqual(observed, ['?project=9&status=importing']);
    assert(click(current.href));
    assert.equal(entries.length, 2, 'clicking the selected filter must not add another history entry');
    setPageQuery(new URLSearchParams({ status: 'failed', project: '12' }));
    assert.equal(current.search, '?status=failed&project=12');
    current = new URL(entries[1]);
    browser.dispatchEvent(new Event('popstate'));
    assert.equal(observed.at(-1), '?project=9&status=importing');
    for (const option of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey', 'defaultPrevented']) assert.equal(click(current.href, { [option]: true }), false);
    assert.equal(click(current.href, { button: 1 }), false);
    assert.equal(click(current.href, {}, '_blank'), false);
    assert.equal(click(current.href, {}, '', true), false);
    assert.equal(click('mygo://localhost/video.html?id=1'), false);
    assert.equal(click('mygo://other/library.html'), false, 'custom schemes with a null origin must still check the host');
    assert.equal(click('https://localhost/library.html'), false);
    assert.equal(click('mygo://localhost/library.html#other'), false);
    assert.equal(entries.length, 3);
  } finally { stop(); }
  const count = observed.length;
  setPageQuery(new URLSearchParams());
  browser.dispatchEvent(new Event('popstate'));
  assert.equal(current.href, 'mygo://localhost/library.html#zh');
  assert.equal(observed.length, count, 'unmounted pages must stop listening');
});

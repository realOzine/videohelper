import assert from 'node:assert/strict';
import test from 'node:test';

test('language notifications visit resubscribing listeners once per change', async () => {
  const saved = new Map<string, string>();
  const html = { lang: '' };
  Object.assign(globalThis, {
    location: { hash: '#en' }, document: { documentElement: html },
    localStorage: { getItem: (key: string) => saved.get(key), setItem: (key: string, value: string) => saved.set(key, value) },
  });
  const { setLang, subscribeLanguage } = await import('./i18n.ts');
  let calls = 0, otherCalls = 0;
  let unsubscribe = () => {};
  const listener = () => {
    calls++;
    assert.ok(calls <= 2, 'a resubscribed listener was revisited during the same notification');
    unsubscribe();
    unsubscribe = subscribeLanguage(listener);
  };
  unsubscribe = subscribeLanguage(listener);
  const stopOther = subscribeLanguage(() => { otherCalls++; });
  try {
    setLang('zh');
    assert.equal(calls, 1);
    assert.equal(otherCalls, 1);
    assert.equal(html.lang, 'zh-CN');
    setLang('en');
    assert.equal(calls, 2);
    assert.equal(otherCalls, 2);
    assert.equal(saved.get('vh-lang'), 'en');
    setLang('en');
    assert.equal(calls, 2);
  } finally { unsubscribe(); stopOther(); }
});

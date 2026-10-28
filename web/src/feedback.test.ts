import assert from 'node:assert/strict';
import test from 'node:test';
import { errorSummaryKey, libraryEmptyKey, runtimeMessageKey } from './feedback.ts';

test('errors use stable codes and keep unknown technical messages out of the summary', () => {
  assert.equal(errorSummaryKey('network'), 'err.network');
  assert.equal(errorSummaryKey('invalid_argument'), errorSummaryKey('invalid_input'));
  assert.equal(errorSummaryKey('invalid'), 'err.input');
  assert.equal(errorSummaryKey('cancelled'), 'desktop.cancelled');
  assert.equal(errorSummaryKey('not_ready'), 'err.notReady');
  assert.equal(errorSummaryKey('media_not_ready'), 'err.notReady');
  assert.equal(errorSummaryKey('unavailable'), 'err.unavailable');
  for (const code of [undefined, '', 'ffprobe: secret source path', '__proto__']) assert.equal(errorSummaryKey(code), 'err.generic');
});

test('an empty filtered list never claims the whole library or project is empty', () => {
  assert.equal(libraryEmptyKey('', ''), 'l.empty');
  assert.equal(libraryEmptyKey('2', ''), 'l.emptyProject');
  for (const project of ['', '2']) for (const status of ['failed', 'cancelled', 'deleted']) assert.equal(libraryEmptyKey(project, status), 'l.emptyFilter');
});

test('runtime phases distinguish cache checks from actual installation and model downloads', () => {
  for (const stage of ['extracting', 'tools', 'checking']) assert.equal(runtimeMessageKey(stage), 'desktop.checking');
  assert.equal(runtimeMessageKey('checking_models'), 'desktop.checkingModels');
  for (const stage of ['python', 'packages']) assert.equal(runtimeMessageKey(stage), 'desktop.installing');
  assert.equal(runtimeMessageKey('models'), 'desktop.models');
  for (const stage of ['installed', 'loading']) assert.equal(runtimeMessageKey(stage), 'desktop.loading');
  assert.equal(runtimeMessageKey('detecting'), 'desktop.detecting');
  for (const stage of [undefined, '', 'unexpected backend message']) assert.equal(runtimeMessageKey(stage), 'desktop.wait');
});

test('warm startup messages never claim downloads or installation in either UI language', async () => {
  Object.assign(globalThis, {
    location: { hash: '#en' }, document: { documentElement: { lang: '' } },
    localStorage: { getItem: () => null, setItem: () => {} },
  });
  const { t, setLang } = await import('./i18n.ts');
  await import('./strings.ts');
  const warm = ['extracting', 'tools', 'checking', 'detecting', 'checking_models', 'installed', 'loading'];
  for (const lang of ['en', 'zh'] as const) {
    setLang(lang);
    for (const stage of warm) {
      const message = t(runtimeMessageKey(stage));
      assert.doesNotMatch(message, /install|download|安装|下载/i, `${lang}: ${stage}`);
      assert.doesNotMatch(message, /^desktop\./, 'the stage must have a translation');
    }
    assert.match(t(runtimeMessageKey('checking_models')), /cached|缓存/);
    assert.match(t(runtimeMessageKey('packages')), /install.*missing|安装缺失/i);
    assert.match(t(runtimeMessageKey('models')), /download.*missing|下载缺失/i);
    assert.match(t(runtimeMessageKey('loading')), /loading|加载/i);
  }
});

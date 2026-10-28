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
  assert.equal(runtimeMessageKey('packages'), 'desktop.installing');
  assert.equal(runtimeMessageKey('models'), 'desktop.models');
  assert.equal(runtimeMessageKey('loading'), 'desktop.loading');
  assert.equal(runtimeMessageKey('unexpected backend message'), 'desktop.wait');
});

import assert from 'node:assert/strict';
import test from 'node:test';
import type { Stage } from './lib.ts';

Object.assign(globalThis, {
  location: { hash: '#en' }, document: { documentElement: { lang: '' } },
  localStorage: { getItem: () => null, setItem: () => {} },
});
const { setLang } = await import('./i18n.ts');
await import('./strings.ts');
const { videoProgress } = await import('./pages/format.ts');
const stage = (name: string, status = 'done'): Stage => ({ stage: name, status, attempt: 1, updated_at: '2026-10-28T00:00:00Z' });

test('ready summary names only completed embedding channels and respects missing audio', () => {
  setLang('en');
  const v = { status: 'ready', has_audio: true };
  assert.equal(videoProgress(v, [stage('embed:visual'), stage('asr')]), 'Processing complete. Search by Picture.');
  assert.equal(videoProgress(v, [stage('embed:visual'), stage('asr'), stage('embed:speech'), stage('embed:audio')]), 'Processing complete. Search by Picture / Speech / Sound.');
  assert.equal(videoProgress({ ...v, has_audio: false }, [stage('embed:visual'), stage('embed:speech'), stage('embed:audio')]), 'Processing complete. Search by Picture.');
  assert.match(videoProgress(v, []), /status is currently unavailable/);
  setLang('zh');
  assert.equal(videoProgress({ ...v, has_audio: false }, [stage('embed:visual')]), '处理完成，可按画面搜索。');
});

test('queued, copying, running, cancelled and failed have distinct truthful summaries without a growing denominator', () => {
  setLang('en');
  const summary = (status: string, stages: Stage[] = []) => videoProgress({ status, has_audio: null }, stages);
  assert.match(summary('indexing'), /^Queued/);
  assert.match(summary('uploading'), /^Copying/);
  assert.match(summary('indexing', [stage('embed:visual', 'running')]), /^Processing\. Search opens when all processing finishes/);
  assert.match(summary('cancelled'), /^Processing canceled/);
  assert.match(summary('failed'), /^Processing did not finish/);
  assert.match(summary('deleted'), /Restore/);
  for (const status of ['indexing', 'importing', 'cancelled', 'failed']) assert.doesNotMatch(summary(status), /0\s*\/\s*0/);
  setLang('zh');
  assert.match(summary('cancelled'), /已取消/);
});

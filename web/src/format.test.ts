import assert from 'node:assert/strict';
import test from 'node:test';
import type { Stage } from './lib.ts';

Object.assign(globalThis, {
  location: { hash: '#en' }, document: { documentElement: { lang: '' } },
  localStorage: { getItem: () => null, setItem: () => {} },
});
const { setLang } = await import('./i18n.ts');
await import('./strings.ts');
const { videoError, videoProgress } = await import('./pages/format.ts');
const { ApiError } = await import('./api.ts');
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

test('errors use localized stable codes or stage context and retain raw diagnostic details', () => {
  const raw = 'ffprobe /private/media/中文 文件.mp4: Invalid data found\nmoov atom not found';
  setLang('en');
  const damaged = videoError(raw, 'probe');
  assert.match(damaged.summary, /could not be read/);
  assert.doesNotMatch(damaged.summary, /ffprobe|moov|private|中文/);
  assert.equal(damaged.detail, raw);
  const cancelled = new ApiError(499, 'cancelled', '任务已取消');
  assert.equal(videoError(cancelled).summary, 'The operation was canceled. Retry when needed.');
  assert.equal(videoError(cancelled).detail, '任务已取消');
  assert.match(videoError(new ApiError(503, 'not_ready', 'backend endpoint died'), 'probe').summary, /Local models/);
  assert.match(videoError(new ApiError(0, 'network', '')).summary, /local service/);
  assert.equal(videoError(new ApiError(0, 'network', '')).detail, undefined);
  assert.match(videoError('opaque tool failure', 'embed:speech:local-id').summary, /encoding did not finish/);
  assert.match(videoError('opaque tool failure', 'new_stage').summary, /technical details/);
  assert.equal(videoError(new Error(raw)).detail, raw);
  setLang('zh');
  assert.match(videoError(raw, 'probe').summary, /无法读取视频/);
  assert.match(videoError(cancelled).summary, /操作已取消/);
  assert.equal(videoError(raw, 'probe').detail, raw);
});

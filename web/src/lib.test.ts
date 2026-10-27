import assert from 'node:assert/strict';
import test from 'node:test';
import { byVideo, channelState, defaultView, eachLimit, marks, modelPrefs, partRange, pipeline, searchFields, tc, ticks } from './lib.ts';

test('gallery groups moments by video in rank order, and picks the opening view', () => {
  const ms = [{ video_id: '7' }, { video_id: '3' }, { video_id: '7' }, { video_id: '9' }];
  assert.deepEqual(byVideo(ms).map((g) => [g.id, g.items.map((x) => x.i)]), [['7', [0, 2]], ['3', [1]], ['9', [3]]]);
  assert.equal(defaultView(ms), 'dir');
  assert.equal(defaultView([{ video_id: '7' }, { video_id: '7' }]), 'detail');
});

const at = '2026-10-26T00:00:00Z';
const stages = [
  { stage: 'prepare_media', status: 'done', attempt: 1, updated_at: at }, { stage: 'probe', status: 'done', attempt: 1, updated_at: at },
  { stage: 'embed:speech:wemm-2b@r1', status: 'pending', attempt: 1, updated_at: at }, { stage: 'asr', status: 'failed', attempt: 3, updated_at: at },
  { stage: 'embed:visual:wemm-2b@r1', status: 'done', attempt: 1, updated_at: at }, { stage: 'finalize', status: 'pending', attempt: 1, updated_at: at },
];

test('timecodes', () => {
  assert.equal(tc(0), '00:00');
  assert.equal(tc(752300, true), '12:32.3');
  assert.equal(tc(3725000), '62:05');
  assert.equal(tc(9990, true), '00:09.9');
});

test('pipeline puts probe first, asr before speech encoding, and drops empty lanes', () => {
  const p = pipeline(stages);
  assert.deepEqual(p.before.map((s) => s.stage), ['probe', 'prepare_media']);
  assert.deepEqual(p.lanes.map(([c, l]) => [c, l.map((s) => s.stage)]), [['visual', ['embed:visual:wemm-2b@r1']], ['speech', ['asr', 'embed:speech:wemm-2b@r1']]]);
  assert.deepEqual(p.after.map((s) => s.stage), ['finalize']);
});

test('channel state', () => {
  assert.deepEqual(channelState(stages), { visual: 'done', speech: 'failed', audio: 'none' });
});

test('upload parts cover the file exactly', () => {
  assert.deepEqual(partRange(1, 16, 40), [0, 16]);
  assert.deepEqual(partRange(3, 16, 40), [32, 40]);
});

test('eachLimit bounds concurrency and stops after a failure', async () => {
  let live = 0, peak = 0;
  const seen: number[] = [];
  await assert.rejects(eachLimit([1, 2, 3, 4, 5, 6], 2, async (n) => {
    live++; peak = Math.max(peak, live); seen.push(n);
    await new Promise((r) => setTimeout(r, 5));
    live--;
    if (n === 2) throw new Error('part 2');
  }), /part 2/);
  assert.equal(peak, 2);
  assert.ok(seen.length < 6, `kept uploading after the failure: ${seen}`);
});

test('dictionary markup splits without HTML', () => {
  assert.deepEqual(marks('共 <b>《a》</b>，运行 <code>x</code>。'), [{ tag: '', text: '共 ' }, { tag: 'b', text: '《a》' }, { tag: '', text: '，运行 ' }, { tag: 'code', text: 'x' }, { tag: '', text: '。' }]);
  assert.deepEqual(marks('<img src=x onerror=1>'), [{ tag: '', text: '<img src=x onerror=1>' }]);
});

test('timeline ticks', () => {
  assert.deepEqual(ticks(90000), [0, 30000, 60000]);
  assert.deepEqual(ticks(28000), [0, 10000, 20000]);
  assert.equal(ticks(3480000).length, 6);
});

test('model choice keeps only what differs from the defaults', () => {
  const chs = [
    { channel: 'visual', default_space_ids: ['wemm-2b@r1'], default_weight: 1 },
    { channel: 'speech', default_space_ids: ['wemm-2b@r1'], default_weight: 1 },
    { channel: 'audio', default_space_ids: ['clap@r1'], default_weight: 0 },
  ];
  const same = modelPrefs(chs, { visual: 'wemm-2b@r1', speech: 'wemm-2b@r1', audio: 'clap@r1' }, { visual: true, speech: true, audio: false });
  assert.deepEqual(same, { spaces: {}, weights: {} });
  assert.deepEqual(searchFields(same), {});
  const p = modelPrefs(chs, { visual: 'wemm-2b-cpu@r1', speech: 'wemm-2b@r1', audio: 'clap@r1' }, { visual: true, speech: false, audio: true });
  assert.deepEqual(searchFields(p), { space_overrides: { visual: 'wemm-2b-cpu@r1' }, channel_weights: { speech: 0, audio: 1 } });
});

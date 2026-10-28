import assert from 'node:assert/strict';
import test from 'node:test';
import { byVideo, channelState, defaultView, hasActiveChannel, marks, modelName, modelPrefs, pipeline, readModelPrefs, searchFields, seekMedia, tc, ticks, timeRange, usableModelPrefs } from './lib.ts';

test('result seeking waits for metadata of the selected video instead of seeking the previous source', () => {
  const video = { readyState: 4, currentSrc: 'http://localhost/media/old.mp4', currentTime: 5 };
  const source = 'http://localhost/media/new.mp4';
  assert.equal(seekMedia(video, source, 1109), false);
  assert.equal(video.currentTime, 5);
  video.currentSrc = source; video.readyState = 0;
  assert.equal(seekMedia(video, source, 1109), false);
  assert.equal(video.currentTime, 5);
  video.readyState = 1;
  assert.equal(seekMedia(video, source, 1109), true);
  assert.equal(video.currentTime, 1.109);
});

test('model names keep the model revision and omit local fingerprints', () => {
  const fingerprint = 'a1'.repeat(32);
  assert.equal(modelName(`wemm-2b@r1.${fingerprint}`), 'WeMM-Embedding-2B · r1');
  assert.equal(modelName(`clap-htsat-unfused@r1.${fingerprint}`), 'CLAP HTSAT unfused · r1');
  assert.equal(modelName(`wemm-2b-cpu@r1.${fingerprint}`), 'WeMM-Embedding-2B · CPU · r1');
});

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

test('short time ranges retain distinct endpoints without changing ordinary ranges', () => {
  assert.equal(timeRange(6000, 6083), '00:06.000 – 00:06.083');
  assert.equal(timeRange(6198, 6199), '00:06.198 – 00:06.199');
  assert.equal(timeRange(6100, 7200), '00:06.1 – 00:07.2');
  assert.equal(timeRange(59999, 60000), '00:59.9 – 01:00.0');
});

test('pipeline puts probe first, asr before speech encoding, and drops empty lanes', () => {
  const p = pipeline([...stages, { stage: 'fetch', status: 'done', attempt: 1, updated_at: at }]);
  assert.deepEqual(p.before.map((s) => s.stage), ['fetch', 'probe', 'prepare_media']);
  assert.deepEqual(p.lanes.map(([c, l]) => [c, l.map((s) => s.stage)]), [['visual', ['embed:visual:wemm-2b@r1']], ['speech', ['asr', 'embed:speech:wemm-2b@r1']]]);
  assert.deepEqual(p.after.map((s) => s.stage), ['finalize']);
});

test('channel state', () => {
  assert.deepEqual(channelState(stages), { visual: 'done', speech: 'failed', audio: 'none' });
});

test('speech is searchable only after speech embedding, not transcription alone', () => {
  const asr = { stage: 'asr', status: 'done', attempt: 1, updated_at: at };
  const embed = { ...asr, stage: 'embed:speech:wemm-2b@r1' };
  assert.equal(channelState([asr]).speech, 'waiting');
  assert.equal(channelState([asr, { ...embed, status: 'running' }]).speech, 'waiting');
  assert.equal(channelState([asr, { ...embed, status: 'failed' }]).speech, 'failed');
  assert.equal(channelState([asr, embed]).speech, 'done');
  assert.equal(channelState([]).speech, 'none');
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

test('saved model preferences reject malformed shapes, channels and non-finite or negative weights', () => {
  const defaults = { spaces: {}, weights: {} };
  for (const raw of [null, '{', 'null', '[]', '{"spaces":[],"weights":{}}', '{"spaces":{},"weights":[]}',
    '{"spaces":{"visual":42},"weights":{}}', '{"spaces":{"visual":"  "},"weights":{}}',
    '{"spaces":{},"weights":{"visual":"0"}}', '{"spaces":{},"weights":{"visual":-1}}',
    '{"spaces":{},"weights":{"visual":1e999}}', '{"spaces":{},"weights":{"unknown":1}}']) {
    assert.deepEqual(readModelPrefs(raw), defaults, String(raw));
  }
  const p = { spaces: { visual: 'wemm@r1' }, weights: { speech: 0, audio: 1 } };
  assert.deepEqual(readModelPrefs(JSON.stringify(p)), p);
});

test('effective channels inherit server defaults and invalid historical choices fall back before search', () => {
  const chs = [
    { channel: 'visual', default_space_ids: ['v1'], default_weight: 1, models: [{ space_id: 'v1', status: 'ready' }] },
    { channel: 'speech', default_space_ids: ['s1'], default_weight: 1, models: [{ space_id: 's1', status: 'indexing' }] },
    { channel: 'audio', default_space_ids: ['a1'], default_weight: 0, models: [{ space_id: 'a1', status: 'ready' }] },
  ];
  const defaults = { spaces: {}, weights: {} };
  assert.equal(hasActiveChannel(chs, defaults), true);
  const partial = readModelPrefs('{"spaces":{},"weights":{"visual":0}}');
  assert.equal(hasActiveChannel(chs, partial), true, 'speech still inherits its positive server weight');
  assert.equal(usableModelPrefs(partial, chs), partial);
  const off = modelPrefs(chs, {}, { visual: false, speech: false, audio: false });
  assert.deepEqual(off.weights, { visual: 0, speech: 0 }, 'audio already defaults to zero');
  assert.equal(hasActiveChannel(chs, off), false);
  assert.deepEqual(searchFields(usableModelPrefs(readModelPrefs(JSON.stringify(off)), chs)), {}, 'legacy all-off storage must search with defaults');
  const sound = modelPrefs(chs, {}, { visual: false, speech: false, audio: true });
  assert.equal(hasActiveChannel(chs, sound), true);
  assert.deepEqual(usableModelPrefs(sound, chs), sound);
  assert.equal(hasActiveChannel(chs.map((c) => ({ ...c, default_weight: c.channel === 'audio' ? 1 : 0 })), defaults), true, 'defaults come from the catalog, not hardcoded visual/speech weights');
  assert.deepEqual(usableModelPrefs({ spaces: { visual: 'old-model-fingerprint' }, weights: {} }, chs), defaults);
  const currentDefault = { spaces: { speech: 's1' }, weights: { audio: 1 } };
  assert.deepEqual(searchFields(usableModelPrefs(currentDefault, chs)), { channel_weights: { audio: 1 } }, 'a model that became the default must not be sent as a strict complete-index override');
});

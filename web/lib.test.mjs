import assert from 'node:assert/strict';
import test from 'node:test';
import { channelState, eachLimit, esc, partRange, pipeline, safeNext, tc, ticks, tile } from './lib.js';

const stages = [
  { stage: 'prepare_media', status: 'done' }, { stage: 'probe', status: 'done' },
  { stage: 'embed:speech:wemm-2b@r1', status: 'pending' }, { stage: 'asr', status: 'failed' },
  { stage: 'embed:visual:wemm-2b@r1', status: 'done' }, { stage: 'finalize', status: 'pending' },
];

test('timecodes', () => {
  assert.equal(tc(0), '00:00');
  assert.equal(tc(752300, true), '12:32.3');
  assert.equal(tc(3725000), '62:05');
  assert.equal(tc(9990, true), '00:09.9');
});

test('pipeline puts asr before speech encoding and drops empty lanes', () => {
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
  const seen = [];
  await assert.rejects(eachLimit([1, 2, 3, 4, 5, 6], 2, async (n) => {
    live++; peak = Math.max(peak, live); seen.push(n);
    await new Promise((r) => setTimeout(r, 5));
    live--;
    if (n === 2) throw new Error('part 2');
  }), /part 2/);
  assert.equal(peak, 2);
  assert.ok(seen.length < 6, `kept uploading after the failure: ${seen}`);
});

test('markup is escaped', () => {
  assert.equal(esc('<a href="x">'), '&lt;a href=&quot;x&quot;&gt;');
  assert.match(tile({ url: 'https://s/1.jpg?a=1&b=2', x: 320, y: 0, w: 320, h: 180 }), /viewBox="320 0 320 180".*href="https:\/\/s\/1\.jpg\?a=1&amp;b=2"/);
});

test('ticks and sign-in redirects', () => {
  assert.deepEqual(ticks(90000), [0, 30000, 60000]);
  assert.deepEqual(ticks(28000), [0, 10000, 20000]);
  assert.equal(ticks(3480000).length, 6);
  assert.equal(safeNext('video.html?id=3'), 'video.html?id=3');
  assert.equal(safeNext('https://evil.example/x.html'), 'search.html');
  assert.equal(safeNext('//evil.example'), 'search.html');
});

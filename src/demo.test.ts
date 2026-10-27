import assert from 'node:assert/strict';
import test from 'node:test';
import { advance, CYCLE, DIR, HOLD, phase, take, TYPE } from './landing/timeline.ts';

test('a round types, shows the gallery, then the detail, then moves to the next query', () => {
  assert.equal(phase(0), 'typing');
  assert.equal(phase(TYPE), 'gallery');
  assert.equal(phase(DIR), 'detail');
  let c = { q: 5, t: CYCLE - 100, sel: 2, hold: 0 };
  c = advance(c, 100, 0, 6);
  assert.deepEqual(c, { q: 0, t: 0, sel: 0, hold: 0 });
});

test('taking over holds the gallery, lets the detail play to its end, then resumes', () => {
  const now = 1000;
  let c = take({ q: 1, t: 0, sel: 0, hold: 0 }, now, { t: TYPE + 1000 });
  assert.equal(advance(c, 100, now + 50, 6).t, TYPE + 1000, 'gallery frozen while held');
  c = take(c, now, { t: CYCLE - 150, sel: 3 });
  c = advance(advance(c, 100, now + 50, 6), 100, now + 60, 6);
  assert.equal(c.t, CYCLE - 100, 'detail stops short of the next query while held');
  assert.equal(c.q, 1);
  c = advance(c, 100, now + HOLD + 1, 6);
  assert.equal(c.q, 2, 'resumes after the hold');
});

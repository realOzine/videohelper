import assert from 'node:assert/strict';
import test from 'node:test';
import { tabIndexForKey } from './tabs.ts';

test('filter tabs wrap with arrows, reach either end, and leave native keys alone', () => {
  assert.equal(tabIndexForKey('ArrowRight', 6, 7), 0);
  assert.equal(tabIndexForKey('ArrowLeft', 0, 7), 6);
  assert.equal(tabIndexForKey('ArrowRight', 2, 7), 3);
  assert.equal(tabIndexForKey('Home', 5, 7), 0);
  assert.equal(tabIndexForKey('End', 2, 7), 6);
  for (const key of ['Tab', 'Enter', 'Escape']) assert.equal(tabIndexForKey(key, 2, 7), null);
  assert.equal(tabIndexForKey('ArrowRight', 0, 0), null);
  assert.equal(tabIndexForKey('ArrowRight', -1, 7), null);
});

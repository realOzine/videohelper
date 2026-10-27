import assert from 'node:assert/strict';
import test from 'node:test';
import { ApiError, connect, get, post } from './api.ts';

test('desktop connection stays in memory and API errors do not navigate to login', async () => {
  let bootstraps = 0;
  Object.assign(globalThis, { window: { mygo: { call: async (method: string) => {
    assert.equal(method, 'Desktop.Bootstrap'); bootstraps++;
    return { base: 'http://127.0.0.1:12345', token: 'process-capability' };
  } } } });
  const originalFetch = globalThis.fetch;
  const calls: [string, RequestInit | undefined][] = [];
  globalThis.fetch = async (url, init) => {
    calls.push([String(url), init]);
    if (String(url).endsWith('/cancel')) return new Promise<Response>((_, reject) => {
      const signal = init?.signal;
      assert.ok(signal, 'list reads must forward cancellation to fetch');
      if (signal.aborted) reject(signal.reason);
      else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
    });
    return String(url).endsWith('/fail') ? new Response(JSON.stringify({ error: { code: 'not_ready', message: 'Models are preparing' } }), { status: 503 }) : new Response('{"videos":[]}');
  };
  try {
    await Promise.all([connect(), connect()]);
    assert.equal(bootstraps, 1);
    assert.deepEqual(await get('/v1/videos'), { videos: [] });
    await post('/v1/imports/url', { url: 'https://example.com/video' });
    assert.equal(calls[0][0], 'http://127.0.0.1:12345/v1/videos');
    assert.equal((calls[0][1]?.headers as Record<string, string>)['X-Desktop-Token'], 'process-capability');
    assert.equal(calls[1][1]?.body, '{"url":"https://example.com/video"}');
    await assert.rejects(get('/fail'), (err: unknown) => err instanceof ApiError && err.code === 'not_ready' && err.message === 'Models are preparing');
    const controller = new AbortController(), reason = new Error('filter changed');
    const pending = get('/cancel', controller.signal);
    controller.abort(reason);
    await assert.rejects(pending, (err: unknown) => err === reason);
  } finally { globalThis.fetch = originalFetch; }
});

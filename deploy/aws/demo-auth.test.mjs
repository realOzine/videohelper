import assert from 'node:assert/strict';
import { createPublicKey, verify } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { setTimeout } from 'node:timers/promises';
import test from 'node:test';

test('stable private key, signed eight-hour CLI token, JWKS-only server without token logs', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'videohelper-auth-'));
  const script = new URL('./demo-auth.mjs', import.meta.url).pathname;
  const env = { ...process.env, VH_AUTH_DIR: directory };
  let server;
  try {
    const issue = () => {
      const result = spawnSync(process.execPath, [script, 'token'], { env, encoding: 'utf8' });
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stderr, '');
      return result.stdout.trim();
    };
    const token = issue();
    const pem = readFileSync(join(directory, 'signing-key.pem'));
    assert.equal(statSync(join(directory, 'signing-key.pem')).mode & 0o777, 0o600);
    const [header, payload, signature] = token.split('.');
    const claims = JSON.parse(Buffer.from(payload, 'base64url'));
    assert.equal(claims.iss, 'http://auth:18001');
    assert.equal(claims.aud, 'videohelper-demo');
    assert.equal(claims.sub, 'videohelper-demo-user');
    assert.equal(claims.exp - claims.iat, 8 * 3600);
    assert.ok(claims.exp > Date.now() / 1000);
    const publicKey = createPublicKey(pem);
    assert.ok(verify('sha256', Buffer.from(`${header}.${payload}`), publicKey, Buffer.from(signature, 'base64url')));
    assert.ok(!verify('sha256', Buffer.from(`${header}.${payload}tampered`), publicKey, Buffer.from(signature, 'base64url')));
    issue();
    assert.deepEqual(readFileSync(join(directory, 'signing-key.pem')), pem);

    let output = '';
    server = spawn(process.execPath, [script, 'serve'], { env, stdio: ['ignore', 'pipe', 'pipe'] });
    server.stdout.on('data', (chunk) => { output += chunk; });
    server.stderr.on('data', (chunk) => { output += chunk; });
    for (let i = 0; i < 100 && !output.includes('JWKS ready'); i++) {
      assert.equal(server.exitCode, null, output);
      await setTimeout(25);
    }
    assert.match(output, /JWKS ready/);
    const response = await fetch('http://127.0.0.1:18001/jwks');
    assert.equal(response.status, 200);
    const { keys } = await response.json();
    assert.equal(keys.length, 1);
    assert.equal(keys[0].kid, JSON.parse(Buffer.from(header, 'base64url')).kid);
    assert.equal(keys[0].d, undefined);
    assert.ok(verify('sha256', Buffer.from(`${header}.${payload}`), createPublicKey({ key: keys[0], format: 'jwk' }), Buffer.from(signature, 'base64url')));
    assert.equal((await fetch('http://127.0.0.1:18001/token')).status, 404);
    assert.equal((await fetch('http://127.0.0.1:18001/jwks', { method: 'POST' })).status, 404);
    server.kill('SIGTERM');
    const [code] = await once(server, 'exit');
    assert.equal(code, 0);
    assert.doesNotMatch(output, /eyJ|PRIVATE KEY/);
    assert.equal(output, 'Demo public JWKS ready on port 18001\n');
  } finally {
    if (server?.exitCode === null) server.kill('SIGKILL');
    rmSync(directory, { recursive: true, force: true });
  }
});

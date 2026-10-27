// Single-user SSM demo: only the operator's local CLI can issue tokens.
import { createHash, createPrivateKey, createPublicKey, generateKeyPairSync, sign } from 'node:crypto';
import { chmodSync, linkSync, mkdirSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { join } from 'node:path';

const directory = process.env.VH_AUTH_DIR || '/data';
const issuer = 'http://auth:18001', audience = 'videohelper-demo';
const command = process.argv[2];
if (!['serve', 'token'].includes(command)) throw new Error('Usage: node demo-auth.mjs serve|token');
mkdirSync(directory, { recursive: true, mode: 0o700 });
const keyPath = join(directory, 'signing-key.pem');
try {
  readFileSync(keyPath);
} catch (error) {
  if (error.code !== 'ENOENT') throw error;
  const { privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
  const temporary = `${keyPath}.${process.pid}`;
  writeFileSync(temporary, privateKey.export({ type: 'pkcs8', format: 'pem' }), { mode: 0o600, flag: 'wx' });
  try {
    // Link atomically without replacing a key created by another process.
    linkSync(temporary, keyPath);
  } catch (error) {
    if (error.code !== 'EEXIST') throw error;
  } finally {
    unlinkSync(temporary);
  }
}
chmodSync(keyPath, 0o600);
const privateKey = createPrivateKey(readFileSync(keyPath));
const publicKey = createPublicKey(privateKey);
const kid = createHash('sha256').update(publicKey.export({ type: 'spki', format: 'der' })).digest('hex');
const jwks = JSON.stringify({ keys: [{ ...publicKey.export({ format: 'jwk' }), kid, alg: 'RS256', use: 'sig' }] });

if (command === 'token') {
  const now = Math.floor(Date.now() / 1000);
  const b64 = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const body = `${b64({ alg: 'RS256', typ: 'JWT', kid })}.${b64({ iss: issuer, aud: audience, sub: 'videohelper-demo-user', name: 'Demo', iat: now, exp: now + 8 * 3600 })}`;
  console.log(`${body}.${sign('sha256', Buffer.from(body), privateKey).toString('base64url')}`);
} else {
  const server = createServer((request, response) => {
    if (request.method !== 'GET' || request.url !== '/jwks') return response.writeHead(404).end();
    response.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'max-age=300' }).end(jwks);
  });
  server.listen(18001, '0.0.0.0', () => console.log('Demo public JWKS ready on port 18001'));
  for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, () => server.close());
}

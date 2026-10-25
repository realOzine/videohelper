// Local development only: a throwaway RS256 issuer for the API's JWT check (config/local.yaml `auth`).
// It serves its public key at /jwks and prints one token; restarting it makes earlier tokens invalid.
// Usage: node web/dev/devauth.mjs [subject]
import { generateKeyPairSync, randomUUID, sign } from 'node:crypto';
import { createServer } from 'node:http';

const port = 18001, issuer = `http://127.0.0.1:${port}`, audience = 'videohelper-local', sub = process.argv[2] || 'dev-user';
const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
const kid = randomUUID();
const jwks = JSON.stringify({ keys: [{ ...publicKey.export({ format: 'jwk' }), kid, alg: 'RS256', use: 'sig' }] });
const b64 = (v) => Buffer.from(JSON.stringify(v)).toString('base64url');
const now = Math.floor(Date.now() / 1000);
const body = `${b64({ alg: 'RS256', typ: 'JWT', kid })}.${b64({ iss: issuer, aud: audience, sub, email: `${sub}@localhost`, name: sub, iat: now, exp: now + 7 * 86400 })}`;
const token = `${body}.${sign('sha256', Buffer.from(body), privateKey).toString('base64url')}`;

createServer((req, res) => {
  if (req.url !== '/jwks') return res.writeHead(404).end();
  res.writeHead(200, { 'Content-Type': 'application/json' }).end(jwks);
}).listen(port, '127.0.0.1', () => {
  console.log(`JWKS at ${issuer}/jwks (issuer ${issuer}, audience ${audience})\ntoken for "${sub}", valid 7 days:\n\n${token}\n`);
});

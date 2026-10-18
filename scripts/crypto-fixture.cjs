// Learner JWTs signed and verified exactly as kaderisasi-web-be does:
// @maximemrf/adonisjs-jwt signs with `jsonwebtoken` and APP_KEY, and
// course_audience_middleware re-verifies with the learner audience.
const { createRequire } = require('node:module');
const { resolve } = require('node:path');
const { readFileSync } = require('node:fs');
const legacy = createRequire(resolve(__dirname, '../tests/interop/package.json'));
const jwt = legacy('jsonwebtoken');
const { Scrypt } = legacy('@adonisjs/hash/drivers/scrypt');
const { Encryption } = legacy('@adonisjs/encryption');
const key = 'synthetic-test-key-never-use-outside-tests';
const password = 'Synthetic-password-2026!';
// kaderisasi-web-be/config/hash.ts
const scrypt = new Scrypt({ cost: 16384, blockSize: 8, parallelization: 1, maxMemory: 33554432 });
const encryption = new Encryption({ secret: key });
(async () => {
  if (process.argv[2] === 'verify') {
    const data = JSON.parse(readFileSync(0, 'utf8'));
    const guard = jwt.verify(data.publicAccess, key, { clockTimestamp: 1700000100 });
    const course = jwt.verify(data.publicAccess, key, { algorithms: ['HS256'], audience: 'kaderisasi-public', clockTimestamp: 1700000100 });
    if (guard.userId !== 42 || course.userId !== 42 || typeof course.exp !== 'number') throw new Error('Go to Adonis learner token compatibility failed');
    if (data.hash !== undefined && !(await scrypt.verify(data.hash, password))) throw new Error('Go to Adonis password hash compatibility failed');
    if (data.encrypted !== undefined && encryption.decrypt(data.encrypted) !== 'fixture@example.test') throw new Error('Go to Adonis encryption compatibility failed');
    if (data.expired !== undefined && encryption.decrypt(data.expired) !== null) throw new Error('Expired Go encryption accepted by Adonis');
    console.log('verified');
  } else {
    console.log(JSON.stringify({
      key,
      publicAccess: jwt.sign({ userId: 42, email: 'fixture@example.test', aud: 'kaderisasi-public', iat: 1700000000 }, key, { expiresIn: '1d' }),
      legacyPublicAccess: jwt.sign({ userId: 42, email: 'fixture@example.test', iat: 1700000000 }, key, { expiresIn: '1d' }),
      password,
      hash: await scrypt.make(password),
      // Expiry is relative to the real clock, as in web-be's reset links.
      encrypted: encryption.encrypt('fixture@example.test', '30 minutes'),
      expired: encryption.encrypt('fixture@example.test', '1ms'),
      unexpiring: encryption.encrypt('fixture@example.test'),
      purposed: encryption.encrypt('fixture@example.test', '30 minutes', 'other'),
      object: encryption.encrypt({ email: 'fixture@example.test' }, '30 minutes'),
    }));
  }
})().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});

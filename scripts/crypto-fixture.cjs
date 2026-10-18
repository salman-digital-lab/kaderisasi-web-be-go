// Learner JWTs signed and verified exactly as kaderisasi-web-be does:
// @maximemrf/adonisjs-jwt signs with `jsonwebtoken` and APP_KEY, and
// course_audience_middleware re-verifies with the learner audience.
const { createRequire } = require('node:module');
const { resolve } = require('node:path');
const { readFileSync } = require('node:fs');
const legacy = createRequire(resolve(__dirname, '../tests/interop/package.json'));
const jwt = legacy('jsonwebtoken');
const key = 'synthetic-test-key-never-use-outside-tests';
try {
  if (process.argv[2] === 'verify') {
    const data = JSON.parse(readFileSync(0, 'utf8'));
    const guard = jwt.verify(data.publicAccess, key, { clockTimestamp: 1700000100 });
    const course = jwt.verify(data.publicAccess, key, { algorithms: ['HS256'], audience: 'kaderisasi-public', clockTimestamp: 1700000100 });
    if (guard.userId !== 42 || course.userId !== 42 || typeof course.exp !== 'number') throw new Error('Go to Adonis learner token compatibility failed');
    console.log('verified');
  } else {
    console.log(JSON.stringify({
      key,
      publicAccess: jwt.sign({ userId: 42, email: 'fixture@example.test', aud: 'kaderisasi-public', iat: 1700000000 }, key, { expiresIn: '1d' }),
      legacyPublicAccess: jwt.sign({ userId: 42, email: 'fixture@example.test', iat: 1700000000 }, key, { expiresIn: '1d' }),
    }));
  }
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}

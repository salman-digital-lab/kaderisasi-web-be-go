// Differential contract check for the Phase 1 auth routes.
//
// Creates an owned schema (web_go_<run>) in the test database, migrates it
// with Ace from ../kaderisasi-admin-be, starts Adonis web-be and the Go web
// API against it with a synthetic APP_KEY and a local SMTP sink, runs the same
// scenarios against both, and compares status, body and database effects.
// The schema is dropped afterwards, also on failure. Never touches `public`.
//
//   node scripts/contract-auth.mjs
import { spawn, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { randomBytes } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseEnv, isDeepStrictEqual } from 'node:util';
import assert from 'node:assert/strict';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspace = resolve(root, '..');
const webBe = resolve(workspace, 'kaderisasi-web-be');
const migrations = resolve(workspace, 'kaderisasi-admin-be');
const { Client } = createRequire(resolve(webBe, 'package.json'))('pg');

const testEnv = parseEnv(readFileSync(resolve(workspace, 'env/test/web-be-test-env'), 'utf8'));
const migrationEnv = parseEnv(readFileSync(resolve(workspace, 'env/test/admin-be-test-env'), 'utf8'));
const prod = parseEnv(readFileSync(resolve(workspace, 'env/prod/web-be-prod-env'), 'utf8'));
for (const env of [testEnv, migrationEnv]) {
  if (['DB_HOST', 'DB_PORT', 'DB_DATABASE'].every((key) => env[key] === prod[key])) throw new Error('Refusing to run against the production database');
}

const run = randomBytes(8).toString('hex');
const schema = `web_go_${run}`;
const marker = `web Go contract ${run}`;
const appKey = 'synthetic-contract-key-only-never-production';
const ports = { adonis: 3346, go: 3347 };
const artifacts = resolve(root, '.artifacts');
mkdirSync(artifacts, { recursive: true });

const db = new Client({ host: testEnv.DB_HOST, port: Number(testEnv.DB_PORT), user: testEnv.DB_USER, password: testEnv.DB_PASSWORD, database: testEnv.DB_DATABASE, connectionTimeoutMillis: 10000, statement_timeout: 30000 });

// --- SMTP sink: records every message by recipient ----------------------
const mailbox = [];
const smtp = createServer((socket) => {
  socket.setEncoding('utf8');
  let buffer = '', data = null, rcpt = '';
  const reply = (line) => socket.write(line + '\r\n');
  reply('220 sink ESMTP');
  socket.on('data', (chunk) => {
    buffer += chunk;
    let index;
    while ((index = buffer.indexOf('\r\n')) >= 0) {
      const line = buffer.slice(0, index); buffer = buffer.slice(index + 2);
      if (data !== null) {
        if (line === '.') { mailbox.push({ to: rcpt, data: data.join('\r\n') }); data = null; reply('250 queued'); }
        else data.push(line.startsWith('..') ? line.slice(1) : line);
        continue;
      }
      const verb = line.split(' ')[0].toUpperCase();
      if (verb === 'EHLO') reply('250-sink\r\n250 AUTH LOGIN PLAIN');
      else if (verb === 'HELO') reply('250 sink');
      else if (verb === 'AUTH') { if (/^AUTH PLAIN \S+/i.test(line)) reply('235 ok'); else { socket.authStep = 1; reply('334 VXNlcm5hbWU6'); } }
      else if (socket.authStep === 1) { socket.authStep = 2; reply('334 UGFzc3dvcmQ6'); }
      else if (socket.authStep === 2) { socket.authStep = 0; reply('235 ok'); }
      else if (verb === 'RCPT') { rcpt = line.match(/<([^>]*)>/)?.[1] ?? ''; reply('250 ok'); }
      else if (verb === 'DATA') { data = []; reply('354 go'); }
      else if (verb === 'QUIT') { reply('221 bye'); socket.end(); }
      else reply('250 ok');
    }
  });
});
await new Promise((ok) => smtp.listen(0, '127.0.0.1', ok));
const smtpPort = String(smtp.address().port);

// --- processes -----------------------------------------------------------
const children = [];
function start(name, command, args, cwd, env) {
  const child = spawn(command, args, { cwd, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] });
  let log = '';
  child.stdout.on('data', (d) => (log += d));
  child.stderr.on('data', (d) => (log += d));
  child.on('exit', () => writeFileSync(resolve(artifacts, `contract-auth-${name}.log`), log));
  children.push(child);
  return () => log;
}
async function waitFor(port, log) {
  for (let i = 0; i < 120; i++) {
    try { if ((await fetch(`http://127.0.0.1:${port}/health`)).ok) return; } catch {}
    await new Promise((ok) => setTimeout(ok, 500));
  }
  throw new Error(`server on ${port} did not start:\n${log()}`);
}

const shared = { APP_KEY: appKey, NODE_ENV: 'development', HOST: '127.0.0.1', TZ: testEnv.TZ ?? 'Asia/Jakarta', LOG_LEVEL: 'warn', SMTP_HOST: '127.0.0.1', SMTP_PORT: smtpPort,
  SMTP_USERNAME: 'sink', SMTP_PASSWORD: 'sink', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset-password',
  GOOGLE_CLIENT_ID: 'contract-client.apps.googleusercontent.com', GOOGLE_CLIENT_SECRET: 'contract-secret', GOOGLE_REDIRECT_URI: 'http://web.contract.test/api/auth/google/callback' };

const failures = [];
let scenarios = 0;
let framesIgnored = 0;
try {
  await db.connect();
  await db.query('BEGIN');
  await db.query(`CREATE SCHEMA "${schema}"`);
  await db.query(`COMMENT ON SCHEMA "${schema}" IS '${marker}'`);
  await db.query('COMMIT');
  const migrated = spawnSync('node', ['ace', 'migration:run', '--force'], { cwd: migrations, encoding: 'utf8',
    env: { ...process.env, ...migrationEnv, DB_SCHEMA: schema, PGOPTIONS: `-c search_path=${schema}`, ADMIN_BOOTSTRAP_EMAILS: 'bootstrap@contract.test', NODE_ENV: 'test' } });
  writeFileSync(resolve(artifacts, 'contract-auth-migrations.log'), migrated.stdout + migrated.stderr);
  if (migrated.status !== 0) throw new Error('Ace migration failed; see .artifacts/contract-auth-migrations.log');
  await db.query(`SET search_path TO "${schema}"`);
  const current = await db.query('SELECT current_schemas(false)::text[] AS schemas');
  assert.deepEqual(current.rows[0].schemas, [schema], 'unsafe search path');

  if (spawnSync('go', ['build', '-o', resolve(root, 'bin/web-api'), './cmd/web-api'], { cwd: root, stdio: 'inherit' }).status !== 0) throw new Error('go build failed');
  const integration = spawnSync('go', ['test', '-race', '-count=1', '-v', '-tags=integration', './internal/member/'], { cwd: root, encoding: 'utf8',
    env: { ...process.env, ...testEnv, ...shared, DB_SCHEMA: schema } });
  console.log((integration.stdout + integration.stderr).split('\n').filter((line) => /^(---|ok|FAIL|PASS)/.test(line.trim())).join('\n'));
  scenarios++;
  if (integration.status !== 0) failures.push({ name: 'go integration tests (internal/member)' });
  const adonisLog = start('adonis', 'node', ['--import', 'ts-node-maintained/register/esm', 'bin/server.ts'], webBe,
    { ...testEnv, ...shared, PORT: String(ports.adonis), PGOPTIONS: `-c search_path=${schema}` });
  const goLog = start('go', resolve(root, 'bin/web-api'), [], root,
    { ...testEnv, ...shared, PORT: String(ports.go), DB_SCHEMA: schema });
  await waitFor(ports.adonis, adonisLog);
  await waitFor(ports.go, goLog);

  // --- helpers -----------------------------------------------------------
  const targets = ['adonis', 'go'];
  const email = (target, name) => `${name}.${target}@contract.test`;
  async function call(target, method, path, { json, form, raw, type, token } = {}) {
    const headers = {};
    let body;
    if (json !== undefined) { headers['content-type'] = 'application/json'; body = JSON.stringify(json); }
    if (form !== undefined) { headers['content-type'] = 'application/x-www-form-urlencoded'; body = new URLSearchParams(form).toString(); }
    if (raw !== undefined) { headers['content-type'] = type ?? 'application/json'; body = raw; }
    if (token) headers.authorization = `Bearer ${token}`;
    const response = await fetch(`http://127.0.0.1:${ports[target]}${path}`, { method, headers, body });
    const text = await response.text();
    let parsed; try { parsed = JSON.parse(text); } catch {}
    return { status: response.status, type: response.headers.get('content-type'), cache: response.headers.get('cache-control'), text, json: parsed };
  }
  // Replace per-run values so the two targets can be compared.
  function normalize(value, target) {
    const ids = new Map();
    const walk = (v, key) => {
      if (typeof v === 'string') {
        if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}(Z|[+-]\d{2}:\d{2})$/.test(v)) return `<date ${v.slice(-6)}>`;
        if (/^eyJ[\w-]+\.[\w-]+\.[\w-]+$/.test(v)) return '<jwt>';
        if (key === 'member_id' && /^\d{8}$/.test(v)) return '<member_id>';
        return v.replaceAll(`.${target}@`, '.<target>@');
      }
      if (Array.isArray(v)) return v.map((item) => walk(item));
      if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, item]) => [k, (k === 'id' || k === 'user_id') && typeof item === 'number' ? '<id>' : walk(item, k)]));
      return v;
    };
    return walk(value);
  }
  // Adonis (NODE_ENV=development) adds debug stack frames to framework
  // errors; Go deliberately does not (docs/MIGRATION.md).
  function compare(name, results) {
    scenarios++;
    const [a, b] = targets.map((t) => results[t]);
    if (a.json && Array.isArray(a.json.frames) && b.json && !('frames' in b.json)) { delete a.json.frames; framesIgnored++; }
    const left = { status: a.status, type: a.type, body: a.json !== undefined ? normalize(a.json, 'adonis') : a.text };
    const right = { status: b.status, type: b.type, body: b.json !== undefined ? normalize(b.json, 'go') : b.text };
    if (!isDeepStrictEqual(left, right)) failures.push({ name, adonis: left, go: right });
    else if (a.json !== undefined && JSON.stringify(left.body) !== JSON.stringify(right.body)) failures.push({ name, keyOrder: true, adonis: left.body, go: right.body });
    return results;
  }
  async function both(name, method, path, options = () => ({})) {
    const results = {};
    for (const target of targets) results[target] = await call(target, method, path, options(target));
    return compare(name, results);
  }
  async function rows(target, name) {
    const users = await db.query('SELECT * FROM public_users WHERE email=$1', [email(target, name)]);
    const profiles = users.rows.length ? await db.query('SELECT * FROM profiles WHERE user_id=$1 ORDER BY id', [users.rows[0].id]) : { rows: [] };
    const shape = (row) => Object.fromEntries(Object.entries(row).map(([k, v]) => [k, k.endsWith('_at') ? (v ? '<timestamp>' : v) : k === 'password' ? (v ? v.replace(/\$[^$]+\$[^$]+$/, '$<salt>$<hash>') : v) : k === 'id' || k === 'user_id' ? '<id>' : k === 'member_id' ? (v ? (v === String(row.id).padStart(8, '0') ? '<padded id>' : v) : v) : v]));
    return normalize({ users: users.rows.map(shape), profiles: profiles.rows.map(shape) }, target);
  }
  async function compareRows(name, user) {
    scenarios++;
    const [a, b] = await Promise.all(targets.map((t) => rows(t, user)));
    if (!isDeepStrictEqual(a, b)) failures.push({ name: `${name} (database)`, adonis: a, go: b });
  }

  // --- scenarios ---------------------------------------------------------
  const register = (t, name, extra = {}) => ({ json: { fullname: `Member ${name}`, email: email(t, name), password: 'Synthetic-password-2026!', ...extra } });
  await both('register: empty body', 'POST', '/v2/auth/register');
  await both('register: empty object', 'POST', '/v2/auth/register', () => ({ json: {} }));
  await both('register: wrong types', 'POST', '/v2/auth/register', () => ({ json: { fullname: 1, email: true, password: ['x'] } }));
  await both('register: empty strings', 'POST', '/v2/auth/register', () => ({ json: { fullname: '', email: '', password: '' } }));
  await both('register: invalid email', 'POST', '/v2/auth/register', () => ({ json: { fullname: 'x', email: 'not-an-email', password: 'p' } }));
  await both('register: invalid JSON', 'POST', '/v2/auth/register', () => ({ raw: '{"fullname":' }));
  await both('register: non-object JSON', 'POST', '/v2/auth/register', () => ({ raw: '"text"' }));
  await both('register: array JSON', 'POST', '/v2/auth/register', () => ({ raw: '["a"]' }));
  await both('register: success', 'POST', '/v2/auth/register', (t) => register(t, 'alpha'));
  await compareRows('register: success', 'alpha');
  await both('register: duplicate', 'POST', '/v2/auth/register', (t) => register(t, 'alpha'));
  await both('register: mixed case and spaces', 'POST', '/v2/auth/register', (t) => ({ json: { fullname: 'Beta', email: `  ${email(t, 'BETA').toUpperCase()}  `, password: 'pw' } }));
  await compareRows('register: mixed case and spaces', 'beta');
  await both('register: form body', 'POST', '/v2/auth/register', (t) => ({ form: { fullname: 'Gamma', email: email(t, 'gamma'), password: 'pw' } }));
  await compareRows('register: form body', 'gamma');
  await both('register: query string fields', 'POST', `/v2/auth/register?fullname=Delta&password=pw`, (t) => ({ json: { email: email(t, 'delta') } }));
  await compareRows('register: query string fields', 'delta');

  await both('check-email: invalid', 'POST', '/v2/auth/check-email', () => ({ json: { email: 'x' } }));
  await both('check-email: registered', 'POST', '/v2/auth/check-email', (t) => ({ json: { email: email(t, 'alpha').toUpperCase() } }));
  await both('check-email: available', 'POST', '/v2/auth/check-email', (t) => ({ json: { email: email(t, 'nobody') } }));

  await both('login: validation', 'POST', '/v2/auth/login', () => ({ json: { email: 'x' } }));
  await both('login: unknown', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'nobody'), password: 'x' } }));
  await both('login: wrong password', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'alpha'), password: 'wrong' } }));
  const logins = await both('login: success', 'POST', '/v2/auth/login', (t) => ({ json: { email: ` ${email(t, 'alpha').toUpperCase()} `, password: 'Synthetic-password-2026!' } }));
  for (const t of targets) {
    await db.query("INSERT INTO public_users (email, password, account_status, created_at) VALUES ($1, NULL, 'active', now()), ($2, 'legacy-plain-text', 'active', now()), ($3, $4, 'inactive', now())",
      [email(t, 'nopassword'), email(t, 'legacyhash'), email(t, 'inactive'), (await db.query('SELECT password FROM public_users WHERE email=$1', [email(t, 'alpha')])).rows[0].password]);
  }
  await both('login: no password', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'nopassword'), password: 'x' } }));
  await both('login: non-scrypt hash', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'legacyhash'), password: 'x' } }));
  await both('login: inactive account', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'inactive'), password: 'Synthetic-password-2026!' } }));

  // Tokens issued by each side work on the other one.
  const tokens = Object.fromEntries(targets.map((t) => [t, logins[t].json?.data?.token?.token]));
  await both('logout: anonymous', 'PUT', '/v2/auth/logout');
  await both('logout: junk token', 'PUT', '/v2/auth/logout', () => ({ token: 'junk' }));
  await both('logout: token from the other side', 'PUT', '/v2/auth/logout', (t) => ({ token: tokens[t === 'go' ? 'adonis' : 'go'] }));
  await both('notifications: Go-issued token through the proxy', 'GET', '/v2/notifications/unread-count', () => ({ token: tokens.go }));

  await both('forgot-password: validation', 'POST', '/v2/auth/forgot-password', () => ({ json: {} }));
  await both('forgot-password: unknown', 'POST', '/v2/auth/forgot-password', (t) => ({ json: { email: email(t, 'nobody') } }));
  mailbox.length = 0;
  await both('forgot-password: success', 'POST', '/v2/auth/forgot-password', (t) => ({ json: { email: email(t, 'alpha') } }));
  const links = {};
  for (const t of targets) {
    const message = mailbox.find((m) => m.to === email(t, 'alpha'));
    if (!message) { failures.push({ name: `forgot-password: ${t} sent no email` }); continue; }
    const [headers, ...rest] = message.data.split('\r\n\r\n');
    const body = rest.join('\r\n\r\n').replace(/=\r\n/g, '').replace(/=([0-9A-F]{2})/g, (_, h) => String.fromCharCode(parseInt(h, 16)));
    links[t] = body.match(/reset-password\?token=([\w.-]+)/)?.[1];
    const subject = headers.match(/^Subject: (.*)$/m)?.[1];
    const from = headers.match(/^From: (.*)$/m)?.[1];
    links[`${t}:html`] = body.replace(/token=[\w.-]+/, 'token=<token>');
    links[`${t}:headers`] = { subject, from };
  }
  scenarios++;
  if (links['adonis:html'] !== links['go:html'] || !isDeepStrictEqual(links['adonis:headers'], links['go:headers'])) failures.push({ name: 'forgot-password: email content', adonis: links['adonis:headers'], go: links['go:headers'], htmlEqual: links['adonis:html'] === links['go:html'] });

  await both('reset-password: no token', 'PUT', '/v2/auth/reset-password', () => ({ json: { password: 'x' } }));
  await both('reset-password: bad token', 'PUT', '/v2/auth/reset-password?token=a.b.c', () => ({ json: { password: 'x' } }));
  await both('reset-password: validation first', 'PUT', '/v2/auth/reset-password?token=a.b.c', () => ({ json: {} }));
  // Cross-implementation: each side accepts the link the other side mailed.
  for (const t of targets) {
    const other = t === 'go' ? 'adonis' : 'go';
    const result = await call(t, 'PUT', `/v2/auth/reset-password?token=${encodeURIComponent(links[other] ?? '')}`, { json: { password: 'Wrong-account-2026!' } });
    scenarios++;
    // The other side's link names the other side's account; it must still decrypt.
    if (result.status !== 200) failures.push({ name: `reset-password: ${t} rejected the ${other} link`, status: result.status, body: result.text });
  }
  for (const t of targets) {
    const result = await call(t, 'PUT', `/v2/auth/reset-password?token=${encodeURIComponent(links[t] ?? '')}`, { json: { password: 'New-password-2026!' } });
    scenarios++;
    if (result.status !== 200) failures.push({ name: `reset-password: ${t} own link`, status: result.status, body: result.text });
  }
  await both('login: after reset', 'POST', '/v2/auth/login', (t) => ({ json: { email: email(t, 'alpha'), password: 'New-password-2026!' } }));
  await compareRows('reset-password: stored hash', 'alpha');

  await both('google: missing fields', 'POST', '/v2/auth/google', () => ({ json: {} }));
  await both('google: short verifier', 'POST', '/v2/auth/google', () => ({ json: { code: 'c', codeVerifier: 'short', nonce: 'n'.repeat(32) } }));
} finally {
  for (const child of children) child.kill('SIGTERM');
  await new Promise((ok) => setTimeout(ok, 500));
  smtp.close();
  try {
    await db.query('SET search_path TO DEFAULT');
    const owner = await db.query('SELECT obj_description(oid) AS comment FROM pg_namespace WHERE nspname=$1', [schema]);
    if (owner.rows.length && owner.rows[0].comment === marker) await db.query(`DROP SCHEMA "${schema}" CASCADE`);
    else if (owner.rows.length) console.error(`Schema ${schema} has an unexpected owner marker; not dropped`);
  } finally { await db.end(); }
}

writeFileSync(resolve(artifacts, 'contract-auth.json'), JSON.stringify({ run, scenarios, failures, at: new Date().toISOString() }, null, 2) + '\n');
const real = failures.filter((f) => !f.keyOrder);
for (const f of failures) console.log(`${f.keyOrder ? 'KEY ORDER' : 'DIFF'}  ${f.name}\n  adonis: ${JSON.stringify(f.adonis ?? f)}\n  go:     ${JSON.stringify(f.go ?? '')}`);
console.log(`${scenarios} comparisons, ${real.length} differences, ${failures.length - real.length} key-order-only differences, ${framesIgnored} Adonis debug frame lists ignored. Schema ${schema} dropped.`);
process.exitCode = real.length ? 1 : 0;

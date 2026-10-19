// Shared differential-contract harness: an owned web_go_<run> schema in the
// test database migrated by Ace, Adonis web-be and the Go web API started
// against it, and response comparison. Used by scripts/contract-*.mjs.
import { spawn, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { randomBytes } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseEnv, isDeepStrictEqual } from 'node:util';

export const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspace = resolve(root, '..');
const webBe = resolve(workspace, 'kaderisasi-web-be');
const migrations = resolve(workspace, 'kaderisasi-admin-be');
const { Client } = createRequire(resolve(webBe, 'package.json'))('pg');

export const testEnv = parseEnv(readFileSync(resolve(workspace, 'env/test/web-be-test-env'), 'utf8'));
const migrationEnv = parseEnv(readFileSync(resolve(workspace, 'env/test/admin-be-test-env'), 'utf8'));
const prod = parseEnv(readFileSync(resolve(workspace, 'env/prod/web-be-prod-env'), 'utf8'));
for (const env of [testEnv, migrationEnv]) {
  if (['DB_HOST', 'DB_PORT', 'DB_DATABASE'].every((key) => env[key] === prod[key])) throw new Error('Refusing to run against the production database');
}
export const artifacts = resolve(root, '.artifacts');
mkdirSync(artifacts, { recursive: true });

export const appKey = 'synthetic-contract-key-only-never-production';

/**
 * Runs `body(ctx)` with an owned, migrated schema and both servers started.
 * The schema is dropped afterwards, also on failure.
 */
export async function withContract(name, { ports, env = {} }, body) {
  const run = randomBytes(8).toString('hex');
  const schema = `web_go_${run}`;
  const marker = `web Go contract ${run}`;
  const db = new Client({ host: testEnv.DB_HOST, port: Number(testEnv.DB_PORT), user: testEnv.DB_USER, password: testEnv.DB_PASSWORD, database: testEnv.DB_DATABASE, connectionTimeoutMillis: 10000, statement_timeout: 30000 });
  const children = [];
  const shared = { APP_KEY: appKey, NODE_ENV: 'development', HOST: '127.0.0.1', TZ: testEnv.TZ ?? 'Asia/Jakarta', LOG_LEVEL: 'warn', ...env };
  const start = (label, command, args, cwd, childEnv) => {
    const child = spawn(command, args, { cwd, env: { ...process.env, ...childEnv }, stdio: ['ignore', 'pipe', 'pipe'] });
    let log = '';
    child.stdout.on('data', (d) => (log += d));
    child.stderr.on('data', (d) => (log += d));
    child.on('exit', () => writeFileSync(resolve(artifacts, `${name}-${label}.log`), log));
    children.push(child);
    return () => log;
  };
  const waitFor = async (port, log) => {
    for (let i = 0; i < 120; i++) {
      try { if ((await fetch(`http://127.0.0.1:${port}/health`)).ok) return; } catch {}
      await new Promise((ok) => setTimeout(ok, 500));
    }
    throw new Error(`server on ${port} did not start:\n${log()}`);
  };
  try {
    await db.connect();
    await db.query('BEGIN');
    await db.query(`CREATE SCHEMA "${schema}"`);
    await db.query(`COMMENT ON SCHEMA "${schema}" IS '${marker}'`);
    await db.query('COMMIT');
    const migrated = spawnSync('node', ['ace', 'migration:run', '--force'], { cwd: migrations, encoding: 'utf8',
      env: { ...process.env, ...migrationEnv, DB_SCHEMA: schema, PGOPTIONS: `-c search_path=${schema}`, ADMIN_BOOTSTRAP_EMAILS: 'bootstrap@contract.test', NODE_ENV: 'test' } });
    writeFileSync(resolve(artifacts, `${name}-migrations.log`), migrated.stdout + migrated.stderr);
    if (migrated.status !== 0) throw new Error(`Ace migration failed; see .artifacts/${name}-migrations.log`);
    await db.query(`SET search_path TO "${schema}"`);
    const current = await db.query('SELECT current_schemas(false)::text[] AS schemas');
    if (current.rows[0].schemas.length !== 1 || current.rows[0].schemas[0] !== schema) throw new Error('unsafe search path');
    if (spawnSync('go', ['build', '-o', resolve(root, 'bin/web-api'), './cmd/web-api'], { cwd: root, stdio: 'inherit' }).status !== 0) throw new Error('go build failed');
    const ctx = { db, schema, ports, shared, env: { ...testEnv, ...shared } };
    ctx.startServers = async () => {
      const adonisLog = start('adonis', 'node', ['--import', 'ts-node-maintained/register/esm', 'bin/server.ts'], webBe,
        { ...testEnv, ...shared, PORT: String(ports.adonis), PGOPTIONS: `-c search_path=${schema}` });
      const goLog = start('go', resolve(root, 'bin/web-api'), [], root,
        { ...testEnv, ...shared, PORT: String(ports.go), DB_SCHEMA: schema, WEB_BE_LEGACY_URL: `http://127.0.0.1:${ports.adonis}` });
      await waitFor(ports.adonis, adonisLog);
      await waitFor(ports.go, goLog);
    };
    return await body(ctx);
  } finally {
    for (const child of children) child.kill('SIGTERM');
    await new Promise((ok) => setTimeout(ok, 500));
    try {
      await db.query('SET search_path TO DEFAULT');
      const owner = await db.query('SELECT obj_description(oid) AS comment FROM pg_namespace WHERE nspname=$1', [schema]);
      if (owner.rows.length && owner.rows[0].comment === marker) await db.query(`DROP SCHEMA "${schema}" CASCADE`);
      else if (owner.rows.length) console.error(`Schema ${schema} has an unexpected owner marker; not dropped`);
      console.log(`Schema ${schema} dropped.`);
    } finally { await db.end(); }
  }
}

/** Compares responses from the two targets; collects differences. */
export class Comparison {
  constructor(ports, { headers = [], normalize = (v) => v } = {}) {
    this.ports = ports;
    this.headers = headers;
    this.normalize = normalize;
    this.failures = [];
    this.count = 0;
    this.framesIgnored = 0;
  }

  async call(target, method, path, { json, headers = {} } = {}) {
    const init = { method, headers: { ...headers } };
    if (json !== undefined) { init.headers['content-type'] = 'application/json'; init.body = JSON.stringify(json); }
    const response = await fetch(`http://127.0.0.1:${this.ports[target]}${path}`, init);
    const text = await response.text();
    let parsed; try { parsed = JSON.parse(text); } catch {}
    const picked = Object.fromEntries(this.headers.map((h) => [h, response.headers.get(h)]));
    return { status: response.status, type: response.headers.get('content-type'), headers: picked, text, json: parsed };
  }

  compare(name, a, b) {
    this.count++;
    if (a.json && Array.isArray(a.json.frames) && b.json && !('frames' in b.json)) { delete a.json.frames; this.framesIgnored++; }
    const shape = (r, target) => ({ status: r.status, type: r.type, headers: r.headers, body: r.json !== undefined ? this.normalize(r.json, target) : r.text });
    const left = shape(a, 'adonis');
    const right = shape(b, 'go');
    if (!isDeepStrictEqual(left, right)) this.failures.push({ name, adonis: left, go: right });
    else if (a.json !== undefined && JSON.stringify(left.body) !== JSON.stringify(right.body)) this.failures.push({ name, keyOrder: true, adonis: left.body, go: right.body });
    else if (a.json !== undefined && a.text !== b.text) this.failures.push({ name, bytes: true, adonis: a.text.slice(0, 400), go: b.text.slice(0, 400) });
  }

  async both(name, method, path, options) {
    const a = await this.call('adonis', method, path, options);
    const b = await this.call('go', method, path, options);
    this.compare(name, a, b);
    return { adonis: a, go: b };
  }

  report(name) {
    writeFileSync(resolve(artifacts, `${name}.json`), JSON.stringify({ comparisons: this.count, failures: this.failures, at: new Date().toISOString() }, null, 2) + '\n');
    const real = this.failures.filter((f) => !f.keyOrder && !f.bytes);
    for (const f of this.failures) {
      const kind = f.keyOrder ? 'KEY ORDER' : f.bytes ? 'BYTES' : 'DIFF';
      console.log(`${kind}  ${f.name}\n  adonis: ${JSON.stringify(f.adonis ?? '').slice(0, 1500)}\n  go:     ${JSON.stringify(f.go ?? '').slice(0, 1500)}`);
    }
    console.log(`${this.count} comparisons, ${real.length} differences, ${this.failures.length - real.length} key-order or byte-only differences, ${this.framesIgnored} Adonis debug frame lists ignored.`);
    return real.length === 0;
  }
}

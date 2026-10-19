// Differential contract check for phase 6: courses (with the course
// audience check) and the owner certificate routes. Twin schemas and fake S3
// stores holding the same course documents.
//
//   node scripts/contract-courses.mjs
import { isDeepStrictEqual } from 'node:util';
import { withContract, Comparison, learnerToken, fakeS3, normalizeValue, appKey } from './contract-lib.mjs';
import { createHmac } from 'node:crypto';

const ports = { adonis: 3354, go: 3355 };
const json = (v) => JSON.stringify(v);
const smtp = { SMTP_HOST: '127.0.0.1', SMTP_PORT: '2', SMTP_USERNAME: 'x', SMTP_PASSWORD: 'x', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset' };

async function seed(q) {
  const user = async (email, level) => {
    const { rows: [u] } = await q(`INSERT INTO public_users (email, account_status, created_at) VALUES ($1, 'active', now()) RETURNING id`, [email]);
    if (level !== undefined) await q(`INSERT INTO profiles (user_id, name, level, created_at) VALUES ($1, $2, $3, now())`, [u.id, email.split('@')[0], level]);
    return u.id;
  };
  const novice = await user('novice@contract.test', null);
  const middle = await user('middle@contract.test', 3);
  const senior = await user('senior@contract.test', 10);
  const ghost = await user('ghost@contract.test');

  const course = async (title, level, status, description = '<p>Desk <script>x</script><b>ripsi</b></p>') =>
    (await q(`INSERT INTO courses (title, summary, description, minimum_level, status) VALUES ($1, $2, $3, $4, $5) RETURNING id`, [title, `Ringkasan ${title}`, description, level, status])).rows[0].id;
  const courses = {};
  for (let i = 0; i < 15; i++) courses[`c${i}`] = await course(`Kelas ${i % 3 === 0 ? 'Dasar' : 'Lanjut'} ${i}`, [0, 3, 6, 10][i % 4], i === 13 ? 'draft' : i === 14 ? 'archived' : 'published');
  const lesson = async (courseId, title, position, deleted = false) =>
    (await q(`INSERT INTO course_lessons (course_id, title, description, youtube_video_id, position, deleted_at) VALUES ($1, $2, '<h2 style="text-align:center;color:red">Materi</h2><img src=x>', 'dQw4w9WgXcQ', $3, $4) RETURNING id`,
      [courseId, title, position, deleted ? new Date('2025-01-01') : null])).rows[0].id;
  const l1 = await lesson(courses.c0, 'Pengantar', 1);
  const l2 = await lesson(courses.c0, 'Lanjutan', 2);
  const l3 = await lesson(courses.c0, 'Sama posisi', 2);
  const gone = await lesson(courses.c0, 'Dihapus', 3, true);
  const l5 = await lesson(courses.c1, 'Level 3', 1);
  const l6 = await lesson(courses.c2, 'Level 6', 1);
  const doc = async (lessonId, name, key, deleted = false) =>
    (await q(`INSERT INTO course_documents (lesson_id, storage_key, filename, size_bytes, deleted_at) VALUES ($1, $2, $3, 1234, $4) RETURNING id`, [lessonId, key, name, deleted ? new Date() : null])).rows[0].id;
  const d1 = await doc(l1, 'Materi Ü & "kutip" (1).pdf', 'courses/d1.pdf');
  const d2 = await doc(l1, 'hapus.pdf', 'courses/d2.pdf', true);
  const d3 = await doc(gone, 'dihapus.pdf', 'courses/d3.pdf');
  const d4 = await doc(l2, 'hilang.pdf', 'courses/missing.pdf');
  await q(`INSERT INTO course_lesson_progress (user_id, lesson_id, first_visited_at, last_visited_at, completed_at) VALUES
    ($1, $2, '2025-01-01', '2025-01-02', '2025-01-02'), ($1, $3, '2025-01-01', '2025-01-03', NULL), ($1, $4, '2025-01-01', '2025-01-04', '2025-01-04')`, [senior, l1, l2, gone]);

  // Certificates
  const activity = (await q(`INSERT INTO activities (name, slug, is_published, created_at) VALUES ('Kegiatan', 'kegiatan', true, now()) RETURNING id`)).rows[0].id;
  const reg = async (userId, status) => (await q(`INSERT INTO activity_registrations (user_id, activity_id, status, created_at) VALUES ($1, $2, $3, now()) RETURNING id`, [userId, activity, status])).rows[0].id;
  const rActive = await reg(senior, 'LULUS KEGIATAN');
  const rRevoked = await reg(senior, 'LULUS KEGIATAN');
  const rNone = await reg(senior, 'LULUS KEGIATAN');
  const rPending = await reg(senior, 'TERDAFTAR');
  const rApproval = await reg(senior, 'LULUS KEGIATAN');
  const rSigned = await reg(senior, 'LULUS KEGIATAN');
  const rOther = await reg(middle, 'LULUS KEGIATAN');
  const rBroken = await reg(senior, 'LULUS KEGIATAN');
  const guestReg = (await q(`INSERT INTO activity_registrations (user_id, activity_id, status, created_at) VALUES (NULL, $1, 'LULUS KEGIATAN', now()) RETURNING id`, [activity])).rows[0].id;
  const { rows: [template] } = await q(`INSERT INTO certificate_templates (name, created_at) VALUES ('T', now()) RETURNING id`);
  const plain = { name: 'Templat', background_image: 'bg.png', template_data: { backgroundUrl: null, elements: [{ id: 'e', type: 'variable-text', variable: '{{name}}' }], canvasWidth: 800, canvasHeight: 600 } };
  const needsApproval = { ...plain, template_data: { ...plain.template_data, elements: [{ id: 'a', type: 'variable-text', variable: '{{ approval }}' }] } };
  const participant = { registration_id: 0, user_id: senior, name: 'Senior', email: 's@x', university: 'ITB', activity_name: 'Kegiatan', activity_date: '1 Januari 2025', '7': 'angka' };
  const issue = async (code, regId, userId, { revoked = null, templateSnapshot = plain, approval = null, activitySnapshot = { id: activity, name: 'Kegiatan', activity_start: '2025-01-01' }, participantSnapshot = participant } = {}) =>
    q(`INSERT INTO issued_certificates (certificate_code, registration_id, activity_id, user_id, template_id, template_snapshot, participant_snapshot, activity_snapshot, approval_snapshot, issued_at, revoked_at, revoked_reason, created_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '2025-04-01T00:00:00.123Z', $10, $11, now())`,
      [code, regId, activity, userId, template.id, json(templateSnapshot), json(participantSnapshot), json(activitySnapshot), approval && json(approval), revoked, revoked && 'Salah nama']);
  await issue('SAL-ACTIVE', rActive, senior);
  await issue('SAL-OLD', rRevoked, senior, { revoked: '2025-04-02T00:00:00Z' });
  await issue('SAL-NEW', rRevoked, senior, { participantSnapshot: { ...participant, gender: null } });
  await issue('SAL-APPROVAL', rApproval, senior, { templateSnapshot: needsApproval });
  await issue('SAL-SIGNED', rSigned, senior, { templateSnapshot: needsApproval, approval: { signer_name: 'Ketua', signer_title: 'Ketua', approved_at: '2025-03-01', secret: 'x' } });
  await issue('SAL-OTHER', rOther, middle);
  await issue('SAL-BROKEN', rBroken, senior, { templateSnapshot: { name: 'x' } });
  await issue('SAL-NOSNAP', guestReg, null, { activitySnapshot: null });
  return { novice, middle, senior, ghost, courses, l1, l2, l3, gone, l5, l6, d1, d2, d3, d4, rActive, rRevoked, rNone, rPending, rApproval, rSigned, rOther, rBroken, guestReg };
}

const recent = (v) => /^\d{4}-\d{2}-\d{2}T/.test(v) && Math.abs(Date.now() - Date.parse(v)) < 864e5;
function normalize(v, key) {
  if (typeof v === 'string') return recent(v) ? '<now>' : normalizeValue(key, v);
  if (Array.isArray(v)) return v.map((item) => normalize(item));
  if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, item]) => [k, normalize(item, k)]));
  return v;
}
const normalizeText = (t) => t.replace(/"(\d{4}-\d{2}-\d{2}T[^"]+)"/g, (m, v) => (recent(v) ? '"<now>"' : m));
const comparison = new Comparison(ports, { headers: ['cache-control', 'content-disposition', 'x-content-type-options'], normalize: (v) => normalize(v), normalizeText });

const s3 = { adonis: await fakeS3(), go: await fakeS3() };
for (const store of Object.values(s3)) for (const key of ['courses/d1.pdf', 'courses/d2.pdf', 'courses/d3.pdf']) store.objects.set(`contract/${key}`, { body: Buffer.from(`%PDF-1.4 ${key}`), contentType: 'application/pdf' });
const drive = { DRIVE_DISK: 'minio', DRIVE_REGION: 'us-east-1', DRIVE_BUCKET: 'contract', DRIVE_ACCESS_KEY_ID: 'x', DRIVE_SECRET_ACCESS_KEY: 'x' };
const ok = await withContract('contract-courses', { ports, env: { ...smtp, ...drive }, twin: true,
  adonisEnv: { DRIVE_ENDPOINT: s3.adonis.endpoint }, goEnv: { DRIVE_ENDPOINT: s3.go.endpoint } }, async (ctx) => {
  const ids = await ctx.seed(seed);
  await ctx.startServers();
  const auth = (id) => ({ authorization: `Bearer ${learnerToken(id)}` });
  const both = (name, method, path, options = {}) => comparison.both(name, method, path, options);
  async function compareState(name) {
    comparison.count++;
    const read = (t) => ctx.inSchema(t, async (q) => normalize((await q('SELECT user_id, lesson_id, first_visited_at, last_visited_at, completed_at FROM course_lesson_progress ORDER BY user_id, lesson_id')).rows
      .map((row) => Object.fromEntries(Object.entries(row).map(([k, v]) => [k, v instanceof Date ? (Math.abs(Date.now() - v) < 864e5 ? '<now>' : v.toISOString()) : v])))));
    const a = await read('adonis');
    const g = await read('go');
    if (!isDeepStrictEqual(a, g)) comparison.failures.push({ name: `${name} (database)`, adonis: a, go: g });
  }

  // Course audience: tokens without an audience or expiry, cookies only.
  const sign = (payload) => {
    const encode = (v) => Buffer.from(JSON.stringify(v)).toString('base64url');
    const head = `${encode({ alg: 'HS256', typ: 'JWT' })}.${encode(payload)}`;
    return `${head}.${createHmac('sha256', appKey).update(head).digest('base64url')}`;
  };
  const now = Math.floor(Date.now() / 1000);
  for (const [label, headers] of [['anonymous', {}], ['no audience', { authorization: `Bearer ${sign({ userId: ids.senior, iat: now, exp: now + 60 })}` }],
    ['no expiry', { authorization: `Bearer ${sign({ userId: ids.senior, aud: 'kaderisasi-public', iat: now })}` }], ['cookie only', { cookie: `token=${learnerToken(ids.senior)}` }]])
    await both(`courses ${label}`, 'GET', '/v2/courses', { headers });

  // Listing
  for (const who of ['novice', 'middle', 'senior', 'ghost']) await both(`courses ${who}`, 'GET', '/v2/courses', { headers: auth(ids[who]) });
  for (const qs of ['?page=2&per_page=5', '?per_page=0', '?per_page=1000', '?page=1.5', '?page=abc', '?page=9999', '?search=%20dasar%20', '?search=_', '?search=%25', '?search=a&search=b', '?page=2&page=3', `?search=${'x'.repeat(250)}`, '?per_page=1e1'])
    await both(`courses senior ${qs.slice(0, 40)}`, 'GET', `/v2/courses${qs}`, { headers: auth(ids.senior) });

  // Course, lesson, document
  const c = ids.courses;
  for (const [label, id, who] of [['c0 senior', c.c0, 'senior'], ['c0 novice', c.c0, 'novice'], ['c1 novice', c.c1, 'novice'], ['c1 middle', c.c1, 'middle'], ['draft', c.c13, 'senior'], ['archived', c.c14, 'senior'],
    ['ghost', c.c0, 'ghost'], ['zero', '0', 'senior'], ['leading zero', `0${c.c0}`, 'senior'], ['huge', '2147483648', 'senior'], ['text', 'abc', 'senior'], ['missing', '99999', 'senior']])
    await both(`course ${label}`, 'GET', `/v2/courses/${id}`, { headers: auth(ids[who]) });
  for (const [label, id, lesson, who] of [['l1', c.c0, ids.l1, 'senior'], ['l3', c.c0, ids.l3, 'senior'], ['deleted', c.c0, ids.gone, 'senior'], ['other course', c.c0, ids.l5, 'senior'],
    ['l5 middle', c.c1, ids.l5, 'middle'], ['l6 middle', c.c2, ids.l6, 'middle'], ['bad lesson', c.c0, 'x', 'senior'], ['novice', c.c0, ids.l1, 'novice']])
    await both(`lesson ${label}`, 'GET', `/v2/courses/${id}/lessons/${lesson}`, { headers: auth(ids[who]) });
  for (const [label, lesson, doc, who] of [['d1', ids.l1, ids.d1, 'senior'], ['deleted doc', ids.l1, ids.d2, 'senior'], ['deleted lesson', ids.gone, ids.d3, 'senior'], ['missing object', ids.l2, ids.d4, 'senior'],
    ['wrong lesson', ids.l2, ids.d1, 'senior'], ['bad id', ids.l1, '-1', 'senior'], ['ghost', ids.l1, ids.d1, 'ghost']]) {
    const r = await both(`download ${label}`, 'GET', `/v2/courses/${c.c0}/lessons/${lesson}/documents/${doc}/download`, { headers: auth(ids[who]) });
    if (r.adonis.text !== r.go.text) comparison.failures.push({ name: `download ${label} (body)`, adonis: r.adonis.text.slice(0, 80), go: r.go.text.slice(0, 80) });
  }

  // Progress
  for (const [label, method, path, who, body] of [
    ['visit l2', 'POST', `/v2/courses/${c.c0}/lessons/${ids.l2}/visit`, 'senior'], ['visit new', 'POST', `/v2/courses/${c.c0}/lessons/${ids.l3}/visit`, 'middle'],
    ['visit deleted', 'POST', `/v2/courses/${c.c0}/lessons/${ids.gone}/visit`, 'senior'], ['visit level', 'POST', `/v2/courses/${c.c2}/lessons/${ids.l6}/visit`, 'middle'],
    ['complete', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l2}/completion`, 'senior', { completed: true }], ['complete again', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l2}/completion`, 'senior', { completed: true }],
    ['visit completed', 'POST', `/v2/courses/${c.c0}/lessons/${ids.l2}/visit`, 'senior'], ['uncomplete', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l1}/completion`, 'senior', { completed: false }],
    ['complete new', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l1}/completion`, 'middle', { completed: true }], ['complete string', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l1}/completion`, 'senior', { completed: 'true' }],
    ['complete missing', 'PUT', `/v2/courses/${c.c0}/lessons/${ids.l1}/completion`, 'senior', {}], ['complete bad ids', 'PUT', `/v2/courses/x/lessons/${ids.l1}/completion`, 'senior', { completed: true }],
    ['complete invalid and bad ids', 'PUT', '/v2/courses/x/lessons/y/completion', 'senior', {}],
  ]) {
    await both(`progress ${label}`, method, path, { headers: auth(ids[who]), json: body });
    await compareState(`progress ${label}`);
  }
  await both('courses senior after progress', 'GET', '/v2/courses', { headers: auth(ids.senior) });
  await both('course c0 after progress', 'GET', `/v2/courses/${c.c0}`, { headers: auth(ids.senior) });

  // Certificates
  for (const id of [ids.rActive, ids.rRevoked, ids.rNone, ids.rPending, ids.rApproval, ids.rSigned, ids.rOther, ids.rBroken, ids.guestReg, 99999, '0', 'abc', '9007199254740992', '9007199254740991'])
    for (const who of ['senior', 'middle']) {
      await both(`registration state ${id} ${who}`, 'GET', `/v2/certificates/registrations/${id}`, { headers: auth(ids[who]) });
      await both(`download by registration ${id} ${who}`, 'POST', `/v2/certificates/${id}/download`, { headers: auth(ids[who]) });
    }
  for (const code of ['SAL-ACTIVE', 'sal-active', '%20SAL-ACTIVE', 'SAL-OLD', 'SAL-NEW', 'SAL-APPROVAL', 'SAL-SIGNED', 'SAL-OTHER', 'SAL-BROKEN', 'SAL-NOSNAP', 'NOPE', 'bad*code'])
    for (const who of ['senior', 'middle']) {
      await both(`access ${code} ${who}`, 'GET', `/v2/certificates/code/${code}/access`, { headers: auth(ids[who]) });
      await both(`download ${code} ${who}`, 'GET', `/v2/certificates/code/${code}/download`, { headers: auth(ids[who]) });
    }
  await both('access anonymous', 'GET', '/v2/certificates/code/SAL-ACTIVE/access');
  return comparison.report('contract-courses');
});
s3.adonis.close();
s3.go.close();
process.exitCode = ok ? 0 : 1;

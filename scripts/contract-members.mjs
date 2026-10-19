// Differential contract check for the Phase 3 member routes: profiles,
// profile history, achievements, notifications, ruang curhat and
// members/submit. Adonis and Go each get an identically seeded schema and
// their own fake S3, so ids match and tables and stored objects can be
// compared after every mutation.
//
//   node scripts/contract-members.mjs
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { withContract, Comparison, learnerToken, fakeS3, snapshot, normalizeValue, root } from './contract-lib.mjs';

const ports = { adonis: 3350, go: 3351 };
const sharp = createRequire(resolve(root, '../kaderisasi-web-be/package.json'))('sharp');
const json = (v) => (v === undefined ? null : JSON.stringify(v));
const smtp = { SMTP_HOST: '127.0.0.1', SMTP_PORT: '2', SMTP_USERNAME: 'x', SMTP_PASSWORD: 'x', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset' };

async function seed(q) {
  await q(`INSERT INTO provinces (id, name) VALUES (31, 'DKI Jakarta'), (32, 'Jawa Barat')`);
  await q(`INSERT INTO cities (id, province_id, name) VALUES (3273, 32, 'Kota Bandung'), (3171, 31, 'Jakarta Pusat')`);
  await q(`INSERT INTO universities (id, name) VALUES (9001, 'Institut Teknologi Bandung')`);
  await q(`INSERT INTO admin_users (id, email, normalized_email, password, display_name, created_at, updated_at, role_code) VALUES
    (501, 'counselor@contract.test', 'counselor@contract.test', NULL, 'Konselor', '2025-01-01T00:00:00Z', NULL, 'counselor')`);
  const users = [];
  const user = async (email, status, profile) => {
    const { rows: [u] } = await q(`INSERT INTO public_users (email, account_status, member_id, created_at, updated_at) VALUES ($1, $2, NULL, '2025-02-01T10:00:00.123+07:00', NULL) RETURNING id`, [email, status]);
    if (profile) {
      await q(`INSERT INTO profiles (user_id, name, gender, whatsapp, province_id, city_id, university_id, intake_year, level, badges, birth_date, education_history, work_history, extra_data, picture, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, '2025-02-01T10:00:00.456+07:00', '2025-02-02T10:00:00.789+07:00')`,
        [u.id, profile.name ?? 'Anggota', profile.gender ?? 'M', profile.whatsapp ?? '0812', profile.province_id ?? 32, profile.city_id ?? 3273, profile.university_id ?? 9001, 2021, profile.level ?? 0,
          json(profile.badges ?? []), profile.birth_date ?? '2001-05-17', json(profile.education_history ?? []), json(profile.work_history ?? []), json(profile.extra_data ?? {}), profile.picture ?? null]);
    }
    users.push(u.id);
    return u.id;
  };
  const alice = await user('alice@contract.test', 'active', { name: 'Alice', extra_data: { preferred_name: 'Al', alumni_regional_assignment: ['Bandung'], '2': 'two' }, picture: 'profile/old.webp' });
  const bob = await user('bob@contract.test', 'active', { name: 'Bob', province_id: 31, city_id: 3171, university_id: 9001, badges: ['SSC-1'], extra_data: null });
  const noProfile = await user('ghost@contract.test', 'active', null);
  const inactive = await user('inactive@contract.test', 'no_account', { name: 'Inactive' });
  const quirky = await user('quirky@contract.test', 'active', { name: 'Quirky', extra_data: 'stringly', education_history: '"[{\\"major\\":\\" m \\"}]"', badges: ' SSC ' });

  // Activities, registrations (visibility rules) and certificates.
  const activity = async (name, config) => (await q(`INSERT INTO activities (name, slug, is_published, additional_config, created_at) VALUES ($1, $2, true, $3, '2025-01-01T00:00:00Z') RETURNING id`,
    [name, name.toLowerCase().replace(/\W+/g, '-'), json(config)])).rows[0].id;
  const visible = await activity('Visible Activity', { images: ['cover.png'], certificate_template_id: 1 });
  const hiddenNoDate = await activity('Hidden Activity', { status_visibility: { is_visible: false } });
  const hiddenFuture = await activity('Future Reveal', { status_visibility: { is_visible: false, visible_at: '2099-01-01T00:00:00Z' } });
  const hiddenPast = await activity('Past Reveal', { status_visibility: { is_visible: false, visible_at: '2020-01-01T00:00:00Z' } });
  const badDate = await activity('Bad Reveal', { status_visibility: { is_visible: false, visible_at: 'not a date' } });
  const registration = async (activityId, status, at) => (await q(`INSERT INTO activity_registrations (user_id, activity_id, status, questionnaire_answer, scoring_data, created_at) VALUES ($1, $2, $3, '{"q":1}', '{"secret":true}', $4) RETURNING id`,
    [alice, activityId, status, at])).rows[0].id;
  const r1 = await registration(visible, 'LULUS KEGIATAN', '2025-03-01T00:00:00Z');
  await registration(hiddenNoDate, 'DITERIMA', '2025-03-02T00:00:00Z');
  await registration(hiddenFuture, 'TIDAK DITERIMA', '2025-03-03T00:00:00Z');
  await registration(hiddenPast, 'TERDAFTAR', '2025-03-04T00:00:00Z');
  await registration(badDate, 'LULUS KEGIATAN', '2025-03-05T00:00:00Z');
  const { rows: [template] } = await q(`INSERT INTO certificate_templates (name, created_at) VALUES ('T', now()) RETURNING id`);
  await q(`INSERT INTO issued_certificates (certificate_code, registration_id, activity_id, user_id, template_id, template_snapshot, participant_snapshot, activity_snapshot, issued_at, revoked_at, created_at)
    VALUES ('SAL-OLD', $1, $2, $3, $4, '{"template_data":{"elements":[]}}', '{"name":"Alice"}', '{"name":"V"}', '2025-04-01T00:00:00Z', '2025-04-02T00:00:00Z', now()),
           ('SAL-NEW', $1, $2, $3, $4, '{"template_data":{"elements":[]}}', '{"name":"Alice"}', '{"name":"V"}', '2025-05-01T00:00:00Z', NULL, now())`, [r1, visible, alice, template.id]);

  // Achievements and counseling.
  for (const [i, [name, date, status, score]] of [['Juara 1', '2025-06-01', 1, 50], ['Finalis', '2025-07-15', 0, 0], ['Ditolak', null, 2, 0], ['Juara 2', '2024-12-31', 1, 30]].entries()) {
    await q(`INSERT INTO achievements (user_id, name, description, achievement_date, type, score, proof, status, remark, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '2025-08-01T00:00:00Z', NULL)`,
      [alice, name, `Deskripsi ${name}`, date, i % 3, score, `achievements/${alice}_old${i}.pdf`, status, status === 2 ? 'Bukti kurang' : null]);
  }
  await q(`INSERT INTO ruang_curhats (user_id, problem_ownership, owner_name, problem_category, problem_description, handling_technic, counselor_gender, counselor_id, status, created_at) VALUES
    ($1, 1, NULL, 'Akademik', 'Susah fokus', 'Konseling', 'F', 501, 1, '2025-02-03T00:00:00Z'), ($1, 2, 'Teman', 'Keluarga', 'Konflik', 'Diskusi', 'M', NULL, 0, '2025-02-04T00:00:00Z')`, [alice]);

  // Announcements: published (with and without links), draft, withdrawn.
  for (let i = 0; i < 24; i++) {
    const state = i === 5 ? 'draft' : i === 6 ? 'withdrawn' : 'published';
    const { rows: [a] } = await q(`INSERT INTO announcements (title, body, link_label, link_url, audience, state, published_at, created_at, updated_at)
      VALUES ($1, $2, $3, $4, '{"type":"all"}', $5, $6, now(), now()) RETURNING id`,
      [`Pengumuman ${i}`, `Isi <b>${i}</b> & lainnya`, i % 3 ? null : 'Buka', i % 3 ? null : 'https://e.x/p', state, state === 'draft' ? null : new Date(Date.UTC(2025, 0, 1, 0, 0, i, i * 7)).toISOString().replace('Z', `${String(i).padStart(3, '0')}Z`).replace(/\.(\d{3})(\d{3})Z/, '.$1$2Z')]);
    await q(`INSERT INTO announcement_recipients (announcement_id, public_user_id, read_at) VALUES ($1, $2, $3)`, [a.id, alice, i % 4 === 0 ? '2025-01-10T00:00:00.123456Z' : null]);
    if (i < 2) await q(`INSERT INTO announcement_recipients (announcement_id, public_user_id) VALUES ($1, $2)`, [a.id, inactive]);
  }
  return { alice, bob, noProfile, inactive, quirky };
}

const today = new Date().toISOString().slice(0, 10);
const nowish = new RegExp(`^(${today}|${new Date(Date.now() + 864e5).toISOString().slice(0, 10)}|${new Date(Date.now() - 864e5).toISOString().slice(0, 10)})T`);
function normalize(v, key) {
  if (typeof v === 'string') {
    if (nowish.test(v) && /T\d{2}:\d{2}:\d{2}/.test(v)) return '<now>';
    return normalizeValue(key, v);
  }
  if (Array.isArray(v)) return v.map((item) => normalize(item));
  if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, item]) => [k, k === 'cutoff' || k === 'next_cursor' && item ? '<cursor>' : normalize(item, k)]));
  return v;
}
const normalizeText = (t) => t
  .replace(/"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))"/g, (m, v) => (nowish.test(v) ? '"<now>"' : m))
  .replace(/_\d{13}\./g, '_<ms>.').replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g, '<uuid>')
  .replace(/"(next_cursor|cutoff)":"[^"]+"/g, '"$1":"<cursor>"');
const comparison = new Comparison(ports, { headers: ['cache-control'], normalize: (v) => normalize(v), normalizeText });
const tables = ['public_users', 'profiles', 'achievements', 'ruang_curhats', 'announcement_recipients'];

const s3 = { adonis: await fakeS3(), go: await fakeS3() };
const drive = { DRIVE_DISK: 'minio', DRIVE_REGION: 'us-east-1', DRIVE_BUCKET: 'contract', DRIVE_ACCESS_KEY_ID: 'x', DRIVE_SECRET_ACCESS_KEY: 'x' };
const ok = await withContract('contract-members', { ports, env: { ...smtp, ...drive }, twin: true,
  adonisEnv: { DRIVE_ENDPOINT: s3.adonis.endpoint }, goEnv: { DRIVE_ENDPOINT: s3.go.endpoint } }, async (ctx) => {
  const ids = await ctx.seed(seed);
  await ctx.startServers();
  const auth = (id) => ({ authorization: `Bearer ${learnerToken(id)}` });
  const both = (name, method, path, options = {}) => comparison.both(name, method, path, options);
  async function compareState(name) {
    comparison.count++;
    const a = await ctx.inSchema('adonis', (q) => snapshot(q, tables));
    const g = await ctx.inSchema('go', (q) => snapshot(q, tables));
    if (!isDeepStrictEqual(a, g)) {
      const diff = Object.keys(a).filter((t) => !isDeepStrictEqual(a[t], g[t]));
      comparison.failures.push({ name: `${name} (database: ${diff.join(', ')})`, adonis: diff.map((t) => a[t]), go: diff.map((t) => g[t]) });
    }
    const keys = (store) => [...store.objects.entries()].map(([k, o]) => `${normalizeValue('', k)} ${o.contentType} ${o.acl ?? ''} ${o.cacheControl ?? ''}`).sort();
    if (!isDeepStrictEqual(keys(s3.adonis), keys(s3.go))) comparison.failures.push({ name: `${name} (storage)`, adonis: keys(s3.adonis), go: keys(s3.go) });
  }

  // Profiles
  for (const [label, id] of Object.entries(ids)) await both(`profile ${label}`, 'GET', '/v2/profiles', { headers: auth(id) });
  await both('profile anonymous', 'GET', '/v2/profiles');
  for (const [label, id] of [['alice', ids.alice], ['no profile', ids.noProfile]]) await both(`profile activities ${label}`, 'GET', '/v2/profiles/activities', { headers: auth(id) });
  const updates = [
    {}, { name: 'Alice Baru' }, { name: 'Alice Baru' }, { gender: 'X' }, { province_id: '31', city_id: 3171.5 }, { province_id: 1.5 },
    { birth_date: '2000-02-29' }, { birth_date: 'kemarin' }, { whatsapp: '' }, { name: 5 },
    { extra_data: { preferred_name: 'Ali', kaderisasi_path: { ssc: 3, lmd: null } } }, { extra_data: { kaderisasi_path: { ssc: 0, spectra: 2.5 } } },
    { extra_data: { salman_activity_history: ['A', 2] } }, { extra_data: { current_activity_focus: ['academic', 'bogus'] } },
    { education_history: [{ degree: 'bachelor', institution: ' ITB ', intake_year: '2019' }, { degree: 'phd', intake_year: ' ' }] },
    { education_history: [{ intake_year: 1800 }] }, { work_history: [{ job_title: 'Dev', company: 'C', start_year: 2020, end_year: 2019 }] },
    { work_history: [{ job_title: ' ', company: 'C' }] }, { work_history: [{ job_title: 'Dev', company: 'C', start_year: '2020', end_year: 2024 }] },
    { country: 'Indonesia', personal_id: '3273', linkedin: 'in/a', tiktok: '@a', line: 'a', instagram: '@a', origin_province_id: 32, origin_city_id: 3273 },
  ];
  for (const [i, body] of updates.entries()) {
    await both(`update profile #${i} ${JSON.stringify(body).slice(0, 80)}`, 'PUT', '/v2/profiles', { headers: auth(ids.alice), json: body });
    await compareState(`update profile #${i}`);
  }
  await both('update quirky profile extra_data', 'PUT', '/v2/profiles', { headers: auth(ids.quirky), json: { extra_data: { preferred_name: 'Q' } } });
  await both('update quirky education', 'PUT', '/v2/profiles', { headers: auth(ids.quirky), json: { education_history: [] } });
  await both('update without profile', 'PUT', '/v2/profiles', { headers: auth(ids.noProfile), json: { name: 'x' } });
  await compareState('profile updates');

  // Picture
  const png = await sharp({ create: { width: 900, height: 600, channels: 3, background: '#3366cc' } }).png().toBuffer();
  const jpg = await sharp({ create: { width: 300, height: 200, channels: 3, background: '#cc6633' } }).jpeg().toBuffer();
  for (const [label, parts] of [
    ['png', [['file', { data: png, name: 'me.png', type: 'image/png' }]]], ['jpg named png', [['file', { data: jpg, name: 'me.png', type: 'image/png' }]]],
    ['text named png', [['file', { data: Buffer.from('nope'), name: 'me.png', type: 'image/png' }]]], ['pdf', [['file', { data: Buffer.from('%PDF-1.4\n'), name: 'me.pdf', type: 'application/pdf' }]]],
    ['missing', [['other', 'x']]], ['field not file', [['file', 'text']]], ['too large', [['file', { data: Buffer.concat([png, Buffer.alloc(3 << 20)]), name: 'big.png', type: 'image/png' }]]],
  ]) {
    await both(`picture ${label}`, 'POST', '/v2/profiles/picture', { headers: auth(ids.alice), multipart: parts });
    await compareState(`picture ${label}`);
  }
  await both('picture without profile', 'POST', '/v2/profiles/picture', { headers: auth(ids.noProfile), multipart: [['file', { data: png, name: 'me.png', type: 'image/png' }]] });

  // Profile history
  for (const qs of ['', '?page=2&per_page=2', '?search=%20REVEAL%20', '?search=lulus', '?search=reveal&search_scope=name', '?status=LULUS%20KEGIATAN', '?status=all', '?per_page=500', '?page=0', '?page=1.5'])
    for (const section of ['activities', 'consultations', 'achievements']) await both(`history ${section} ${qs}`, 'GET', `/v2/profiles/history/${section}${qs}`, { headers: auth(ids.alice) });
  await both('history unknown', 'GET', '/v2/profiles/history/unknown', { headers: auth(ids.alice) });

  // Achievements
  for (const qs of ['', '?page=2&per_page=2', '?per_page=-1', '?page=0']) await both(`my achievements ${qs}`, 'GET', `/v2/achievements${qs}`, { headers: auth(ids.alice) });
  for (const id of ['1', '2', '999', 'abc', '0', '2147483648']) await both(`achievement ${id}`, 'GET', `/v2/achievements/${id}`, { headers: auth(ids.alice) });
  await both('achievement of another member', 'GET', '/v2/achievements/1', { headers: auth(ids.bob) });
  const pdf = Buffer.from('%PDF-1.7\n1 0 obj << >> endobj\n%%EOF\n');
  for (const [label, parts] of [
    ['valid', [['name', 'Lomba'], ['description', 'Juara'], ['achievement_date', '2025-09-01'], ['type', '1'], ['proof', { data: pdf, name: 'bukti.pdf', type: 'application/pdf' }]]],
    ['bad type', [['name', 'Lomba'], ['description', 'x'], ['achievement_date', '2025-09-01'], ['type', '1.5'], ['proof', { data: pdf, name: 'bukti.pdf', type: 'application/pdf' }]]],
    ['bad extension', [['name', 'Lomba'], ['description', 'x'], ['achievement_date', '2025-09-01'], ['type', '1'], ['proof', { data: Buffer.from('hello'), name: 'bukti.txt', type: 'text/plain' }]]],
    ['missing proof', [['name', 'Lomba'], ['description', 'x'], ['achievement_date', '2025-09-01'], ['type', '1']]],
    ['empty fields', [['name', ''], ['proof', { data: pdf, name: 'bukti.pdf', type: 'application/pdf' }]]],
  ]) {
    await both(`store achievement ${label}`, 'POST', '/v2/achievements', { headers: auth(ids.alice), multipart: parts });
    await compareState(`store achievement ${label}`);
  }
  for (const [label, id, parts] of [
    ['rename', '2', [['name', 'Finalis Nasional']]], ['same values', '2', [['name', 'Finalis Nasional']]], ['type zero', '2', [['type', '0']]],
    ['new proof', '2', [['proof', { data: pdf, name: 'baru.pdf', type: 'application/pdf' }]]], ['date', '1', [['achievement_date', '2025-01-02']]],
    ['missing', '999', [['name', 'x']]], ['invalid id', 'abc', [['name', 'x']]], ['bad proof', '1', [['proof', { data: Buffer.from('x'), name: 'x.exe', type: 'application/x' }]]],
  ]) {
    await both(`update achievement ${label}`, 'PUT', `/v2/achievements/${id}`, { headers: auth(ids.alice), multipart: parts });
    await compareState(`update achievement ${label}`);
  }

  // Ruang curhat
  await both('curhat history', 'GET', '/v2/ruang-curhat', { headers: auth(ids.alice) });
  for (const [label, body] of [['valid', { problem_ownership: '1', problem_category: 'Akademik', problem_description: 'Stres', handling_technic: 'Chat', counselor_gender: 'F' }],
    ['with owner', { problem_ownership: 2, owner_name: 'Adik', problem_category: 'K', problem_description: 'D', handling_technic: 'T', counselor_gender: 'M' }],
    ['invalid', { problem_ownership: 'x' }], ['fractional', { problem_ownership: 1.5, problem_category: 'K', problem_description: 'D', handling_technic: 'T', counselor_gender: 'M' }]]) {
    await both(`store curhat ${label}`, 'POST', '/v2/ruang-curhat', { headers: auth(ids.alice), json: body });
    await compareState(`store curhat ${label}`);
  }
  await both('curhat history after', 'GET', '/v2/ruang-curhat', { headers: auth(ids.alice) });

  // Notifications
  const list = await both('notifications', 'GET', '/v2/notifications', { headers: auth(ids.alice) });
  const cursor = list.go.json?.data?.next_cursor;
  await both('notifications next page', 'GET', `/v2/notifications?cursor=${cursor}`, { headers: auth(ids.alice) });
  for (const qs of ['?unread=true', '?unread=yes', '?cursor=bad', '?cursor=e30', `?cursor=${Buffer.from('{"id":1,"time":"2025-01-01T00:00:10.000Z"}').toString('base64url')}`, `?cursor=${'x'.repeat(501)}`])
    await both(`notifications ${qs.slice(0, 40)}`, 'GET', `/v2/notifications${qs}`, { headers: auth(ids.alice) });
  await both('notifications inactive account', 'GET', '/v2/notifications', { headers: auth(ids.inactive) });
  await both('unread count', 'GET', '/v2/notifications/unread-count', { headers: auth(ids.alice) });
  for (const id of ['2', '6', '7', '99', 'abc', '%202']) await both(`notification ${id}`, 'GET', `/v2/notifications/${id}`, { headers: auth(ids.alice) });
  for (const id of ['2', '3', '6', 'x']) {
    await both(`read notification ${id}`, 'PUT', `/v2/notifications/${id}/read`, { headers: auth(ids.alice) });
  }
  await compareState('notifications read');
  for (const [label, body] of [['missing', {}], ['future', { cutoff: '2999-01-01T00:00:00Z' }], ['bad', { cutoff: 'yesterday' }], ['valid', { cutoff: '2025-01-01T00:00:15.000Z' }]]) {
    await both(`read all ${label}`, 'PUT', '/v2/notifications/read-all', { headers: auth(ids.alice), json: body });
  }
  await compareState('notifications read all');
  await both('unread count after', 'GET', '/v2/notifications/unread-count', { headers: auth(ids.alice) });

  // members/submit
  for (const [label, body] of [['minimal', { name: 'Anggota Baru' }], ['full', { name: 'Lengkap', gender: 'F', province_id: 32, city_id: 3273, birth_date: '1999-12-31', extra_data: { kaderisasi_path: { ssc: 2 } }, education_history: [{ major: ' TI ' }], work_history: [{ job_title: 'A', company: 'B' }] }],
    ['invalid', { gender: 'X' }], ['bad date', { name: 'Tanggal', birth_date: 'besok' }]]) {
    await both(`submit member ${label}`, 'POST', '/v2/members/submit', { json: body });
    await compareState(`submit member ${label}`);
  }
  return comparison.report('contract-members');

});
s3.adonis.close();
s3.go.close();
process.exitCode = ok ? 0 : 1;

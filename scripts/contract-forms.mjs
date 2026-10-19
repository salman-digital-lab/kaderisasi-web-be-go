// Differential contract check for phases 4 and 5: activity and club
// registrations, custom forms, form sessions, uploads and standalone
// responses. Adonis and Go each get an identically seeded schema and their
// own fake S3. Form session tokens are random, so every request that carries
// one is sent with the token its own server issued.
//
//   node scripts/contract-forms.mjs
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { withContract, Comparison, learnerToken, fakeS3, normalizeValue, root } from './contract-lib.mjs';

const ports = { adonis: 3352, go: 3353 };
const sharp = createRequire(resolve(root, '../kaderisasi-web-be/package.json'))('sharp');
const { PDFDocument } = createRequire(resolve(root, 'tests/interop/package.json'))('pdf-lib');
const json = (v) => JSON.stringify(v);
const smtp = { SMTP_HOST: '127.0.0.1', SMTP_PORT: '2', SMTP_USERNAME: 'x', SMTP_PASSWORD: 'x', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset' };

const field = (key, label, type, required, extra = {}) => ({ key, label, type, required, ...extra });
const schemas = {
  activity: { fields: [{ id: 's1', section_name: 'Data', fields: [field('nama', 'Nama', 'text', true), field('pilihan', 'Pilihan', 'select', false, { options: [{ label: 'A', value: 'a' }, { label: 'B', value: 'b' }] }),
    field('berkas', 'Berkas', 'file', false, { file: { accept: 'pdf_or_image', maxFiles: 2, maxSizeMB: 1 } })] }] },
  simple: { fields: [{ section_name: 'Data', fields: [field('nama', 'Nama', 'text', true)] }] },
  club: { fields: [{ section_name: 'Data', fields: [field('alasan', 'Alasan', 'textarea', true), field('cv', 'CV', 'file', false, { file: { accept: 'pdf', maxFiles: 1, maxSizeMB: 2 } })] }] },
  publicForm: { settings: { accessMode: 'public' }, fields: [{ section_name: 'profile_data', fields: [field('pesan', 'Pesan', 'text', true), field('foto', 'Foto', 'file', false, { file: { accept: 'image', maxFiles: 1, maxSizeMB: 1 } })] }] },
  membersForm: { settings: { accessMode: 'members' }, fields: [{ section_name: 'Data', fields: [field('pesan', 'Pesan', 'text', true)] }] },
};
const scoring = (registration, activity, overrides = {}) => ({
  schema_version: 1,
  published: {
    schema_version: 1, activity_id: activity, registration_id: registration, revision: 3, published_at: '2025-05-05T05:05:05.000Z', draft: { note: 'Bagus', extra: 1 },
    rubric: { note: 'Rubrik', grades: [{ label: 'A', minimum: 80, x: 1 }], groups: [{ id: 'g', name: 'G', secret: 1, criteria: [{ id: 'c', name: 'C', maximum: 10, weight: 1, hidden: true }] }] },
    result: { total: 88, grade: 'A', complete: true, criteria: [{ criterion_id: 'c', score: 9, normalized: 90, grade: 'A', comment: 'x' }] }, ...overrides,
  },
});

async function seed(q) {
  const user = async (email) => (await q(`INSERT INTO public_users (email, account_status, created_at) VALUES ($1, 'active', '2025-02-01T10:00:00Z') RETURNING id`, [email])).rows[0].id;
  const alice = await user('alice@contract.test');
  const bob = await user('bob@contract.test');
  const carol = await user('carol@contract.test');
  await q(`INSERT INTO profiles (user_id, name, created_at) VALUES ($1, 'Alice', now()), ($2, 'Bob', now())`, [alice, bob]);

  const activity = async (slug, { type = 1, published = true, open = true, start = 'current_date - 2', end = 'current_date + 5', config = {} } = {}) =>
    (await q(`INSERT INTO activities (name, slug, activity_type, is_published, is_registration_open, registration_start, registration_end, additional_config, created_at, updated_at)
      VALUES ($1, $1, $2, $3, $4, ${start}, ${end}, $5, '2025-01-01T00:00:00Z', '2025-01-02T00:00:00Z') RETURNING id`, [slug, type, published, open, json(config)])).rows[0].id;
  const open = await activity('open-act', { config: { allow_guest_registration: true } });
  const closed = await activity('closed-act', { end: 'current_date - 1', config: { allow_guest_registration: true } });
  const noGuest = await activity('no-guest', { type: 2 });
  const noForm = await activity('no-form', { config: { allow_guest_registration: true } });
  await activity('unpublished', { published: false, config: { allow_guest_registration: true } });
  const scored = await activity('scored');
  const hidden = await activity('hidden-status', { config: { status_visibility: { is_visible: false, visible_at: '2099-01-01T00:00:00Z' } } });
  const fresh = await activity('fresh-act');

  const form = async (name, type, featureId, schema, { active = true, info = null, updated = '2025-03-01T00:00:00Z' } = {}) =>
    (await q(`INSERT INTO custom_forms (form_name, form_description, feature_type, feature_id, form_schema, is_active, post_submission_info, created_at, updated_at)
      VALUES ($1, 'Deskripsi', $2, $3, $4, $5, $6, '2025-01-01T00:00:00Z', $7) RETURNING id`, [name, type, featureId, json(schema), active, info, updated])).rows[0].id;
  await form('Old open form', 'activity_registration', open, schemas.simple, { updated: '2024-01-01T00:00:00Z' });
  const openForm = await form('Open form', 'activity_registration', open, schemas.activity, { info: '<p onclick="x">Terima <b>kasih</b><script>x</script></p>' });
  await form('Closed form', 'activity_registration', closed, schemas.simple);
  await form('No guest form', 'activity_registration', noGuest, schemas.simple);
  await form('Scored form', 'activity_registration', scored, schemas.simple);
  const freshForm = await form('Fresh form', 'activity_registration', fresh, schemas.activity);

  const club = async (name, { show = true, open = true, end = null } = {}) =>
    (await q(`INSERT INTO clubs (name, is_show, is_registration_open, registration_end_date, start_period, created_at, updated_at) VALUES ($1, $2, $3, ${end ?? 'NULL'}, '2025-01-01', '2025-01-01T00:00:00Z', NULL) RETURNING id`, [name, show, open])).rows[0].id;
  const openClub = await club('Open club');
  const closedClub = await club('Closed club', { open: false });
  const hiddenClub = await club('Hidden club', { show: false });
  const expiredClub = await club('Expired club', { end: 'current_date - 1' });
  const lastDayClub = await club('Last day club', { end: 'current_date' });
  const clubForm = await form('Club form', 'club_registration', openClub, schemas.club);
  await form('Last day form', 'club_registration', lastDayClub, schemas.simple);
  const publicForm = await form('Public form', 'independent_form', null, schemas.publicForm, { info: '' });
  const membersForm = await form('Members form', 'independent_form', null, schemas.membersForm);
  const inactiveForm = await form('Inactive form', 'independent_form', null, schemas.publicForm, { active: false });
  const nullForm = (await q(`INSERT INTO custom_forms (form_name, feature_type, form_schema, created_at) VALUES ('Null schema', 'independent_form', NULL, now()) RETURNING id`)).rows[0].id;

  const registration = async (userId, activityId, status, answers, scoringData = null, at = '2025-03-01T00:00:00Z') =>
    (await q(`INSERT INTO activity_registrations (user_id, activity_id, status, questionnaire_answer, scoring_data, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id`,
      [userId, activityId, status, json(answers), scoringData && json(scoringData), at])).rows[0].id;
  const r1 = await registration(alice, open, 'TERDAFTAR', { nama: 'Alice', pilihan: 'a' });
  const r2 = await registration(alice, scored, 'LULUS KEGIATAN', { nama: 'Alice' });
  await q(`UPDATE activity_registrations SET scoring_data = $1 WHERE id = $2`, [json(scoring(r2, scored)), r2]);
  const r3 = await registration(bob, scored, 'TERDAFTAR', {}, scoring(999, scored));
  await registration(carol, scored, 'TERDAFTAR', {}, scoring(0, scored, { result: null }));
  await q(`UPDATE activity_registrations SET scoring_data = $1 WHERE user_id = $2`, [json(scoring(0, scored, { result: null, registration_id: 0 })), carol]);
  await q(`UPDATE activity_registrations SET scoring_data = jsonb_set(scoring_data, '{published,registration_id}', to_jsonb(id)) WHERE user_id = $1`, [carol]);
  await registration(alice, hidden, 'DITERIMA', {});
  const { rows: [template] } = await q(`INSERT INTO certificate_templates (name, created_at) VALUES ('T', now()) RETURNING id`);
  await q(`INSERT INTO issued_certificates (certificate_code, registration_id, activity_id, user_id, template_id, template_snapshot, participant_snapshot, activity_snapshot, issued_at, created_at)
    VALUES ('SAL-SCORED', $1, $2, $3, $4, '{"template_data":{"elements":[]}}', '{"name":"Alice"}', '{"name":"S"}', '2025-04-01T00:00:00Z', now())`, [r2, scored, alice, template.id]);

  const clubRegistration = async (userId, clubId, status, at) =>
    (await q(`INSERT INTO club_registrations (club_id, member_id, status, additional_data, created_at, updated_at) VALUES ($1, $2, $3, '{"alasan":"x"}', $4, $4) RETURNING id`, [clubId, userId, status, at])).rows[0].id;
  const c1 = await clubRegistration(alice, closedClub, 'APPROVED', '2025-01-05T00:00:00Z');
  await clubRegistration(alice, hiddenClub, 'PENDING', '2025-01-06T00:00:00Z');
  await clubRegistration(alice, expiredClub, 'REJECTED', '2025-01-07T00:00:00Z');
  await clubRegistration(bob, closedClub, 'PENDING', '2025-01-08T00:00:00Z');
  await q(`INSERT INTO club_member_roles (club_registration_id, role_name, start_date, end_date, is_primary, sort_order, created_at, updated_at) VALUES
    ($1, 'Anggota', NULL, NULL, false, 2, now(), now()), ($1, 'Ketua', '2025-01-01', '2025-12-31', true, 1, now(), now()), ($1, 'Sekretaris', NULL, NULL, false, 1, now(), now())`, [c1]);
  return { alice, bob, carol, open, closed, noGuest, noForm, scored, hidden, fresh, openForm, freshForm, openClub, closedClub, hiddenClub, expiredClub, lastDayClub, clubForm, publicForm, membersForm, inactiveForm, nullForm, r1, r3 };
}

function normalize(v, key) {
  if (typeof v === 'string') return key === 'token' && /^[a-f0-9]{64}$/.test(v) ? '<token>' : /^\d{4}-\d{2}-\d{2}T/.test(v) && Date.now() - Date.parse(v) < 864e5 * 2 && Date.parse(v) <= Date.now() + 6e4 ? '<now>' : normalizeValue(key, v);
  if (Array.isArray(v)) return v.map((item) => normalize(item));
  if (v && typeof v === 'object') {
    const webp = v.mime_type === 'image/webp';
    return Object.fromEntries(Object.entries(v).map(([k, item]) => [k, webp && (k === 'size' || k === 'size_bytes') ? '<webp size>' : normalize(item, k)]));
  }
  return v;
}
const normalizeText = (t) => t.replace(/"(\d{4}-\d{2}-\d{2}T[^"]+)"/g, (m, v) => (Date.now() - Date.parse(v) < 864e5 * 2 && Date.parse(v) <= Date.now() + 6e4 ? '"<now>"' : m))
  .replace(/"token":"[a-f0-9]{64}"/g, '"token":"<token>"').replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g, '<uuid>')
  .replace(/("mime_type":"image\/webp","size":)\d+/g, '$1"<webp size>"');
const comparison = new Comparison(ports, { headers: ['cache-control'], normalize: (v) => normalize(v), normalizeText });

const s3 = { adonis: await fakeS3(), go: await fakeS3() };
const drive = { DRIVE_DISK: 'minio', DRIVE_REGION: 'us-east-1', DRIVE_BUCKET: 'contract', DRIVE_ACCESS_KEY_ID: 'x', DRIVE_SECRET_ACCESS_KEY: 'x' };
const ok = await withContract('contract-forms', { ports, env: { ...smtp, ...drive }, twin: true,
  adonisEnv: { DRIVE_ENDPOINT: s3.adonis.endpoint }, goEnv: { DRIVE_ENDPOINT: s3.go.endpoint } }, async (ctx) => {
  const ids = await ctx.seed(seed);
  await ctx.startServers();
  const auth = (id) => ({ authorization: `Bearer ${learnerToken(id)}` });
  const both = (name, method, path, options = {}) => comparison.both(name, method, path, options);
  // per(name, method, path, build): build(target) returns that target's options.
  const per = async (name, method, path, build) => {
    const a = await comparison.call('adonis', method, path, build('adonis'));
    const g = await comparison.call('go', method, path, build('go'));
    comparison.compare(name, a, g);
    return { adonis: a, go: g };
  };
  const tables = {
    activity_registrations: 'SELECT * FROM activity_registrations ORDER BY id',
    club_registrations: 'SELECT * FROM club_registrations ORDER BY id',
    custom_form_sessions: 'SELECT id, form_id, user_id, schema_hash, expires_at, completed_at FROM custom_form_sessions ORDER BY created_at, expires_at',
    custom_form_attachments: 'SELECT * FROM custom_form_attachments ORDER BY created_at, id',
    custom_form_responses: 'SELECT * FROM custom_form_responses ORDER BY created_at',
  };
  async function compareState(name) {
    comparison.count++;
    const read = (target) => ctx.inSchema(target, async (q) => {
      const out = {};
      for (const [table, sql] of Object.entries(tables)) out[table] = normalize((await q(sql)).rows.map((row) => Object.fromEntries(Object.entries(row).map(([k, v]) => [k, v instanceof Date ? '<time>' : v]))));
      return out;
    });
    const a = await read('adonis');
    const g = await read('go');
    if (!isDeepStrictEqual(a, g)) {
      const diff = Object.keys(a).filter((t) => !isDeepStrictEqual(a[t], g[t]));
      comparison.failures.push({ name: `${name} (database: ${diff.join(', ')})`, adonis: diff.map((t) => a[t]), go: diff.map((t) => g[t]) });
    }
    const keys = (store) => [...store.objects.entries()].map(([k, o]) => `${normalizeValue('', k)} ${o.contentType} ${o.acl ?? ''} ${o.cacheControl ?? ''}`).sort();
    if (!isDeepStrictEqual(keys(s3.adonis), keys(s3.go))) comparison.failures.push({ name: `${name} (storage)`, adonis: keys(s3.adonis), go: keys(s3.go) });
  }
  const sessionHash = async (formId) => (await comparison.call('adonis', 'GET', `/v2/custom-forms/by-feature?feature_type=independent_form&feature_id=${formId}`)).json?.data?.schema_hash;
  // Feature forms are not independent; read their hash through the matching feature query.
  const featureHash = async (type, featureId) => (await comparison.call('adonis', 'GET', `/v2/custom-forms/by-feature?feature_type=${type}&feature_id=${featureId}`)).json?.data?.schema_hash;
  const hash = {
    open: await featureHash('activity_registration', ids.open), fresh: await featureHash('activity_registration', ids.fresh), club: await featureHash('club_registration', ids.openClub),
    public: await sessionHash(ids.publicForm), members: await sessionHash(ids.membersForm),
  };
  // session(name, formId, schemaHash, headers) creates a session on each server.
  const session = async (name, formId, schema_hash, headers = {}) => {
    const r = await both(`session ${name}`, 'POST', `/v2/custom-forms/${formId}/sessions`, { headers, json: { schema_hash } });
    return { adonis: r.adonis.json?.data?.token, go: r.go.json?.data?.token };
  };

  const pdfDoc = await PDFDocument.create();
  pdfDoc.addPage([200, 200]);
  const pdf = Buffer.from(await pdfDoc.save());
  const emptyPdf = Buffer.from('%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [] /Count 0 >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n');
  const png = await sharp({ create: { width: 3000, height: 1500, channels: 3, background: '#3366cc' } }).png().toBuffer();
  const smallJpg = await sharp({ create: { width: 300, height: 200, channels: 3, background: '#cc6633' } }).jpeg().toBuffer();
  const uploadID = (n) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;

  // --- Phase 4: activity registration reads --------------------------------
  for (const slug of ['open-act', 'scored', 'hidden-status', 'no-guest', 'missing', '%20open-act'])
    for (const who of ['alice', 'bob', 'carol']) await both(`registration check ${slug} ${who}`, 'GET', `/v2/profiles/activities/${slug}`, { headers: auth(ids[who]) });
  for (const slug of ['open-act', 'scored', 'hidden-status', 'no-guest', 'missing'])
    for (const who of ['alice', 'bob', 'carol']) await both(`registration data ${slug} ${who}`, 'GET', `/v2/activities/${slug}/registration`, { headers: auth(ids[who]) });
  await both('registration data anonymous', 'GET', '/v2/activities/open-act/registration');

  // --- Phase 4: clubs --------------------------------------------------------
  for (const id of [ids.openClub, ids.closedClub, ids.hiddenClub, ids.expiredClub, ids.lastDayClub, 999, 'abc', '1.5']) {
    await both(`club register ${id}`, 'POST', `/v2/clubs/${id}/register`, { headers: auth(ids.alice) });
    await both(`club status ${id}`, 'GET', `/v2/clubs/${id}/registration-status`, { headers: auth(ids.alice) });
    await both(`club update ${id}`, 'PUT', `/v2/clubs/${id}/registration`, { headers: auth(ids.alice), json: { additional_data: { a: 1 } } });
  }
  for (const qs of ['', '?page=2&limit=1', '?limit=2', '?status=PENDING', '?status=', '?page=0', '?page=-1&limit=2', '?limit=abc', '?page=1.5&limit=1'])
    await both(`my club registrations ${qs}`, 'GET', `/v2/club-registrations/my-registrations${qs}`, { headers: auth(ids.alice) });
  await both('my club registrations none', 'GET', '/v2/club-registrations/my-registrations', { headers: auth(ids.carol) });
  for (const [who, id] of [['alice', ids.closedClub], ['alice', ids.hiddenClub], ['bob', ids.closedClub], ['carol', ids.openClub], ['alice', 'abc']]) {
    await both(`cancel club ${who} ${id}`, 'DELETE', `/v2/clubs/${id}/registration`, { headers: auth(ids[who]) });
    await compareState(`cancel club ${who} ${id}`);
  }

  // --- Phase 5: forms by feature ---------------------------------------------
  for (const qs of ['', '?feature_type=activity_registration', `?feature_type=activity_registration&feature_id=${ids.open}`, `?feature_type=activity_registration&feature_id=${ids.open}&view=success`,
    '?feature_type=activity_registration&feature_id=999', '?feature_type=activity_registration&feature_id=abc', `?feature_type=activity_registration&feature_id=${ids.noForm}`,
    `?feature_type=club_registration&feature_id=${ids.openClub}`, '?feature_type=club_registration', '?feature_type=club_registration&feature_id=x',
    `?feature_type=independent_form&feature_id=${ids.publicForm}`, `?feature_type=independent_form&feature_id=${ids.publicForm}&view=success`, '?feature_type=independent_form',
    `?feature_type=independent_form&feature_id=${ids.nullForm}`, `?feature_type=independent_form&feature_id=${ids.openForm}`, '?feature_type=bogus&feature_id=1', '?feature_type=independent_form&feature_id=1e400'])
    await both(`by feature ${qs}`, 'GET', `/v2/custom-forms/by-feature${qs}`);

  // --- Phase 5: sessions -----------------------------------------------------
  await both('session invalid body', 'POST', `/v2/custom-forms/${ids.publicForm}/sessions`, { json: { schema_hash: 'x' } });
  await both('session wrong hash', 'POST', `/v2/custom-forms/${ids.publicForm}/sessions`, { json: { schema_hash: 'a'.repeat(64) } });
  await both('session missing form', 'POST', '/v2/custom-forms/999/sessions', { json: { schema_hash: 'a'.repeat(64) } });
  await both('session bad id', 'POST', '/v2/custom-forms/abc/sessions', { json: { schema_hash: 'a'.repeat(64) } });
  await both('session bad auth', 'POST', `/v2/custom-forms/${ids.publicForm}/sessions`, { headers: { authorization: 'Bearer nope' }, json: { schema_hash: hash.public } });
  await both('session members as guest', 'POST', `/v2/custom-forms/${ids.membersForm}/sessions`, { json: { schema_hash: hash.members } });
  await both('session inactive', 'POST', `/v2/custom-forms/${ids.inactiveForm}/sessions`, { json: { schema_hash: hash.public } });
  await both('session club as guest', 'POST', `/v2/custom-forms/${ids.clubForm}/sessions`, { json: { schema_hash: hash.club } });
  await compareState('sessions rejected');

  // --- Phase 5: uploads (guest session on the open activity form) -----------
  const guest = await session('guest open', ids.openForm, hash.open);
  const upload = (name, path, target, sessionToken, upload, parts, headers = {}) =>
    per(name, 'POST', path, (t) => ({ headers: { ...headers, ...(sessionToken ? { 'x-form-session': sessionToken[t] } : {}), ...(upload ? { 'x-form-upload': upload } : {}) }, multipart: parts }));
  const filePart = (data, name, type) => [['file', { data, name, type }]];
  const berkas = `/v2/custom-forms/${ids.openForm}/files/berkas`;
  for (const [label, path, up, parts, headers] of [
    ['pdf', berkas, uploadID(1), filePart(pdf, 'surat.pdf', 'application/pdf')],
    ['pdf again (idempotent)', berkas, uploadID(1), filePart(pdf, 'surat.pdf', 'application/pdf')],
    ['png resized', berkas, uploadID(2), filePart(png, '../../foto\u0001.png', 'image/png')],
    ['too many', berkas, uploadID(3), filePart(smallJpg, 'x.jpg', 'image/jpeg')],
    ['bad upload id', berkas, 'nope', filePart(pdf, 'a.pdf', 'application/pdf')],
    ['txt', berkas, uploadID(4), filePart(Buffer.from('hello'), 'a.txt', 'text/plain')],
    ['no file', berkas, uploadID(4), [['other', 'x']]],
    ['unknown field', `/v2/custom-forms/${ids.openForm}/files/nama`, uploadID(4), filePart(pdf, 'a.pdf', 'application/pdf')],
    ['missing form', '/v2/custom-forms/999/files/berkas', uploadID(4), filePart(pdf, 'a.pdf', 'application/pdf')],
    ['reused id other field', `/v2/custom-forms/${ids.openForm}/files/nama`, uploadID(1), filePart(pdf, 'a.pdf', 'application/pdf')],
    ['bad auth', berkas, uploadID(4), filePart(pdf, 'a.pdf', 'application/pdf'), { authorization: 'Bearer x' }],
  ]) {
    await upload(`upload ${label}`, path, null, guest, up, parts, headers);
    await compareState(`upload ${label}`);
  }
  await upload('upload without session', berkas, null, null, uploadID(5), filePart(pdf, 'a.pdf', 'application/pdf'));
  // Remove the image, then re-upload a small one and a broken pdf.
  await per('remove upload', 'DELETE', `/v2/custom-forms/${ids.openForm}/files/${uploadID(2)}`, (t) => ({ headers: { 'x-form-session': guest[t] } }));
  await compareState('remove upload');
  await per('remove upload again', 'DELETE', `/v2/custom-forms/${ids.openForm}/files/${uploadID(2)}`, (t) => ({ headers: { 'x-form-session': guest[t] } }));
  await per('remove bad id', 'DELETE', `/v2/custom-forms/${ids.openForm}/files/nope`, (t) => ({ headers: { 'x-form-session': guest[t] } }));
  await per('remove bad form', 'DELETE', `/v2/custom-forms/abc/files/${uploadID(2)}`, (t) => ({ headers: { 'x-form-session': guest[t] } }));
  await both('remove without session', 'DELETE', `/v2/custom-forms/${ids.openForm}/files/${uploadID(1)}`);
  for (const [label, up, parts] of [['empty pdf', uploadID(6), filePart(emptyPdf, 'kosong.pdf', 'application/pdf')], ['jpg', uploadID(7), filePart(smallJpg, 'foto.jpeg', 'image/jpeg')]]) {
    await upload(`upload ${label}`, berkas, null, guest, up, parts);
    await compareState(`upload ${label}`);
  }

  // --- Phase 4: guest registration -------------------------------------------
  const guestData = { name: ' Tamu ', email: 'tamu@contract.test', phone: '0812' };
  for (const [label, slug, body] of [
    ['missing guest data', 'open-act', { questionnaire_answer: {} }], ['bad email', 'open-act', { guest_data: { name: 'x', email: 'no' }, questionnaire_answer: {} }],
    ['missing activity', 'missing', { guest_data: guestData, questionnaire_answer: {} }], ['unpublished', 'unpublished', { guest_data: guestData, questionnaire_answer: {} }],
    ['no guest', 'no-guest', { guest_data: guestData, questionnaire_answer: { nama: 'x' } }], ['closed', 'closed-act', { guest_data: guestData, questionnaire_answer: { nama: 'x' } }],
    ['no form', 'no-form', { guest_data: guestData, questionnaire_answer: {} }], ['invalid answers', 'open-act', { guest_data: guestData, questionnaire_answer: { pilihan: 'z' } }],
    ['unknown answer key', 'open-act', { guest_data: guestData, questionnaire_answer: { nama: 'x', other: 1 } }],
    ['no session', 'open-act', { guest_data: guestData, questionnaire_answer: { nama: 'x' } }],
    ['bad token', 'open-act', { guest_data: guestData, session_token: 'f'.repeat(64), questionnaire_answer: { nama: 'x' } }],
  ]) {
    await both(`guest register ${label}`, 'POST', `/v2/activities/${slug}/guest-register`, { json: body });
    await compareState(`guest register ${label}`);
  }
  await per('guest register foreign attachment', 'POST', '/v2/activities/open-act/guest-register', (t) => ({ json: { guest_data: guestData, session_token: guest[t], questionnaire_answer: { nama: 'x', berkas: [uploadID(9)] } } }));
  await per('guest register valid', 'POST', '/v2/activities/open-act/guest-register', (t) => ({ json: { guest_data: guestData, session_token: guest[t], questionnaire_answer: { pilihan: 'b', nama: 'Tamu', berkas: [uploadID(1), uploadID(7)] } } }));
  await compareState('guest register valid');
  await per('guest register reused session', 'POST', '/v2/activities/open-act/guest-register', (t) => ({ json: { guest_data: guestData, session_token: guest[t], questionnaire_answer: { nama: 'Tamu' } } }));
  await per('upload after completion', 'POST', berkas, (t) => ({ headers: { 'x-form-session': guest[t], 'x-form-upload': uploadID(8) }, multipart: filePart(pdf, 'a.pdf', 'application/pdf') }));

  // --- Phase 4: questionnaire edit -------------------------------------------
  for (const [label, who, slug, body] of [
    ['unchanged', 'alice', 'open-act', { questionnaire_answer: { nama: 'Alice', pilihan: 'a' } }], ['reordered', 'alice', 'open-act', { questionnaire_answer: { pilihan: 'a', nama: 'Alice' } }],
    ['changed', 'alice', 'open-act', { questionnaire_answer: { nama: 'Alice Baru' } }], ['invalid', 'alice', 'open-act', { questionnaire_answer: { nama: '' } }],
    ['missing answers', 'alice', 'open-act', {}], ['bad token format', 'alice', 'open-act', { session_token: 'x', questionnaire_answer: { nama: 'A' } }],
    ['new file without session', 'alice', 'open-act', { questionnaire_answer: { nama: 'A', berkas: [uploadID(1)] } }],
    ['not registered', 'carol', 'open-act', { questionnaire_answer: { nama: 'C' } }], ['missing activity', 'alice', 'missing', { questionnaire_answer: {} }],
    ['no form', 'alice', 'hidden-status', { questionnaire_answer: { free: ['x'] } }],
  ]) {
    await both(`edit registration ${label}`, 'PUT', `/v2/activities/${slug}/registration`, { headers: auth(ids[who]), json: body });
    await compareState(`edit registration ${label}`);
  }
  const aliceOpen = await session('alice open', ids.openForm, hash.open, auth(ids.alice));
  await upload('alice upload', berkas, null, aliceOpen, uploadID(11), filePart(pdf, 'a.pdf', 'application/pdf'), auth(ids.alice));
  await per('edit registration with file', 'PUT', '/v2/activities/open-act/registration', (t) => ({ headers: auth(ids.alice), json: { session_token: aliceOpen[t], questionnaire_answer: { nama: 'Alice', berkas: [uploadID(11)] } } }));
  await compareState('edit registration with file');
  await both('edit registration keep file', 'PUT', '/v2/activities/open-act/registration', { headers: auth(ids.alice), json: { questionnaire_answer: { nama: 'Alice 2', berkas: [uploadID(11)] } } });
  await compareState('edit registration keep file');

  // --- Phase 5: register through custom forms ---------------------------------
  for (const [label, who, body] of [
    ['no type', 'bob', {}], ['activity no id', 'bob', { feature_type: 'activity_registration' }], ['bogus type', 'bob', { feature_type: 'bogus', feature_id: 1 }],
    ['independent', 'bob', { feature_type: 'independent_form', feature_id: ids.publicForm }], ['closed activity', 'bob', { feature_type: 'activity_registration', feature_id: ids.closed, custom_form_data: { nama: 'x' } }],
    ['bad id', 'bob', { feature_type: 'activity_registration', feature_id: 'abc' }], ['no form', 'bob', { feature_type: 'activity_registration', feature_id: ids.noForm, custom_form_data: {} }],
    ['invalid data', 'bob', { feature_type: 'activity_registration', feature_id: ids.open, custom_form_data: { nama: 5 } }],
    ['already registered', 'alice', { feature_type: 'activity_registration', feature_id: ids.open, custom_form_data: { nama: 'x' } }],
    ['no session', 'bob', { feature_type: 'activity_registration', feature_id: ids.open, custom_form_data: { nama: 'x' } }],
    ['club closed', 'bob', { feature_type: 'club_registration', feature_id: ids.closedClub, custom_form_data: {} }], ['club bad id', 'bob', { feature_type: 'club_registration', feature_id: 'x' }],
    ['club invalid', 'bob', { feature_type: 'club_registration', feature_id: ids.openClub, custom_form_data: {} }],
    ['club last day', 'bob', { feature_type: 'club_registration', feature_id: ids.lastDayClub, custom_form_data: { nama: 'Bob' } }],
    ['club last day again', 'bob', { feature_type: 'club_registration', feature_id: ids.lastDayClub, custom_form_data: { nama: 'Bob' } }],
  ]) {
    await both(`register ${label}`, 'POST', '/v2/custom-forms/register', { headers: auth(ids[who]), json: body });
    await compareState(`register ${label}`);
  }
  await both('register anonymous', 'POST', '/v2/custom-forms/register', { json: { feature_type: 'activity_registration', feature_id: ids.open } });
  const bobFresh = await session('bob fresh', ids.freshForm, hash.fresh, auth(ids.bob));
  await upload('bob upload', `/v2/custom-forms/${ids.freshForm}/files/berkas`, null, bobFresh, uploadID(21), filePart(pdf, 'b.pdf', 'application/pdf'), auth(ids.bob));
  await per('register activity with file', 'POST', '/v2/custom-forms/register', (t) => ({ headers: auth(ids.bob),
    json: { feature_type: 'activity_registration', feature_id: String(ids.fresh), session_token: bobFresh[t], custom_form_data: { nama: 'Bob', berkas: [uploadID(21)] } } }));
  await compareState('register activity with file');
  const bobClub = await session('bob club', ids.clubForm, hash.club, auth(ids.bob));
  await upload('bob cv', `/v2/custom-forms/${ids.clubForm}/files/cv`, null, bobClub, uploadID(22), filePart(pdf, 'cv.pdf', 'application/pdf'), auth(ids.bob));
  await upload('bob cv image', `/v2/custom-forms/${ids.clubForm}/files/cv`, null, bobClub, uploadID(23), filePart(smallJpg, 'cv.jpg', 'image/jpeg'), auth(ids.bob));
  await per('register club with file', 'POST', '/v2/custom-forms/register', (t) => ({ headers: auth(ids.bob),
    json: { feature_type: 'club_registration', feature_id: ids.openClub, session_token: bobClub[t], custom_form_data: { alasan: 'Ingin', cv: [uploadID(22)] } } }));
  await compareState('register club with file');
  await both('club status after register', 'GET', `/v2/clubs/${ids.openClub}/registration-status`, { headers: auth(ids.bob) });
  await both('my club registrations bob', 'GET', '/v2/club-registrations/my-registrations', { headers: auth(ids.bob) });
  await both('cancel club bob open', 'DELETE', `/v2/clubs/${ids.openClub}/registration`, { headers: auth(ids.bob) });
  await compareState('cancel club bob open');

  // --- Concurrency: duplicate registrations racing each other ----------------
  // Each server gets the same burst; the multiset of statuses and the rows
  // created must match (one success, the rest ALREADY_REGISTERED).
  const burst = async (name, body) => {
    const outcome = {};
    for (const target of ['adonis', 'go']) {
      const results = await Promise.all(Array.from({ length: 8 }, () => comparison.call(target, 'POST', '/v2/custom-forms/register', { headers: auth(ids.carol), json: body })));
      outcome[target] = results.map((r) => `${r.status} ${r.json?.message}`).sort();
    }
    console.log(`${name}: adonis ${JSON.stringify(outcome.adonis)} go ${JSON.stringify(outcome.go)}`);
    comparison.count++;
    if (!isDeepStrictEqual(outcome.adonis, outcome.go)) comparison.failures.push({ name, adonis: outcome.adonis, go: outcome.go });
    await compareState(name);
  };
  await burst('concurrent activity registration', { feature_type: 'activity_registration', feature_id: ids.noGuest, custom_form_data: { nama: 'Carol' } });
  await burst('concurrent club registration', { feature_type: 'club_registration', feature_id: ids.lastDayClub, custom_form_data: { nama: 'Carol' } });

  // --- Phase 5: standalone responses -----------------------------------------
  const pub = await session('public guest', ids.publicForm, hash.public);
  await upload('public photo', `/v2/custom-forms/${ids.publicForm}/files/foto`, null, pub, uploadID(31), filePart(smallJpg, 'foto.jpg', 'image/jpeg'));
  await upload('public pdf rejected', `/v2/custom-forms/${ids.publicForm}/files/foto`, null, pub, uploadID(32), filePart(pdf, 'a.pdf', 'application/pdf'));
  for (const [label, path, build] of [
    ['invalid body', `/v2/custom-forms/${ids.publicForm}/responses`, () => ({ json: { session_token: 'x' } })],
    ['missing form', '/v2/custom-forms/999/responses', (t) => ({ json: { session_token: pub[t], answers: {} } })],
    ['bad id', '/v2/custom-forms/abc/responses', (t) => ({ json: { session_token: pub[t], answers: {} } })],
    ['hex id', '/v2/custom-forms/0x1/responses', (t) => ({ json: { session_token: pub[t], answers: {} } })],
    ['wrong session', `/v2/custom-forms/${ids.publicForm}/responses`, () => ({ json: { session_token: 'e'.repeat(64), answers: {} } })],
    ['invalid answers', `/v2/custom-forms/${ids.publicForm}/responses`, (t) => ({ json: { session_token: pub[t], answers: { pesan: '' } } })],
    ['as member with guest session', `/v2/custom-forms/${ids.publicForm}/responses`, (t) => ({ headers: auth(ids.bob), json: { session_token: pub[t], answers: { pesan: 'x' } } })],
    ['valid', `/v2/custom-forms/${ids.publicForm}/responses`, (t) => ({ json: { session_token: pub[t], answers: { pesan: 'Halo', foto: [uploadID(31)] } } })],
    ['repeat', `/v2/custom-forms/${ids.publicForm}/responses`, (t) => ({ json: { session_token: pub[t], answers: { pesan: 'Lain' } } })],
  ]) {
    const r = await per(`submit ${label}`, 'POST', path, build);
    if (label === 'valid' || label === 'repeat') {
      // Response ids are random; check each server answered with its own stored id.
      for (const t of ['adonis', 'go']) {
        const stored = (await ctx.inSchema(t, (q) => q('SELECT id FROM custom_form_responses ORDER BY created_at DESC LIMIT 1'))).rows[0]?.id;
        if (r[t].json?.data?.id !== stored) comparison.failures.push({ name: `submit ${label} (${t} id)`, adonis: stored, go: r[t].json });
      }
    }
    await compareState(`submit ${label}`);
  }
  const member = await session('members bob', ids.membersForm, hash.members, auth(ids.bob));
  await per('submit members as guest', 'POST', `/v2/custom-forms/${ids.membersForm}/responses`, (t) => ({ json: { session_token: member[t], answers: { pesan: 'x' } } }));
  await per('submit members', 'POST', `/v2/custom-forms/${ids.membersForm}/responses`, (t) => ({ headers: auth(ids.bob), json: { session_token: member[t], answers: { pesan: 'Hai' } } }));
  await compareState('submit members');
  return comparison.report('contract-forms');
});
s3.adonis.close();
s3.go.close();
process.exitCode = ok ? 0 : 1;

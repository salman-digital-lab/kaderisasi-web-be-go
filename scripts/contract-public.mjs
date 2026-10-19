// Differential contract check for the Phase 2 public reads: activities,
// categories, clubs, the monthly leaderboard and public certificate lookups.
// Seeds synthetic fixtures into an owned schema (see contract-lib.mjs).
//
//   node scripts/contract-public.mjs
import { withContract, Comparison } from './contract-lib.mjs';

const ports = { adonis: 3348, go: 3349 };
const smtp = { SMTP_HOST: '127.0.0.1', SMTP_PORT: '2', SMTP_USERNAME: 'x', SMTP_PASSWORD: 'x', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset' };

const json = (v) => JSON.stringify(v);
const today = new Date(Date.now() + 7 * 3600 * 1000).toISOString().slice(0, 10); // Asia/Jakarta date
const shiftDay = (days) => new Date(Date.parse(today) + days * 86400000).toISOString().slice(0, 10);

const richText = [
  '<h2 style="text-align: center">Profil</h2><p>Teks &amp; <strong>tebal</strong> 😀</p><p><br></p>',
  '<p>a<script>alert(1)</script><img src=x onerror=alert(1)><a href="javascript:x">j</a><a href="https://e.x/?a=1&b=2" target="_blank" rel="noopener" class="c">ok</a></p>',
  '<div><p style="color:red;text-align:right">x<p>y</div><ul><li>1<li>2</ul><!-- c -->',
  ' line separator  and "quotes" <b>\'single\'</b>',
  '',
];

async function seed(db) {
  const q = (sql, params) => db.query(sql, params);
  await q(`INSERT INTO universities (id, name) VALUES (9001, 'Institut Teknologi Bandung'), (9002, 'Universitas "Kutip" & <Co>')`);
  const users = [];
  for (let i = 1; i <= 18; i++) {
    const { rows } = await q(`INSERT INTO public_users (email, password, account_status, member_id, created_at, updated_at) VALUES ($1, NULL, $2, $3, $4, $5) RETURNING id`,
      [`member${i}@contract.test`, i % 5 === 0 ? 'no_account' : 'active', `M${String(i).padStart(7, '0')}`, `2025-0${1 + (i % 8)}-1${i % 9}T0${i % 9}:15:30.${String(100 + i).slice(1)}+07:00`, i % 4 === 0 ? null : '2026-01-02T03:04:05.678Z']);
    users.push(rows[0].id);
    if (i % 6 === 0) continue; // some members have no profile
    await q(`INSERT INTO profiles (user_id, name, gender, picture, university_id, intake_year, level, badges, birth_date, education_history, work_history, extra_data, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now())`,
      [rows[0].id, `Anggota ${i} ü`, i % 2 ? 'M' : 'F', i % 3 ? `profile/${i}.png` : null, i % 3 === 0 ? null : i % 2 ? 9001 : 9002, 2020 + (i % 5), i % 4,
        [json(['SSC-1', 2, null]), json('["a"," b "]'), json(' solo '), null, json({})][i % 5],
        i % 3 ? `199${i % 10}-0${1 + (i % 9)}-15` : null,
        [json([{ degree: 'bachelor', institution: ' ITB ', faculty: 'FTI', major: 'TI', intake_year: '2019' }, { degree: 'phd', institution: 1, intake_year: 'x' }]), json('[]'), null, json('"[{\\"major\\":\\"m\\"}]"')][i % 4],
        [json([{ job_title: ' Dev ', company: 'C', start_year: 2020, end_year: '0x7E8' }]), json([1, null, 'x']), null][i % 3],
        [json({ preferred_name: 'P', '10': 'ten', '2': 'two', kaderisasi_path: { ssc: 1.0, lmd: null } }), null, json({})][i % 3]]);
  }

  const clubs = [];
  const clubRows = [
    { name: 'Karisma', type: 'UNIT', show: true, open: true, end: null, media: { items: [{ media_url: 'club/a_1.png', media_type: 'image', extra: 1 }, { media_url: 'club/../x.png', media_type: 'image' }, { media_url: 'https://www.youtube.com/embed/abc_D-1', media_type: 'video', video_source: 'youtube' }, { media_url: 'https://www.youtube.com/embed/abc?x=1', media_type: 'video', video_source: 'youtube' }, { media_url: 'https://youtube.com/embed/abc', media_type: 'video', video_source: 'youtube' }, 'junk', null] }, info: { registration_info: richText[0], extra: 'kept' } },
    { name: 'Gamais 100% _x_', type: 'CLUB_KEPROFESIAN', show: true, open: true, end: shiftDay(-1), media: { items: 'nope' }, info: { registration_info: richText[1] } },
    { name: 'Bahasa Arab', type: 'CLUB_BAHASA', show: true, open: true, end: today, media: null, info: null },
    { name: 'Avisman Jabar', type: 'AVISMAN_REGIONAL', show: true, open: true, end: shiftDay(3), media: [], info: 'plain string' },
    { name: 'Hidden club', type: 'UNIT', show: false, open: true, end: null, media: { items: [] }, info: { registration_info: 'x' } },
    { name: 'Closed club', type: 'UNIT', show: true, open: false, end: null, media: { items: [] }, info: { registration_info: 5 } },
    { name: 'Null show', type: 'UNIT', show: null, open: null, end: null, media: { items: [] }, info: { other: true } },
    { name: 'Ünïcode club', type: 'UNIT', show: true, open: true, end: null, media: { items: [] }, info: [1, 2], description: null },
    { name: 'Leadership broken', type: 'UNIT', show: true, open: false, end: null, media: { items: [] }, info: {} },
  ];
  for (let i = 0; i < 14; i++) clubRows.push({ name: `Klub ${i}`, type: 'UNIT', show: true, open: i % 2 === 0, end: i % 3 ? null : shiftDay(i - 5), media: { items: [] }, info: { registration_info: `<p>${i}</p>` } });
  for (const [i, c] of clubRows.entries()) {
    const { rows } = await q(`INSERT INTO clubs (name, description, short_description, logo, media, start_period, end_period, is_show, created_at, updated_at, registration_info, is_registration_open, registration_end_date, club_type)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
      [c.name, 'description' in c ? c.description : richText[i % richText.length], i % 2 ? `Short ${i}` : null, i % 3 ? `club/logo_${i}.png` : null,
        c.media === null ? null : json(c.media), i % 2 ? '2025-08-29' : null, i % 2 ? '2026-08-29' : null, c.show, '2025-08-29T17:07:44.306+07:00', i % 4 ? '2025-08-29T18:12:31.761+07:00' : null,
        c.info === null ? null : json(c.info), c.open, c.end, c.type]);
    clubs.push(rows[0].id);
  }

  const activities = [];
  for (let i = 0; i < 32; i++) {
    const club = i % 4 === 0 ? clubs[0] : i % 7 === 0 ? clubs[1] : null;
    const config = [{ images: ['a.png'], status_visibility: { is_visible: true }, mandatory_profile_data: [], custom_selection_status: [], additional_questionnaire: [{ name: 'q1', type: 'text', label: 'T\n', required: false }] }, null, { '1': 'x', a: { b: [1.5, 2e21, -0] } }][i % 3];
    const { rows } = await q(`INSERT INTO activities (name, slug, description, badge, activity_start, activity_end, registration_start, registration_end, selection_start, selection_end, activity_type, activity_category, additional_config, minimum_level, is_published, created_at, updated_at, is_registration_open, club_id)
      VALUES ($1, $2, $3, 'badge', $4, $5, '2025-02-01', '2025-02-10', $6, NULL, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id`,
      [i === 3 ? 'Kajian 50% _diskon_' : i === 5 ? 'Ünïcode kegiatan' : `Kegiatan ${i} Salman`, i === 9 ? 'slug-with-ü' : `kegiatan-${i}`, i % 5 ? `<p>raw ${i} <script>kept</script></p>` : null,
        i % 6 ? `2025-${String(1 + (i % 12)).padStart(2, '0')}-${String(1 + (i % 28)).padStart(2, '0')}` : null, i % 6 ? '2025-12-31' : null, i % 2 ? '2025-01-05' : null,
        i % 3, [0, 1, 2, null][i % 4], config === null ? null : json(config), i % 3, i % 9 !== 8, '2025-02-19T20:17:05.921+07:00', i % 5 ? '2026-03-27T12:22:33.863+07:00' : null, i % 2 === 0, club]);
    activities.push(rows[0].id);
  }

  // Leadership: approved and pending registrations, with and without profiles.
  const registration = async (club, user, status) => (await q(`INSERT INTO club_registrations (club_id, member_id, status, created_at) VALUES ($1, $2, $3, now()) RETURNING id`, [club, user, status])).rows[0].id;
  const role = (reg, name, start, primary, order, created) => q(`INSERT INTO club_member_roles (club_registration_id, role_name, start_date, end_date, is_primary, sort_order, created_at, updated_at) VALUES ($1, $2, $3, NULL, $4, $5, $6, $6)`, [reg, name, start, primary, order, created]);
  await role(await registration(clubs[0], users[0], 'APPROVED'), 'Ketua', '2025-01-01', true, 0, '2025-01-01T00:00:00Z');
  await role(await registration(clubs[0], users[5], 'APPROVED'), 'Sekretaris', null, false, 1, '2025-01-02T00:00:00Z'); // member without profile
  await role(await registration(clubs[0], users[2], 'APPROVED'), 'Wakil', '2025-02-01', false, 0, '2025-01-03T00:00:00Z');
  await role(await registration(clubs[0], users[3], 'PENDING'), 'Pending', null, false, 0, '2025-01-04T00:00:00Z');
  await role(await registration(clubs[8], users[4], 'APPROVED'), 'No timestamps', null, false, 0, null);

  // Monthly leaderboard: a paginated month, other months, today, null user.
  for (let i = 0; i < 16; i++) {
    await q(`INSERT INTO monthly_leaderboards (user_id, month, score_academic, score, created_at, updated_at) VALUES ($1, '2026-03-01', 7, $2, now(), $3)`, [users[i], 100 - i * 3, i % 3 ? null : '2026-03-05T10:00:00Z']);
  }
  await q(`INSERT INTO monthly_leaderboards (user_id, month, score, created_at) VALUES ($1, '2026-02-01', 50, now()), (NULL, '2026-02-01', 40, now()), ($2, $3, 30, now()), ($4, '2026-03-15', 20, now())`, [users[1], users[2], today, users[3]]);

  // Certificates.
  const { rows: [template] } = await q(`INSERT INTO certificate_templates (name, template_data, created_at, updated_at) VALUES ('Template', '{}', now(), now()) RETURNING id`);
  const approvalTemplate = { name: 'Sertifikat', background_image: 'bg.png', template_data: { backgroundUrl: null, canvasWidth: 1123, canvasHeight: 794.5, elements: [{ id: 'e1', type: 'variable-text', variable: '{{ approval }}', x: 1, y: 2.25, width: 3, height: 4 }] } };
  const plainTemplate = { name: 'Biasa', background_image: null, template_data: { backgroundUrl: null, canvasWidth: 10, canvasHeight: 10, elements: [{ id: 'e', type: 'static-text', content: '<b>x</b>' }, { type: 'variable-text', variable: 'name' }] } };
  const participant = { registration_id: 1, user_id: 1, name: 'Peserta Satu', email: 'p@contract.test', university: 'ITB', gender: 'M', activity_name: 'Kegiatan', activity_date: '19 Februari 2025' };
  const activitySnapshot = { id: 1, name: 'Kegiatan', activity_start: '2025-02-19', certificate_settings: { version: 1, institution: 'Salman', include_scores: false } };
  const approval = { signer_name: 'Ketua', signer_title: 'Ketua BMKA', approved_at: '2025-03-01T00:00:00.000Z', request_id: 3, signer_id: 4, content_hash: 'abc' };
  const certificates = [
    ['SAL-0001', plainTemplate, participant, activitySnapshot, null, null],
    ['SAL-0002', approvalTemplate, participant, activitySnapshot, approval, null],
    ['SAL-0003', approvalTemplate, participant, activitySnapshot, null, null], // approval missing → not found
    ['SAL-0004', approvalTemplate, participant, activitySnapshot, { ...approval, signer_title: '' }, null],
    ['SAL-0005', plainTemplate, { ...participant, gender: null, university: undefined }, null, null, '2025-04-01T08:00:00.123+07:00'], // revoked, no activity snapshot
    ['SAL-0006', plainTemplate, 'just a string', activitySnapshot, 'approval string', null],
    ['SAL-0007', null, participant, activitySnapshot, null, null], // template null: verify works, show fails
    ['SAL-0008', { name: 'Broken' }, participant, activitySnapshot, null, null], // no template_data → 500
    ['SAL-0009', { template_data: { elements: 'x' } }, participant, activitySnapshot, null, null],
    ['SAL-0010', { template_data: { elements: [{ type: 'variable-text', variable: 7 }] } }, participant, activitySnapshot, null, null],
    ['SAL-0011', plainTemplate, null, activitySnapshot, null, null], // participant null → 500
    ['SAL-0012', plainTemplate, {}, 'text activity', approval, null],
  ];
  for (const [i, [code, tpl, part, act, appr, revoked]] of certificates.entries()) {
    const { rows: [reg] } = await q(`INSERT INTO activity_registrations (user_id, activity_id, status, created_at) VALUES ($1, $2, 'LULUS KEGIATAN', now()) RETURNING id`, [users[i], activities[i]]);
    await q(`INSERT INTO issued_certificates (certificate_code, registration_id, activity_id, user_id, template_id, template_snapshot, participant_snapshot, activity_snapshot, approval_snapshot, issued_at, revoked_at, revoked_reason, created_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '2025-03-01T09:30:00.250+07:00', $10, $11, now())`,
      [code, reg.id, activities[i], users[i], template.id, json(tpl), json(part), json(act), appr === null ? null : json(appr), revoked, revoked ? 'Salah nama' : null]);
  }
  return { users, clubs, activities, codes: certificates.map((c) => c[0]) };
}

// Ids and timestamps differ only by insertion; values come from the same
// fixtures, so the bodies must be identical.
const comparison = new Comparison(ports, { headers: ['cache-control', 'ratelimit-limit', 'ratelimit-remaining', 'retry-after'] });

const ok = await withContract('contract-public', { ports, env: smtp }, async (ctx) => {
  const fixtures = await seed(ctx.db);
  await ctx.startServers();
  const both = (name, path, headers) => comparison.both(name, 'GET', path, { headers });
  let ip = 0;
  const fresh = () => ({ 'x-forwarded-for': `198.51.100.${(ip++ % 250) + 1}, 10.0.0.${ip % 200}` });

  // Activities
  for (const qs of ['', '?page=1', '?page=2&per_page=5', '?page=7&per_page=5', '?per_page=0', '?per_page=-1', '?per_page=abc', '?per_page=1.5', '?page=0',
    '?page=-1', '?page=abc', '?page=2.5', '?page=1e1&per_page=2', '?page=', '?page=%20', '?page=0x2', '?page=Infinity', '?per_page=Infinity', '?page=1&page=2',
    '?search=kegiatan', '?search=50%25', '?search=_', '?search=%C3%BC', '?search=', '?category=0', '?category=1', '?category=abc', '?category=', '?category=%201%20',
    `?club_id=${fixtures.clubs[0]}`, '?club_id=abc', '?category=2&search=salman&per_page=3&page=2', '?search=nothing-matches', '?per_page=-1&page=3', '?per_page=-2&page=0', '?page=3&per_page=-5&category=1&club_id=1']) {
    await both(`activities ${qs}`, `/v2/activities${qs}`);
  }
  await both('categories', '/v2/activities/categories');
  for (let i = 0; i < 32; i++) await both(`activity kegiatan-${i}`, `/v2/activities/kegiatan-${i}`);
  for (const slug of ['slug-with-%C3%BC', 'missing', 'kegiatan-8', '%20kegiatan-1', 'KEGIATAN-1']) await both(`activity ${slug}`, `/v2/activities/${slug}`);

  // Clubs
  for (const qs of ['', '?page=2&per_page=5', '?per_page=100', '?per_page=0', '?per_page=abc', '?page=-3', '?page=1.5', '?search=klub', '?search=%20klub%201%20', '?search=100%25',
    '?search=_x_', '?club_type=UNIT', '?club_type=CLUB_BAHASA', '?club_type=unit', '?club_type=', '?search=' + 'k'.repeat(120), '?page=99']) {
    await both(`clubs ${qs}`, `/v2/clubs${qs}`);
  }
  for (const id of fixtures.clubs) await both(`club ${id}`, `/v2/clubs/${id}`);
  for (const id of ['abc', '1.5', '99999', '0', '-1', '99999999999', '%20' + fixtures.clubs[0]]) await both(`club ${id}`, `/v2/clubs/${id}`);

  // Monthly leaderboard
  for (const qs of ['?month=2026-03-01', '?month=2026-03-01&page=2&per_page=5', '?month=2026-03-01&per_page=0', '?month=2026-03-01&page=0', '?month=2026-03',
    '?month=2026-02-01', '?month=2026-02-28T20:00:00Z', '?month=2026-03-01T00:30:00%2B09:00', '?month=bad', '?month=', '', '?month=2026-W09-7', '?month=2026-060',
    '?month=20260301', '?month=2026-02-30', '?month=2026-03-15', '?page=abc&month=2026-03-01', '?month=2026-03-01&per_page=-1', '?month=bad&per_page=-1', '?month=2026-03-01&page=2&per_page=-3', '?month=2026-03-01&per_page=abc']) {
    await both(`monthly ${qs}`, `/v2/achievements/monthly${qs}`);
  }

  // Certificates (a fresh client address per request stays under the limit)
  for (const code of [...fixtures.codes, 'sal-0001', '%20SAL-0002%20', 'SAL_0001', 'MISSING-1', 'x'.repeat(97), 'ÜÖ']) {
    await both(`certificate ${code}`, `/v2/certificates/code/${code}`, fresh());
    await both(`verify ${code}`, `/v2/certificates/verify/${code}`, fresh());
  }
  // Throttle: 32 lookups from one client; both routes share the bucket.
  for (let i = 0; i < 32; i++) {
    const path = i % 2 ? '/v2/certificates/verify/SAL-0001' : '/v2/certificates/code/SAL-0001';
    await both(`throttle #${i + 1}`, path, { 'x-forwarded-for': '203.0.113.99' });
  }
  return comparison.report('contract-public');
});
process.exitCode = ok ? 0 : 1;

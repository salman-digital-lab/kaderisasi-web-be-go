-- Pembinaan Beasiswa Perintis. Kept identical in kaderisasi-web-be-go and
-- kaderisasi-admin-be-go (both generate package dbgen). Workflows load a row
-- (FOR UPDATE inside a transaction), change it in Go and save every mutable
-- column back; the table CHECKs guard each state.

-- ---- participants ------------------------------------------------------

-- name: PerintisParticipant :one
SELECT * FROM perintis_participants WHERE user_id = $1;

-- name: PerintisParticipantForUpdate :one
SELECT * FROM perintis_participants WHERE user_id = $1 FOR UPDATE;

-- name: PerintisParticipants :many
SELECT * FROM perintis_participants ORDER BY user_id;

-- name: PerintisParticipantByNIM :one
SELECT * FROM perintis_participants WHERE nim = $1 ORDER BY user_id LIMIT 1;

-- name: InsertPerintisParticipant :one
INSERT INTO perintis_participants (
  user_id, is_mentee, is_mentor, is_coordinator, nim, university_id, kip_k, category,
  kaderisasi_override, kaderisasi_override_note, mentor_user_id, coordinator_user_id,
  status, inactive_reason, updated_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: SavePerintisParticipant :one
UPDATE perintis_participants SET
  is_mentee = $2, is_mentor = $3, is_coordinator = $4, nim = $5, university_id = $6,
  kip_k = $7, category = $8, kaderisasi_override = $9, kaderisasi_override_note = $10,
  mentor_user_id = $11, coordinator_user_id = $12, status = $13, inactive_reason = $14,
  updated_by = $15, updated_at = now()
WHERE user_id = $1
RETURNING *;

-- ---- people (read-only kaderisasi data) ---------------------------------

-- name: PerintisPeople :many
SELECT
  u.id AS user_id,
  u.email,
  u.account_status,
  COALESCE(p.name, split_part(COALESCE(u.email, ''), '@', 1))::text AS name,
  p.gender,
  p.whatsapp,
  p.instagram,
  p.linkedin,
  p.university_id,
  un.name AS university,
  p.major,
  p.intake_year,
  COALESCE(p.level, 0)::int AS level,
  COALESCE(p.badges, '[]'::jsonb) AS badges,
  c.name AS city,
  pr.name AS province
FROM public_users u
LEFT JOIN profiles p ON p.user_id = u.id
LEFT JOIN universities un ON un.id = p.university_id
LEFT JOIN cities c ON c.id = p.city_id
LEFT JOIN provinces pr ON pr.id = p.province_id
WHERE u.id = ANY(sqlc.arg(user_ids)::int[])
ORDER BY u.id;

-- name: PerintisPublicUserByEmail :one
SELECT id, email, account_status FROM public_users
WHERE lower(email) = lower(sqlc.arg(email)::text)
ORDER BY id LIMIT 1;

-- name: InsertPerintisPublicUser :one
INSERT INTO public_users (email, created_at, updated_at, account_status)
VALUES (lower(sqlc.arg(email)::text), now(), now(), 'no_account')
RETURNING id;

-- name: InsertPerintisProfile :exec
INSERT INTO profiles (user_id, name, created_at, updated_at)
SELECT sqlc.arg(user_id)::int, sqlc.arg(name)::text, now(), now()
WHERE NOT EXISTS (SELECT 1 FROM profiles WHERE user_id = sqlc.arg(user_id)::int);

-- name: PerintisUniversityByName :one
SELECT id FROM universities WHERE lower(name) = lower(sqlc.arg(name)::text) ORDER BY id LIMIT 1;

-- name: PerintisActivities :many
SELECT ar.user_id, a.name, a.activity_type, a.activity_category, a.activity_start,
       COALESCE(ar.status, '')::text AS status,
       (a.certificate_template_id IS NOT NULL)::boolean AS has_certificate
FROM activity_registrations ar
JOIN activities a ON a.id = ar.activity_id
WHERE ar.user_id = ANY(sqlc.arg(user_ids)::int[])
ORDER BY a.activity_start DESC NULLS LAST, a.id DESC;

-- name: PerintisClubs :many
SELECT cr.member_id AS user_id, c.name, COALESCE(cr.status, '')::text AS status,
       COALESCE((
         SELECT r.role_name FROM club_member_roles r
         WHERE r.club_registration_id = cr.id
         ORDER BY r.is_primary DESC, r.sort_order, r.id LIMIT 1
       ), 'Anggota')::text AS role
FROM club_registrations cr
JOIN clubs c ON c.id = cr.club_id
WHERE cr.member_id = ANY(sqlc.arg(user_ids)::int[]) AND cr.status = 'APPROVED'
ORDER BY c.name;

-- name: PerintisAchievements :many
SELECT user_id, COALESCE(name, '')::text AS name, type, achievement_date, COALESCE(status, 0)::int AS status
FROM achievements
WHERE user_id = ANY(sqlc.arg(user_ids)::int[])
ORDER BY achievement_date DESC NULLS LAST, id DESC;

-- ---- journals ------------------------------------------------------------

-- name: EscalatePerintisFlags :execrows
UPDATE perintis_journals SET flag_status = 'escalated', updated_at = now()
WHERE flag_status = 'open' AND flag_due_at < sqlc.arg(at)::timestamptz;

-- name: PerintisJournal :one
SELECT * FROM perintis_journals WHERE id = $1;

-- name: PerintisJournalForUpdate :one
SELECT * FROM perintis_journals WHERE id = $1 FOR UPDATE;

-- name: PerintisJournalByMonth :one
SELECT * FROM perintis_journals WHERE mentee_user_id = $1 AND month = $2;

-- name: PerintisJournalByMonthForUpdate :one
SELECT * FROM perintis_journals WHERE mentee_user_id = $1 AND month = $2 FOR UPDATE;

-- name: PerintisJournalsOfMentees :many
SELECT * FROM perintis_journals
WHERE mentee_user_id = ANY(sqlc.arg(user_ids)::int[])
ORDER BY month, id;

-- name: PerintisJournalsOfMentorsInMonth :many
SELECT * FROM perintis_journals
WHERE mentor_user_id = ANY(sqlc.arg(mentor_ids)::int[]) AND month = sqlc.arg(month)::date
ORDER BY id;

-- name: PerintisJournalsInMonth :many
SELECT * FROM perintis_journals WHERE month = $1 ORDER BY id;

-- name: PerintisFollowupJournals :many
SELECT * FROM perintis_journals
WHERE (flag_reason IS NOT NULL AND (status = 'flagged' OR followup_at IS NOT NULL))
   OR flag_status IN ('escalated', 'resolved')
ORDER BY id;

-- name: InsertPerintisJournal :one
INSERT INTO perintis_journals (mentee_user_id, month, mentor_user_id, category, form_version, revision)
VALUES ($1, $2, $3, $4, $5, 0)
RETURNING *;

-- name: SavePerintisJournal :one
UPDATE perintis_journals SET
  mentor_user_id = sqlc.arg(mentor_user_id), status = sqlc.arg(status), answers = sqlc.arg(answers),
  files = sqlc.arg(files), pulse_energy = sqlc.arg(pulse_energy), pulse_academic = sqlc.arg(pulse_academic),
  pulse_finance = sqlc.arg(pulse_finance), pulse_relation = sqlc.arg(pulse_relation),
  wants_contact = sqlc.arg(wants_contact), honesty_confirmed_at = sqlc.arg(honesty_confirmed_at),
  submitted_at = sqlc.arg(submitted_at), is_late = sqlc.arg(is_late), revision = sqlc.arg(revision),
  reopened_by = sqlc.arg(reopened_by), reopen_reason = sqlc.arg(reopen_reason),
  response_appreciation = sqlc.arg(response_appreciation), response_text = sqlc.arg(response_text),
  revision_request = sqlc.arg(revision_request), flag_reason = sqlc.arg(flag_reason),
  responded_by = sqlc.arg(responded_by), responded_at = sqlc.arg(responded_at),
  response_read_at = sqlc.arg(response_read_at), flag_trigger = sqlc.arg(flag_trigger),
  flag_status = sqlc.arg(flag_status), flag_due_at = sqlc.arg(flag_due_at),
  flag_handled_by = sqlc.arg(flag_handled_by), flag_handled_at = sqlc.arg(flag_handled_at),
  flag_private_note = sqlc.arg(flag_private_note), followup_by = sqlc.arg(followup_by),
  followup_at = sqlc.arg(followup_at), followup_note = sqlc.arg(followup_note), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- ---- session reports -----------------------------------------------------

-- name: PerintisReport :one
SELECT * FROM perintis_session_reports WHERE id = $1;

-- name: PerintisReportForUpdate :one
SELECT * FROM perintis_session_reports WHERE id = $1 FOR UPDATE;

-- name: PerintisReportByMonth :one
SELECT * FROM perintis_session_reports WHERE mentor_user_id = $1 AND month = $2;

-- name: PerintisReportByMonthForUpdate :one
SELECT * FROM perintis_session_reports WHERE mentor_user_id = $1 AND month = $2 FOR UPDATE;

-- name: PerintisReportsOfMentor :many
SELECT * FROM perintis_session_reports WHERE mentor_user_id = $1 ORDER BY month;

-- name: PerintisReportsInMonth :many
SELECT * FROM perintis_session_reports WHERE month = $1 ORDER BY id;

-- name: PerintisCaseReports :many
SELECT * FROM perintis_session_reports WHERE case_status IS NOT NULL ORDER BY id;

-- name: PerintisUnreadReviews :one
SELECT count(*) FROM perintis_session_reports
WHERE mentor_user_id = $1 AND reviewed_at IS NOT NULL AND review_read_at IS NULL;

-- name: InsertPerintisReport :one
INSERT INTO perintis_session_reports (mentor_user_id, month, curriculum_code)
VALUES ($1, $2, $3)
RETURNING *;

-- name: SavePerintisReport :one
UPDATE perintis_session_reports SET
  session_date = sqlc.arg(session_date), mode = sqlc.arg(mode), location_or_link = sqlc.arg(location_or_link),
  curriculum_code = sqlc.arg(curriculum_code), deviation_reason = sqlc.arg(deviation_reason),
  summary = sqlc.arg(summary), photos = sqlc.arg(photos), case_note = sqlc.arg(case_note),
  case_status = sqlc.arg(case_status), followup_by = sqlc.arg(followup_by),
  followup_at = sqlc.arg(followup_at), followup_note = sqlc.arg(followup_note),
  status = sqlc.arg(status), submitted_at = sqlc.arg(submitted_at), is_late = sqlc.arg(is_late),
  review_appreciation = sqlc.arg(review_appreciation), review_note = sqlc.arg(review_note),
  reviewer_kind = sqlc.arg(reviewer_kind), reviewed_by = sqlc.arg(reviewed_by),
  reviewed_at = sqlc.arg(reviewed_at), review_read_at = sqlc.arg(review_read_at),
  verified_at = sqlc.arg(verified_at), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- ---- names of staff on follow-ups and reviews ------------------------------

-- name: PerintisAdminNames :many
SELECT id, COALESCE(display_name, email)::text AS name FROM admin_users
WHERE id = ANY(sqlc.arg(ids)::int[]);

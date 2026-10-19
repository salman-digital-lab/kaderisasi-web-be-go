-- Phase 2 public reads. Statements mirror the SQL Lucid/knex sends, so rows
-- without a total ORDER BY come back in the same order.

-- ActivitiesController.index: filters are optional; category and club_id are
-- the raw query-string text, cast by PostgreSQL like knex's bound strings.
-- name: CountPublishedActivities :one
SELECT count(*) AS total FROM activities
WHERE (sqlc.narg(category)::text IS NULL OR activity_category = CAST(sqlc.narg(category)::text AS integer))
  AND (sqlc.narg(club_id)::text IS NULL OR club_id = CAST(sqlc.narg(club_id)::text AS integer))
  AND name ILIKE @pattern::text AND is_published = true;

-- name: ListPublishedActivities :many
SELECT * FROM activities
WHERE (sqlc.narg(category)::text IS NULL OR activity_category = CAST(sqlc.narg(category)::text AS integer))
  AND (sqlc.narg(club_id)::text IS NULL OR club_id = CAST(sqlc.narg(club_id)::text AS integer))
  AND name ILIKE @pattern::text AND is_published = true
ORDER BY id DESC
LIMIT sqlc.narg(row_limit)::bigint OFFSET sqlc.narg(row_offset)::bigint;

-- name: PublishedActivityCategories :many
SELECT DISTINCT activity_category, activity_category FROM activities WHERE is_published = true;

-- name: PublishedActivityBySlug :one
SELECT * FROM activities WHERE slug = @slug::text AND is_published = true LIMIT 1;

-- name: PublishedActivitiesForClub :many
SELECT * FROM activities WHERE club_id = @club_id::integer AND is_published = true ORDER BY activity_start ASC;

-- Club.find(params.id): the raw route parameter, cast by PostgreSQL.
-- name: ClubByID :one
SELECT * FROM clubs WHERE id = CAST(@id::text AS integer) LIMIT 1;

-- name: CountVisibleClubs :one
SELECT count(*) AS total FROM clubs
WHERE name ILIKE @pattern::text AND is_show = true AND (sqlc.narg(club_type)::text IS NULL OR club_type = sqlc.narg(club_type)::text);

-- name: ListVisibleClubs :many
SELECT * FROM clubs
WHERE name ILIKE @pattern::text AND is_show = true AND (sqlc.narg(club_type)::text IS NULL OR club_type = sqlc.narg(club_type)::text)
ORDER BY id DESC
LIMIT @row_limit::bigint OFFSET @row_offset::bigint;

-- ClubsController.show leadership: roles of approved registrations, then
-- the registration, member and first profile (Lucid preloads).
-- name: ClubLeadership :many
SELECT role.*, cr.id AS registration_id, cr.member_id, u.id AS user_id,
       p.user_id AS profile_user_id, COALESCE(p.name, '') AS profile_name, p.picture AS profile_picture, (p.user_id IS NOT NULL)::boolean AS has_profile
FROM club_member_roles role
LEFT JOIN club_registrations cr ON cr.id = role.club_registration_id
LEFT JOIN public_users u ON u.id = cr.member_id
LEFT JOIN LATERAL (SELECT user_id, name, picture FROM profiles WHERE profiles.user_id = u.id LIMIT 1) p ON true
WHERE EXISTS (SELECT 1 FROM club_registrations r WHERE role.club_registration_id = r.id AND r.club_id = @club_id::integer AND r.status = 'APPROVED')
ORDER BY role.sort_order ASC, role.is_primary DESC, role.created_at ASC;

-- LeaderboardsController.monthly
-- name: CountMonthlyLeaderboard :one
SELECT count(*) AS total FROM monthly_leaderboards WHERE month IS NOT DISTINCT FROM sqlc.narg(month)::date;

-- name: ListMonthlyLeaderboard :many
SELECT * FROM monthly_leaderboards WHERE month IS NOT DISTINCT FROM sqlc.narg(month)::date
ORDER BY score DESC
LIMIT sqlc.narg(row_limit)::bigint OFFSET sqlc.narg(row_offset)::bigint;

-- name: PublicUsersByIDs :many
SELECT * FROM public_users WHERE id = ANY(@ids::integer[]);

-- name: ProfilesByUserIDs :many
SELECT * FROM profiles WHERE user_id = ANY(@ids::integer[]);

-- name: UniversitiesByIDs :many
SELECT id, name FROM universities WHERE id = ANY(@ids::integer[]);

-- CertificatesController.showByCode / verify
-- name: IssuedCertificateByCode :one
SELECT * FROM issued_certificates WHERE certificate_code = @code::text LIMIT 1;

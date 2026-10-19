-- ProfilesController.show: the profile with its belongsTo relations.
-- name: ProvincesByIDs :many
SELECT id, name FROM provinces WHERE id = ANY(@ids::integer[]);

-- name: CitiesByIDs :many
SELECT id, province_id, name FROM cities WHERE id = ANY(@ids::integer[]);

-- name: ProfileForUpdate :one
SELECT * FROM profiles WHERE user_id = $1 LIMIT 1 FOR UPDATE;

-- name: SetProfilePicture :exec
UPDATE profiles SET picture = $2, updated_at = $3 WHERE id = $1;

-- name: ProfileUsingPicture :one
SELECT id FROM profiles WHERE picture = $1 LIMIT 1;

-- ProfilesController.activities
-- name: RegistrationsByUser :many
SELECT * FROM activity_registrations WHERE user_id = $1;

-- name: ActivitiesByIDs :many
SELECT * FROM activities WHERE id = ANY(@ids::integer[]);

-- name: IssuedCertificatesByRegistrations :many
SELECT * FROM issued_certificates WHERE registration_id = ANY(@ids::integer[]) ORDER BY id ASC;

-- Achievements
-- name: AchievementForUser :one
SELECT * FROM achievements WHERE id = CAST(@id::text AS integer) AND user_id = @user_id::integer LIMIT 1;

-- name: AchievementsForUserCount :one
SELECT count(*) AS total FROM achievements WHERE user_id = $1;

-- name: AchievementsForUser :many
SELECT * FROM achievements WHERE user_id = @user_id::integer ORDER BY achievement_date DESC
LIMIT sqlc.narg(row_limit)::bigint OFFSET sqlc.narg(row_offset)::bigint;

-- name: CreateAchievement :one
INSERT INTO achievements (user_id, name, description, achievement_date, type, score, proof, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 0, $6, 0, $7, $8) RETURNING id;

-- Ruang curhat
-- name: CreateRuangCurhat :one
INSERT INTO ruang_curhats (problem_ownership, owner_name, problem_category, problem_description, handling_technic, counselor_gender, status, user_id, created_at, updated_at)
VALUES (CAST(@problem_ownership::text AS integer), sqlc.narg(owner_name)::text, @problem_category::text, @problem_description::text, @handling_technic::text, @counselor_gender::text, 0, @user_id::integer, @created_at::timestamptz, @updated_at::timestamptz) RETURNING id;

-- name: RuangCurhatsForUser :many
SELECT * FROM ruang_curhats WHERE user_id = $1;

-- name: AdminUsersByIDs :many
SELECT * FROM admin_users WHERE id = ANY(@ids::integer[]);

-- MembersController.submit
-- name: CreateMemberProfile :exec
INSERT INTO profiles (user_id, name, badges, gender, personal_id, whatsapp, line, instagram, tiktok, linkedin, province_id, city_id,
  country, birth_date, origin_province_id, origin_city_id, education_history, work_history, extra_data, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21);

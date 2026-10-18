-- name: PublicUserByID :one
SELECT * FROM public_users WHERE id=$1;

-- Lucid findBy('email', ...): exact match, first row.
-- name: PublicUserByEmail :one
SELECT * FROM public_users WHERE email=$1 LIMIT 1;

-- Google sign-in matches case-insensitively and refuses ambiguity (limit 2).
-- name: PublicUsersByLowerEmail :many
SELECT * FROM public_users WHERE lower(email) = @email::text LIMIT 2;

-- name: LockEmail :exec
SELECT pg_advisory_xact_lock(hashtext(@email::text));

-- name: CreatePublicUser :one
INSERT INTO public_users (email, password, account_status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: SetMemberID :exec
UPDATE public_users SET member_id=$2, updated_at=$3 WHERE id=$1;

-- name: SetPassword :exec
UPDATE public_users SET password=$2, updated_at=$3 WHERE id=$1;

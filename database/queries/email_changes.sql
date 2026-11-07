-- Self-service member email changes (internal/emailchange).

-- name: LockPublicUser :one
SELECT * FROM public_users WHERE id = $1 FOR UPDATE;

-- name: OtherPublicUserByLowerEmail :one
SELECT id FROM public_users WHERE lower(email) = @email::text AND id <> @id::integer LIMIT 1;

-- name: OpenEmailChange :one
SELECT * FROM public_user_email_changes
WHERE user_id = $1 AND used_at IS NULL AND cancelled_at IS NULL
FOR UPDATE;

-- name: CancelOpenEmailChange :exec
UPDATE public_user_email_changes SET cancelled_at = @now::timestamptz, updated_at = @now::timestamptz
WHERE user_id = @user_id AND used_at IS NULL AND cancelled_at IS NULL;

-- name: CancelEmailChange :exec
UPDATE public_user_email_changes SET cancelled_at = @now::timestamptz, updated_at = @now::timestamptz WHERE id = @id;

-- name: CreateEmailChange :one
INSERT INTO public_user_email_changes
  (user_id, old_email, new_email, code_hash, code_expires_at, token_hash, token_expires_at, created_at, updated_at)
VALUES (@user_id, @old_email, @new_email, sqlc.narg('code_hash'), sqlc.narg('code_expires_at'),
  sqlc.narg('token_hash'), sqlc.narg('token_expires_at'), @now::timestamptz, @now::timestamptz)
RETURNING *;

-- name: CountEmailChangeAttempt :one
UPDATE public_user_email_changes SET code_attempts = code_attempts + 1, updated_at = @now::timestamptz
WHERE id = @id RETURNING code_attempts;

-- name: VerifyEmailChangeCode :exec
UPDATE public_user_email_changes
SET code_verified_at = @now::timestamptz, token_hash = @token_hash, token_expires_at = @token_expires_at, updated_at = @now::timestamptz
WHERE id = @id;

-- name: EmailChangeByToken :one
SELECT * FROM public_user_email_changes WHERE token_hash = $1;

-- name: UseEmailChange :exec
UPDATE public_user_email_changes SET used_at = @now::timestamptz, updated_at = @now::timestamptz WHERE id = @id;

-- name: SetPublicUserEmail :one
UPDATE public_users SET email = @email, updated_at = @now::timestamptz WHERE id = @id RETURNING *;

-- Every request counts toward the limits, also cancelled ones, so cancelling
-- does not let a member send more emails.
-- name: RecentEmailChanges :one
SELECT count(*) FILTER (WHERE created_at > @since::timestamptz)::integer AS in_window,
  coalesce(max(created_at), '-infinity'::timestamptz)::timestamptz AS latest
FROM public_user_email_changes WHERE user_id = @user_id;

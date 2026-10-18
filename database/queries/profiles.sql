-- Lucid Profile.findBy('user_id', ...): first row, no ordering.
-- name: ProfileByUserID :one
SELECT * FROM profiles WHERE user_id=$1 LIMIT 1;

-- name: CreateProfile :exec
INSERT INTO profiles (user_id, name, created_at, updated_at) VALUES ($1, $2, $3, $4);

-- name: PublicUserByID :one
SELECT * FROM public_users WHERE id=$1;

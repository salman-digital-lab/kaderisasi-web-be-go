-- P3RI committee registration for members. The form is hardcoded in
-- internal/p3ri/form_1448h.json; answers are stored as JSON keyed by question id.

-- name: P3riRegistrationForUser :one
SELECT * FROM p3ri_registrations WHERE user_id = $1;

-- name: P3riRegistrationForUserForUpdate :one
SELECT * FROM p3ri_registrations WHERE user_id = $1 FOR UPDATE;

-- name: InsertP3riRegistration :one
INSERT INTO p3ri_registrations (user_id, choices, answers, files)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateP3riRegistrationAnswers :one
UPDATE p3ri_registrations SET
  choices = $2, answers = $3, files = $4, submitted_at = now(), updated_at = now()
WHERE user_id = $1
RETURNING *;

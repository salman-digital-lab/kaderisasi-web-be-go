-- custom forms, sessions, attachments and responses (form_session_service.ts,
-- form_upload_service.ts, custom_forms_controller.ts)

-- name: CustomFormByID :one
SELECT * FROM custom_forms WHERE id = CAST(@id::text AS integer) LIMIT 1;

-- name: CustomFormByIDForShare :one
SELECT * FROM custom_forms WHERE id = CAST(@id::text AS integer) LIMIT 1 FOR SHARE;

-- name: CustomFormByIDForUpdate :one
SELECT * FROM custom_forms WHERE id = @id::integer LIMIT 1 FOR UPDATE;

-- name: StandaloneFormForUpdate :one
SELECT * FROM custom_forms WHERE id = CAST(@id::text AS integer) AND feature_type = 'independent_form' LIMIT 1 FOR UPDATE;

-- name: IndependentForm :one
SELECT * FROM custom_forms WHERE id = CAST(@id::text AS integer) AND feature_type = 'independent_form' LIMIT 1;

-- name: ActiveFeatureForm :one
SELECT * FROM custom_forms WHERE feature_type = @feature_type::text AND feature_id = CAST(@feature_id::text AS integer) AND is_active = true
ORDER BY updated_at DESC, id DESC LIMIT 1;

-- name: ActivityByID :one
SELECT * FROM activities WHERE id = CAST(@id::text AS integer) LIMIT 1;

-- name: ActivityByIDForUpdate :one
SELECT * FROM activities WHERE id = CAST(@id::text AS integer) LIMIT 1 FOR UPDATE;

-- name: ActivityBySlugForUpdate :one
SELECT * FROM activities WHERE slug = @slug::text LIMIT 1 FOR UPDATE;

-- name: ActivityBySlug :one
SELECT * FROM activities WHERE slug = @slug::text LIMIT 1;

-- name: CreateFormSession :exec
INSERT INTO custom_form_sessions (id, form_id, user_id, token_hash, schema_hash, expires_at)
VALUES (@id::uuid, @form_id::integer, sqlc.narg(user_id)::integer, @token_hash::text, @schema_hash::text, @expires_at::timestamptz);

-- name: LockFormSession :one
SELECT * FROM custom_form_sessions WHERE token_hash = @token_hash::text AND form_id = @form_id::integer LIMIT 1 FOR UPDATE;

-- name: ReadFormSession :one
SELECT * FROM custom_form_sessions WHERE token_hash = @token_hash::text AND form_id = @form_id::integer LIMIT 1;

-- name: LockFormSessionByID :one
SELECT * FROM custom_form_sessions WHERE id = @id::uuid LIMIT 1 FOR UPDATE;

-- name: CompleteFormSession :exec
UPDATE custom_form_sessions SET completed_at = @at::timestamptz WHERE id = @id::uuid;

-- name: RetainedAttachments :many
SELECT a.id FROM custom_form_attachments a JOIN custom_form_sessions s ON s.id = a.session_id
WHERE s.form_id = @form_id::integer AND s.user_id IS NOT DISTINCT FROM sqlc.narg(user_id)::integer AND a.field_key = @field_key::text
  AND a.id = ANY(@ids::uuid[]) AND a.claimed_at IS NOT NULL;

-- name: UnclaimedAttachmentsForUpdate :many
SELECT * FROM custom_form_attachments WHERE session_id = @session_id::uuid AND field_key = @field_key::text AND id = ANY(@ids::uuid[]) AND claimed_at IS NULL FOR UPDATE;

-- name: ClaimAttachments :exec
UPDATE custom_form_attachments SET claimed_at = @at::timestamptz WHERE id = ANY(@ids::uuid[]);

-- name: AttachmentByID :one
SELECT * FROM custom_form_attachments WHERE id = @id::uuid LIMIT 1;

-- name: CountFieldAttachments :one
SELECT count(*) AS total FROM custom_form_attachments WHERE session_id = @session_id::uuid AND field_key = @field_key::text;

-- name: InsertAttachment :one
INSERT INTO custom_form_attachments (id, session_id, field_key, storage_key, original_name, download_name, mime_type, size_bytes, source_size_bytes, width, height)
VALUES (@id::uuid, @session_id::uuid, @field_key::text, @storage_key::text, @original_name::text, @download_name::text, @mime_type::text,
  @size_bytes::integer, @source_size_bytes::integer, sqlc.narg(width)::integer, sqlc.narg(height)::integer)
ON CONFLICT (id) DO NOTHING RETURNING *;

-- name: AttachmentWithStorageKey :one
SELECT id FROM custom_form_attachments WHERE storage_key = @storage_key::text LIMIT 1;

-- name: UnclaimedSessionAttachmentForUpdate :one
SELECT * FROM custom_form_attachments WHERE id = CAST(@id::text AS uuid) AND session_id = @session_id::uuid AND claimed_at IS NULL LIMIT 1 FOR UPDATE;

-- name: DeleteAttachment :exec
DELETE FROM custom_form_attachments WHERE id = @id::uuid;

-- name: ResponseForSession :one
SELECT * FROM custom_form_responses WHERE session_id = @session_id::uuid LIMIT 1;

-- name: InsertFormResponse :exec
INSERT INTO custom_form_responses (id, form_id, session_id, user_id, form_snapshot, answers)
VALUES (@id::uuid, @form_id::integer, @session_id::uuid, sqlc.narg(user_id)::integer, @form_snapshot::jsonb, @answers::jsonb);

-- Registrations
-- name: RegistrationForUser :one
SELECT * FROM activity_registrations WHERE user_id = @user_id::integer AND activity_id = CAST(@activity_id::text AS integer) LIMIT 1;

-- name: RegistrationForUpdate :one
SELECT * FROM activity_registrations WHERE id = @id::integer LIMIT 1 FOR UPDATE;

-- name: IssuedForRegistration :one
SELECT * FROM issued_certificates WHERE registration_id = @registration_id::integer LIMIT 1;

-- name: CreateActivityRegistration :one
INSERT INTO activity_registrations (user_id, activity_id, status, guest_data, questionnaire_answer, created_at, updated_at)
VALUES (sqlc.narg(user_id)::integer, CAST(@activity_id::text AS integer), @status::text, sqlc.narg(guest_data)::jsonb, @questionnaire_answer::jsonb, @created_at::timestamptz, @updated_at::timestamptz)
RETURNING id;

-- name: SetQuestionnaireAnswer :exec
UPDATE activity_registrations SET questionnaire_answer = @answer::jsonb, updated_at = @updated_at::timestamptz WHERE id = @id::integer;

-- name: ClubRegistrationFor :one
SELECT * FROM club_registrations WHERE club_id = CAST(@club_id::text AS integer) AND member_id = @member_id::integer LIMIT 1;

-- name: ClubRegistrationForUpdate :one
SELECT * FROM club_registrations WHERE club_id = CAST(@club_id::text AS integer) AND member_id = @member_id::integer LIMIT 1 FOR UPDATE;

-- name: DeleteClubRegistration :exec
DELETE FROM club_registrations WHERE id = @id::integer;

-- name: CreateClubRegistration :one
INSERT INTO club_registrations (member_id, club_id, status, additional_data, created_at, updated_at)
VALUES (@member_id::integer, @club_id::integer, @status::text, @additional_data::jsonb, @created_at::timestamptz, @updated_at::timestamptz)
RETURNING id;

-- name: CountClubRegistrations :one
SELECT count(*) AS total FROM club_registrations WHERE member_id = @member_id::integer AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ListClubRegistrations :many
SELECT * FROM club_registrations WHERE member_id = @member_id::integer AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC LIMIT sqlc.narg(row_limit)::bigint OFFSET sqlc.narg(row_offset)::bigint;

-- name: ClubsByIDs :many
SELECT * FROM clubs WHERE id = ANY(@ids::integer[]);

-- name: RolesForRegistrations :many
SELECT * FROM club_member_roles WHERE club_registration_id = ANY(@ids::integer[]) ORDER BY sort_order ASC, is_primary DESC;

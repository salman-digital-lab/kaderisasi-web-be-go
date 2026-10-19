-- notification_service.ts
-- name: NotificationClock :one
WITH publication_lock AS MATERIALIZED (SELECT pg_advisory_xact_lock(1790045423))
SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')::text AS cutoff FROM publication_lock;

-- name: ListNotifications :many
SELECT r.id, a.title, a.body, a.link_label, a.link_url,
  to_char(a.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')::text AS published_at, r.read_at
FROM announcement_recipients r JOIN announcements a ON a.id = r.announcement_id
WHERE r.public_user_id = @user_id::integer AND a.state = 'published'
  AND (NOT @unread::boolean OR r.read_at IS NULL)
  AND (sqlc.narg(before_time)::text IS NULL OR (a.published_at, r.id) < (sqlc.narg(before_time)::timestamptz, sqlc.narg(before_id)::integer))
ORDER BY a.published_at DESC, r.id DESC LIMIT 21;

-- name: CountUnreadNotifications :one
SELECT count(*) AS count FROM announcement_recipients r JOIN announcements a ON a.id = r.announcement_id
WHERE r.public_user_id = $1 AND a.state = 'published' AND r.read_at IS NULL;

-- name: ShowNotification :one
SELECT r.id, a.title, a.body, a.link_label, a.link_url,
  to_char(a.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')::text AS published_at, r.read_at
FROM announcement_recipients r JOIN announcements a ON a.id = r.announcement_id
WHERE r.public_user_id = @user_id::integer AND a.state = 'published' AND r.id = @id::integer LIMIT 1;

-- name: ReadNotification :exec
UPDATE announcement_recipients r SET read_at=COALESCE(read_at,clock_timestamp()) FROM announcements a
WHERE a.id=r.announcement_id AND a.state='published' AND r.id= @id::integer AND r.public_user_id= @user_id::integer;

-- name: ReadAllNotifications :exec
UPDATE announcement_recipients r SET read_at=clock_timestamp() FROM announcements a
WHERE a.id=r.announcement_id AND a.state='published' AND r.public_user_id= @user_id::integer AND r.read_at IS NULL AND a.published_at<= CAST(@cutoff::text AS timestamptz);

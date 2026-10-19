-- courses (course_service.ts) and owner certificate routes (certificate_service.ts)

-- name: CourseLevel :one
SELECT id, level FROM profiles WHERE user_id = @user_id::integer ORDER BY id LIMIT 1;

-- name: CourseLevelForShare :one
SELECT id, level FROM profiles WHERE user_id = @user_id::integer ORDER BY id LIMIT 1 FOR SHARE;

-- name: CountReadableCourses :one
SELECT count(*) AS total FROM courses WHERE status = 'published' AND minimum_level <= @level::integer AND title ILIKE @pattern::text;

-- name: ListReadableCourses :many
SELECT c.id, c.title, c.summary, c.minimum_level,
  (SELECT count(*) FROM course_lessons l WHERE l.course_id = c.id AND l.deleted_at IS NULL)::bigint AS total_lessons,
  (SELECT count(*) FROM course_lessons l JOIN course_lesson_progress p ON p.lesson_id = l.id
    WHERE l.course_id = c.id AND l.deleted_at IS NULL AND p.user_id = @user_id::integer AND p.completed_at IS NOT NULL)::bigint AS completed_lessons,
  COALESCE(r.id, 0)::integer AS resume_lesson_id -- 0: none (lesson ids are serial)
FROM courses c LEFT JOIN LATERAL (SELECT l.id FROM course_lessons l JOIN course_lesson_progress p ON p.lesson_id = l.id
    WHERE l.course_id = c.id AND l.deleted_at IS NULL AND p.user_id = @user_id::integer ORDER BY p.last_visited_at DESC, p.id DESC LIMIT 1) r ON true
WHERE c.status = 'published' AND c.minimum_level <= @level::integer AND c.title ILIKE @pattern::text
ORDER BY c.id DESC LIMIT @row_limit::bigint OFFSET @row_offset::bigint;

-- name: ReadableCourse :one
SELECT c.id, c.title, c.summary, c.minimum_level, c.description,
  (SELECT count(*) FROM course_lessons l WHERE l.course_id = c.id AND l.deleted_at IS NULL)::bigint AS total_lessons,
  (SELECT count(*) FROM course_lessons l JOIN course_lesson_progress p ON p.lesson_id = l.id
    WHERE l.course_id = c.id AND l.deleted_at IS NULL AND p.user_id = @user_id::integer AND p.completed_at IS NOT NULL)::bigint AS completed_lessons,
  COALESCE(r.id, 0)::integer AS resume_lesson_id -- 0: none (lesson ids are serial)
FROM courses c LEFT JOIN LATERAL (SELECT l.id FROM course_lessons l JOIN course_lesson_progress p ON p.lesson_id = l.id
    WHERE l.course_id = c.id AND l.deleted_at IS NULL AND p.user_id = @user_id::integer ORDER BY p.last_visited_at DESC, p.id DESC LIMIT 1) r ON true
WHERE c.id = @id::integer AND c.status = 'published' AND c.minimum_level <= @level::integer LIMIT 1;

-- name: CourseLessons :many
SELECT id, title, position,
  EXISTS (SELECT 1 FROM course_lesson_progress p WHERE p.lesson_id = course_lessons.id AND p.user_id = @user_id::integer AND p.completed_at IS NOT NULL) AS completed
FROM course_lessons WHERE course_id = @course_id::integer AND deleted_at IS NULL ORDER BY position, id;

-- name: CourseLesson :one
SELECT id, description, youtube_video_id FROM course_lessons
WHERE course_id = @course_id::integer AND id = @id::integer AND deleted_at IS NULL LIMIT 1;

-- name: LessonDocuments :many
SELECT id, filename, size_bytes FROM course_documents WHERE lesson_id = @lesson_id::integer AND deleted_at IS NULL ORDER BY id;

-- name: ReadableCourseForShare :one
SELECT id FROM courses WHERE id = @id::integer AND status = 'published' AND minimum_level <= @level::integer LIMIT 1 FOR SHARE;

-- name: CourseLessonForShare :one
SELECT id FROM course_lessons WHERE course_id = @course_id::integer AND id = @id::integer AND deleted_at IS NULL LIMIT 1 FOR SHARE;

-- name: RecordCourseProgress :one
INSERT INTO course_lesson_progress (user_id, lesson_id, completed_at)
VALUES (@user_id::integer, @lesson_id::integer, CASE WHEN NULLIF(@completed::text, '')::boolean THEN now() ELSE NULL END)
ON CONFLICT (user_id, lesson_id) DO UPDATE SET last_visited_at = now(),
  completed_at = CASE WHEN NULLIF(@completed::text, '')::boolean IS NULL THEN course_lesson_progress.completed_at
    WHEN NULLIF(@completed::text, '')::boolean THEN COALESCE(course_lesson_progress.completed_at, now()) ELSE NULL END
RETURNING completed_at, last_visited_at;

-- name: ReadableCourseID :one
SELECT id FROM courses WHERE id = @id::integer AND status = 'published' AND minimum_level <= @level::integer LIMIT 1;

-- name: ReadableDocument :one
SELECT d.filename, d.storage_key FROM course_documents d JOIN course_lessons lesson ON lesson.id = d.lesson_id
WHERE d.id = @id::integer AND d.lesson_id = @lesson_id::integer AND lesson.course_id = @course_id::integer
  AND lesson.deleted_at IS NULL AND d.deleted_at IS NULL LIMIT 1;

-- name: RegistrationByIDText :one
SELECT * FROM activity_registrations WHERE id = CAST(@id::text AS integer) LIMIT 1;

-- name: LatestIssuedForRegistration :one
SELECT * FROM issued_certificates WHERE registration_id = @registration_id::integer ORDER BY id DESC LIMIT 1;

-- name: CurrentIssuedForOwner :one
SELECT * FROM issued_certificates WHERE registration_id = @registration_id::integer AND user_id = @user_id::integer AND revoked_at IS NULL LIMIT 1;

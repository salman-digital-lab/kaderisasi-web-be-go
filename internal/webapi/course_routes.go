package webapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/catalog"
	"kaderisasi/web/internal/certificate"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/richtext"
	"kaderisasi/web/internal/validators"
	"kaderisasi/web/internal/vine"
)

// Phase 6: courses_controller.ts / course_service.ts and the owner routes of
// certificates_controller.ts.
func (s *Server) registerCourses() {
	course := func(h http.HandlerFunc) http.Handler { return s.RequireAuth(s.CourseAudience(h)) }
	s.mux.Handle("GET /v2/courses", course(s.courses))
	s.mux.Handle("GET /v2/courses/{id}", course(s.course))
	s.mux.Handle("GET /v2/courses/{id}/lessons/{lessonId}", course(s.courseLesson))
	s.mux.Handle("POST /v2/courses/{id}/lessons/{lessonId}/visit", course(s.visitLesson))
	s.mux.Handle("PUT /v2/courses/{id}/lessons/{lessonId}/completion", course(s.completeLesson))
	s.mux.Handle("GET /v2/courses/{id}/lessons/{lessonId}/documents/{documentId}/download", course(s.downloadDocument))

	auth := func(h http.HandlerFunc) http.Handler { return s.RequireAuth(h) }
	s.mux.Handle("GET /v2/certificates/code/{code}/access", auth(s.certificateAccess))
	s.mux.Handle("GET /v2/certificates/code/{code}/download", auth(s.certificateDownloadByCode))
	s.mux.Handle("GET /v2/certificates/registrations/{registrationId}", auth(s.certificateRegistrationState))
	s.mux.Handle("POST /v2/certificates/{id}/download", auth(s.certificateDownloadByRegistration))
}

// errCourseNotFound is CourseAccessError.
var errCourseNotFound = errors.New("COURSE_NOT_FOUND")

var courseIDPattern = regexp.MustCompile(`^[1-9]\d*$`)

// courseIdentifier validates a raw route parameter.
func courseIdentifier(raw string) (int32, error) {
	if !courseIDPattern.MatchString(raw) {
		return 0, errCourseNotFound
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id > math.MaxInt32 {
		return 0, errCourseNotFound
	}
	return int32(id), nil
}

// courseRespond is CoursesController.respond's catch.
func courseRespond(w http.ResponseWriter, err error, ok func()) {
	var invalid *vine.Error
	switch {
	case err == nil:
		ok()
	case errors.Is(err, errCourseNotFound):
		writeJSON(w, http.StatusNotFound, message{"COURSE_NOT_FOUND"})
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusUnprocessableEntity, member.KV("message", "VALIDATION_ERROR", "errors", invalid.Issues))
	default:
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
	}
}

// courseLevel is levelFor; nil means the member has no profile.
func courseLevel(ctx context.Context, q *dbgen.Queries, userID int32, share bool) (*int32, error) {
	var row dbgen.CourseLevelRow
	var err error
	if share {
		var shared dbgen.CourseLevelForShareRow
		shared, err = q.CourseLevelForShare(ctx, userID)
		row = dbgen.CourseLevelRow(shared)
	} else {
		row, err = q.CourseLevel(ctx, userID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	level := int32(0)
	if row.Level != nil {
		level = *row.Level
	}
	return &level, nil
}

func courseSummary(id int32, title, summary string, minimum int32, total, completed int64, resume int32) member.Object {
	var resumeID any
	if resume != 0 {
		resumeID = resume
	}
	return member.KV("id", id, "title", title, "summary", summary, "minimum_level", minimum,
		"total_lessons", total, "completed_lessons", completed, "resume_lesson_id", resumeID)
}

// positiveQuery is coursePagination's positive().
func positiveQuery(values []string, fallback, max float64) float64 {
	n := catalog.QueryNumber(values, math.NaN())
	if n != math.Trunc(n) || math.IsInf(n, 0) || n <= 0 {
		return fallback
	}
	return math.Min(n, max)
}

func (s *Server) courses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, perPage := positiveQuery(q["page"], 1, 1_000_000), positiveQuery(q["per_page"], 12, 100)
	search := ""
	if values := q["search"]; len(values) == 1 {
		search = sliceUnits(js.Trim(values[0]), 200)
	}
	db := dbgen.New(s.Pool)
	userID := learner(r).ID
	var data member.Object
	err := func() error {
		level, err := courseLevel(r.Context(), db, userID, false)
		if err != nil {
			return err
		}
		if level == nil {
			data = member.KV("data", []member.Object{}, "meta", member.KV("total", 0, "per_page", perPage, "current_page", page, "last_page", 1))
			return nil
		}
		pattern := "%" + search + "%"
		total, err := db.CountReadableCourses(r.Context(), dbgen.CountReadableCoursesParams{Level: *level, Pattern: pattern})
		if err != nil {
			return err
		}
		rows := []dbgen.ListReadableCoursesRow{}
		if total > 0 {
			rows, err = db.ListReadableCourses(r.Context(), dbgen.ListReadableCoursesParams{UserID: userID, Level: *level, Pattern: pattern,
				RowOffset: int64((page - 1) * perPage), RowLimit: int64(perPage)})
			if err != nil {
				return err
			}
		}
		list := []member.Object{}
		for _, c := range rows {
			list = append(list, courseSummary(c.ID, c.Title, c.Summary, c.MinimumLevel, c.TotalLessons, c.CompletedLessons, c.ResumeLessonID))
		}
		last := math.Max(1, math.Ceil(float64(total)/perPage))
		data = member.KV("data", list, "meta", member.KV("total", total, "per_page", perPage, "current_page", page, "last_page", last))
		return nil
	}()
	courseRespond(w, err, func() { writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data)) })
}

// readableCourse is readableCourse(userId, id).
func readableCourse(ctx context.Context, q *dbgen.Queries, userID, id int32) (dbgen.ReadableCourseRow, error) {
	level, err := courseLevel(ctx, q, userID, false)
	if err != nil {
		return dbgen.ReadableCourseRow{}, err
	}
	if level == nil {
		return dbgen.ReadableCourseRow{}, errCourseNotFound
	}
	row, err := q.ReadableCourse(ctx, dbgen.ReadableCourseParams{UserID: userID, ID: id, Level: *level})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errCourseNotFound
	}
	return row, err
}

func lessonSummaries(rows []dbgen.CourseLessonsRow) []member.Object {
	out := []member.Object{}
	for _, l := range rows {
		out = append(out, member.KV("id", l.ID, "title", l.Title, "position", l.Position, "completed", l.Completed))
	}
	return out
}

func (s *Server) course(w http.ResponseWriter, r *http.Request) {
	var data member.Object
	err := func() error {
		id, err := courseIdentifier(rawSegment(r, 2))
		if err != nil {
			return err
		}
		db := dbgen.New(s.Pool)
		userID := learner(r).ID
		row, err := readableCourse(r.Context(), db, userID, id)
		if err != nil {
			return err
		}
		lessons, err := db.CourseLessons(r.Context(), dbgen.CourseLessonsParams{UserID: userID, CourseID: id})
		if err != nil {
			return err
		}
		data = append(courseSummary(row.ID, row.Title, row.Summary, row.MinimumLevel, row.TotalLessons, row.CompletedLessons, row.ResumeLessonID),
			member.KV("description", richtext.Sanitize(row.Description), "lessons", lessonSummaries(lessons))...)
		return nil
	}()
	courseRespond(w, err, func() { writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data)) })
}

func (s *Server) courseLesson(w http.ResponseWriter, r *http.Request) {
	var data member.Object
	err := func() error {
		id, err := courseIdentifier(rawSegment(r, 2))
		if err != nil {
			return err
		}
		lessonID, err := courseIdentifier(rawSegment(r, 4))
		if err != nil {
			return err
		}
		db := dbgen.New(s.Pool)
		userID := learner(r).ID
		course, err := readableCourse(r.Context(), db, userID, id)
		if err != nil {
			return err
		}
		lesson, err := db.CourseLesson(r.Context(), dbgen.CourseLessonParams{CourseID: id, ID: lessonID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCourseNotFound
		}
		if err != nil {
			return err
		}
		lessons, err := db.CourseLessons(r.Context(), dbgen.CourseLessonsParams{UserID: userID, CourseID: id})
		if err != nil {
			return err
		}
		documents, err := db.LessonDocuments(r.Context(), lessonID)
		if err != nil {
			return err
		}
		var selected *dbgen.CourseLessonsRow
		for i := range lessons {
			if lessons[i].ID == lessonID {
				selected = &lessons[i]
				break
			}
		}
		if selected == nil {
			return errCourseNotFound
		}
		docs := []member.Object{}
		for _, d := range documents {
			docs = append(docs, member.KV("id", d.ID, "filename", d.Filename, "size_bytes", d.SizeBytes))
		}
		data = member.KV("course", courseSummary(course.ID, course.Title, course.Summary, course.MinimumLevel, course.TotalLessons, course.CompletedLessons, course.ResumeLessonID),
			"lesson", member.KV("id", selected.ID, "title", selected.Title, "position", selected.Position, "completed", selected.Completed,
				"description", richtext.Sanitize(lesson.Description), "youtube_video_id", lesson.YoutubeVideoID),
			"lessons", lessonSummaries(lessons), "documents", docs)
		return nil
	}()
	courseRespond(w, err, func() { writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data)) })
}

func isoUTC(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05.000Z")
}

// recordProgress is recordCourseProgress; completed is "", "true" or "false".
func (s *Server) recordProgress(ctx context.Context, userID, id, lessonID int32, completed string) (member.Object, error) {
	var out member.Object
	err := s.inTx(ctx, func(q *dbgen.Queries) error {
		level, err := courseLevel(ctx, q, userID, true)
		if err != nil {
			return err
		}
		if level == nil {
			return errCourseNotFound
		}
		if _, err := q.ReadableCourseForShare(ctx, dbgen.ReadableCourseForShareParams{ID: id, Level: *level}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errCourseNotFound
			}
			return err
		}
		if _, err := q.CourseLessonForShare(ctx, dbgen.CourseLessonForShareParams{CourseID: id, ID: lessonID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errCourseNotFound
			}
			return err
		}
		row, err := q.RecordCourseProgress(ctx, dbgen.RecordCourseProgressParams{UserID: userID, LessonID: lessonID, Completed: completed})
		if err != nil {
			return err
		}
		out = member.KV("completed", row.CompletedAt.Valid, "completed_at", isoUTC(row.CompletedAt), "last_visited_at", isoUTC(row.LastVisitedAt))
		return nil
	})
	return out, err
}

func (s *Server) lessonIDs(r *http.Request) (int32, int32, error) {
	id, err := courseIdentifier(rawSegment(r, 2))
	if err != nil {
		return 0, 0, err
	}
	lessonID, err := courseIdentifier(rawSegment(r, 4))
	return id, lessonID, err
}

func (s *Server) visitLesson(w http.ResponseWriter, r *http.Request) {
	var data member.Object
	err := func() error {
		id, lessonID, err := s.lessonIDs(r)
		if err != nil {
			return err
		}
		data, err = s.recordProgress(r.Context(), learner(r).ID, id, lessonID, "")
		return err
	}()
	courseRespond(w, err, func() { writeJSON(w, http.StatusOK, member.KV("message", "UPDATE_DATA_SUCCESS", "data", data)) })
}

func (s *Server) completeLesson(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	var data member.Object
	err := func() error {
		payload, err := vine.ValidateObject(validators.CourseCompletion, body)
		if err != nil {
			return err
		}
		completed, _ := payload.Get("completed")
		id, lessonID, err := s.lessonIDs(r)
		if err != nil {
			return err
		}
		data, err = s.recordProgress(r.Context(), learner(r).ID, id, lessonID, strconv.FormatBool(completed.Bool))
		return err
	}()
	courseRespond(w, err, func() { writeJSON(w, http.StatusOK, member.KV("message", "UPDATE_DATA_SUCCESS", "data", data)) })
}

// encodeURIComponent is JavaScript's encodeURIComponent.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(strconv.FormatUint(uint64(c)>>4, 16)+strconv.FormatUint(uint64(c)&15, 16)))
		}
	}
	return b.String()
}

func (s *Server) downloadDocument(w http.ResponseWriter, r *http.Request) {
	var file []byte
	var filename string
	err := func() error {
		id, lessonID, err := s.lessonIDs(r)
		if err != nil {
			return err
		}
		documentID, err := courseIdentifier(rawSegment(r, 6))
		if err != nil {
			return err
		}
		db := dbgen.New(s.Pool)
		level, err := courseLevel(r.Context(), db, learner(r).ID, false)
		if err != nil {
			return err
		}
		if level == nil {
			return errCourseNotFound
		}
		if _, err := db.ReadableCourseID(r.Context(), dbgen.ReadableCourseIDParams{ID: id, Level: *level}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errCourseNotFound
			}
			return err
		}
		doc, err := db.ReadableDocument(r.Context(), dbgen.ReadableDocumentParams{ID: documentID, LessonID: lessonID, CourseID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCourseNotFound
		}
		if err != nil {
			return err
		}
		if s.Config.DriveBucket == "" {
			writeJSON(w, http.StatusServiceUnavailable, message{"COURSE_STORAGE_UNAVAILABLE"})
			return errHandled
		}
		file, _, err = s.Storage.Get(r.Context(), doc.StorageKey)
		filename = doc.Filename
		return err
	}()
	if errors.Is(err, errHandled) {
		return
	}
	courseRespond(w, err, func() {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `attachment; filename="materi.pdf"; filename*=UTF-8''`+encodeURIComponent(filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(file)
	})
}

// --- certificates ------------------------------------------------------------

// certificateError is respondWithCertificateError.
func certificateError(w http.ResponseWriter, kind string) {
	switch kind {
	case "FORBIDDEN":
		writeJSON(w, http.StatusForbidden, message{kind})
	case "CERTIFICATE_NOT_ISSUED":
		writeJSON(w, http.StatusConflict, message{kind})
	case "CERTIFICATE_REVOKED":
		writeJSON(w, http.StatusGone, message{kind})
	case "REGISTRATION_NOT_FOUND":
		writeJSON(w, http.StatusNotFound, message{kind})
	default:
		writeJSON(w, http.StatusNotFound, message{"CERTIFICATE_NOT_FOUND"})
	}
}

var registrationIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// parsePositiveID returns the id as text, or "" when it is not a safe
// positive integer.
func parsePositiveID(raw string) string {
	if !registrationIDPattern.MatchString(raw) {
		return ""
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err != nil || n > 1<<53-1 {
		return ""
	}
	return raw
}

// ownerRegistration loads a registration for its owner; kind is a
// certificate error when it cannot be read.
func ownerRegistration(ctx context.Context, q *dbgen.Queries, id string, userID int32) (dbgen.ActivityRegistration, string, error) {
	reg, err := q.RegistrationByIDText(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return reg, "REGISTRATION_NOT_FOUND", nil
	}
	if err != nil {
		return reg, "", err
	}
	if reg.UserID == nil || *reg.UserID != userID {
		return reg, "FORBIDDEN", nil
	}
	return reg, "", nil
}

func latestIssued(ctx context.Context, q *dbgen.Queries, registrationID int32) (*dbgen.IssuedCertificate, error) {
	issued, err := q.LatestIssuedForRegistration(ctx, registrationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &issued, nil
}

func (s *Server) certificateRegistrationState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	id := parsePositiveID(rawSegment(r, 3))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_REGISTRATION_ID"})
		return
	}
	q := dbgen.New(s.Pool)
	reg, kind, err := ownerRegistration(r.Context(), q, id, learner(r).ID)
	if err == nil && kind == "" {
		var issued *dbgen.IssuedCertificate
		if issued, err = latestIssued(r.Context(), q, reg.ID); err == nil {
			found := map[int32]dbgen.IssuedCertificate{}
			if issued != nil {
				found[reg.ID] = *issued
			}
			var state ownerFields
			if state, err = ownerState(reg, found, s.Config.Location); err == nil {
				writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.KV("state", state.Get("state"),
					"registration_id", reg.ID, "certificate_code", state.Get("certificate_code"), "issued_at", state.Get("issued_at"),
					"revoked_at", state.Get("revoked_at"))))
				return
			}
		}
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
		return
	}
	certificateError(w, kind)
}

func (s *Server) certificateDownloadByRegistration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	id := parsePositiveID(rawSegment(r, 2))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_REGISTRATION_ID"})
		return
	}
	data, kind, err := func() (member.Object, string, error) {
		q := dbgen.New(s.Pool)
		reg, kind, err := ownerRegistration(r.Context(), q, id, learner(r).ID)
		if err != nil || kind != "" {
			return nil, kind, err
		}
		issued, err := latestIssued(r.Context(), q, reg.ID)
		if err != nil {
			return nil, "", err
		}
		approved := false
		if issued != nil {
			if approved, err = certificate.HasApproval(*issued); err != nil {
				return nil, "", err
			}
		}
		if !approved {
			return nil, "CERTIFICATE_NOT_ISSUED", nil
		}
		if issued.RevokedAt.Valid {
			return nil, "CERTIFICATE_REVOKED", nil
		}
		data, err := certificate.Owner(*issued, s.Config.Location)
		return data, "", err
	}()
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
	case kind != "":
		certificateError(w, kind)
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "CERTIFICATE_DOWNLOAD_READY", "data", data))
	}
}

func (s *Server) certificateDownloadByCode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	issued, err := s.issuedByCode(r)
	var data member.Object
	kind := ""
	if err == nil {
		switch {
		case issued == nil:
			kind = "CERTIFICATE_NOT_FOUND"
		case issued.UserID == nil || *issued.UserID != learner(r).ID:
			kind = "FORBIDDEN"
		case issued.RevokedAt.Valid:
			kind = "CERTIFICATE_REVOKED"
		default:
			data, err = certificate.Owner(*issued, s.Config.Location)
		}
	}
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
	case kind != "":
		certificateError(w, kind)
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "CERTIFICATE_DOWNLOAD_READY", "data", data))
	}
}

// certificateAccess has no catch in web-be: failures are uncaught.
func (s *Server) certificateAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	userID := learner(r).ID
	issued, err := s.issuedByCode(r)
	if err != nil {
		uncaught(w, err)
		return
	}
	if issued == nil {
		certificateError(w, "CERTIFICATE_NOT_FOUND")
		return
	}
	reason := "owner"
	if issued.UserID == nil || *issued.UserID != userID {
		reason = "not_owner"
	} else if issued.RevokedAt.Valid {
		reason = "revoked"
	}
	data := member.KV("can_download", reason == "owner", "reason", reason)
	if reason == "revoked" {
		current, err := dbgen.New(s.Pool).CurrentIssuedForOwner(r.Context(), dbgen.CurrentIssuedForOwnerParams{RegistrationID: issued.RegistrationID, UserID: userID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			uncaught(w, err)
			return
		}
		if err == nil {
			approved, err := certificate.HasApproval(current)
			if err != nil {
				uncaught(w, err)
				return
			}
			if approved {
				data = append(data, member.KV("current_code", current.CertificateCode)...)
			}
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
}

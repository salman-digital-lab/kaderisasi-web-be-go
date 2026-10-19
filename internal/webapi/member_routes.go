package webapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/catalog"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/storage"
	"kaderisasi/web/internal/validators"
	"kaderisasi/web/internal/vine"
)

// Phase 3: profile_histories_controller.ts, notifications_controller.ts,
// leaderboards_controller.ts (store, update, myAchievements),
// ruang_curhats_controller.ts and members_controller.ts.
func (s *Server) registerMember() {
	auth := func(h http.HandlerFunc) http.Handler { return s.RequireAuth(h) }
	s.mux.Handle("GET /v2/profiles/history/{section}", auth(s.profileHistory))
	s.mux.Handle("GET /v2/achievements/{id}", auth(s.achievement))
	s.mux.Handle("GET /v2/achievements", auth(s.myAchievements))
	s.mux.Handle("POST /v2/achievements", auth(s.storeAchievement))
	s.mux.Handle("PUT /v2/achievements/{id}", auth(s.updateAchievement))
	s.mux.Handle("GET /v2/notifications", auth(s.notifications))
	s.mux.Handle("GET /v2/notifications/unread-count", auth(s.unreadNotifications))
	s.mux.Handle("PUT /v2/notifications/read-all", auth(s.readAllNotifications))
	s.mux.Handle("GET /v2/notifications/{id}", auth(s.showNotification))
	s.mux.Handle("PUT /v2/notifications/{id}/read", auth(s.readNotification))
	s.mux.Handle("POST /v2/ruang-curhat", auth(s.storeRuangCurhat))
	s.mux.Handle("GET /v2/ruang-curhat", auth(s.ruangCurhatHistory))
	s.mux.HandleFunc("POST /v2/members/submit", s.submitMember)
}

// --- profile history -----------------------------------------------------

type historyQuery struct {
	page, perPage int64
	search        string
	status        string
	nameOnly      bool
}

func parseHistoryQuery(q js.Value) historyQuery {
	positive := func(key string, fallback, max int64) int64 {
		v, ok := q.Get(key)
		if !ok {
			return fallback
		}
		n := vine.ToNumber(v)
		if math.IsNaN(n) || n != math.Trunc(n) || math.Abs(n) > 1<<53-1 || n <= 0 {
			return fallback
		}
		return int64(math.Min(n, float64(max)))
	}
	out := historyQuery{page: positive("page", 1, 1_000_000), perPage: positive("per_page", 6, 100)}
	if scope, ok := q.Get("search_scope"); ok && scope.Kind == 's' && scope.String == "name" {
		out.nameOnly = true
	}
	if v, ok := q.Get("search"); ok && v.Kind == 's' {
		out.search = strings.ToLower(sliceUnits(js.Trim(v.String), 200))
	}
	if v, ok := q.Get("status"); ok && v.Kind == 's' && v.String != "all" {
		out.status = sliceUnits(v.String, 100)
	}
	return out
}

func sliceUnits(s string, n int) string {
	units := 0
	for i, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if units+w > n {
			return s[:i]
		}
		units += w
	}
	return s
}

func (s *Server) profileHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := parseHistoryQuery(orderedQuery(r.URL.RawQuery, false))
	db := dbgen.New(s.Pool)
	user := learner(r).ID
	offset := (q.page - 1) * q.perPage
	var items, summary []byte
	var total int64
	var err error
	switch rawSegment(r, 3) {
	case "activities":
		if q.nameOnly {
			var row dbgen.ActivityHistoryByNameRow
			row, err = db.ActivityHistoryByName(r.Context(), dbgen.ActivityHistoryByNameParams{Column1: user, Column2: q.search, Column3: q.status, Column4: q.perPage, Column5: offset})
			items, total, summary = row.Items, row.Total, row.Summary
		} else {
			var row dbgen.ActivityHistoryRow
			row, err = db.ActivityHistory(r.Context(), dbgen.ActivityHistoryParams{Column1: user, Column2: q.search, Column3: q.status, Column4: q.perPage, Column5: offset})
			items, total, summary = row.Items, row.Total, row.Summary
		}
	case "consultations":
		var row dbgen.ConsultationHistoryRow
		row, err = db.ConsultationHistory(r.Context(), dbgen.ConsultationHistoryParams{Column1: user, Column2: q.search, Column3: q.status, Column4: q.perPage, Column5: offset})
		items, total, summary = row.Items, row.Total, row.Summary
	case "achievements":
		var row dbgen.AchievementHistoryRow
		row, err = db.AchievementHistory(r.Context(), dbgen.AchievementHistoryParams{Column1: user, Column2: q.search, Column3: q.status, Column4: q.perPage, Column5: offset})
		items, total, summary = row.Items, row.Total, row.Summary
	default:
		writeJSON(w, http.StatusNotFound, message{"NOT_FOUND"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, message{err.Error()})
		return
	}
	last := int64(math.Max(1, math.Ceil(float64(total)/float64(q.perPage))))
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.KV(
		"items", member.JSONB(items), "meta", member.KV("total", total, "current_page", q.page, "per_page", q.perPage, "last_page", last),
		"summary", member.JSONB(summary))))
}

// --- achievements ----------------------------------------------------------

var positiveID = regexp.MustCompile(`^[1-9]\d*$`)

func (s *Server) achievement(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id := rawSegment(r, 2)
	if n, err := strconv.ParseFloat(id, 64); !positiveID.MatchString(id) || err != nil || n > 2147483647 {
		writeJSON(w, http.StatusNotFound, message{"ACHIEVEMENT_NOT_FOUND"})
		return
	}
	row, err := dbgen.New(s.Pool).AchievementForUser(r.Context(), dbgen.AchievementForUserParams{ID: id, UserID: learner(r).ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, message{"ACHIEVEMENT_NOT_FOUND"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, message{err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.Achievement(row, s.Config.Location)))
}

func (s *Server) myAchievements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := catalog.Page{Page: catalog.QueryNumber(q["page"], 1), PerPage: catalog.QueryNumber(q["per_page"], 10)}
	db := dbgen.New(s.Pool)
	user := learner(r).ID
	userRef := user
	total, err := db.AchievementsForUserCount(r.Context(), &userRef)
	if err != nil {
		generalError(w, knexError(`select count(*) as "total" from "achievements" where "user_id" = $1`, err))
		return
	}
	out := []member.Object{}
	if total > 0 {
		limit, offset, err := page.LimitOffset()
		var rows []dbgen.Achievement
		if err == nil {
			rows, err = db.AchievementsForUser(r.Context(), dbgen.AchievementsForUserParams{UserID: user, RowLimit: limit, RowOffset: offset})
			p := placeholders{n: 1}
			err = knexError(`select * from "achievements" where "user_id" = $1 order by "achievement_date" desc`+limitOffset(&p, limit, offset), err)
		}
		if err != nil {
			generalError(w, err)
			return
		}
		for _, a := range rows {
			out = append(out, member.Achievement(a, s.Config.Location))
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", catalog.Paginated(total, page, out)))
}

// badRequestValidation is the achievements controllers' 400 envelope.
func badRequestValidation(w http.ResponseWriter, err *vine.Error) {
	writeJSON(w, http.StatusBadRequest, struct {
		Message string       `json:"message"`
		Error   []vine.Issue `json:"error"`
	}{err.Issues[0].Message, err.Issues})
}

// fromISO is DateTime.fromISO(value).toSQLDate(); ok is false when invalid.
func (s *Server) fromISO(value string) (pgtype.Date, bool) {
	d := catalog.MonthDate([]string{value}, s.now(), s.Config.Location)
	if d == nil || value == "" {
		return pgtype.Date{}, false
	}
	return pgtype.Date{Time: time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), Valid: true}, true
}

func proofKey(userID int32, upload *vine.Upload) string {
	ext := "undefined"
	if upload.HasExtname {
		ext = upload.Extname
	}
	return fmt.Sprintf("achievements/%d_%d.%s", userID, time.Now().UnixMilli(), ext)
}

func (s *Server) storeAchievement(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	user := learner(r)
	payload, err := vine.ValidateObject(validators.Achievement, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		badRequestValidation(w, invalid)
		return
	}
	str := func(k string) string { v, _ := payload.Get(k); return v.String }
	proof, _ := payload.Get("proof")
	upload := proof.File.(*vine.Upload)
	key := proofKey(user.ID, upload)
	contentType := upload.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.Storage.Put(r.Context(), key, upload.Data, storage.Object{ContentType: contentType, PublicRead: true}); err != nil {
		failure(w, err)
		return
	}
	date, valid := s.fromISO(str("achievement_date"))
	if !valid {
		failure(w, errors.New(`Invalid value for "Achievement.achievementDate"`))
		return
	}
	kind, _ := payload.Get("type")
	created := member.Now()
	typ := int32(kind.Number)
	id, err := dbgen.New(s.Pool).CreateAchievement(r.Context(), dbgen.CreateAchievementParams{UserID: &user.ID, Name: ptr(str("name")),
		Description: ptr(str("description")), AchievementDate: date, Type: &typ, Proof: &key, CreatedAt: member.Timestamp(created), UpdatedAt: member.Timestamp(created)})
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, member.KV("message", "CREATE_DATA_SUCCESS", "data", member.KV("user_id", user.ID, "name", str("name"),
		"description", str("description"), "achievement_date", date.Time.Format("2006-01-02"), "type", kind.Number, "score", 0, "proof", key,
		"status", 0, "created_at", member.ISO(created, s.Config.Location), "updated_at", member.ISO(created, s.Config.Location), "id", id)))
}

func ptr[T any](v T) *T { return &v }

func (s *Server) updateAchievement(w http.ResponseWriter, r *http.Request) {
	user := learner(r)
	id := rawSegment(r, 2)
	db := dbgen.New(s.Pool)
	row, err := db.AchievementForUser(r.Context(), dbgen.AchievementForUserParams{ID: id, UserID: user.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, message{"ACHIEVEMENT_NOT_FOUND"})
		return
	}
	if err != nil {
		failure(w, knexError(`select * from "achievements" where "id" = $1 and "user_id" = $2 limit $3`, err))
		return
	}
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.UpdateAchievement, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		badRequestValidation(w, invalid)
		return
	}
	current := member.Achievement(row, s.Config.Location)
	changes := member.Object{}
	set := func(k string, v any) { changes = changes.Set(k, v) }
	set("status", 0)
	if v, ok := payload.Get("name"); ok && v.Truthy() {
		set("name", v.String)
	}
	if v, ok := payload.Get("description"); ok && v.Truthy() {
		set("description", v.String)
	}
	if v, ok := payload.Get("achievement_date"); ok && v.Truthy() {
		date, valid := s.fromISO(v.String)
		if !valid {
			failure(w, errors.New(`Invalid value for "Achievement.achievementDate"`))
			return
		}
		set("achievement_date", date.Time.Format("2006-01-02"))
	}
	if v, ok := payload.Get("type"); ok && v.Truthy() {
		set("type", v.Number)
	}
	if v, ok := payload.Get("proof"); ok && v.Kind == 'F' {
		upload := v.File.(*vine.Upload)
		if row.Proof != nil && *row.Proof != "" {
			if err := s.Storage.Delete(r.Context(), *row.Proof); err != nil {
				s.logger().Warn("Error deleting old file", "error", err.Error())
			}
		}
		key := proofKey(user.ID, upload)
		contentType := "application/octet-stream"
		if upload.Type != "" && upload.Subtype != "" {
			contentType = upload.Type + "/" + upload.Subtype
		}
		if err := s.Storage.Put(r.Context(), key, upload.Data, storage.Object{ContentType: contentType, PublicRead: true}); err != nil {
			failure(w, err)
			return
		}
		set("proof", key)
	}
	updated, err := member.SaveAchievement(r.Context(), s.Pool, row, current, changes, s.Config.Location)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "UPDATE_DATA_SUCCESS", "data", updated))
}

// --- ruang curhat ------------------------------------------------------------

func (s *Server) storeRuangCurhat(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.StoreRuangCurhat, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		validationErrors(w, invalid) // validated before the try block
		return
	}
	user := learner(r).ID
	text := func(k string) string { v, _ := payload.Get(k); return v.String }
	var owner *string
	if v, ok := payload.Get("owner_name"); ok {
		owner = &v.String
	}
	ownership, _ := payload.Get("problem_ownership")
	created := member.Now()
	id, err := dbgen.New(s.Pool).CreateRuangCurhat(r.Context(), dbgen.CreateRuangCurhatParams{ProblemOwnership: js.ToString(ownership.Number), OwnerName: owner,
		ProblemCategory: text("problem_category"), ProblemDescription: text("problem_description"), HandlingTechnic: text("handling_technic"),
		CounselorGender: text("counselor_gender"), UserID: user, CreatedAt: member.Timestamp(created), UpdatedAt: member.Timestamp(created)})
	if err != nil {
		columns := map[string]bool{"problem_ownership": true, "problem_category": true, "problem_description": true, "handling_technic": true,
			"counselor_gender": true, "status": true, "user_id": true, "created_at": true, "updated_at": true}
		if owner != nil {
			columns["owner_name"] = true
		}
		failure(w, member.InsertError("ruang_curhats", columns, err))
		return
	}
	data := member.Object{}
	for _, k := range payload.Keys {
		v := payload.Fields[k]
		data = append(data, member.Pair{Key: k, Value: v})
	}
	data = append(data, member.KV("status", 0, "user_id", user, "created_at", member.ISO(created, s.Config.Location),
		"updated_at", member.ISO(created, s.Config.Location), "id", id)...)
	writeJSON(w, http.StatusOK, member.KV("messages", "CREATE_DATA_SUCCESS", "data", data))
}

func (s *Server) ruangCurhatHistory(w http.ResponseWriter, r *http.Request) {
	db := dbgen.New(s.Pool)
	user := learner(r).ID
	rows, err := db.RuangCurhatsForUser(r.Context(), &user)
	if err != nil {
		failure(w, err)
		return
	}
	ids := []int32{}
	for _, row := range rows {
		if row.CounselorID != nil {
			ids = append(ids, *row.CounselorID)
		}
	}
	admins := map[int32]dbgen.AdminUser{}
	if len(ids) > 0 {
		list, err := db.AdminUsersByIDs(r.Context(), ids)
		if err != nil {
			failure(w, err)
			return
		}
		for _, a := range list {
			admins[a.ID] = a
		}
	}
	out := []member.Object{}
	for _, row := range rows {
		var admin any
		if row.CounselorID != nil {
			if a, ok := admins[*row.CounselorID]; ok {
				admin = member.AdminUser(a, s.Config.Location)
			}
		}
		out = append(out, append(member.RuangCurhat(row, s.Config.Location), member.KV("adminUser", admin)...))
	}
	writeJSON(w, http.StatusOK, member.KV("messages", "GET_DATA_SUCCESS", "data", out))
}

// --- members/submit ------------------------------------------------------------

func (s *Server) submitMember(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.SelfSubmit, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		writeJSON(w, http.StatusInternalServerError, struct {
			Message string       `json:"message"`
			Error   []vine.Issue `json:"error"`
		}{invalid.Issues[0].Message, invalid.Issues})
		return
	}
	if err := member.SubmitMember(r.Context(), s.Pool, payload); err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, message{"SUBMIT_SUCCESS"})
}

// --- notifications -----------------------------------------------------------

type statusError struct {
	status  int
	message string
}

func (e *statusError) Error() string { return e.message }

// notification runs notifications_controller.ts handle(): active members
// only, Vine errors as 422 VALIDATION_ERROR, 404/422 failures by message.
func (s *Server) notification(w http.ResponseWriter, r *http.Request, action func(user dbgen.PublicUser) (any, error)) {
	w.Header().Set("Cache-Control", "private, no-store")
	user := learner(r)
	if user.AccountStatus != "active" {
		writeJSON(w, http.StatusUnauthorized, message{"UNAUTHORIZED"})
		return
	}
	data, err := action(user)
	var invalid *vine.Error
	var failed *statusError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusUnprocessableEntity, struct {
			Message string       `json:"message"`
			Errors  []vine.Issue `json:"errors"`
		}{"VALIDATION_ERROR", invalid.Issues})
	case errors.As(err, &failed):
		writeJSON(w, failed.status, message{failed.message})
	case err != nil:
		s.logger().Error("notification request failed", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, message{err.Error()})
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
	}
}

var validTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?Z$`)

func timestampValid(s string) (time.Time, bool) {
	if !validTimestamp.MatchString(s) {
		return time.Time{}, false
	}
	return js.ParseDateString(s)
}

func notificationJSON(id int32, title, body string, label, link *string, published string, read pgtype.Timestamptz) member.Object {
	var readAt any
	if read.Valid {
		readAt = read.Time.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	return member.KV("id", id, "title", title, "body", body, "link_label", label, "link_url", link, "published_at", published, "read_at", readAt)
}

// decodeCursor is Buffer.from(cursor, 'base64url') (lenient) + JSON.parse.
func decodeCursor(cursor string) (int32, string, bool) {
	clean := strings.Builder{}
	for _, c := range cursor {
		switch {
		case c == '=':
			goto done
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			clean.WriteRune(c)
		case c == '-' || c == '+':
			clean.WriteByte('-')
		case c == '_' || c == '/':
			clean.WriteByte('_')
		}
	}
done:
	text := clean.String()
	if len(text)%4 == 1 {
		text = text[:len(text)-1]
	}
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return 0, "", false
	}
	var value struct {
		ID   json.RawMessage `json:"id"`
		Time json.RawMessage `json:"time"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return 0, "", false
	}
	id, err := js.Parse(value.ID)
	if err != nil || id.Kind != 'f' || id.Number != math.Trunc(id.Number) || id.Number <= 0 || id.Number > 2147483647 {
		return 0, "", false
	}
	var at string
	if json.Unmarshal(value.Time, &at) != nil {
		return 0, "", false
	}
	if _, ok := timestampValid(at); !ok {
		return 0, "", false
	}
	return int32(id.Number), at, true
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	s.notification(w, r, func(user dbgen.PublicUser) (any, error) {
		body, err := all(r)
		if err != nil {
			return nil, err
		}
		input, err := vine.ValidateObject(validators.NotificationList, body)
		if err != nil {
			return nil, err
		}
		params := dbgen.ListNotificationsParams{UserID: user.ID}
		if cursor, ok := input.Get("cursor"); ok && cursor.String != "" {
			id, at, ok := decodeCursor(cursor.String)
			if !ok {
				return nil, &statusError{422, "INVALID_CURSOR"}
			}
			params.BeforeTime, params.BeforeID = &at, &id
		}
		if unread, ok := input.Get("unread"); ok && unread.String == "true" {
			params.Unread = true
		}
		db := dbgen.New(s.Pool)
		cutoff, err := db.NotificationClock(r.Context())
		if err != nil {
			return nil, err
		}
		rows, err := db.ListNotifications(r.Context(), params)
		if err != nil {
			return nil, err
		}
		more := len(rows) > 20
		if more {
			rows = rows[:20]
		}
		items := []member.Object{}
		for _, n := range rows {
			items = append(items, notificationJSON(n.ID, n.Title, n.Body, n.LinkLabel, n.LinkUrl, n.PublishedAt, n.ReadAt))
		}
		next := ""
		if more && len(rows) > 0 {
			last := rows[len(rows)-1]
			raw, _ := json.Marshal(struct {
				ID   int32  `json:"id"`
				Time string `json:"time"`
			}{last.ID, last.PublishedAt})
			next = base64.RawURLEncoding.EncodeToString(raw)
		}
		return member.KV("items", items, "next_cursor", next, "cutoff", cutoff), nil
	})
}

func (s *Server) unreadNotifications(w http.ResponseWriter, r *http.Request) {
	s.notification(w, r, func(user dbgen.PublicUser) (any, error) {
		id := user.ID
		count, err := dbgen.New(s.Pool).CountUnreadNotifications(r.Context(), &id)
		if err != nil {
			return nil, err
		}
		return member.KV("unread", count), nil
	})
}

func notificationID(r *http.Request) (int32, error) {
	n := js.Number(rawSegment(r, 2))
	if math.IsNaN(n) || n != math.Trunc(n) || n <= 0 || n > 2147483647 {
		return 0, &statusError{404, "NOTIFICATION_NOT_FOUND"}
	}
	return int32(n), nil
}

func (s *Server) showOne(r *http.Request, user, id int32) (any, error) {
	n, err := dbgen.New(s.Pool).ShowNotification(r.Context(), dbgen.ShowNotificationParams{UserID: user, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &statusError{404, "NOTIFICATION_NOT_FOUND"}
	}
	if err != nil {
		return nil, err
	}
	return notificationJSON(n.ID, n.Title, n.Body, n.LinkLabel, n.LinkUrl, n.PublishedAt, n.ReadAt), nil
}

func (s *Server) showNotification(w http.ResponseWriter, r *http.Request) {
	s.notification(w, r, func(user dbgen.PublicUser) (any, error) {
		id, err := notificationID(r)
		if err != nil {
			return nil, err
		}
		return s.showOne(r, user.ID, id)
	})
}

func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	s.notification(w, r, func(user dbgen.PublicUser) (any, error) {
		id, err := notificationID(r)
		if err != nil {
			return nil, err
		}
		if err := dbgen.New(s.Pool).ReadNotification(r.Context(), dbgen.ReadNotificationParams{ID: id, UserID: user.ID}); err != nil {
			return nil, err
		}
		return s.showOne(r, user.ID, id)
	})
}

func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	s.notification(w, r, func(user dbgen.PublicUser) (any, error) {
		body, err := all(r)
		if err != nil {
			return nil, err
		}
		input, err := vine.ValidateObject(validators.NotificationReadAll, body)
		if err != nil {
			return nil, err
		}
		cutoff, _ := input.Get("cutoff")
		db := dbgen.New(s.Pool)
		now, err := db.NotificationClock(r.Context())
		if err != nil {
			return nil, err
		}
		at, ok := timestampValid(cutoff.String)
		clock, _ := js.ParseDateString(now)
		if !ok || at.Truncate(time.Millisecond).After(clock.Truncate(time.Millisecond)) {
			return nil, &statusError{422, "INVALID_CUTOFF"}
		}
		if err := db.ReadAllNotifications(r.Context(), dbgen.ReadAllNotificationsParams{UserID: user.ID, Cutoff: cutoff.String}); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

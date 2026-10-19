package webapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/catalog"
	"kaderisasi/web/internal/certificate"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/member"
)

// Phase 2: public reads from activities_controller.ts (index, categories,
// show), clubs_controller.ts, leaderboards_controller.ts (monthly) and
// certificates_controller.ts (showByCode, verify).
func (s *Server) registerCatalog() {
	s.mux.HandleFunc("GET /v2/activities", s.activities)
	s.mux.HandleFunc("GET /v2/activities/categories", s.activityCategories)
	s.mux.HandleFunc("GET /v2/activities/{slug}", s.activity)
	s.mux.HandleFunc("GET /v2/clubs", s.clubs)
	s.mux.HandleFunc("GET /v2/clubs/{id}", s.club)
	s.mux.HandleFunc("GET /v2/achievements/monthly", s.monthlyLeaderboard)
	s.mux.Handle("GET /v2/certificates/code/{code}", s.throttle.wrap(http.HandlerFunc(s.certificateByCode)))
	s.mux.Handle("GET /v2/certificates/verify/{code}", s.throttle.wrap(http.HandlerFunc(s.verifyCertificate)))
}

// generalError is these controllers' catch block.
func generalError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}{"GENERAL_ERROR", err.Error()})
}

func textParam(values []string) *string {
	if len(values) == 0 || values[0] == "" {
		return nil // request.qs().x is falsy
	}
	return &values[0]
}

func likePattern(search string) string {
	if search == "" {
		return "%%"
	}
	return "%" + search + "%"
}

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := catalog.Page{Page: catalog.QueryNumber(q["page"], 1), PerPage: catalog.QueryNumber(q["per_page"], 10)}
	db := dbgen.New(s.Pool)
	filter := dbgen.CountPublishedActivitiesParams{Category: textParam(q["category"]), ClubID: textParam(q["club_id"]), Pattern: likePattern(strings.Join(q["search"], ","))}
	where := activityWhere(filter.Category != nil, filter.ClubID != nil)
	total, err := db.CountPublishedActivities(r.Context(), filter)
	if err != nil {
		generalError(w, knexError(`select count(*) as "total" from "activities" where `+where.sql, err))
		return
	}
	rows := []dbgen.Activity{}
	if total > 0 {
		limit, offset, err := page.LimitOffset()
		if err == nil {
			rows, err = db.ListPublishedActivities(r.Context(), dbgen.ListPublishedActivitiesParams{Category: filter.Category, ClubID: filter.ClubID,
				Pattern: filter.Pattern, RowLimit: limit, RowOffset: offset})
			err = knexError(`select * from "activities" where `+where.sql+` order by "id" desc`+limitOffset(&where.p, limit, offset), err)
		}
		if err != nil {
			generalError(w, err)
			return
		}
	}
	// The controller answers with "messages" (sic).
	writeJSON(w, http.StatusOK, member.KV("messages", "GET_DATA_SUCCESS", "data", catalog.Paginated(total, page, catalog.Activities(rows, s.Config.Location))))
}

type whereClause struct {
	sql string
	p   placeholders
}

// activityWhere is the knex where clause of ActivitiesController.index.
func activityWhere(category, club bool) whereClause {
	var w whereClause
	parts := []string{}
	if category {
		parts = append(parts, `"activity_category" = `+w.p.next())
	}
	if club {
		parts = append(parts, `"club_id" = `+w.p.next())
	}
	parts = append(parts, `"name" ilike `+w.p.next(), `"is_published" = `+w.p.next())
	w.sql = strings.Join(parts, " and ")
	return w
}

// limitOffset is knex's pagination SQL; a zero offset is omitted.
func limitOffset(p *placeholders, limit, offset *int64) string {
	sql := ""
	if limit != nil {
		sql += " limit " + p.next()
	}
	if offset != nil && *offset != 0 {
		sql += " offset " + p.next()
	}
	return sql
}

func (s *Server) activityCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := dbgen.New(s.Pool).PublishedActivityCategories(r.Context())
	if err != nil {
		generalError(w, knexError(`select distinct "activity_category", "activity_category" from "activities" where "is_published" = $1`, err))
		return
	}
	ids := make([]*int32, len(rows))
	for i, row := range rows {
		ids[i] = row.ActivityCategory
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", ids))
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	db := dbgen.New(s.Pool)
	a, err := db.PublishedActivityBySlug(r.Context(), rawSegment(r, 2))
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, message{"ACTIVITY_NOT_FOUND"})
		return
	}
	if err != nil {
		generalError(w, knexError(`select * from "activities" where "slug" = $1 and "is_published" = $2 limit $3`, err))
		return
	}
	var club any
	if a.ClubID != nil {
		c, err := db.ClubByID(r.Context(), strconv.Itoa(int(*a.ClubID)))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			generalError(w, err)
			return
		}
		if err == nil {
			club = catalog.Club(c, s.Config.Location)
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", append(catalog.Activity(a, s.Config.Location), member.KV("club", club)...)))
}

func (s *Server) clubs(w http.ResponseWriter, r *http.Request) {
	q := catalog.NormalizeClubQuery(r.URL.Query())
	db := dbgen.New(s.Pool)
	pattern := likePattern(q.Search)
	total, err := db.CountVisibleClubs(r.Context(), dbgen.CountVisibleClubsParams{Pattern: pattern, ClubType: q.ClubType})
	if err != nil {
		generalError(w, err)
		return
	}
	out := []member.Object{}
	if total > 0 {
		limit, offset, err := q.Page.LimitOffset()
		var rows []dbgen.Club
		if err == nil {
			rows, err = db.ListVisibleClubs(r.Context(), dbgen.ListVisibleClubsParams{Pattern: pattern, ClubType: q.ClubType, RowLimit: *limit, RowOffset: *offset})
		}
		if err != nil {
			generalError(w, err)
			return
		}
		now := s.now()
		for _, c := range rows {
			out = append(out, catalog.PublicClub(c, s.Config.Location, now))
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", catalog.Paginated(total, q.Page, out)))
}

func (s *Server) club(w http.ResponseWriter, r *http.Request) {
	db := dbgen.New(s.Pool)
	c, err := db.ClubByID(r.Context(), rawSegment(r, 2))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		generalError(w, knexError(`select * from "clubs" where "id" = $1 limit $2`, err))
		return
	}
	if err != nil || c.IsShow == nil || !*c.IsShow {
		writeJSON(w, http.StatusNotFound, message{"CLUB_NOT_FOUND"})
		return
	}
	data := catalog.PublicClub(c, s.Config.Location, s.now())
	activities, err := db.PublishedActivitiesForClub(r.Context(), c.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	roles, err := db.ClubLeadership(r.Context(), c.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	leadership, err := catalog.Leadership(roles, s.Config.Location)
	if err != nil {
		generalError(w, err)
		return
	}
	data = append(data, member.KV("activities", catalog.Activities(activities, s.Config.Location), "leadership", leadership)...)
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
}

func (s *Server) monthlyLeaderboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := catalog.Page{Page: catalog.QueryNumber(q["page"], 1), PerPage: catalog.QueryNumber(q["per_page"], 10)}
	month := pgtype.Date{}
	if d := catalog.MonthDate(q["month"], s.now(), s.Config.Location); d != nil {
		month = pgtype.Date{Time: *d, Valid: true}
	}
	db := dbgen.New(s.Pool)
	monthSQL := `"month" = $1`
	if !month.Valid {
		monthSQL = `"month" is null`
	}
	total, err := db.CountMonthlyLeaderboard(r.Context(), month)
	if err != nil {
		generalError(w, knexError(`select count(*) as "total" from "monthly_leaderboards" where `+monthSQL, err))
		return
	}
	entries := []catalog.LeaderboardEntry{}
	if total > 0 {
		entries, err = s.leaderboardEntries(r, db, month, page, monthSQL)
		if err != nil {
			generalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", catalog.Paginated(total, page, catalog.Leaderboard(entries, s.Config.Location))))
}

// leaderboardEntries loads the page, then preloads user → profile →
// university the way Lucid does (one query per level, first match wins).
func (s *Server) leaderboardEntries(r *http.Request, db *dbgen.Queries, month pgtype.Date, page catalog.Page, monthSQL string) ([]catalog.LeaderboardEntry, error) {
	limit, offset, err := page.LimitOffset()
	if err != nil {
		return nil, err
	}
	rows, err := db.ListMonthlyLeaderboard(r.Context(), dbgen.ListMonthlyLeaderboardParams{Month: month, RowLimit: limit, RowOffset: offset})
	if err != nil {
		p := placeholders{}
		if month.Valid {
			p.n = 1
		}
		return nil, knexError(`select * from "monthly_leaderboards" where `+monthSQL+` order by "score" desc`+limitOffset(&p, limit, offset), err)
	}
	entries := make([]catalog.LeaderboardEntry, len(rows))
	userIDs := []int32{}
	for i, row := range rows {
		entries[i].Row = row
		if row.UserID != nil {
			userIDs = append(userIDs, *row.UserID)
		}
	}
	if len(userIDs) == 0 {
		return entries, nil
	}
	users, err := db.PublicUsersByIDs(r.Context(), userIDs)
	if err != nil {
		return nil, err
	}
	profiles, err := db.ProfilesByUserIDs(r.Context(), userIDs)
	if err != nil {
		return nil, err
	}
	universityIDs := []int32{}
	for _, p := range profiles {
		if p.UniversityID != nil {
			universityIDs = append(universityIDs, *p.UniversityID)
		}
	}
	universities := []dbgen.UniversitiesByIDsRow{}
	if len(universityIDs) > 0 {
		if universities, err = db.UniversitiesByIDs(r.Context(), universityIDs); err != nil {
			return nil, err
		}
	}
	for i := range entries {
		if entries[i].Row.UserID == nil {
			continue
		}
		for j := range users {
			if users[j].ID == *entries[i].Row.UserID {
				entries[i].User = &users[j]
				break
			}
		}
		if entries[i].User == nil {
			continue
		}
		for j := range profiles {
			if profiles[j].UserID != nil && *profiles[j].UserID == entries[i].User.ID {
				entries[i].Profile = &profiles[j]
				break
			}
		}
		if entries[i].Profile == nil || entries[i].Profile.UniversityID == nil {
			continue
		}
		for j := range universities {
			if universities[j].ID == *entries[i].Profile.UniversityID {
				entries[i].University = &universities[j]
				break
			}
		}
	}
	return entries, nil
}

func (s *Server) issuedByCode(r *http.Request) (*dbgen.IssuedCertificate, error) {
	code, ok := certificate.NormalizeCode(rawSegment(r, 3))
	if !ok {
		return nil, nil
	}
	issued, err := dbgen.New(s.Pool).IssuedCertificateByCode(r.Context(), code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	approved, err := certificate.HasApproval(issued)
	if err != nil || !approved {
		return nil, err
	}
	return &issued, nil
}

func (s *Server) certificateByCode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	issued, err := s.issuedByCode(r)
	var data member.Object
	if err == nil && issued != nil {
		data, err = certificate.Public(*issued, s.Config.Location)
	}
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
	case issued == nil:
		writeJSON(w, http.StatusNotFound, message{"CERTIFICATE_NOT_FOUND"})
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
	}
}

func (s *Server) verifyCertificate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	issued, err := s.issuedByCode(r)
	var data member.Object
	if err == nil && issued != nil {
		data, err = certificate.Verification(*issued, s.Config.Location)
	}
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
	case issued == nil:
		writeJSON(w, http.StatusNotFound, member.KV("message", "CERTIFICATE_NOT_FOUND", "data", member.KV("valid", false, "state", "not_found")))
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "CERTIFICATE_VERIFIED", "data", data))
	}
}

package webapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"kaderisasi/web/internal/catalog"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/forms"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/validators"
	"kaderisasi/web/internal/vine"
)

// Phase 4: activities_controller.ts (registrationCheck, getRegistrationData,
// guestRegister, questionnaireEdit) and club_registrations_controller.ts.
func (s *Server) registerRegistrations() {
	auth := func(h http.HandlerFunc) http.Handler { return s.RequireAuth(h) }
	s.mux.Handle("GET /v2/profiles/activities/{slug}", auth(s.registrationCheck))
	s.mux.Handle("GET /v2/activities/{slug}/registration", auth(s.registrationData))
	s.mux.Handle("PUT /v2/activities/{slug}/registration", auth(s.questionnaireEdit))
	s.mux.HandleFunc("POST /v2/activities/{slug}/guest-register", s.guestRegister)
	s.mux.Handle("POST /v2/clubs/{id}/register", auth(s.clubRegister))
	s.mux.Handle("GET /v2/clubs/{id}/registration-status", auth(s.clubRegistrationStatus))
	s.mux.Handle("PUT /v2/clubs/{id}/registration", auth(s.updateClubRegistration))
	s.mux.Handle("DELETE /v2/clubs/{id}/registration", auth(s.cancelClubRegistration))
	s.mux.Handle("GET /v2/club-registrations/my-registrations", auth(s.myClubRegistrations))
}

const (
	activityBySlugSQL     = `select * from "activities" where "slug" = $1 limit $2`
	registrationForSQL    = `select * from "activity_registrations" where "user_id" = $1 and "activity_id" = $2 limit $3`
	clubByIDSQL           = `select * from "clubs" where "id" = $1 limit $2`
	clubRegistrationSQL   = `select * from "club_registrations" where "club_id" = $1 and "member_id" = $2 limit $3`
	activeFeatureFormSQL  = `select * from "custom_forms" where "feature_type" = $1 and "feature_id" = $2 and "is_active" = $3 order by "updated_at" desc, "id" desc limit $4`
	issuedForRegistration = `select * from "issued_certificates" where "registration_id" = $1 limit $2`
)

func itoa(n int32) string { return strconv.Itoa(int(n)) }

// activityBySlug is Activity.findBy('slug', slug); a missing row is nil.
func activityBySlug(ctx context.Context, q *dbgen.Queries, slug string) (*dbgen.Activity, error) {
	a, err := q.ActivityBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, knexError(activityBySlugSQL, err)
	}
	return &a, nil
}

func registrationFor(ctx context.Context, q *dbgen.Queries, userID, activityID int32) (*dbgen.ActivityRegistration, error) {
	reg, err := q.RegistrationForUser(ctx, dbgen.RegistrationForUserParams{UserID: userID, ActivityID: itoa(activityID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, knexError(registrationForSQL, err)
	}
	return &reg, nil
}

func (s *Server) certificateFields(ctx context.Context, q *dbgen.Queries, reg dbgen.ActivityRegistration) (ownerFields, error) {
	issued := map[int32]dbgen.IssuedCertificate{}
	c, err := q.IssuedForRegistration(ctx, reg.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, knexError(issuedForRegistration, err)
	}
	if err == nil {
		issued[reg.ID] = c
	}
	return ownerState(reg, issued, s.Config.Location)
}

func activeForm(ctx context.Context, q *dbgen.Queries, featureType string, featureID int32) (*dbgen.CustomForm, error) {
	f, err := q.ActiveFeatureForm(ctx, dbgen.ActiveFeatureFormParams{FeatureType: featureType, FeatureID: itoa(featureID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, knexError(activeFeatureFormSQL, err)
	}
	return &f, nil
}

func (s *Server) registrationCheck(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.Pool)
	userID := learner(r).ID
	activity, err := activityBySlug(r.Context(), q, rawSegment(r, 3))
	if err == nil && activity == nil {
		err = errRowNotFound
	}
	if err != nil {
		generalError(w, err)
		return
	}
	data := member.KV("status", "BELUM TERDAFTAR")
	reg, err := registrationFor(r.Context(), q, userID, activity.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	if reg != nil {
		config := js.Value{Kind: 'n'}
		if v, ok := member.JSONB(activity.AdditionalConfig).(js.Value); ok {
			config = v
		}
		hidden, visibleAt := statusHidden(config, s.now())
		if hidden {
			data = data.Set("status", "BELUM DIUMUMKAN")
			if visibleAt != nil {
				data = data.Set("visible_at", visibleAt)
			}
		} else {
			data = data.Set("status", reg.Status).Set("registration_id", reg.ID)
			if reg.ID != 0 {
				state, err := s.certificateFields(r.Context(), q, *reg)
				if err != nil {
					generalError(w, err)
					return
				}
				data = data.Set("certificate_state", state.Get("state")).Set("certificate_code", state.Get("certificate_code")).
					Set("certificate_issued_at", state.Get("issued_at")).Set("certificate_revoked_at", state.Get("revoked_at"))
			}
		}
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
}

func (s *Server) registrationData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := dbgen.New(s.Pool)
	userID := learner(r).ID
	activity, err := activityBySlug(r.Context(), q, rawSegment(r, 2))
	if err == nil && activity == nil {
		err = errRowNotFound
	}
	if err != nil {
		generalError(w, err)
		return
	}
	reg, err := registrationFor(r.Context(), q, userID, activity.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	if reg == nil {
		writeJSON(w, http.StatusNotFound, message{"REGISTRATION_NOT_FOUND"})
		return
	}
	state, err := s.certificateFields(r.Context(), q, *reg)
	if err != nil {
		generalError(w, err)
		return
	}
	score, err := member.PublishedScore(reg.ScoringData, reg.ID, activity.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	data := append(member.Registration(*reg, s.Config.Location), member.KV("scoring_result", score,
		"certificate_state", state.Get("state"), "certificate_code", state.Get("certificate_code"),
		"certificate_issued_at", state.Get("issued_at"), "certificate_revoked_at", state.Get("revoked_at"))...)
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
}

// validationFailure is the controllers' E_VALIDATION_ERROR catch: 500 with
// the first message and the list.
func validationFailure(w http.ResponseWriter, err *vine.Error) {
	msg := "GENERAL_ERROR"
	if len(err.Issues) > 0 && err.Issues[0].Message != "" {
		msg = err.Issues[0].Message
	}
	writeJSON(w, http.StatusInternalServerError, member.KV("message", msg, "error", err.Issues))
}

// formFailure answers a FormRequestError, or the generic catch.
func formFailure(w http.ResponseWriter, err error) {
	var request *forms.RequestError
	if errors.As(err, &request) {
		writeJSON(w, request.Status, message{request.Message})
		return
	}
	generalError(w, err)
}

func invalidSubmission(w http.ResponseWriter, errs []forms.SubmissionError) {
	writeJSON(w, http.StatusUnprocessableEntity, member.KV("message", "INVALID_FORM_SUBMISSION", "errors", errs))
}

func sessionToken(payload js.Value) *string {
	if v, ok := payload.Get("session_token"); ok && v.Kind == 's' {
		return &v.String
	}
	return nil
}

// errHandled marks a transaction body that already wrote its response.
var errHandled = errors.New("handled")

func (s *Server) inTx(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(dbgen.New(tx)); err != nil && !errors.Is(err, errHandled) {
		return err
	} else if err != nil {
		// A response returned inside db.transaction still commits.
		if cerr := tx.Commit(ctx); cerr != nil {
			return cerr
		}
		return errHandled
	}
	return tx.Commit(ctx)
}

func (s *Server) guestRegister(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.GuestActivityRegistration, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		validationFailure(w, invalid)
		return
	}
	if err != nil {
		generalError(w, err)
		return
	}
	slug := rawSegment(r, 2)
	err = s.inTx(r.Context(), func(q *dbgen.Queries) error {
		a, err := q.ActivityBySlugForUpdate(r.Context(), slug)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return knexError(activityBySlugSQL+" for update", err)
		}
		if err != nil || a.IsPublished == nil || !*a.IsPublished {
			writeJSON(w, http.StatusNotFound, message{"ACTIVITY_NOT_FOUND"})
			return errHandled
		}
		if !forms.GuestRegistrationAllowed(a) {
			writeJSON(w, http.StatusForbidden, message{"GUEST_REGISTRATION_NOT_ALLOWED"})
			return errHandled
		}
		now := s.now()
		if !forms.ActivityOpen(a, now) {
			writeJSON(w, http.StatusForbidden, message{"REGISTRATION_CLOSED"})
			return errHandled
		}
		form, err := activeForm(r.Context(), q, "activity_registration", a.ID)
		if err != nil {
			return err
		}
		if form == nil {
			writeJSON(w, http.StatusBadRequest, message{"ACTIVE_CUSTOM_FORM_REQUIRED"})
			return errHandled
		}
		answers, ok := payload.Get("questionnaire_answer")
		if !ok || answers.Kind == 'n' {
			answers = js.Value{Kind: 'o', Fields: map[string]js.Value{}}
		}
		valid, data, errs, err := forms.Validate(forms.Schema(*form), answers)
		if err != nil {
			return err
		}
		if !valid {
			invalidSubmission(w, errs)
			return errHandled
		}
		if err := forms.Claim(r.Context(), q, *form, sessionToken(payload), nil, data, nil, now); err != nil {
			return err
		}
		guest, _ := payload.Get("guest_data")
		guestJSON, _ := guest.MarshalJSON()
		answerJSON, _ := data.MarshalJSON()
		created := member.Now()
		status := "TERDAFTAR"
		id, err := q.CreateActivityRegistration(r.Context(), dbgen.CreateActivityRegistrationParams{ActivityID: itoa(a.ID), Status: status,
			GuestData: guestJSON, QuestionnaireAnswer: answerJSON, CreatedAt: member.Timestamp(created), UpdatedAt: member.Timestamp(created)})
		if err != nil {
			return member.InsertError("activity_registrations", map[string]bool{"activity_id": true, "created_at": true, "guest_data": true,
				"questionnaire_answer": true, "status": true, "updated_at": true, "user_id": true}, err)
		}
		writeJSON(w, http.StatusOK, member.KV("message", "ACTIVITY_REGISTER_SUCCESS", "data", member.KV("user_id", nil, "activity_id", a.ID,
			"status", status, "guest_data", guest, "questionnaire_answer", data, "created_at", member.ISO(created, s.Config.Location),
			"updated_at", member.ISO(created, s.Config.Location), "id", id)))
		return errHandled
	})
	if err != nil && !errors.Is(err, errHandled) {
		formFailure(w, err)
	}
}

func (s *Server) questionnaireEdit(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	user := learner(r)
	payload, err := vine.ValidateObject(validators.ActivityRegistration, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		validationFailure(w, invalid)
		return
	}
	if err != nil {
		generalError(w, err)
		return
	}
	q := dbgen.New(s.Pool)
	activity, err := activityBySlug(r.Context(), q, rawSegment(r, 2))
	if err != nil {
		generalError(w, err)
		return
	}
	if activity == nil {
		writeJSON(w, http.StatusNotFound, message{"ACTIVITY_NOT_FOUND"})
		return
	}
	registered, err := registrationFor(r.Context(), q, user.ID, activity.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	if registered == nil {
		writeJSON(w, http.StatusNotFound, message{"REGISTRATION_NOT_FOUND"})
		return
	}
	form, err := activeForm(r.Context(), q, "activity_registration", activity.ID)
	if err != nil {
		generalError(w, err)
		return
	}
	answers, _ := payload.Get("questionnaire_answer")
	if form != nil {
		valid, data, errs, err := forms.Validate(forms.Schema(*form), answers)
		if err != nil {
			generalError(w, err)
			return
		}
		if !valid {
			invalidSubmission(w, errs)
			return
		}
		answers = data
	}
	var updated member.Object
	err = s.inTx(r.Context(), func(tq *dbgen.Queries) error {
		locked, err := tq.RegistrationForUpdate(r.Context(), registered.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errRowNotFound
		}
		if err != nil {
			return knexError(`select * from "activity_registrations" where "id" = $1 limit $2 for update`, err)
		}
		previous := js.Value{Kind: 'n'}
		if v, ok := member.JSONB(locked.QuestionnaireAnswer).(js.Value); ok {
			previous = v
		}
		if form != nil {
			if err := forms.Claim(r.Context(), tq, *form, sessionToken(payload), &user.ID, answers, &previous, s.now()); err != nil {
				return err
			}
		}
		updated = member.Registration(locked, s.Config.Location).Set("questionnaire_answer", answers)
		if member.ValueEqual(previous, answers) {
			return nil
		}
		at := member.Now()
		raw, _ := answers.MarshalJSON()
		if err := tq.SetQuestionnaireAnswer(r.Context(), dbgen.SetQuestionnaireAnswerParams{Answer: raw, UpdatedAt: member.Timestamp(at), ID: locked.ID}); err != nil {
			return member.UpdateError("activity_registrations", []string{"questionnaire_answer", "updated_at"}, err)
		}
		updated = updated.Set("updated_at", member.ISO(at, s.Config.Location))
		return nil
	})
	if err != nil {
		formFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "UPDATE_DATA_SUCCESS", "data", updated))
}

// --- clubs ----------------------------------------------------------------

// clubFind is Club.find(params.id).
func clubFind(ctx context.Context, q *dbgen.Queries, id string) (*dbgen.Club, error) {
	c, err := q.ClubByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, knexError(clubByIDSQL, err)
	}
	return &c, nil
}

func (s *Server) clubRegister(w http.ResponseWriter, r *http.Request) {
	c, err := clubFind(r.Context(), dbgen.New(s.Pool), rawSegment(r, 2))
	switch {
	case err != nil:
		generalError(w, err)
	case c == nil || c.IsShow == nil || !*c.IsShow:
		writeJSON(w, http.StatusNotFound, message{"CLUB_NOT_FOUND"})
	case !catalog.RegistrationOpen(*c, s.now()):
		writeJSON(w, http.StatusBadRequest, message{"REGISTRATION_CLOSED"})
	default:
		writeJSON(w, http.StatusBadRequest, message{"CUSTOM_FORM_REQUIRED"})
	}
}

// clubRegistrations serializes registrations with their preloaded club and
// roles (sort_order asc, is_primary desc).
func (s *Server) clubRegistrations(ctx context.Context, q *dbgen.Queries, rows []dbgen.ClubRegistration) ([]member.Object, error) {
	out := []member.Object{}
	if len(rows) == 0 {
		return out, nil
	}
	clubIDs, ids := []int32{}, []int32{}
	for _, row := range rows {
		if row.ClubID != nil {
			clubIDs = append(clubIDs, *row.ClubID)
		}
		ids = append(ids, row.ID)
	}
	clubs := map[int32]dbgen.Club{}
	if len(clubIDs) > 0 {
		list, err := q.ClubsByIDs(ctx, clubIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			clubs[c.ID] = c
		}
	}
	roles, err := q.RolesForRegistrations(ctx, ids)
	if err != nil {
		return nil, err
	}
	zone := s.Config.Location
	for _, row := range rows {
		var club any
		if row.ClubID != nil {
			if c, ok := clubs[*row.ClubID]; ok {
				club = catalog.Club(c, zone)
			}
		}
		list := []member.Object{}
		for _, role := range roles {
			if role.ClubRegistrationID == row.ID {
				list = append(list, member.KV("id", role.ID, "club_registration_id", role.ClubRegistrationID, "role_name", role.RoleName,
					"start_date", member.LucidDate(role.StartDate), "end_date", member.LucidDate(role.EndDate), "is_primary", role.IsPrimary,
					"sort_order", role.SortOrder, "created_at", member.DateTime(role.CreatedAt, zone), "updated_at", member.DateTime(role.UpdatedAt, zone)))
			}
		}
		out = append(out, member.KV("id", row.ID, "club_id", row.ClubID, "member_id", row.MemberID, "status", row.Status,
			"additional_data", member.JSONB(row.AdditionalData), "created_at", member.DateTime(row.CreatedAt, zone),
			"updated_at", member.DateTime(row.UpdatedAt, zone), "club", club, "roles", list))
	}
	return out, nil
}

func (s *Server) clubRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.Pool)
	c, err := clubFind(r.Context(), q, rawSegment(r, 2))
	if err != nil {
		generalError(w, err)
		return
	}
	if c == nil {
		writeJSON(w, http.StatusNotFound, message{"CLUB_NOT_FOUND"})
		return
	}
	var registration any
	row, err := q.ClubRegistrationFor(r.Context(), dbgen.ClubRegistrationForParams{ClubID: itoa(c.ID), MemberID: learner(r).ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		generalError(w, knexError(clubRegistrationSQL, err))
		return
	}
	if err == nil {
		list, err := s.clubRegistrations(r.Context(), q, []dbgen.ClubRegistration{row})
		if err != nil {
			generalError(w, err)
			return
		}
		registration = list[0]
	}
	writeJSON(w, http.StatusOK, member.KV("message", "REGISTRATION_STATUS_RETRIEVED",
		"data", member.KV("isRegistered", registration != nil, "registration", registration)))
}

// inputNumber is Number(request.input(key, fallback)).
func inputNumber(input js.Value, key string, fallback float64) float64 {
	v, ok := input.Get(key)
	if !ok {
		return fallback
	}
	if v.Kind == 'n' {
		return 0
	}
	return vine.ToNumber(v)
}

func (s *Server) myClubRegistrations(w http.ResponseWriter, r *http.Request) {
	input, ok := s.body(w, r)
	if !ok {
		return
	}
	user := learner(r)
	page := catalog.Page{Page: inputNumber(input, "page", 1), PerPage: inputNumber(input, "limit", 20)}
	var status *string
	where := `"member_id" = $1`
	if v, ok := input.Get("status"); ok && v.Truthy() {
		text := vine.ToString(v)
		status = &text
		where += ` and "status" = $2`
	}
	q := dbgen.New(s.Pool)
	total, err := q.CountClubRegistrations(r.Context(), dbgen.CountClubRegistrationsParams{MemberID: user.ID, Status: status})
	if err != nil {
		generalError(w, knexError(`select count(*) as "total" from "club_registrations" where `+where, err))
		return
	}
	rows := []dbgen.ClubRegistration{}
	if total > 0 {
		limit, offset, err := page.LimitOffset()
		if err == nil {
			rows, err = q.ListClubRegistrations(r.Context(), dbgen.ListClubRegistrationsParams{MemberID: user.ID, Status: status, RowLimit: limit, RowOffset: offset})
			p := placeholders{n: 1}
			if status != nil {
				p.n = 2
			}
			err = knexError(`select * from "club_registrations" where `+where+` order by "created_at" desc`+limitOffset(&p, limit, offset), err)
		}
		if err != nil {
			generalError(w, err)
			return
		}
	}
	list, err := s.clubRegistrations(r.Context(), q, rows)
	if err != nil {
		generalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "MY_CLUB_REGISTRATIONS_RETRIEVED", "data", catalog.Paginated(total, page, list)))
}

func (s *Server) updateClubRegistration(w http.ResponseWriter, r *http.Request) {
	_, err := dbgen.New(s.Pool).ClubRegistrationFor(r.Context(), dbgen.ClubRegistrationForParams{ClubID: rawSegment(r, 2), MemberID: learner(r).ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeJSON(w, http.StatusNotFound, message{"REGISTRATION_NOT_FOUND"})
	case err != nil:
		generalError(w, knexError(clubRegistrationSQL, err))
	default:
		writeJSON(w, http.StatusBadRequest, message{"CANNOT_UPDATE_REGISTRATION"})
	}
}

func (s *Server) cancelClubRegistration(w http.ResponseWriter, r *http.Request) {
	result := ""
	err := s.inTx(r.Context(), func(q *dbgen.Queries) error {
		row, err := q.ClubRegistrationForUpdate(r.Context(), dbgen.ClubRegistrationForUpdateParams{ClubID: rawSegment(r, 2), MemberID: learner(r).ID})
		if errors.Is(err, pgx.ErrNoRows) {
			result = "REGISTRATION_NOT_FOUND"
			return nil
		}
		if err != nil {
			return knexError(clubRegistrationSQL+" for update", err)
		}
		if row.Status == nil || *row.Status != "PENDING" {
			result = "CANNOT_CANCEL_REGISTRATION"
			return nil
		}
		if err := q.DeleteClubRegistration(r.Context(), row.ID); err != nil {
			return knexError(`delete from "club_registrations" where "id" = $1`, err)
		}
		return nil
	})
	switch {
	case err != nil:
		generalError(w, err)
	case result == "REGISTRATION_NOT_FOUND":
		writeJSON(w, http.StatusNotFound, message{result})
	case result == "CANNOT_CANCEL_REGISTRATION":
		writeJSON(w, http.StatusBadRequest, message{result})
	default:
		writeJSON(w, http.StatusOK, member.KV("message", "CLUB_REGISTRATION_DELETED", "data", nil))
	}
}

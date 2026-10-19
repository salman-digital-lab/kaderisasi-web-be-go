package webapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/catalog"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/forms"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/richtext"
	"kaderisasi/web/internal/storage"
	"kaderisasi/web/internal/validators"
	"kaderisasi/web/internal/vine"
)

// Phase 5: custom_forms_controller.ts and form_responses_controller.ts.
func (s *Server) registerForms() {
	s.mux.HandleFunc("GET /v2/custom-forms/by-feature", s.formByFeature)
	s.mux.Handle("POST /v2/custom-forms/register", s.RequireAuth(http.HandlerFunc(s.formRegister)))
	s.mux.HandleFunc("POST /v2/custom-forms/{id}/sessions", s.formSession)
	s.mux.HandleFunc("POST /v2/custom-forms/{id}/responses", s.formSubmit)
	s.mux.HandleFunc("POST /v2/custom-forms/{id}/files/{key}", s.formUpload)
	s.mux.HandleFunc("DELETE /v2/custom-forms/{id}/files/{attachmentId}", s.formRemoveUpload)
}

// uncaught is Adonis's rendering of an exception no controller handled
// (debug stack frames are not reproduced).
func uncaught(w http.ResponseWriter, err error) {
	name := "Error"
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		name = "error" // node-postgres DatabaseError
	}
	writeJSON(w, http.StatusInternalServerError, member.KV("message", err.Error(), "name", name, "status", http.StatusInternalServerError))
}

// formFail is FormResponsesController.fail: a FormRequestError answers with
// its status, anything else is uncaught.
func formFail(w http.ResponseWriter, err error) {
	var request *forms.RequestError
	if errors.As(err, &request) {
		writeJSON(w, request.Status, message{request.Message})
		return
	}
	uncaught(w, err)
}

// formUser is FormResponsesController.user: no Authorization header means a
// guest; otherwise the guard must accept the request and the header token
// must carry the learner audience for the same member.
func (s *Server) formUser(r *http.Request) (*int32, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, nil
	}
	unauthorized := &forms.RequestError{Status: http.StatusUnauthorized, Message: "UNAUTHORIZED"}
	learner, err := s.authenticate(r)
	if err != nil {
		return nil, unauthorized
	}
	claims, err := auth.VerifyPublicAccess(s.Config.AppKey, strings.TrimPrefix(header, "Bearer "), s.now())
	if err != nil || !claims.HasAudience || claims.UserID != learner.User.ID {
		return nil, unauthorized
	}
	id := learner.User.ID
	return &id, nil
}

func firstQuery(r *http.Request, key string) string {
	if v := r.URL.Query()[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (s *Server) formByFeature(w http.ResponseWriter, r *http.Request) {
	featureType, featureID := firstQuery(r, "feature_type"), firstQuery(r, "feature_id")
	success := firstQuery(r, "view") == "success"
	if featureType == "" {
		writeJSON(w, http.StatusBadRequest, message{"FEATURE_TYPE_REQUIRED"})
		return
	}
	q := dbgen.New(s.Pool)
	if featureType == "activity_registration" {
		var activity *dbgen.Activity
		if featureID != "" {
			a, err := q.ActivityByID(r.Context(), featureID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				generalError(w, knexError(`select * from "activities" where "id" = $1 limit $2`, err))
				return
			}
			if err == nil {
				activity = &a
			}
		}
		if activity == nil || activity.IsPublished == nil || !*activity.IsPublished {
			writeJSON(w, http.StatusNotFound, message{"ACTIVITY_NOT_FOUND"})
			return
		}
	}
	columns := "*"
	if success {
		columns = `"id", "post_submission_info"`
	}
	var form dbgen.CustomForm
	var err error
	if featureType == "independent_form" {
		if featureID == "" {
			writeJSON(w, http.StatusBadRequest, message{"FORM_ID_REQUIRED"})
			return
		}
		form, err = q.IndependentForm(r.Context(), featureID)
		err = knexError(`select `+columns+` from "custom_forms" where "id" = $1 and "feature_type" = $2 limit $3`, err)
	} else {
		if featureID == "" {
			writeJSON(w, http.StatusBadRequest, message{"FEATURE_ID_REQUIRED"})
			return
		}
		form, err = q.ActiveFeatureForm(r.Context(), dbgen.ActiveFeatureFormParams{FeatureType: featureType, FeatureID: featureID})
		err = knexError(`select `+columns+` from "custom_forms" where "feature_type" = $1 and "feature_id" = $2 and "is_active" = $3 order by "updated_at" desc, "id" desc limit $4`, err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, message{"CUSTOM_FORM_NOT_FOUND"})
		return
	}
	if err != nil {
		generalError(w, err)
		return
	}
	var info any
	if form.PostSubmissionInfo != nil {
		info = *form.PostSubmissionInfo
		if *form.PostSubmissionInfo != "" {
			info = richtext.Sanitize(*form.PostSubmissionInfo)
		}
	}
	if success {
		writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.KV("id", form.ID, "post_submission_info", info)))
		return
	}
	zone := s.Config.Location
	data := member.KV("id", form.ID, "form_name", form.FormName, "form_description", form.FormDescription, "feature_id", form.FeatureID,
		"form_schema", member.JSONB(form.FormSchema), "is_active", form.IsActive, "created_at", member.DateTime(form.CreatedAt, zone),
		"updated_at", member.DateTime(form.UpdatedAt, zone), "feature_type", form.FeatureType, "post_submission_info", info,
		"schema_hash", forms.SchemaHash(forms.Schema(form)))
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", data))
}

// bodyToken is a request.body() session_token as claimFormAttachments sees
// it: undefined when falsy, otherwise its string form.
func bodyToken(body js.Value) *string {
	v, ok := body.Get("session_token")
	if !ok || !v.Truthy() {
		return nil
	}
	text := vine.ToString(v)
	return &text
}

func isClubRegistrationUnique(err error) bool {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" {
		return false
	}
	c := pg.ConstraintName
	return c == "" || (strings.Contains(c, "club_registrations") && strings.Contains(c, "club_id") && strings.Contains(c, "member_id"))
}

func (s *Server) formRegister(w http.ResponseWriter, r *http.Request) {
	body, err := parseBody(r)
	var invalid *bodyError
	if errors.As(err, &invalid) {
		writeJSON(w, invalid.status, member.KV("message", invalid.message, "name", invalid.name, "status", invalid.status))
		return
	}
	if err != nil {
		generalError(w, err)
		return
	}
	featureType, _ := body.Get("feature_type")
	featureID, _ := body.Get("feature_id")
	data, _ := body.Get("custom_form_data")
	if !featureType.Truthy() {
		writeJSON(w, http.StatusBadRequest, message{"FEATURE_TYPE_REQUIRED"})
		return
	}
	kind := ""
	if featureType.Kind == 's' {
		kind = featureType.String
	}
	if (kind == "activity_registration" || kind == "club_registration") && !featureID.Truthy() {
		writeJSON(w, http.StatusBadRequest, message{"FEATURE_ID_REQUIRED"})
		return
	}
	user := learner(r)
	switch kind {
	case "activity_registration":
		err = s.registerActivityForm(w, r, user, featureID, data, bodyToken(body))
	case "club_registration":
		err = s.registerClubForm(w, r, user, featureID, data, bodyToken(body))
	case "independent_form":
		writeJSON(w, http.StatusBadRequest, message{"USE_STANDALONE_FORM_RESPONSE_ENDPOINT"})
	default:
		writeJSON(w, http.StatusBadRequest, message{"INVALID_FEATURE_TYPE"})
	}
	if err != nil && !errors.Is(err, errHandled) {
		formFailure(w, err)
	}
}

func (s *Server) registerActivityForm(w http.ResponseWriter, r *http.Request, user dbgen.PublicUser, featureID, answers js.Value, token *string) error {
	id := vine.ToString(featureID)
	return s.inTx(r.Context(), func(q *dbgen.Queries) error {
		a, err := q.ActivityByIDForUpdate(r.Context(), id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return knexError(`select * from "activities" where "id" = $1 limit $2 for update`, err)
		}
		now := s.now()
		if err != nil || !forms.ActivityOpen(a, now) {
			writeJSON(w, http.StatusBadRequest, message{"REGISTRATION_CLOSED"})
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
		valid, data, errs, err := forms.Validate(forms.Schema(*form), answers)
		if err != nil {
			return err
		}
		if !valid {
			invalidSubmission(w, errs)
			return errHandled
		}
		_, err = q.RegistrationForUser(r.Context(), dbgen.RegistrationForUserParams{UserID: user.ID, ActivityID: id})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return knexError(registrationForSQL, err)
		}
		if err == nil {
			writeJSON(w, http.StatusConflict, message{"ALREADY_REGISTERED"})
			return errHandled
		}
		if err := forms.Claim(r.Context(), q, *form, token, &user.ID, data, nil, now); err != nil {
			return err
		}
		answerJSON, _ := data.MarshalJSON()
		created := member.Now()
		status := "TERDAFTAR"
		regID, err := q.CreateActivityRegistration(r.Context(), dbgen.CreateActivityRegistrationParams{UserID: &user.ID, ActivityID: id, Status: status,
			QuestionnaireAnswer: answerJSON, CreatedAt: member.Timestamp(created), UpdatedAt: member.Timestamp(created)})
		if err != nil {
			return member.InsertError("activity_registrations", map[string]bool{"activity_id": true, "created_at": true,
				"questionnaire_answer": true, "status": true, "updated_at": true, "user_id": true}, err)
		}
		// activityId is assigned from the request body as given.
		writeJSON(w, http.StatusCreated, member.KV("message", "ACTIVITY_REGISTER_SUCCESS", "data", member.KV("user_id", user.ID, "activity_id", featureID,
			"status", status, "questionnaire_answer", data, "created_at", member.ISO(created, s.Config.Location),
			"updated_at", member.ISO(created, s.Config.Location), "id", regID)))
		return errHandled
	})
}

func (s *Server) registerClubForm(w http.ResponseWriter, r *http.Request, user dbgen.PublicUser, featureID, answers js.Value, token *string) error {
	q := dbgen.New(s.Pool)
	club, err := clubFind(r.Context(), q, vine.ToString(featureID))
	if err != nil {
		return err
	}
	now := s.now()
	if club == nil || !catalog.RegistrationOpen(*club, now) {
		writeJSON(w, http.StatusBadRequest, message{"REGISTRATION_CLOSED"})
		return errHandled
	}
	form, err := activeForm(r.Context(), q, "club_registration", club.ID)
	if err != nil {
		return err
	}
	if form == nil {
		writeJSON(w, http.StatusBadRequest, message{"ACTIVE_CUSTOM_FORM_REQUIRED"})
		return errHandled
	}
	valid, data, errs, err := forms.Validate(forms.Schema(*form), answers)
	if err != nil {
		return err
	}
	if !valid {
		invalidSubmission(w, errs)
		return errHandled
	}
	_, err = q.ClubRegistrationFor(r.Context(), dbgen.ClubRegistrationForParams{ClubID: itoa(club.ID), MemberID: user.ID})
	if err == nil {
		writeJSON(w, http.StatusConflict, message{"ALREADY_REGISTERED"})
		return errHandled
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return knexError(`select * from "club_registrations" where "member_id" = $1 and "club_id" = $2 limit $3`, err)
	}
	var created member.Object
	err = s.inTx(r.Context(), func(tq *dbgen.Queries) error {
		locked, err := tq.CustomFormByIDForUpdate(r.Context(), form.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errRowNotFound
		}
		if err != nil {
			return knexError(`select * from "custom_forms" where "id" = $1 limit $2 for update`, err)
		}
		if err := forms.Claim(r.Context(), tq, locked, token, &user.ID, data, nil, s.now()); err != nil {
			return err
		}
		raw, _ := data.MarshalJSON()
		at := member.Now()
		status := "PENDING"
		id, err := tq.CreateClubRegistration(r.Context(), dbgen.CreateClubRegistrationParams{MemberID: user.ID, ClubID: club.ID, Status: status,
			AdditionalData: raw, CreatedAt: member.Timestamp(at), UpdatedAt: member.Timestamp(at)})
		if err != nil {
			if isClubRegistrationUnique(err) {
				return err
			}
			return member.InsertError("club_registrations", map[string]bool{"additional_data": true, "club_id": true, "created_at": true,
				"member_id": true, "status": true, "updated_at": true}, err)
		}
		created = member.KV("member_id", user.ID, "club_id", club.ID, "status", status, "additional_data", data,
			"created_at", member.ISO(at, s.Config.Location), "updated_at", member.ISO(at, s.Config.Location), "id", id)
		return nil
	})
	if isClubRegistrationUnique(err) {
		writeJSON(w, http.StatusConflict, message{"ALREADY_REGISTERED"})
		return errHandled
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, member.KV("message", "CLUB_REGISTER_SUCCESS", "data", created))
	return errHandled
}

// validateUsing is request.validateUsing: an uncaught E_VALIDATION_ERROR.
func (s *Server) validateUsing(w http.ResponseWriter, r *http.Request, schema vine.Obj) (js.Value, bool) {
	body, ok := s.body(w, r)
	if !ok {
		return js.Value{}, false
	}
	payload, err := vine.ValidateObject(schema, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		validationErrors(w, invalid)
		return js.Value{}, false
	}
	if err != nil {
		uncaught(w, err)
		return js.Value{}, false
	}
	return payload, true
}

func (s *Server) formSession(w http.ResponseWriter, r *http.Request) {
	input, ok := s.validateUsing(w, r, validators.FormSession)
	if !ok {
		return
	}
	userID, err := s.formUser(r)
	if err != nil {
		formFail(w, err)
		return
	}
	q := dbgen.New(s.Pool)
	form, err := q.CustomFormByID(r.Context(), rawSegment(r, 2))
	if errors.Is(err, pgx.ErrNoRows) {
		formFail(w, &forms.RequestError{Status: http.StatusNotFound, Message: "CUSTOM_FORM_NOT_FOUND"})
		return
	}
	if err != nil {
		uncaught(w, knexError(`select * from "custom_forms" where "id" = $1 limit $2`, err))
		return
	}
	hash, _ := input.Get("schema_hash")
	if forms.SchemaHash(forms.Schema(form)) != hash.String {
		formFail(w, &forms.RequestError{Status: http.StatusConflict, Message: "FORM_SCHEMA_CHANGED"})
		return
	}
	token, schemaHash, err := forms.CreateSession(r.Context(), q, form, userID, s.now())
	if err != nil {
		formFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, member.KV("data", member.KV("token", token, "schema_hash", schemaHash)))
}

// formNumber is Number(ctx.params.id) as node-postgres would send it.
func formNumber(r *http.Request) string { return js.ToString(js.Number(rawSegment(r, 2))) }

func (s *Server) formSubmit(w http.ResponseWriter, r *http.Request) {
	input, ok := s.validateUsing(w, r, validators.FormSubmission)
	if !ok {
		return
	}
	formID := formNumber(r)
	userID, err := s.formUser(r)
	if err != nil {
		formFail(w, err)
		return
	}
	token, _ := input.Get("session_token")
	answers, _ := input.Get("answers")
	var responseID string
	err = s.inTx(r.Context(), func(q *dbgen.Queries) error {
		form, err := q.StandaloneFormForUpdate(r.Context(), formID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &forms.RequestError{Status: http.StatusNotFound, Message: "CUSTOM_FORM_NOT_FOUND"}
		}
		if err != nil {
			return knexError(`select * from "custom_forms" where "id" = $1 and "feature_type" = $2 limit $3 for update`, err)
		}
		now := s.now()
		session, err := forms.Session(r.Context(), q, form, token.String, userID, true, now)
		if err != nil {
			return err
		}
		existing, err := q.ResponseForSession(r.Context(), session.ID)
		if err == nil {
			responseID = forms.UUIDString(existing.ID)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return knexError(`select * from "custom_form_responses" where "session_id" = $1 limit $2`, err)
		}
		if err := forms.RequireOpen(r.Context(), q, form, userID, now); err != nil {
			return err
		}
		schema := forms.Schema(form)
		if session.SchemaHash != forms.SchemaHash(schema) {
			return &forms.RequestError{Status: http.StatusConflict, Message: "FORM_SCHEMA_CHANGED"}
		}
		valid, data, errs, err := forms.Validate(forms.StandaloneSchema(schema), answers)
		if err != nil {
			return err
		}
		if !valid {
			msg := "INVALID_FORM_SUBMISSION"
			if len(errs) > 0 {
				msg = errs[0].Message
			}
			return &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: msg}
		}
		if err := forms.Claim(r.Context(), q, form, &token.String, userID, data, nil, now); err != nil {
			return err
		}
		responseID = forms.UUID()
		snapshot := js.Value{Kind: 'o', Keys: []string{"title", "schema"}, Fields: map[string]js.Value{
			"title": {Kind: 's', String: form.FormName}, "schema": schema}}
		rawSnapshot, _ := snapshot.MarshalJSON()
		rawAnswers, _ := data.MarshalJSON()
		if err := q.InsertFormResponse(r.Context(), dbgen.InsertFormResponseParams{ID: forms.ParseUUID(responseID), FormID: form.ID, SessionID: session.ID,
			UserID: userID, FormSnapshot: rawSnapshot, Answers: rawAnswers}); err != nil {
			return knexError(`insert into "custom_form_responses" ("answers", "form_id", "form_snapshot", "id", "session_id", "user_id") values ($1, $2, $3, $4, $5, $6)`, err)
		}
		return nil
	})
	if err != nil {
		formFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, member.KV("message", "FORM_SUBMITTED", "data", member.KV("id", responseID)))
}

var uploadIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

var formUploadRule = vine.File{Size: "10mb", Extnames: []string{"pdf", "jpg", "jpeg", "png", "webp"}}

type uploadCheck struct {
	sessionID pgtype.UUID
	settings  forms.FileSettings
	previous  *dbgen.CustomFormAttachment
}

// validateUpload is form_upload_service.ts validate(); q runs inside a
// transaction when locking.
func (s *Server) validateUpload(ctx context.Context, q *dbgen.Queries, lock bool, formID string, userID *int32, token, fieldKey, uploadID string) (uploadCheck, error) {
	var form dbgen.CustomForm
	var err error
	if lock {
		form, err = q.CustomFormByIDForShare(ctx, formID)
		err = knexError(`select * from "custom_forms" where "id" = $1 limit $2 for share`, err)
	} else {
		form, err = q.CustomFormByID(ctx, formID)
		err = knexError(`select * from "custom_forms" where "id" = $1 limit $2`, err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uploadCheck{}, &forms.RequestError{Status: http.StatusNotFound, Message: "CUSTOM_FORM_NOT_FOUND"}
	}
	if err != nil {
		return uploadCheck{}, err
	}
	now := s.now()
	if err := forms.RequireOpen(ctx, q, form, userID, now); err != nil {
		return uploadCheck{}, err
	}
	session, err := forms.Session(ctx, q, form, token, userID, lock, now)
	if err != nil {
		return uploadCheck{}, err
	}
	if session.CompletedAt.Valid {
		return uploadCheck{}, &forms.RequestError{Status: http.StatusConflict, Message: "FORM_SESSION_COMPLETED"}
	}
	schema := forms.Schema(form)
	if session.SchemaHash != forms.SchemaHash(schema) {
		return uploadCheck{}, &forms.RequestError{Status: http.StatusConflict, Message: "FORM_SCHEMA_CHANGED"}
	}
	settings, ok := forms.FileField(schema, fieldKey)
	if !ok {
		return uploadCheck{}, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "INVALID_FILE_FIELD"}
	}
	previous, err := q.AttachmentByID(ctx, forms.ParseUUID(uploadID))
	if err == nil {
		if previous.SessionID != session.ID || previous.FieldKey != fieldKey {
			return uploadCheck{}, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "INVALID_FORM_ATTACHMENT"}
		}
		return uploadCheck{session.ID, settings, &previous}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uploadCheck{}, knexError(`select * from "custom_form_attachments" where "id" = $1 limit $2`, err)
	}
	total, err := q.CountFieldAttachments(ctx, dbgen.CountFieldAttachmentsParams{SessionID: session.ID, FieldKey: fieldKey})
	if err != nil {
		return uploadCheck{}, knexError(`select count(*) as "total" from "custom_form_attachments" where "session_id" = $1 and "field_key" = $2`, err)
	}
	if total >= int64(settings.MaxFiles) {
		return uploadCheck{}, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "TOO_MANY_FILES"}
	}
	return uploadCheck{sessionID: session.ID, settings: settings}, nil
}

func presentAttachment(a dbgen.CustomFormAttachment) member.Object {
	return member.KV("id", forms.UUIDString(a.ID), "name", a.OriginalName, "download_name", a.DownloadName, "mime_type", a.MimeType,
		"size", a.SizeBytes, "width", a.Width, "height", a.Height)
}

func (s *Server) formUpload(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	userID, err := s.formUser(r)
	if err != nil {
		formFail(w, err)
		return
	}
	file, _ := body.Get("file")
	upload, isFile := file.File.(*vine.Upload)
	if file.Kind != 'F' || !isFile || !formUploadRule.Valid(upload) {
		formFail(w, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "INVALID_FILE"})
		return
	}
	formID := formNumber(r)
	fieldKey := rawSegment(r, 4)
	token := r.Header.Get("X-Form-Session")
	uploadID := forms.UUID()
	if values, ok := r.Header["X-Form-Upload"]; ok && len(values) > 0 {
		uploadID = strings.Join(values, ", ")
	}
	result, err := s.uploadAttachment(r.Context(), formID, userID, token, fieldKey, uploadID, upload)
	if err != nil {
		formFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, member.KV("data", presentAttachment(result)))
}

func (s *Server) uploadAttachment(ctx context.Context, formID string, userID *int32, token, fieldKey, uploadID string, upload *vine.Upload) (dbgen.CustomFormAttachment, error) {
	if !uploadIDPattern.MatchString(uploadID) {
		return dbgen.CustomFormAttachment{}, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "INVALID_FORM_ATTACHMENT"}
	}
	preflight, err := s.validateUpload(ctx, dbgen.New(s.Pool), false, formID, userID, token, fieldKey, uploadID)
	if err != nil {
		return dbgen.CustomFormAttachment{}, err
	}
	if preflight.previous != nil {
		return *preflight.previous, nil
	}
	processed, err := forms.ProcessFile(upload.Data, preflight.settings)
	if err != nil {
		return dbgen.CustomFormAttachment{}, &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: err.Error()}
	}
	key := "custom-forms/" + formID + "/" + forms.UUID() + "." + processed.Ext
	original := forms.OriginalName(upload.ClientName)
	published := false
	defer func() {
		if published {
			return
		}
		// Resolve any uncertain finalization before deciding whether this
		// attempt owns an orphan.
		var referenced bool
		cleanup := context.WithoutCancel(ctx)
		err := s.inTx(cleanup, func(q *dbgen.Queries) error {
			if _, err := q.LockFormSessionByID(cleanup, preflight.sessionID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			_, err := q.AttachmentWithStorageKey(cleanup, key)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			referenced = err == nil
			return nil
		})
		if err == nil && !referenced {
			err = s.Storage.Delete(cleanup, key)
		}
		if err != nil {
			s.logger().Error("Form upload rollback cleanup failed", "error", err.Error(), "storageKey", key)
		}
	}()
	if err := s.Storage.Put(ctx, key, processed.Contents, storage.Object{ContentType: processed.MimeType, CacheControl: "private, no-store", Private: true}); err != nil {
		return dbgen.CustomFormAttachment{}, &forms.RequestError{Status: http.StatusServiceUnavailable, Message: "UPLOAD_FAILED"}
	}
	var result dbgen.CustomFormAttachment
	err = s.inTx(ctx, func(q *dbgen.Queries) error {
		checked, err := s.validateUpload(ctx, q, true, formID, userID, token, fieldKey, uploadID)
		if err != nil {
			return err
		}
		if checked.previous != nil {
			result = *checked.previous
			return nil
		}
		row, err := q.InsertAttachment(ctx, dbgen.InsertAttachmentParams{ID: forms.ParseUUID(uploadID), SessionID: checked.sessionID, FieldKey: fieldKey,
			StorageKey: key, OriginalName: original, DownloadName: forms.DownloadName(original, processed.Ext), MimeType: processed.MimeType,
			SizeBytes: int32(len(processed.Contents)), SourceSizeBytes: int32(len(upload.Data)), Width: processed.Width, Height: processed.Height})
		if err == nil {
			result = row
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return knexError(`insert into "custom_form_attachments" ("download_name", "field_key", "height", "id", "mime_type", "original_name", "session_id", "size_bytes", "source_size_bytes", "storage_key", "width") values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) on conflict ("id") do nothing returning *`, err)
		}
		winner, err := q.AttachmentByID(ctx, forms.ParseUUID(uploadID))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || winner.SessionID != checked.sessionID || winner.FieldKey != fieldKey {
			return &forms.RequestError{Status: http.StatusUnprocessableEntity, Message: "INVALID_FORM_ATTACHMENT"}
		}
		result = winner
		return nil
	})
	if err != nil {
		return dbgen.CustomFormAttachment{}, err
	}
	published = result.StorageKey == key
	return result, nil
}

func (s *Server) formRemoveUpload(w http.ResponseWriter, r *http.Request) {
	userID, err := s.formUser(r)
	if err != nil {
		formFail(w, err)
		return
	}
	formID, attachmentID := rawSegment(r, 2), rawSegment(r, 4)
	err = s.inTx(r.Context(), func(q *dbgen.Queries) error {
		form, err := q.CustomFormByIDForShare(r.Context(), formID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &forms.RequestError{Status: http.StatusNotFound, Message: "CUSTOM_FORM_NOT_FOUND"}
		}
		if err != nil {
			return knexError(`select * from "custom_forms" where "id" = $1 limit $2 for share`, err)
		}
		session, err := forms.Session(r.Context(), q, form, r.Header.Get("X-Form-Session"), userID, true, s.now())
		if err != nil {
			return err
		}
		file, err := q.UnclaimedSessionAttachmentForUpdate(r.Context(), dbgen.UnclaimedSessionAttachmentForUpdateParams{ID: attachmentID, SessionID: session.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return &forms.RequestError{Status: http.StatusNotFound, Message: "ATTACHMENT_NOT_FOUND"}
		}
		if err != nil {
			return knexError(`select * from "custom_form_attachments" where "id" = $1 and "session_id" = $2 and "claimed_at" is null limit $3 for update`, err)
		}
		if err := s.Storage.Delete(r.Context(), file.StorageKey); err != nil {
			return err
		}
		if err := q.DeleteAttachment(r.Context(), file.ID); err != nil {
			return knexError(`delete from "custom_form_attachments" where "id" = $1`, err)
		}
		return nil
	})
	if err != nil {
		formFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, message{"FILE_REMOVED"})
}

package webapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"kaderisasi/web/internal/certificate"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/media"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/storage"
	"kaderisasi/web/internal/validators"
	"kaderisasi/web/internal/vine"
)

// Phase 3: profiles_controller.ts.
func (s *Server) registerProfiles() {
	s.mux.Handle("GET /v2/profiles", s.RequireAuth(http.HandlerFunc(s.showProfile)))
	s.mux.Handle("PUT /v2/profiles", s.RequireAuth(http.HandlerFunc(s.updateProfile)))
	s.mux.Handle("GET /v2/profiles/activities", s.RequireAuth(http.HandlerFunc(s.profileActivities)))
	s.mux.Handle("POST /v2/profiles/picture", s.RequireAuth(http.HandlerFunc(s.uploadPicture)))
}

func learner(r *http.Request) dbgen.PublicUser {
	l, _ := LearnerFrom(r.Context())
	return l.User
}

// errRowNotFound is Lucid's E_ROW_NOT_FOUND message.
var errRowNotFound = errors.New("Row not found")

// validationErrors is Adonis's rendering of an uncaught E_VALIDATION_ERROR.
func validationErrors(w http.ResponseWriter, err *vine.Error) {
	writeJSON(w, http.StatusUnprocessableEntity, struct {
		Errors []vine.Issue `json:"errors"`
	}{err.Issues})
}

func (s *Server) showProfile(w http.ResponseWriter, r *http.Request) {
	db := dbgen.New(s.Pool)
	user := learner(r)
	userData, err := db.PublicUserByID(r.Context(), user.ID)
	var userJSON any
	if err == nil {
		userJSON = member.User(userData, s.Config.Location)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		failure(w, err)
		return
	}
	profile, err := member.ProfileFor(r.Context(), db, user.ID)
	if err == nil && profile == nil {
		err = errRowNotFound
	}
	if err != nil {
		failure(w, err)
		return
	}
	out, err := s.profileWithRelations(r.Context(), db, *profile)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.KV("userData", userJSON, "profile", out)))
}

// profileWithRelations appends the preloaded university, province and city.
func (s *Server) profileWithRelations(ctx context.Context, db *dbgen.Queries, p dbgen.Profile) (member.Object, error) {
	out := member.Profile(p, s.Config.Location)
	var university, province, city any
	if p.UniversityID != nil {
		rows, err := db.UniversitiesByIDs(ctx, []int32{*p.UniversityID})
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			university = member.KV("id", rows[0].ID, "name", rows[0].Name)
		}
	}
	if p.ProvinceID != nil {
		rows, err := db.ProvincesByIDs(ctx, []int32{*p.ProvinceID})
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			province = member.KV("id", rows[0].ID, "name", rows[0].Name)
		}
	}
	if p.CityID != nil {
		rows, err := db.CitiesByIDs(ctx, []int32{*p.CityID})
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			city = member.KV("id", rows[0].ID, "province_id", rows[0].ProvinceID, "name", rows[0].Name)
		}
	}
	return append(out, member.KV("university", university, "province", province, "city", city)...), nil
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.UpdateProfile, body)
	if err != nil {
		failure(w, err)
		return
	}
	db := dbgen.New(s.Pool)
	profile, err := member.ProfileFor(r.Context(), db, learner(r).ID)
	if err == nil && profile == nil {
		err = errRowNotFound
	}
	if err != nil {
		failure(w, err)
		return
	}
	updated, err := member.UpdateProfile(r.Context(), s.Pool, *profile, payload, s.Config.Location)
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "UPDATE_DATA_SUCCESS", "data", updated))
}

// statusHidden is the status_visibility check in profiles_controller.ts:
// hidden while is_visible is false and visible_at is unset or in the future.
func statusHidden(config js.Value, now time.Time) (hidden bool, visibleAt any) {
	visibility, ok := config.Get("status_visibility")
	if !ok || !visibility.Truthy() {
		return false, nil
	}
	isVisible, ok := visibility.Get("is_visible")
	if !ok || isVisible.Kind != 'b' || isVisible.Bool {
		return false, nil
	}
	at, _ := visibility.Get("visible_at")
	if !at.Truthy() {
		return true, nil
	}
	when, valid := js.ParseDate(at)
	if !valid || !now.Before(when) {
		return false, nil
	}
	return true, at
}

func (s *Server) profileActivities(w http.ResponseWriter, r *http.Request) {
	db := dbgen.New(s.Pool)
	userID := learner(r).ID
	registrations, err := db.RegistrationsByUser(r.Context(), &userID)
	if err != nil {
		generalStack(w, err)
		return
	}
	activityIDs, registrationIDs := []int32{}, []int32{}
	for _, reg := range registrations {
		if reg.ActivityID != nil {
			activityIDs = append(activityIDs, *reg.ActivityID)
		}
		registrationIDs = append(registrationIDs, reg.ID)
	}
	activities := map[int32]dbgen.Activity{}
	issued := map[int32]dbgen.IssuedCertificate{}
	if len(registrations) > 0 {
		rows, err := db.ActivitiesByIDs(r.Context(), activityIDs)
		if err != nil {
			generalStack(w, err)
			return
		}
		for _, a := range rows {
			activities[a.ID] = a
		}
		certs, err := db.IssuedCertificatesByRegistrations(r.Context(), registrationIDs)
		if err != nil {
			generalStack(w, err)
			return
		}
		for _, c := range certs {
			issued[c.RegistrationID] = c // ordered by id: the last one wins
		}
	}
	now := s.now()
	out := []member.Object{}
	for _, reg := range registrations {
		var activityJSON any
		config := js.Value{Kind: 'n'}
		if reg.ActivityID != nil {
			if a, ok := activities[*reg.ActivityID]; ok {
				activityJSON = member.Activity(a, s.Config.Location)
				if v, ok := member.JSONB(a.AdditionalConfig).(js.Value); ok {
					config = v
				}
			}
		}
		o := append(member.Registration(reg, s.Config.Location), member.KV("activity", activityJSON)...)
		if hidden, visibleAt := statusHidden(config, now); hidden {
			o = o.Set("status", "BELUM DIUMUMKAN")
			o = append(o, member.KV("visible_at", visibleAt)...)
			out = append(out, o)
			continue
		}
		state, err := ownerState(reg, issued, s.Config.Location)
		if err != nil {
			generalStack(w, err)
			return
		}
		out = append(out, append(o, member.KV("certificate_state", state.Get("state"), "certificate_code", state.Get("certificate_code"),
			"certificate_issued_at", state.Get("issued_at"), "certificate_revoked_at", state.Get("revoked_at"))...))
	}
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", out))
}

// ownerFields is serializeOwnerCertificateState's result.
type ownerFields map[string]any

func (o ownerFields) Get(k string) any { return o[k] }

func ownerState(reg dbgen.ActivityRegistration, issued map[int32]dbgen.IssuedCertificate, zone *time.Location) (ownerFields, error) {
	c, ok := issued[reg.ID]
	approved := false
	if ok {
		var err error
		if approved, err = certificate.HasApproval(c); err != nil {
			return nil, err
		}
	}
	if !ok || !approved {
		state := "not_eligible"
		if reg.Status != nil && *reg.Status == "LULUS KEGIATAN" {
			state = "eligible_not_issued"
		}
		return ownerFields{"state": state, "registration_id": reg.ID, "certificate_code": nil, "issued_at": nil, "revoked_at": nil}, nil
	}
	state := "issued_active"
	var revoked any
	if c.RevokedAt.Valid {
		state = "issued_revoked"
		revoked = member.ISO(c.RevokedAt.Time, zone)
	}
	return ownerFields{"state": state, "registration_id": reg.ID, "certificate_code": c.CertificateCode, "issued_at": member.ISO(c.IssuedAt.Time, zone), "revoked_at": revoked}, nil
}

// generalStack is profiles_controller.ts activities' catch: it returns
// error.stack, which has no Go equivalent; the message stands in for it.
func generalStack(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}{"GENERAL_ERROR", err.Error()})
}

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *Server) uploadPicture(w http.ResponseWriter, r *http.Request) {
	body, ok := s.body(w, r)
	if !ok {
		return
	}
	payload, err := vine.ValidateObject(validators.Image, body)
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		validationErrors(w, invalid) // validateUsing runs before the try block
		return
	}
	file, _ := payload.Get("file")
	upload := file.File.(*vine.Upload)
	key, err := s.replaceProfilePicture(r.Context(), learner(r).ID, upload.Data)
	if errors.Is(err, media.ErrInvalidImage) {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_PICTURE"})
		return
	}
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "UPLOAD_SUCCESS", "data", member.KV("picture", key)))
}

// replaceProfilePicture is profile_picture_service.ts: store the optimised
// image under a new key, point the profile at it, then remove the previous
// object once nothing references it.
func (s *Server) replaceProfilePicture(ctx context.Context, userID int32, input []byte) (string, error) {
	db := dbgen.New(s.Pool)
	if p, err := member.ProfileFor(ctx, db, userID); err != nil || p == nil {
		if err == nil {
			err = errRowNotFound
		}
		return "", err
	}
	contents, err := media.Optimize(input, media.ProfilePicture)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("profile/%d/%s.webp", userID, uuid())
	var previous *string
	err = s.Storage.Put(ctx, key, contents, storage.Object{ContentType: "image/webp", CacheControl: "public, max-age=31536000, immutable", PublicRead: true})
	if err == nil {
		err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			p, err := q.ProfileForUpdate(ctx, &userID)
			if errors.Is(err, pgx.ErrNoRows) {
				return errRowNotFound
			}
			if err != nil {
				return err
			}
			previous = p.Picture
			if p.Picture != nil && *p.Picture == key {
				return nil
			}
			return q.SetProfilePicture(ctx, dbgen.SetProfilePictureParams{ID: p.ID, Picture: &key, UpdatedAt: member.Timestamp(member.Now())})
		})
	}
	if err != nil {
		s.removeUnreferencedPicture(ctx, key)
		return "", err
	}
	if previous != nil && *previous != "" && *previous != key {
		s.removeUnreferencedPicture(ctx, *previous)
	}
	return key, nil
}

func (s *Server) removeUnreferencedPicture(ctx context.Context, key string) {
	ctx = context.WithoutCancel(ctx)
	_, err := dbgen.New(s.Pool).ProfileUsingPicture(ctx, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.Storage.Delete(ctx, key)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger().Warn("Profile picture cleanup deferred; reference check or deletion failed", "key", key)
	}
}

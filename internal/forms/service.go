package forms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
)

// RequestError is FormRequestError: a status and a message.
type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string { return e.Message }

func fail(status int, message string) error { return &RequestError{status, message} }

// Schema parses a form's form_schema.
func Schema(f dbgen.CustomForm) js.Value {
	v, err := js.Parse(f.FormSchema)
	if err != nil {
		return js.Value{Kind: 'n'}
	}
	return v
}

var jakarta = func() *time.Location {
	zone, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return zone
}()

func isoDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// ActivityOpen is isActivityRegistrationOpen.
func ActivityOpen(a dbgen.Activity, now time.Time) bool {
	today := now.In(jakarta).Format("2006-01-02")
	start, end := isoDate(a.RegistrationStart), isoDate(a.RegistrationEnd)
	return a.IsPublished != nil && *a.IsPublished && a.IsRegistrationOpen && start != "" && end != "" && start <= today && today <= end
}

// ClubOpen is isClubRegistrationOpen.
func ClubOpen(c dbgen.Club, now time.Time) bool {
	if c.IsShow == nil || !*c.IsShow || c.IsRegistrationOpen == nil || !*c.IsRegistrationOpen {
		return false
	}
	if !c.RegistrationEndDate.Valid {
		return true
	}
	return c.RegistrationEndDate.Time.Format("2006-01-02") >= now.In(jakarta).Format("2006-01-02")
}

func guestAllowed(a dbgen.Activity) bool {
	config, err := js.Parse(a.AdditionalConfig)
	if err != nil {
		return false
	}
	allow, _ := get(config, "allow_guest_registration")
	return a.ActivityType != nil && *a.ActivityType == 1 && truthy(allow)
}

// GuestRegistrationAllowed is the guest check of guestRegister.
func GuestRegistrationAllowed(a dbgen.Activity) bool { return guestAllowed(a) }

func featureID(f dbgen.CustomForm) string {
	if f.FeatureID == nil {
		return "0"
	}
	return fmt.Sprint(*f.FeatureID)
}

// RequireOpen is requireOpenForm.
func RequireOpen(ctx context.Context, q *dbgen.Queries, f dbgen.CustomForm, userID *int32, now time.Time) error {
	if f.IsActive == nil || !*f.IsActive {
		return fail(409, "FORM_CLOSED")
	}
	kind := ""
	if f.FeatureType != nil {
		kind = *f.FeatureType
	}
	switch kind {
	case "independent_form":
		settings, _ := get(Schema(f), "settings")
		mode, _ := get(settings, "accessMode")
		if !(mode.Kind == 's' && mode.String == "public") && userID == nil {
			return fail(401, "LOGIN_REQUIRED")
		}
	case "activity_registration":
		a, err := q.ActivityByID(ctx, featureID(f))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || !ActivityOpen(a, now) {
			return fail(409, "REGISTRATION_CLOSED")
		}
		if userID == nil && !guestAllowed(a) {
			return fail(403, "GUEST_REGISTRATION_NOT_ALLOWED")
		}
	case "club_registration":
		if userID == nil {
			return fail(401, "LOGIN_REQUIRED")
		}
		c, err := q.ClubByID(ctx, featureID(f))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || !ClubOpen(c, now) {
			return fail(409, "REGISTRATION_CLOSED")
		}
	default:
		return fail(404, "CUSTOM_FORM_NOT_FOUND")
	}
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UUID is crypto.randomUUID.
func UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// CreateSession is createFormSession.
func CreateSession(ctx context.Context, q *dbgen.Queries, f dbgen.CustomForm, userID *int32, now time.Time) (token, hash string, err error) {
	if err := RequireOpen(ctx, q, f, userID, now); err != nil {
		return "", "", err
	}
	token = randomHex(32)
	hash = SchemaHash(Schema(f))
	err = q.CreateFormSession(ctx, dbgen.CreateFormSessionParams{ID: pgUUID(UUID()), FormID: f.ID, UserID: userID, TokenHash: TokenHash(token),
		SchemaHash: hash, ExpiresAt: pgtype.Timestamptz{Time: now.Add(24 * time.Hour).Truncate(time.Millisecond), Valid: true}})
	return token, hash, err
}

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

var tokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func sameUser(a *int32, b *int32) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Session is lockFormSession (lock=true) or readFormSession.
func Session(ctx context.Context, q *dbgen.Queries, f dbgen.CustomForm, token string, userID *int32, lock bool, now time.Time) (dbgen.CustomFormSession, error) {
	if !tokenPattern.MatchString(token) {
		return dbgen.CustomFormSession{}, fail(422, "INVALID_FORM_SESSION")
	}
	var s dbgen.CustomFormSession
	var err error
	if lock {
		s, err = q.LockFormSession(ctx, dbgen.LockFormSessionParams{TokenHash: TokenHash(token), FormID: f.ID})
	} else {
		s, err = q.ReadFormSession(ctx, dbgen.ReadFormSessionParams{TokenHash: TokenHash(token), FormID: f.ID})
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	if err != nil || !sameUser(s.UserID, userID) || !s.ExpiresAt.Time.Truncate(time.Millisecond).After(now.Truncate(time.Millisecond)) {
		return s, fail(422, "INVALID_FORM_SESSION")
	}
	return s, nil
}

// FileSettings is a valid FormFileSettings.
type FileSettings struct {
	Accept              string
	MaxFiles, MaxSizeMB int
}

// ValidFileSettings is validFileSettings.
func ValidFileSettings(v js.Value) (FileSettings, bool) {
	if !truthy(v) || (v.Kind != 'o' && v.Kind != 'a') {
		return FileSettings{}, false
	}
	accept, _ := get(v, "accept")
	files, _ := get(v, "maxFiles")
	size, _ := get(v, "maxSizeMB")
	integer := func(x js.Value, lo, hi float64) bool {
		return x.Kind == 'f' && x.Number == math.Trunc(x.Number) && x.Number >= lo && x.Number <= hi
	}
	if accept.Kind != 's' || (accept.String != "pdf" && accept.String != "image" && accept.String != "pdf_or_image") || !integer(files, 1, 5) || !integer(size, 1, 10) {
		return FileSettings{}, false
	}
	return FileSettings{accept.String, int(files.Number), int(size.Number)}, true
}

// fileFields are the visible file fields of a schema (UploadSchema.fields).
func fileFields(schema js.Value) []js.Value {
	sections, _ := get(schema, "fields")
	var out []js.Value
	for _, section := range sections.Items {
		for _, field := range fieldsOf(section) {
			t, _ := get(field, "type")
			hidden, _ := get(field, "hidden")
			disabled, _ := get(field, "disabled")
			if t.Kind == 's' && t.String == "file" && !truthy(hidden) && !truthy(disabled) {
				out = append(out, field)
			}
		}
	}
	return out
}

// FileField finds the visible file field with the given key.
func FileField(schema js.Value, key string) (FileSettings, bool) {
	for _, field := range fileFields(schema) {
		k, _ := get(field, "key")
		if k.Kind == 's' && k.String == key {
			file, _ := get(field, "file")
			return ValidFileSettings(file)
		}
	}
	return FileSettings{}, false
}

var attachmentID = regexp.MustCompile(`^[a-f0-9-]{36}$`)

func contains(list js.Value, id string) bool {
	if list.Kind != 'a' {
		return false
	}
	for _, item := range list.Items {
		if item.Kind == 's' && item.String == id {
			return true
		}
	}
	return false
}

// Claim is claimFormAttachments. token == nil means undefined.
func Claim(ctx context.Context, q *dbgen.Queries, f dbgen.CustomForm, token *string, userID *int32, answers js.Value, previous *js.Value, now time.Time) error {
	schema := Schema(f)
	fields := fileFields(schema)
	hasToken := token != nil && *token != ""
	if len(fields) == 0 && !hasToken {
		return nil
	}
	if !hasToken && previous == nil {
		return fail(422, "INVALID_FORM_SESSION")
	}
	var session *dbgen.CustomFormSession
	if hasToken {
		s, err := Session(ctx, q, f, *token, userID, true, now)
		if err != nil {
			return err
		}
		session = &s
	}
	if session != nil && session.CompletedAt.Valid {
		return fail(409, "FORM_SESSION_COMPLETED")
	}
	if session != nil && session.SchemaHash != SchemaHash(schema) {
		return fail(409, "FORM_SCHEMA_CHANGED")
	}
	for _, field := range fields {
		key, _ := get(field, "key")
		ids, ok := answers.Get(key.String)
		if !ok {
			continue
		}
		file, _ := get(field, "file")
		settings, valid := ValidFileSettings(file)
		if !valid || ids.Kind != 'a' || len(ids.Items) > settings.MaxFiles {
			return fail(422, "INVALID_FORM_ATTACHMENT")
		}
		seen := map[string]bool{}
		for _, id := range ids.Items {
			if id.Kind != 's' || !attachmentID.MatchString(id.String) || seen[id.String] {
				return fail(422, "INVALID_FORM_ATTACHMENT")
			}
			seen[id.String] = true
		}
		var before js.Value
		if previous != nil {
			before, _ = previous.Get(key.String)
		}
		var retained, fresh []pgtype.UUID
		var freshIDs []string
		for _, id := range ids.Items {
			if contains(before, id.String) {
				retained = append(retained, pgUUID(id.String))
			} else {
				fresh = append(fresh, pgUUID(id.String))
				freshIDs = append(freshIDs, id.String)
			}
		}
		if len(retained) > 0 {
			rows, err := q.RetainedAttachments(ctx, dbgen.RetainedAttachmentsParams{FormID: f.ID, UserID: userID, FieldKey: key.String, Ids: retained})
			if err != nil {
				return err
			}
			if len(rows) != len(retained) {
				return fail(422, "INVALID_FORM_ATTACHMENT")
			}
		}
		if len(fresh) == 0 {
			continue
		}
		if session == nil {
			return fail(422, "INVALID_FORM_SESSION")
		}
		files, err := q.UnclaimedAttachmentsForUpdate(ctx, dbgen.UnclaimedAttachmentsForUpdateParams{SessionID: session.ID, FieldKey: key.String, Ids: fresh})
		if err != nil {
			return err
		}
		if len(files) != len(fresh) {
			return fail(422, "INVALID_FORM_ATTACHMENT")
		}
		for _, file := range files {
			if int64(file.SourceSizeBytes) > int64(settings.MaxSizeMB)*1024*1024 ||
				(settings.Accept == "pdf" && file.MimeType != "application/pdf") || (settings.Accept == "image" && file.MimeType != "image/webp") {
				return fail(422, "INVALID_FORM_ATTACHMENT")
			}
		}
		if err := q.ClaimAttachments(ctx, dbgen.ClaimAttachmentsParams{At: pgtype.Timestamptz{Time: now.Truncate(time.Millisecond), Valid: true}, Ids: fresh}); err != nil {
			return err
		}
	}
	if session != nil {
		return q.CompleteFormSession(ctx, dbgen.CompleteFormSessionParams{At: pgtype.Timestamptz{Time: time.Now().Truncate(time.Millisecond), Valid: true}, ID: session.ID})
	}
	return nil
}

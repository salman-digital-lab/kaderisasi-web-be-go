package p3ri

// Applicant workflows served by kaderisasi-web-be-go under /v2/p3ri.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/storage"
)

// Error is a workflow failure with an HTTP status and an Indonesian message
// the frontend shows as is.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func fail(status int, format string, args ...any) error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// Service runs the applicant workflows. Now is the clock; nil means time.Now.
type Service struct {
	Pool    *pgxpool.Pool
	Storage storage.Store
	Now     func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// FormView is the form with its open state.
type FormView struct {
	Form
	IsOpen bool `json:"is_open"`
}

func (s *Service) Form() FormView {
	f := Current()
	return FormView{Form: f, IsOpen: f.Open(s.now())}
}

// Registration is the applicant's own view; the admin note stays internal.
type Registration struct {
	ID               int32     `json:"id"`
	Status           string    `json:"status"`
	Choices          []string  `json:"choices"`
	AcceptedDivision *string   `json:"accepted_division"`
	Answers          Answers   `json:"answers"`
	Files            []File    `json:"files"`
	Editable         bool      `json:"editable"`
	SubmittedAt      time.Time `json:"submitted_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	StatusUpdatedAt  *string   `json:"status_updated_at"`
}

func (s *Service) view(r dbgen.P3riRegistration) Registration {
	out := Registration{
		ID: r.ID, Status: r.Status, Choices: r.Choices, AcceptedDivision: r.AcceptedDivision,
		Files: []File{}, SubmittedAt: r.SubmittedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		Editable: r.Status == "terdaftar" && Current().Open(s.now()),
	}
	_ = json.Unmarshal(r.Answers, &out.Answers)
	if out.Answers.Common == nil {
		out.Answers.Common = map[string]any{}
	}
	if out.Answers.Divisions == nil {
		out.Answers.Divisions = map[string]map[string]any{}
	}
	_ = json.Unmarshal(r.Files, &out.Files)
	if r.StatusUpdatedAt.Valid {
		at := r.StatusUpdatedAt.Time.Format(time.RFC3339)
		out.StatusUpdatedAt = &at
	}
	return out
}

// Mine returns the applicant's registration, or nil before they submit.
func (s *Service) Mine(ctx context.Context, user int32) (*Registration, error) {
	r, err := dbgen.New(s.Pool).P3riRegistrationForUser(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := s.view(r)
	return &v, nil
}

// Save creates or replaces the applicant's answers while registration is open
// and an admin has not moved them past "terdaftar".
func (s *Service) Save(ctx context.Context, user int32, in Input) (Registration, error) {
	f := Current()
	if !f.Open(s.now()) {
		return Registration{}, fail(409, "Pendaftaran sudah ditutup.")
	}
	answers, files, err := f.Validate(in, func(key string) bool { return OwnsKey(user, key) })
	if err != nil {
		return Registration{}, err
	}
	answersJSON, err := json.Marshal(answers)
	if err != nil {
		return Registration{}, err
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return Registration{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Registration{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := dbgen.New(s.Pool).WithTx(tx)

	var saved dbgen.P3riRegistration
	current, err := q.P3riRegistrationForUserForUpdate(ctx, user)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		saved, err = q.InsertP3riRegistration(ctx, dbgen.InsertP3riRegistrationParams{
			UserID: user, Choices: in.Choices, Answers: answersJSON, Files: filesJSON,
		})
	case err != nil:
	case current.Status != "terdaftar":
		err = fail(409, "Pendaftaran sudah diproses panitia sehingga tidak bisa diubah.")
	default:
		saved, err = q.UpdateP3riRegistrationAnswers(ctx, dbgen.UpdateP3riRegistrationAnswersParams{
			UserID: user, Choices: in.Choices, Answers: answersJSON, Files: filesJSON,
		})
	}
	if err != nil {
		return Registration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Registration{}, err
	}
	return s.view(saved), nil
}

// FilePayload is an upload sent as a data URL (the site compresses photos first).
type FilePayload struct {
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Size    int64  `json:"size"`
	DataURL string `json:"dataUrl"`
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9.-]`)

func keyPrefix(user int32) string { return fmt.Sprintf("p3ri/%s/%d/", Current().Code, user) }

// OwnsKey reports whether key is one of the applicant's uploads.
func OwnsKey(user int32, key string) bool {
	rest, ok := strings.CutPrefix(key, keyPrefix(user))
	return ok && rest != "" && !strings.Contains(rest, "/")
}

// Upload stores a photo privately and returns its reference. Uploads happen
// before the registration is saved; Save checks the key prefix.
func (s *Service) Upload(ctx context.Context, user int32, in FilePayload) (File, error) {
	if !Current().Open(s.now()) {
		return File{}, fail(409, "Pendaftaran sudah ditutup.")
	}
	_, encoded, ok := strings.Cut(in.DataURL, ";base64,")
	if !ok || !strings.HasPrefix(in.DataURL, "data:") {
		return File{}, fail(422, "Berkas tidak terbaca.")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return File{}, fail(422, "Berkas tidak terbaca.")
	}
	if len(data) > MaxFile {
		return File{}, fail(413, "Ukuran berkas maksimal 5 MB.")
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		return File{}, fail(422, "Hanya foto (JPG, PNG, atau WebP).")
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	name := unsafeName.ReplaceAllString(strings.TrimSpace(in.Name), "_")
	if len(name) > 80 {
		name = name[len(name)-80:]
	}
	if name == "" {
		name = "berkas"
	}
	key := keyPrefix(user) + hex.EncodeToString(b[:]) + "-" + name
	if err := s.Storage.Put(ctx, key, data, storage.Object{ContentType: mime, CacheControl: "private, no-store", Private: true}); err != nil {
		return File{}, err
	}
	return File{Key: key, Name: safeLabel(in.Name), Mime: mime, Size: int64(len(data))}, nil
}

// File returns one of the applicant's own uploads.
func (s *Service) File(ctx context.Context, user int32, key string) ([]byte, string, error) {
	if !OwnsKey(user, key) {
		return nil, "", fail(404, "Berkas tidak ditemukan.")
	}
	data, mime, err := s.Storage.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, "", fail(404, "Berkas tidak ditemukan.")
	}
	return data, mime, err
}

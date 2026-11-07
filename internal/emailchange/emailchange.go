// Package emailchange lets a member change their account email.
//
// A member with a password confirms it; a member without one (Google-only)
// first enters a 6-digit code sent to their current email. Either way the new
// email takes effect only after the member opens the confirmation link sent
// to it. Requests live in public_user_email_changes; a member has at most one
// open request, and starting a new one cancels the previous one.
//
// Locks are always taken in the same order: member row, then request row,
// then the advisory lock on the new email (the one Google sign-in uses).
package emailchange

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/dbgen"
)

const (
	CodeLifetime = 15 * time.Minute
	LinkLifetime = 60 * time.Minute
	MaxAttempts  = 5
	// ResendAfter is how long a member waits before starting another request,
	// and MaxPerHour how many requests they may start in an hour. Cancelled
	// requests count too: each request emails an address the member typed.
	ResendAfter = time.Minute
	MaxPerHour  = 5
)

// Steps: what the member does next.
const (
	StepCode = "code" // enter the code sent to the current email
	StepLink = "link" // open the link sent to the new email
)

// Error is a refusal the API reports as {message: Code} with Status.
type Error struct {
	Status int
	Code   string
	// Remaining is the number of code attempts left (INVALID_CODE only).
	Remaining int
}

func (e *Error) Error() string { return e.Code }

func fail(status int, code string) error { return &Error{Status: status, Code: code} }

var (
	ErrUnchanged       = fail(400, "EMAIL_UNCHANGED")
	ErrTaken           = fail(409, "EMAIL_ALREADY_REGISTERED")
	ErrWrongPassword   = fail(401, "WRONG_PASSWORD")
	ErrInactive        = fail(403, "ACCOUNT_INACTIVE")
	ErrTooSoon         = fail(429, "EMAIL_CHANGE_TOO_SOON")
	ErrHourlyLimit     = fail(429, "EMAIL_CHANGE_LIMIT")
	ErrNotFound        = fail(404, "EMAIL_CHANGE_NOT_FOUND")
	ErrCodeExpired     = fail(400, "CODE_EXPIRED")
	ErrTooManyAttempts = fail(429, "TOO_MANY_ATTEMPTS")
	ErrInvalidToken    = fail(400, "INVALID_TOKEN")
)

type Service struct {
	Pool *pgxpool.Pool
	// Key is APP_KEY; it keys the stored code hashes.
	Key string
	Now func() time.Time
}

// Started is what the member must do next and the secret to email for it:
// Code goes to OldEmail (StepCode), Token to NewEmail (StepLink).
type Started struct {
	Step      string
	OldEmail  string
	NewEmail  string
	Code      string
	Token     string
	ExpiresAt time.Time
}

// Pending describes an open request, for the profile page.
type Pending struct {
	Step      string
	NewEmail  string
	ExpiresAt time.Time
}

// Confirmed is the member after the change.
type Confirmed struct {
	User     dbgen.PublicUser
	OldEmail string
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().Truncate(time.Microsecond)
	}
	return time.Now().Truncate(time.Microsecond)
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// signedIn reports an account that can log in: active, or a no_account member
// with an email, whom Google sign-in also lets in (ResolveGoogleAccount).
func signedIn(user dbgen.PublicUser) bool {
	return (user.AccountStatus == "active" || user.AccountStatus == "no_account") && user.Email != nil && *user.Email != ""
}

// inTx runs fn in a transaction that commits even when fn returns an *Error,
// so counted attempts and cancellations persist; other errors roll back.
func (s Service) inTx(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result := fn(dbgen.New(tx))
	var refusal *Error
	if result != nil && !errors.As(result, &refusal) {
		return result
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return result
}

// Start opens a request to change userID's email to newEmail, which the
// caller has already trimmed, lowercased and validated.
func (s Service) Start(ctx context.Context, userID int32, newEmail string, password *string) (Started, error) {
	var started Started
	err := s.inTx(ctx, func(q *dbgen.Queries) error {
		now := s.now()
		user, err := q.LockPublicUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInactive
		}
		if err != nil {
			return err
		}
		if !signedIn(user) {
			return ErrInactive
		}
		if strings.EqualFold(*user.Email, newEmail) {
			return ErrUnchanged
		}
		hasPassword := user.Password != nil && *user.Password != ""
		if hasPassword && (password == nil || !auth.VerifyPassword(*user.Password, *password)) {
			return ErrWrongPassword
		}
		recent, err := q.RecentEmailChanges(ctx, dbgen.RecentEmailChangesParams{Since: ts(now.Add(-time.Hour)), UserID: userID})
		if err != nil {
			return err
		}
		if recent.Latest.InfinityModifier == pgtype.Finite && now.Sub(recent.Latest.Time) < ResendAfter {
			return ErrTooSoon
		}
		if recent.InWindow >= MaxPerHour {
			return ErrHourlyLimit
		}
		if err := s.ensureFree(ctx, q, userID, newEmail); err != nil {
			return err
		}
		if err := q.CancelOpenEmailChange(ctx, dbgen.CancelOpenEmailChangeParams{Now: ts(now), UserID: userID}); err != nil {
			return err
		}
		started = Started{OldEmail: *user.Email, NewEmail: newEmail}
		params := dbgen.CreateEmailChangeParams{UserID: userID, OldEmail: *user.Email, NewEmail: newEmail, Now: ts(now)}
		if hasPassword {
			token, hash, err := newToken()
			if err != nil {
				return err
			}
			started.Step, started.Token, started.ExpiresAt = StepLink, token, now.Add(LinkLifetime)
			params.TokenHash, params.TokenExpiresAt = &hash, ts(started.ExpiresAt)
		} else {
			code, err := newCode()
			if err != nil {
				return err
			}
			hash := s.codeHash(userID, newEmail, code)
			started.Step, started.Code, started.ExpiresAt = StepCode, code, now.Add(CodeLifetime)
			params.CodeHash, params.CodeExpiresAt = &hash, ts(started.ExpiresAt)
		}
		_, err = q.CreateEmailChange(ctx, params)
		return err
	})
	return started, err
}

// VerifyCode checks the code a passwordless member received at their
// current email and, when it matches, issues the confirmation link.
func (s Service) VerifyCode(ctx context.Context, userID int32, code string) (Started, error) {
	var started Started
	err := s.inTx(ctx, func(q *dbgen.Queries) error {
		now := s.now()
		if _, err := q.LockPublicUser(ctx, userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		open, err := q.OpenEmailChange(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if open.CodeHash == nil || open.CodeVerifiedAt.Valid {
			return ErrNotFound
		}
		if !now.Before(open.CodeExpiresAt.Time) {
			if err := q.CancelEmailChange(ctx, dbgen.CancelEmailChangeParams{Now: ts(now), ID: open.ID}); err != nil {
				return err
			}
			return ErrCodeExpired
		}
		want := s.codeHash(userID, open.NewEmail, strings.TrimSpace(code))
		if !hmac.Equal([]byte(want), []byte(*open.CodeHash)) {
			attempts, err := q.CountEmailChangeAttempt(ctx, dbgen.CountEmailChangeAttemptParams{Now: ts(now), ID: open.ID})
			if err != nil {
				return err
			}
			if attempts >= MaxAttempts {
				if err := q.CancelEmailChange(ctx, dbgen.CancelEmailChangeParams{Now: ts(now), ID: open.ID}); err != nil {
					return err
				}
				return ErrTooManyAttempts
			}
			return &Error{Status: 400, Code: "INVALID_CODE", Remaining: MaxAttempts - int(attempts)}
		}
		token, hash, err := newToken()
		if err != nil {
			return err
		}
		started = Started{Step: StepLink, OldEmail: open.OldEmail, NewEmail: open.NewEmail, Token: token, ExpiresAt: now.Add(LinkLifetime)}
		return q.VerifyEmailChangeCode(ctx, dbgen.VerifyEmailChangeCodeParams{Now: ts(now), TokenHash: &hash, TokenExpiresAt: ts(started.ExpiresAt), ID: open.ID})
	})
	return started, err
}

// Confirm applies the change named by the link token. It works without a
// session, so the link can be opened on any device.
func (s Service) Confirm(ctx context.Context, token string) (Confirmed, error) {
	var confirmed Confirmed
	err := s.inTx(ctx, func(q *dbgen.Queries) error {
		now := s.now()
		if token == "" {
			return ErrInvalidToken
		}
		hash := tokenHash(token)
		found, err := q.EmailChangeByToken(ctx, &hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		user, err := q.LockPublicUser(ctx, found.UserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		// Re-read under lock: the request must still be the open one.
		open, err := q.OpenEmailChange(ctx, found.UserID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && open.ID != found.ID) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		cancel := func(refusal error) error {
			if err := q.CancelEmailChange(ctx, dbgen.CancelEmailChangeParams{Now: ts(now), ID: open.ID}); err != nil {
				return err
			}
			return refusal
		}
		if !open.TokenExpiresAt.Valid || !now.Before(open.TokenExpiresAt.Time) {
			return cancel(ErrInvalidToken)
		}
		if !signedIn(user) || *user.Email != open.OldEmail {
			return cancel(ErrInvalidToken)
		}
		if err := s.ensureFree(ctx, q, user.ID, open.NewEmail); err != nil {
			var refusal *Error
			if errors.As(err, &refusal) {
				return cancel(err)
			}
			return err
		}
		updated, err := q.SetPublicUserEmail(ctx, dbgen.SetPublicUserEmailParams{Email: &open.NewEmail, Now: ts(now), ID: user.ID})
		if isUniqueViolation(err) {
			return cancel(ErrTaken)
		}
		if err != nil {
			return err
		}
		if err := q.UseEmailChange(ctx, dbgen.UseEmailChangeParams{Now: ts(now), ID: open.ID}); err != nil {
			return err
		}
		confirmed = Confirmed{User: updated, OldEmail: open.OldEmail}
		return nil
	})
	return confirmed, err
}

// PendingFor returns the member's open request, or nil.
func (s Service) PendingFor(ctx context.Context, userID int32) (*Pending, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	open, err := dbgen.New(tx).OpenEmailChange(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pending := Pending{Step: StepLink, NewEmail: open.NewEmail, ExpiresAt: open.TokenExpiresAt.Time}
	if open.CodeHash != nil && !open.CodeVerifiedAt.Valid {
		pending.Step, pending.ExpiresAt = StepCode, open.CodeExpiresAt.Time
	}
	if !s.now().Before(pending.ExpiresAt) {
		return nil, nil
	}
	return &pending, nil
}

// Cancel withdraws the member's open request, if any.
func (s Service) Cancel(ctx context.Context, userID int32) error {
	return dbgen.New(s.Pool).CancelOpenEmailChange(ctx, dbgen.CancelOpenEmailChangeParams{Now: ts(s.now()), UserID: userID})
}

func (s Service) ensureFree(ctx context.Context, q *dbgen.Queries, userID int32, email string) error {
	if err := q.LockEmail(ctx, email); err != nil {
		return err
	}
	_, err := q.OtherPublicUserByLowerEmail(ctx, dbgen.OtherPublicUserByLowerEmailParams{Email: email, ID: userID})
	if err == nil {
		return ErrTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s Service) codeHash(userID int32, newEmail, code string) string {
	mac := hmac.New(sha256.New, []byte(s.Key))
	fmt.Fprintf(mac, "email-change:%d:%s:%s", userID, newEmail, code)
	return hex.EncodeToString(mac.Sum(nil))
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// newToken returns a random link token and the hash that is stored.
func newToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, tokenHash(token), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

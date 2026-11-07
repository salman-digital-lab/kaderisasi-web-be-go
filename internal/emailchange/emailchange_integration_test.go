//go:build integration

// Email change workflows on a real PostgreSQL in an owned email_go_<run>
// schema (explicit search path, no public fallback). Run with the test
// environment exported:
//
//	set -a; . ../env/test/web-be-test-env; set +a
//	go test -tags=integration -count=1 ./internal/emailchange/
package emailchange

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/database"
)

const usersDDL = `
CREATE TABLE public_users (
  id serial PRIMARY KEY, email varchar(255) UNIQUE, password varchar(255),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz,
  member_id varchar(255), account_status varchar(20) NOT NULL DEFAULT 'no_account');
CREATE UNIQUE INDEX public_users_lower_email_unique ON public_users (lower(email));
`

// changesDDL is the email change part of the sqlc snapshot, unqualified so
// it lands in the owned schema.
func changesDDL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../database/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	start := strings.Index(s, "CREATE TABLE public.public_user_email_changes")
	end := strings.Index(s, "WHERE used_at IS NULL AND cancelled_at IS NULL;")
	if start < 0 || end < start {
		t.Fatal("public_user_email_changes missing from schema.sql")
	}
	return strings.ReplaceAll(s[start:end+len("WHERE used_at IS NULL AND cancelled_at IS NULL;")], "public.", "")
}

type fixture struct {
	svc Service
	now *time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("DB_HOST") == "" {
		t.Skip("needs the test database environment (../env/test/web-be-test-env)")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "email_go_" + hex.EncodeToString(b)
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c.DBSchema = "public"
	bootstrap, err := database.Open(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		bootstrap.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := bootstrap.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("cleanup of %s failed: %v", schema, err)
		} else {
			t.Logf("cleaned owned schema %s", schema)
		}
		bootstrap.Close()
	})
	c.DBSchema = schema
	pool, err := database.Open(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, usersDDL+changesDDL(t)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	return fixture{svc: Service{Pool: pool, Key: "synthetic-test-key", Now: func() time.Time { return now }}, now: &now}
}

func (f fixture) advance(d time.Duration) { *f.now = f.now.Add(d) }

func (f fixture) user(t *testing.T, email, password string) int32 {
	t.Helper()
	var hash *string
	if password != "" {
		h, err := auth.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		hash = &h
	}
	var id int32
	if err := f.svc.Pool.QueryRow(context.Background(), "INSERT INTO public_users (email,password,account_status) VALUES ($1,$2,'active') RETURNING id", email, hash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) email(t *testing.T, id int32) string {
	t.Helper()
	var email string
	if err := f.svc.Pool.QueryRow(context.Background(), "SELECT email FROM public_users WHERE id=$1", id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

func is(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func code(t *testing.T, err error) string {
	t.Helper()
	var refusal *Error
	if !errors.As(err, &refusal) {
		t.Fatalf("not a refusal: %v", err)
	}
	return refusal.Code
}

func ptr(s string) *string { return &s }

func TestPasswordMemberChangesEmailByLink(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.user(t, "lama@example.test", "Rahasia-2026!")
	f.user(t, "dipakai@example.test", "")

	_, err := f.svc.Start(ctx, id, "baru@example.test", nil)
	is(t, err, ErrWrongPassword)
	_, err = f.svc.Start(ctx, id, "baru@example.test", ptr("salah"))
	is(t, err, ErrWrongPassword)
	_, err = f.svc.Start(ctx, id, "LAMA@example.test", ptr("Rahasia-2026!"))
	is(t, err, ErrUnchanged)
	_, err = f.svc.Start(ctx, id, "dipakai@example.test", ptr("Rahasia-2026!"))
	is(t, err, ErrTaken)

	first, err := f.svc.Start(ctx, id, "baru@example.test", ptr("Rahasia-2026!"))
	if err != nil || first.Step != StepLink || first.Token == "" || first.Code != "" || first.OldEmail != "lama@example.test" {
		t.Fatalf("start %+v %v", first, err)
	}
	_, err = f.svc.Start(ctx, id, "baru@example.test", ptr("Rahasia-2026!"))
	is(t, err, ErrTooSoon)
	if p, err := f.svc.PendingFor(ctx, id); err != nil || p == nil || p.Step != StepLink || p.NewEmail != "baru@example.test" {
		t.Fatalf("pending %+v %v", p, err)
	}

	// A newer request replaces the first; its link stops working.
	f.advance(ResendAfter)
	second, err := f.svc.Start(ctx, id, "baru2@example.test", ptr("Rahasia-2026!"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Confirm(ctx, first.Token)
	is(t, err, ErrInvalidToken)
	_, err = f.svc.Confirm(ctx, "not-a-token")
	is(t, err, ErrInvalidToken)

	done, err := f.svc.Confirm(ctx, second.Token)
	if err != nil || *done.User.Email != "baru2@example.test" || done.OldEmail != "lama@example.test" || f.email(t, id) != "baru2@example.test" {
		t.Fatalf("confirm %+v %v", done, err)
	}
	_, err = f.svc.Confirm(ctx, second.Token)
	is(t, err, ErrInvalidToken)
	if p, err := f.svc.PendingFor(ctx, id); err != nil || p != nil {
		t.Fatalf("pending after use %+v %v", p, err)
	}
}

func TestPasswordlessMemberConfirmsCodeFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.user(t, "google@gmail.com", "")

	started, err := f.svc.Start(ctx, id, "kantor@example.test", nil)
	if err != nil || started.Step != StepCode || len(started.Code) != 6 || started.Token != "" {
		t.Fatalf("start %+v %v", started, err)
	}
	wrong := "000000"
	if started.Code == wrong {
		wrong = "111111"
	}
	for left := MaxAttempts - 1; left >= 1; left-- {
		_, err := f.svc.VerifyCode(ctx, id, wrong)
		var refusal *Error
		if !errors.As(err, &refusal) || refusal.Code != "INVALID_CODE" || refusal.Remaining != left {
			t.Fatalf("attempt with %d left: %v", left, err)
		}
	}
	_, err = f.svc.VerifyCode(ctx, id, wrong)
	is(t, err, ErrTooManyAttempts)
	_, err = f.svc.VerifyCode(ctx, id, started.Code)
	is(t, err, ErrNotFound)

	f.advance(ResendAfter)
	started, err = f.svc.Start(ctx, id, "kantor@example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := f.svc.PendingFor(ctx, id); p == nil || p.Step != StepCode {
		t.Fatalf("pending code step %+v", p)
	}
	link, err := f.svc.VerifyCode(ctx, id, " "+started.Code+" ")
	if err != nil || link.Step != StepLink || link.Token == "" || link.NewEmail != "kantor@example.test" {
		t.Fatalf("verify %+v %v", link, err)
	}
	_, err = f.svc.VerifyCode(ctx, id, started.Code)
	is(t, err, ErrNotFound)
	if p, _ := f.svc.PendingFor(ctx, id); p == nil || p.Step != StepLink {
		t.Fatalf("pending link step %+v", p)
	}
	if _, err := f.svc.Confirm(ctx, link.Token); err != nil || f.email(t, id) != "kantor@example.test" {
		t.Fatalf("confirm %v", err)
	}
}

func TestExpiryAndChangesInTheMeantime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	t.Run("code expires", func(t *testing.T) {
		id := f.user(t, "kode@gmail.com", "")
		started, err := f.svc.Start(ctx, id, "kode-baru@example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		f.advance(CodeLifetime)
		_, err = f.svc.VerifyCode(ctx, id, started.Code)
		is(t, err, ErrCodeExpired)
	})

	t.Run("link expires", func(t *testing.T) {
		id := f.user(t, "tautan@example.test", "Rahasia-2026!")
		started, err := f.svc.Start(ctx, id, "tautan-baru@example.test", ptr("Rahasia-2026!"))
		if err != nil {
			t.Fatal(err)
		}
		f.advance(LinkLifetime)
		if p, _ := f.svc.PendingFor(ctx, id); p != nil {
			t.Fatalf("expired request still pending %+v", p)
		}
		_, err = f.svc.Confirm(ctx, started.Token)
		is(t, err, ErrInvalidToken)
		if f.email(t, id) != "tautan@example.test" {
			t.Fatal("email changed by an expired link")
		}
	})

	t.Run("new email registered by someone else first", func(t *testing.T) {
		id := f.user(t, "duluan@example.test", "Rahasia-2026!")
		started, err := f.svc.Start(ctx, id, "rebutan@example.test", ptr("Rahasia-2026!"))
		if err != nil {
			t.Fatal(err)
		}
		f.user(t, "Rebutan@Example.test", "")
		_, err = f.svc.Confirm(ctx, started.Token)
		if code(t, err) != "EMAIL_ALREADY_REGISTERED" || f.email(t, id) != "duluan@example.test" {
			t.Fatalf("confirm %v", err)
		}
		_, err = f.svc.Confirm(ctx, started.Token)
		is(t, err, ErrInvalidToken)
	})

	t.Run("email changed by an admin first", func(t *testing.T) {
		id := f.user(t, "admin-ubah@example.test", "Rahasia-2026!")
		started, err := f.svc.Start(ctx, id, "anggota-ubah@example.test", ptr("Rahasia-2026!"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Pool.Exec(ctx, "UPDATE public_users SET email='admin-pilih@example.test' WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.Confirm(ctx, started.Token)
		is(t, err, ErrInvalidToken)
		if f.email(t, id) != "admin-pilih@example.test" {
			t.Fatal("admin email overwritten")
		}
	})

	t.Run("cancel", func(t *testing.T) {
		id := f.user(t, "batal@example.test", "Rahasia-2026!")
		started, err := f.svc.Start(ctx, id, "batal-baru@example.test", ptr("Rahasia-2026!"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Cancel(ctx, id); err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.Confirm(ctx, started.Token)
		is(t, err, ErrInvalidToken)
	})

	t.Run("no_account members with an email can change it", func(t *testing.T) {
		var id int32
		if err := f.svc.Pool.QueryRow(ctx, "INSERT INTO public_users (email) VALUES ('tanpa-akun@gmail.com') RETURNING id").Scan(&id); err != nil {
			t.Fatal(err)
		}
		started, err := f.svc.Start(ctx, id, "tanpa-akun-baru@example.test", nil)
		if err != nil || started.Step != StepCode {
			t.Fatalf("start %+v %v", started, err)
		}
		link, err := f.svc.VerifyCode(ctx, id, started.Code)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Confirm(ctx, link.Token); err != nil || f.email(t, id) != "tanpa-akun-baru@example.test" {
			t.Fatalf("confirm %v", err)
		}
	})

	t.Run("accounts without an email or blocked", func(t *testing.T) {
		var noEmail, blocked int32
		if err := f.svc.Pool.QueryRow(ctx, "INSERT INTO public_users (email) VALUES (NULL) RETURNING id").Scan(&noEmail); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Pool.QueryRow(ctx, "INSERT INTO public_users (email,account_status) VALUES ('diblokir@example.test','inactive') RETURNING id").Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int32{noEmail, blocked} {
			_, err := f.svc.Start(ctx, id, "x@example.test", nil)
			is(t, err, ErrInactive)
		}
	})
}

func TestRequestLimitsCountCancelledRequests(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.user(t, "batas@example.test", "Rahasia-2026!")
	start := func() error {
		_, err := f.svc.Start(ctx, id, "sasaran@example.test", ptr("Rahasia-2026!"))
		return err
	}
	for i := 0; i < MaxPerHour; i++ {
		if err := start(); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		// Cancelling does not reset the wait.
		if err := f.svc.Cancel(ctx, id); err != nil {
			t.Fatal(err)
		}
		is(t, start(), ErrTooSoon)
		f.advance(ResendAfter)
	}
	is(t, start(), ErrHourlyLimit)
	f.advance(time.Hour - time.Duration(MaxPerHour-1)*ResendAfter)
	if err := start(); err != nil {
		t.Fatalf("after an hour: %v", err)
	}
}

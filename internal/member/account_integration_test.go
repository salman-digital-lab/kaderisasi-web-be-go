//go:build integration

// Ports kaderisasi-web-be/tests/unit/google_account.spec.ts. Run by
// scripts/contract-auth.mjs inside its owned web_go_<run> schema.
package member

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/database"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/google"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	schema := os.Getenv("DB_SCHEMA")
	if !strings.HasPrefix(schema, "web_go_") {
		t.Skip("needs an owned web_go_ schema (scripts/contract-auth.mjs)")
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func address() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "@gmail.com"
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string, password *string, status string, memberID *string) dbgen.PublicUser {
	t.Helper()
	var u dbgen.PublicUser
	err := pool.QueryRow(context.Background(), "INSERT INTO public_users (email, password, account_status, member_id, created_at) VALUES ($1, $2, $3, $4, now()) RETURNING id, email, password, member_id, account_status",
		email, password, status, memberID).Scan(&u.ID, &u.Email, &u.Password, &u.MemberID, &u.AccountStatus)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func ptr(s string) *string { return &s }

func TestResolveGoogleAccount(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	t.Run("preserves pre-migration no_account accounts", func(t *testing.T) {
		user := insertUser(t, pool, address(), ptr("$scrypt$hash"), "no_account", nil)
		got, created, err := ResolveGoogleAccount(ctx, pool, google.Identity{Email: *user.Email, Name: "Google name"})
		if err != nil || created || got.ID != user.ID || *got.Password != "$scrypt$hash" || got.AccountStatus != "no_account" {
			t.Fatalf("%+v created=%v %v", got, created, err)
		}
	})

	t.Run("repeat sign-in keeps user, password, member id and profile", func(t *testing.T) {
		user := insertUser(t, pool, address(), ptr("$scrypt$original"), "active", ptr("original-"+hex.EncodeToString([]byte(address()))[:8]))
		if _, err := pool.Exec(ctx, `INSERT INTO profiles (user_id, name, badges, whatsapp) VALUES ($1, 'Original profile', '["SSC-4"]', '0812345')`, user.ID); err != nil {
			t.Fatal(err)
		}
		got, _, err := ResolveGoogleAccount(ctx, pool, google.Identity{Email: *user.Email, Name: "Changed Google name"})
		if err != nil || got.ID != user.ID || *got.Password != "$scrypt$original" || *got.MemberID != *user.MemberID {
			t.Fatalf("%+v %v", got, err)
		}
		var name, badges, whatsapp string
		_ = pool.QueryRow(ctx, "SELECT name, badges::text, whatsapp FROM profiles WHERE user_id=$1", user.ID).Scan(&name, &badges, &whatsapp)
		if name != "Original profile" || badges != `["SSC-4"]` || whatsapp != "0812345" {
			t.Fatalf("profile changed: %s %s %s", name, badges, whatsapp)
		}
	})

	t.Run("concurrent first sign-ins create exactly one complete account", func(t *testing.T) {
		identity := google.Identity{Email: address(), Name: "New member"}
		ids := make([]int32, 3)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				user, _, err := ResolveGoogleAccount(ctx, pool, identity)
				if err != nil {
					t.Error(err)
				}
				ids[i] = user.ID
			}(i)
		}
		wg.Wait()
		if ids[0] != ids[1] || ids[1] != ids[2] {
			t.Fatalf("different accounts: %v", ids)
		}
		var password, memberID *string
		var profiles int
		_ = pool.QueryRow(ctx, "SELECT password, member_id, (SELECT count(*) FROM profiles WHERE user_id=$1) FROM public_users WHERE id=$1", ids[0]).Scan(&password, &memberID, &profiles)
		if password != nil || memberID == nil || *memberID != MemberID(ids[0]) || profiles != 1 {
			t.Fatalf("password=%v member=%v profiles=%d", password, memberID, profiles)
		}
	})

	t.Run("matches mixed case; refuses inactive and ambiguous accounts", func(t *testing.T) {
		email := address()
		user := insertUser(t, pool, strings.ToUpper(email), ptr("unchanged"), "active", nil)
		got, _, err := ResolveGoogleAccount(ctx, pool, google.Identity{Email: email, Name: "Google"})
		if err != nil || got.ID != user.ID {
			t.Fatalf("mixed case: %+v %v", got, err)
		}
		_, _ = pool.Exec(ctx, "UPDATE public_users SET account_status='inactive' WHERE id=$1", user.ID)
		if _, _, err := ResolveGoogleAccount(ctx, pool, google.Identity{Email: email, Name: "Google"}); err == nil || err.Error() != "GOOGLE_ACCOUNT_INACTIVE" {
			t.Fatalf("inactive: %v", err)
		}
		insertUser(t, pool, email, ptr("other"), "active", nil)
		if _, _, err := ResolveGoogleAccount(ctx, pool, google.Identity{Email: email, Name: "Google"}); err == nil || err.Error() != "GOOGLE_EMAIL_PASSWORD_REQUIRED" {
			t.Fatalf("ambiguous: %v", err)
		}
	})

	t.Run("register hashes with scrypt and creates the profile", func(t *testing.T) {
		email := address()
		user, err := Register(ctx, pool, email, "Synthetic-password-2026!", "Registered member")
		if err != nil || !auth.VerifyPassword(*user.Password, "Synthetic-password-2026!") || *user.MemberID != MemberID(user.ID) {
			t.Fatalf("%+v %v", user, err)
		}
		profile, err := ProfileFor(ctx, dbgen.New(pool), user.ID)
		if err != nil || profile == nil || profile.Name != "Registered member" {
			t.Fatalf("profile %+v %v", profile, err)
		}
	})
}

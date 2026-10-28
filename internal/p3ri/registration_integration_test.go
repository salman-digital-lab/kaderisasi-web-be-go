//go:build integration

// Applicant workflows on a real PostgreSQL in an owned p3ri_go_<run> schema
// (explicit search path, no public fallback). Run with the test environment
// exported, for example:
//
//	set -a; . ../env/test/web-be-test-env; set +a
//	go test -tags=integration -count=1 ./internal/p3ri/
package p3ri

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/database"
	"kaderisasi/web/internal/storage"
)

type memoryStore struct {
	mu    sync.Mutex
	files map[string][]byte
	types map[string]string
}

func (m *memoryStore) Put(_ context.Context, key string, data []byte, o storage.Object) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key], m.types[key] = data, o.ContentType
	return nil
}
func (m *memoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	return nil
}
func (m *memoryStore) Get(_ context.Context, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[key]
	if !ok {
		return nil, "", storage.ErrNotFound
	}
	return data, m.types[key], nil
}

const fixtureTables = `
CREATE TABLE public_users (id serial PRIMARY KEY, email varchar(255));
CREATE TABLE admin_users (id serial PRIMARY KEY, email varchar(255));
INSERT INTO public_users (email) VALUES ('calon@example.test'), ('lain@example.test');
`

// p3riDDL is the p3ri part of the sqlc snapshot, unqualified so it lands in
// the owned schema.
func p3riDDL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../database/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	start := strings.Index(s, "CREATE TABLE public.p3ri_registrations")
	if start < 0 {
		t.Fatal("p3ri_registrations missing from schema.sql")
	}
	end := strings.Index(s[start:], "\n\n")
	if end < 0 {
		end = len(s) - start
	}
	return strings.ReplaceAll(s[start:start+end], "public.", "")
}

func newService(t *testing.T, now *time.Time) (*Service, *memoryStore) {
	t.Helper()
	if os.Getenv("DB_HOST") == "" {
		t.Skip("needs the test database environment (../env/test/web-be-test-env)")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "p3ri_go_" + hex.EncodeToString(b)
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
	if _, err := pool.Exec(ctx, fixtureTables+p3riDDL(t)); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{files: map[string][]byte{}, types: map[string]string{}}
	return &Service{Pool: pool, Storage: store, Now: func() time.Time { return *now }}, store
}

// A 1x1 PNG.
const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func TestRegistrationLifecycle(t *testing.T) {
	ctx := context.Background()
	now := Current().OpensAt.Add(time.Hour)
	svc, _ := newService(t, &now)

	if mine, err := svc.Mine(ctx, 1); err != nil || mine != nil {
		t.Fatalf("before submitting: %v %v", mine, err)
	}

	// Upload a photo, then submit with it for every file question.
	file, err := svc.Upload(ctx, 1, FilePayload{Name: "foto saya.png", DataURL: "data:image/png;base64," + pixel})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(file.Key, "p3ri/1448h/1/") || file.Mime != "image/png" {
		t.Fatalf("upload %+v", file)
	}
	if _, err := svc.Upload(ctx, 1, FilePayload{DataURL: "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("halo"))}); err == nil {
		t.Fatal("text upload accepted")
	}
	in := validInput(t, "berbuka", "website")
	for _, id := range []string{"foto_diri", "twibbon", "bukti_follow", "bukti_share"} {
		in.Answers.Common[id] = raw(t, file)
	}
	saved, err := svc.Save(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "terdaftar" || !saved.Editable || len(saved.Files) != 4 || saved.Choices[1] != "website" {
		t.Fatalf("saved %+v", saved)
	}
	if data, mime, err := svc.File(ctx, 1, file.Key); err != nil || mime != "image/png" || len(data) == 0 {
		t.Fatalf("own file: %v %s", err, mime)
	}
	if _, _, err := svc.File(ctx, 2, file.Key); err == nil {
		t.Fatal("another member read the file")
	}

	// Edit: change choices; the old division's answers are dropped.
	in.Choices = []string{"sahur"}
	edited, err := svc.Save(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID != saved.ID || len(edited.Answers.Divisions) != 1 || edited.Answers.Divisions["sahur"] == nil {
		t.Fatalf("edited %+v", edited.Answers.Divisions)
	}

	// Another member cannot reuse the first member's file keys.
	if _, err := svc.Save(ctx, 2, in); err == nil {
		t.Fatal("foreign file keys accepted")
	}

	// Once the committee moves the registration on, it is locked.
	if _, err := svc.Pool.Exec(ctx, "UPDATE p3ri_registrations SET status='seleksi_berkas' WHERE user_id=1"); err != nil {
		t.Fatal(err)
	}
	var failure *Error
	if _, err := svc.Save(ctx, 1, in); !errors.As(err, &failure) || failure.Status != 409 {
		t.Fatalf("locked registration: %v", err)
	}
	mine, err := svc.Mine(ctx, 1)
	if err != nil || mine == nil || mine.Editable || mine.Status != "seleksi_berkas" {
		t.Fatalf("mine after lock %+v %v", mine, err)
	}

	// After the deadline nothing can be written.
	now = Current().ClosesAt
	if _, err := svc.Upload(ctx, 2, FilePayload{DataURL: "data:image/png;base64," + pixel}); !errors.As(err, &failure) || failure.Status != 409 {
		t.Fatalf("upload after deadline: %v", err)
	}

	var stored map[string]any
	if err := svc.Pool.QueryRow(ctx, "SELECT answers FROM p3ri_registrations WHERE user_id=1").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(stored); !strings.Contains(string(b), `"nama_lengkap":"Jawaban nama_lengkap"`) {
		t.Fatalf("stored answers %s", b)
	}
}

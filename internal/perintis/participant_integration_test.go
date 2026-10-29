//go:build integration

// End-to-end participant workflows on a real PostgreSQL in an owned
// perintis_go_<run> schema (explicit search path, no public fallback). Run
// with the test environment exported, for example:
//
//	set -a; . ../env/test/web-be-test-env; set +a
//	go test -tags=integration -count=1 ./internal/perintis/
package perintis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
		return nil, "", errors.New("missing")
	}
	return data, m.types[key], nil
}

// Minimal copies of the kaderisasi tables the perintis queries read.
const fixtureTables = `
CREATE TABLE public_users (id serial PRIMARY KEY, email varchar(255), password varchar(255), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz, member_id varchar(255), account_status varchar(20) NOT NULL DEFAULT 'no_account');
CREATE TABLE admin_users (id serial PRIMARY KEY, email varchar(255) NOT NULL, display_name varchar(255));
CREATE TABLE provinces (id integer PRIMARY KEY, name varchar(100));
CREATE TABLE cities (id integer PRIMARY KEY, province_id integer, name varchar(255));
CREATE TABLE universities (id integer PRIMARY KEY, name varchar(100) NOT NULL);
CREATE TABLE profiles (id serial PRIMARY KEY, user_id integer, name varchar(255) NOT NULL, gender varchar(1), whatsapp varchar(35), instagram varchar(50), linkedin varchar(255), province_id integer, city_id integer, university_id integer, major varchar(50), intake_year integer, level integer DEFAULT 0, badges jsonb DEFAULT '[]', created_at timestamptz, updated_at timestamptz);
CREATE TABLE activities (id serial PRIMARY KEY, name varchar(255) NOT NULL, activity_start date, activity_type integer, activity_category integer, certificate_template_id integer);
CREATE TABLE activity_registrations (id serial PRIMARY KEY, user_id integer, activity_id integer, status varchar(50));
CREATE TABLE clubs (id serial PRIMARY KEY, name varchar(255) NOT NULL);
CREATE TABLE club_registrations (id serial PRIMARY KEY, club_id integer, member_id integer, status varchar(50));
CREATE TABLE club_member_roles (id serial PRIMARY KEY, club_registration_id integer NOT NULL, role_name varchar(150) NOT NULL, is_primary boolean NOT NULL DEFAULT false, sort_order integer NOT NULL DEFAULT 0);
CREATE TABLE achievements (id serial PRIMARY KEY, user_id integer, name varchar(255), achievement_date date, type integer, status integer DEFAULT 0);
`

// perintisDDL is the perintis part of the sqlc snapshot, unqualified so it
// lands in the owned schema.
func perintisDDL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../database/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	start := strings.Index(s, "CREATE TABLE public.perintis_participants")
	if start < 0 {
		t.Fatal("perintis tables missing from schema.sql")
	}
	return strings.ReplaceAll(s[start:], "public.", "")
}

type fixture struct {
	pool  *pgxpool.Pool
	svc   *Service
	store *memoryStore
	now   time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("DB_HOST") == "" {
		t.Skip("needs the test database environment (../env/test/web-be-test-env)")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "perintis_go_" + hex.EncodeToString(b)
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
	if _, err := pool.Exec(ctx, fixtureTables+perintisDDL(t)); err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, store: &memoryStore{files: map[string][]byte{}, types: map[string]string{}},
		now: time.Date(2027, 1, 4, 10, 0, 0, 0, WIB)}
	f.svc = &Service{Pool: pool, Storage: f.store, Now: func() time.Time { return f.now }}
	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != status {
		t.Fatalf("want %d, got %v", status, err)
	}
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

const jpeg = "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AKp//2Q=="

func TestParticipantWorkflows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO universities VALUES (1, 'Institut Teknologi Bandung')`)
	f.exec(t, `INSERT INTO public_users (id, email, account_status) VALUES
		(1, 'aisyah@example.test', 'active'), (2, 'rizky@example.test', 'active'),
		(3, 'hana@example.test', 'active'), (4, 'bima@example.test', 'no_account'), (5, 'luar@example.test', 'active')`)
	f.exec(t, `INSERT INTO profiles (user_id, name, gender, whatsapp, university_id, major, intake_year, level, badges) VALUES
		(1, 'Aisyah Rahmawati', 'F', '0812', 1, 'Teknik Kimia', 2025, 3, '["SSC-1"]'),
		(2, 'Rizky Pratama', 'M', '0813', 1, 'Informatika', 2023, 6, '[]'),
		(3, 'Hana Lestari', 'F', '0814', 1, 'Biologi', 2022, 10, '[]'),
		(4, 'Bima Saputra', 'M', '0815', 1, 'Matematika', 2025, 0, '[]')`)
	f.exec(t, `INSERT INTO activities (id, name, activity_start, activity_type, activity_category) VALUES (1, 'Kajian Desember', '2026-12-15', 0, 2), (2, 'LMD 242', '2026-08-01', 3, 2)`)
	f.exec(t, `INSERT INTO activity_registrations (user_id, activity_id, status) VALUES (1, 1, 'LULUS KEGIATAN'), (1, 2, 'TERDAFTAR')`)
	f.exec(t, `INSERT INTO clubs (id, name) VALUES (1, 'Salman Media')`)
	f.exec(t, `INSERT INTO club_registrations (id, club_id, member_id, status) VALUES (1, 1, 1, 'APPROVED')`)
	f.exec(t, `INSERT INTO club_member_roles (club_registration_id, role_name, is_primary) VALUES (1, 'Staf Acara', true)`)
	f.exec(t, `INSERT INTO perintis_participants (user_id, is_coordinator) VALUES (3, true)`)
	f.exec(t, `INSERT INTO perintis_participants (user_id, is_mentor, coordinator_user_id) VALUES (2, true, 3)`)
	f.exec(t, `INSERT INTO perintis_participants (user_id, is_mentee, nim, university_id, kip_k, category, mentor_user_id) VALUES
		(1, true, '10525107', 1, true, 'biaya_hidup', 2), (4, true, '10525114', 1, false, 'dasar', 2)`)

	// me, profile and enrolment
	me := must[Me](t)(f.svc.Me(ctx, 1))
	if len(me.Roles) != 1 || me.Roles[0] != "mentee" || me.Mentor == nil || me.Mentor.Name != "Rizky Pratama" ||
		me.Coordinator == nil || me.Coordinator.Name != "Hana Lestari" || me.Profile.Gender != "P" ||
		me.Profile.Level != "Aktivis" || me.Profile.SSCStatus != "Lulus" || me.Profile.LMDStatus != "Terdaftar" || !me.Markers.LapbulPending {
		t.Fatalf("me: %+v", me)
	}
	_, err := f.svc.Me(ctx, 5)
	wantStatus(t, err, 403)
	coordinatorView := must[FullProfile](t)(f.svc.MenteeProfile(ctx, 3, 1))
	if !coordinatorView.SensitiveHidden || coordinatorView.Participant.KipK != nil || len(coordinatorView.Clubs) != 1 {
		t.Fatalf("koordinator sees economic data: %+v", coordinatorView.Participant)
	}
	_, err = f.svc.MenteeProfile(ctx, 4, 1)
	wantStatus(t, err, 403)

	// mentee lapbul: list, draft with optimistic lock, submit with wellbeing flag
	list := must[[]JournalListItem](t)(f.svc.MyJournals(ctx, 1))
	if len(list) != 2 || list[0].Month != "2026-12-01" || list[0].Window != "open" || list[0].Status != "not_started" {
		t.Fatalf("list: %+v", list)
	}
	detail := must[JournalDetail](t)(f.svc.MyJournal(ctx, 1, "2026-12-01"))
	if !detail.CanEdit || detail.Journal != nil || detail.AutoData.Tingkat != 2 || len(detail.AutoData.Activities) != 1 || detail.AutoData.Clubs[0] != "Salman Media · Staf Acara" {
		t.Fatalf("detail: %+v", detail)
	}
	// A mentee enrolled before the kategori is known cannot start a lapbul.
	f.exec(t, `INSERT INTO public_users (id, email, account_status) VALUES (6, 'baru@example.test', 'active')`)
	f.exec(t, `INSERT INTO perintis_participants (user_id, is_mentee, status) VALUES (6, true, 'active')`)
	if bare := must[JournalDetail](t)(f.svc.MyJournal(ctx, 6, "2026-12-01")); bare.CanEdit || bare.LockedReason == nil || !strings.Contains(*bare.LockedReason, "kategori") {
		t.Fatalf("mentee without kategori: %+v", bare)
	}
	_, err = f.svc.SaveJournal(ctx, 6, "2026-12-01", SaveJournalInput{Revision: 0})
	wantStatus(t, err, 409)
	if m := must[Me](t)(f.svc.Me(ctx, 6)); m.Markers.LapbulPending {
		t.Fatal("a lapbul that cannot be started is not pending")
	}
	f.exec(t, `UPDATE perintis_participants SET category = 'dasar' WHERE user_id = 6`)
	if ready := must[JournalDetail](t)(f.svc.MyJournal(ctx, 6, "2026-12-01")); !ready.CanEdit || ready.Category != "dasar" {
		t.Fatalf("kategori set: %+v", ready)
	}
	if m := must[Me](t)(f.svc.Me(ctx, 6)); !m.Markers.LapbulPending {
		t.Fatal("lapbul is pending once the kategori is set")
	}
	_, err = f.svc.MyJournal(ctx, 1, "2027-01-01")
	wantStatus(t, err, 404)
	_, err = f.svc.MyJournal(ctx, 2, "2026-12-01")
	wantStatus(t, err, 403)
	text := func(s string) *string { return &s }
	pages, changed := 12.0, false
	answers := Answers{S1b: text("Minggu UAS"), S1c: text("Keluarga sehat.\nIbu berjualan lagi."), S2b: text("Sesi kelompok"),
		Targets: []string{"Jika pagi, maka baca", "Jika Subuh, maka tilawah", "Jika Ahad, maka rencana"}, S3a: text("Waktu perjalanan"),
		S5aPages: &pages, S5aVerse: text("Al-Insyirah 5–6"), S5b: text("Dhuha"), S5cKinds: []string{"Tenaga"}, S6: text("Citra"),
		S8aChanged: &changed, S8b: []string{"Makan"}}
	low := Pulse{Energy: p(2), Academic: p(1), Finance: p(3), Relation: p(4)}
	saved := must[JournalDetail](t)(f.svc.SaveJournal(ctx, 1, "2026-12-01", SaveJournalInput{Revision: 0, Answers: Answers{S1b: text("draf")}, Pulse: low}))
	if saved.Journal == nil || saved.Journal.Revision != 1 || saved.Journal.Status != "draft" {
		t.Fatalf("draft: %+v", saved.Journal)
	}
	_, err = f.svc.SaveJournal(ctx, 1, "2026-12-01", SaveJournalInput{Revision: 0})
	wantStatus(t, err, 409)
	_, err = f.svc.SubmitJournal(ctx, 1, "2026-12-01")
	wantStatus(t, err, 422)
	must[JournalDetail](t)(f.svc.SaveJournal(ctx, 1, "2026-12-01", SaveJournalInput{Revision: 1, Answers: answers, Pulse: low, WantsContact: true, HonestyConfirmed: true}))
	uploaded := must[JournalDetail](t)(f.svc.UploadJournalFile(ctx, 1, "2026-12-01", "s3b", FilePayload{Name: "bukti.jpg", DataURL: jpeg}))
	if len(uploaded.Journal.Files) != 1 || uploaded.Journal.Files[0].Mime != "image/jpeg" {
		t.Fatalf("file: %+v", uploaded.Journal.Files)
	}
	key := uploaded.Journal.Files[0].Key
	_, err = f.svc.UploadJournalFile(ctx, 1, "2026-12-01", "s3b", FilePayload{Name: "x.exe", DataURL: "data:application/octet-stream;base64,TVqQAAMAAAAEAAAA"})
	wantStatus(t, err, 422)
	submitted := must[JournalDetail](t)(f.svc.SubmitJournal(ctx, 1, "2026-12-01"))
	j := submitted.Journal
	if j.Status != "submitted" || j.IsLate || j.Flag == nil || j.Flag.Trigger != "both" || j.Flag.Status != "open" || submitted.CanEdit ||
		*j.Answers.S1c != "Keluarga sehat.\nIbu berjualan lagi." {
		t.Fatalf("submitted: %+v", j)
	}
	_, err = f.svc.SaveJournal(ctx, 1, "2026-12-01", SaveJournalInput{Revision: j.Revision})
	wantStatus(t, err, 409)

	// files: the mentee and the mentor can read, others cannot
	if data, mime, err := f.svc.File(ctx, 1, key); err != nil || mime != "image/jpeg" || len(data) == 0 {
		t.Fatalf("mentee file: %v", err)
	}
	must[[]byte](t)(func() ([]byte, error) { d, _, err := f.svc.File(ctx, 2, key); return d, err }())
	_, _, err = f.svc.File(ctx, 4, key)
	wantStatus(t, err, 404)
	_, _, err = f.svc.File(ctx, 3, key)
	wantStatus(t, err, 404)

	// mentor: dashboard, response, flag
	dash := must[MentorDashboard](t)(f.svc.MentorDashboard(ctx, 2, ""))
	if dash.Month != "2026-12-01" || dash.ToRespond != 1 || len(dash.Rows) != 2 || dash.Rows[0].Mentee.UserID != 1 || dash.Rows[0].Flag == nil || dash.Tingkat != 2 {
		t.Fatalf("dashboard: %+v", dash)
	}
	if !dash.Rows[1].InactiveStreak {
		t.Fatal("two months without a lapbul is an inactive streak")
	}
	_, err = f.svc.MentorDashboard(ctx, 1, "")
	wantStatus(t, err, 403)
	reader := must[JournalForMentor](t)(f.svc.MentorJournal(ctx, 2, j.ID))
	if !reader.CanRespond || reader.Mentee.KipK == nil || !*reader.Mentee.KipK {
		t.Fatalf("reader: %+v", reader)
	}
	_, err = f.svc.Respond(ctx, 2, j.ID, RespondInput{Appreciation: "Bagus", Text: "Lanjutkan", Decision: "revision_requested"})
	wantStatus(t, err, 422)
	responded := must[JournalForMentor](t)(f.svc.Respond(ctx, 2, j.ID, RespondInput{Appreciation: "Terima kasih", Text: "Target tilawah bagus.\nLanjut.", Decision: "accepted"}))
	if responded.Journal.Status != "accepted" || responded.Journal.Response.RespondedBy != "Rizky Pratama" {
		t.Fatalf("respond: %+v", responded.Journal.Response)
	}
	handled := must[JournalForMentor](t)(f.svc.HandleFlag(ctx, 2, j.ID, "Sudah ditelepon"))
	if handled.Journal.Flag.Status != "handled" || handled.Journal.Flag.HandledBy == nil {
		t.Fatalf("flag: %+v", handled.Journal.Flag)
	}
	if m := must[Me](t)(f.svc.Me(ctx, 1)); !m.Markers.ResponseUnread {
		t.Fatal("new response is unread")
	}

	// An answered lapbul stays with its mentor after the mentee moves (PRD §8.7):
	// the old mentor still lists it, the new one sees who holds it.
	f.exec(t, `INSERT INTO public_users (id, email, account_status) VALUES (7, 'galih@example.test', 'active')`)
	f.exec(t, `INSERT INTO profiles (user_id, name, university_id, intake_year, level, badges) VALUES (7, 'Galih Permana', 1, 2023, 0, '[]')`)
	f.exec(t, `INSERT INTO perintis_participants (user_id, is_mentor) VALUES (7, true)`)
	f.exec(t, `UPDATE perintis_participants SET mentor_user_id = 7 WHERE user_id = 1`)
	rowOf := func(d MentorDashboard, mentee int32) *DashboardRow {
		for i := range d.Rows {
			if d.Rows[i].Mentee.UserID == mentee {
				return &d.Rows[i]
			}
		}
		return nil
	}
	old := must[MentorDashboard](t)(f.svc.MentorDashboard(ctx, 2, ""))
	if r := rowOf(old, 1); r == nil || !r.FormerMentee || r.JournalID == nil || *r.JournalID != j.ID || r.InactiveStreak {
		t.Fatalf("old mentor keeps the answered lapbul: %+v", r)
	}
	moved := must[MentorDashboard](t)(f.svc.MentorDashboard(ctx, 7, ""))
	if r := rowOf(moved, 1); r == nil || r.FormerMentee || r.JournalID != nil || r.HeldBy == nil || *r.HeldBy != "Rizky Pratama" || r.Flag != nil || r.Status != "accepted" {
		t.Fatalf("new mentor sees who holds the lapbul: %+v", r)
	}
	if held := must[JournalForMentor](t)(f.svc.MentorJournal(ctx, 2, j.ID)); !held.MovedAway {
		t.Fatal("old mentor is told the mentee moved")
	}
	f.exec(t, `UPDATE perintis_participants SET mentor_user_id = 2 WHERE user_id = 1`)
	f.exec(t, `DELETE FROM perintis_participants WHERE user_id = 7`)
	if back := must[JournalForMentor](t)(f.svc.MentorJournal(ctx, 2, j.ID)); back.MovedAway {
		t.Fatal("current mentor is not told the mentee moved")
	}
	if err := f.svc.ReadResponse(ctx, 1, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if m := must[Me](t)(f.svc.Me(ctx, 1)); m.Markers.ResponseUnread {
		t.Fatal("read response still unread")
	}

	// escalation: Bima asks for contact; after three days the flag escalates
	bima := Answers{S1b: text("a"), S1c: text("b"), S2b: text("c"), Targets: []string{"1", "2", "3"}, S3a: text("d"),
		S5aPages: &pages, S5aVerse: text("e"), S5b: text("f"), S5cKinds: []string{"Ilmu"}, S6: text("g")}
	ok := Pulse{Energy: p(4), Academic: p(4), Finance: p(4), Relation: p(4)}
	must[JournalDetail](t)(f.svc.SaveJournal(ctx, 4, "2026-12-01", SaveJournalInput{Revision: 0, Answers: bima, Pulse: ok, WantsContact: true, HonestyConfirmed: true}))
	bimaJournal := must[JournalDetail](t)(f.svc.SubmitJournal(ctx, 4, "2026-12-01")).Journal
	f.now = f.now.AddDate(0, 0, 4)
	escalated := must[JournalForMentor](t)(f.svc.HandleFlag(ctx, 2, bimaJournal.ID, "Sudah dihubungi, terlambat"))
	if escalated.Journal.Flag.Status != "escalated" || escalated.Journal.Flag.PrivateNote == nil || escalated.Journal.Flag.HandledAt != nil {
		t.Fatalf("escalated flag stays with pengelola: %+v", escalated.Journal.Flag)
	}
	must[JournalForMentor](t)(f.svc.Respond(ctx, 2, bimaJournal.ID, RespondInput{Appreciation: "a", Text: "b", Decision: "flagged", FlagReason: "Butuh bantuan biaya"}))

	// report: photo, save, submit; koordinator verifies
	photo := must[File](t)(f.svc.UploadPhoto(ctx, 2, FilePayload{Name: "foto kelompok.jpg", DataURL: jpeg}))
	_, err = f.svc.UploadPhoto(ctx, 1, FilePayload{Name: "x.jpg", DataURL: jpeg})
	wantStatus(t, err, 403)
	outside := "2027-01-03"
	_, err = f.svc.SaveReport(ctx, 2, "2026-12-01", SaveReportInput{SessionDate: &outside, Mode: "luring"})
	wantStatus(t, err, 422) // the session must be in the report month
	_, err = f.svc.SaveReport(ctx, 2, "2026-12-01", SaveReportInput{Mode: "luring", Photos: []File{{Key: "perintis/reports/9/a.jpg"}}})
	wantStatus(t, err, 422)
	date := "2026-12-20"
	case1 := "Satu mentee kesulitan biaya kos."
	report := must[ReportDetail](t)(f.svc.SaveReport(ctx, 2, "2026-12-01", SaveReportInput{SessionDate: &date, Mode: "luring", LocationOrLink: "Selasar Salman",
		FollowsCurriculum: true, Summary: "Sesi berjalan baik.", Photos: []File{photo}, CaseNote: &case1}))
	if !report.CanEdit || report.Report.CurriculumCode != "T2-12" || report.Report.CaseStatus == nil {
		t.Fatalf("report draft: %+v", report.Report)
	}
	report = must[ReportDetail](t)(f.svc.SubmitReport(ctx, 2, "2026-12-01"))
	if report.Report.Status != "submitted" || report.Report.IsLate || report.CanEdit || !report.Tasks.T3 {
		t.Fatalf("report submit: %+v %+v", report.Report, report.Tasks)
	}
	coordinatorReport := must[ReportDetail](t)(f.svc.Report(ctx, 3, 2, "2026-12-01"))
	if coordinatorReport.Report.CaseNote != nil || !coordinatorReport.Report.HasCase {
		t.Fatal("koordinator sees the case note")
	}
	if _, _, err := f.svc.File(ctx, 3, photo.Key); err != nil {
		t.Fatalf("koordinator reads group photo: %v", err)
	}
	overview := must[CoordinatorOverview](t)(f.svc.CoordinatorOverview(ctx, 3, ""))
	if len(overview.Rows) != 1 || overview.Rows[0].Journals.Submitted != 2 || overview.Rows[0].EscalatedFlags != 1 || overview.Rows[0].Report == nil {
		t.Fatalf("overview: %+v", overview.Rows)
	}
	if m := must[Me](t)(f.svc.Me(ctx, 3)); m.Markers.ReportsToVerify != 1 {
		t.Fatalf("markers: %+v", m.Markers)
	}
	_, err = f.svc.ReviewReport(ctx, 3, report.Report.ID, ReviewInput{Decision: "verify"})
	wantStatus(t, err, 422)
	verified := must[ReportDetail](t)(f.svc.ReviewReport(ctx, 3, report.Report.ID, ReviewInput{Decision: "verify", Appreciation: "Rapi", Note: "Lanjutkan"}))
	if verified.Report.Status != "verified" || !verified.Tasks.T1 || !verified.CanUnverify || verified.Report.Review.ReviewedBy != "Hana Lestari" {
		t.Fatalf("verify: %+v", verified)
	}
	unverified := must[ReportDetail](t)(f.svc.ReviewReport(ctx, 3, report.Report.ID, ReviewInput{Decision: "unverify"}))
	if unverified.Report.Status != "submitted" || unverified.Report.VerifiedAt != nil {
		t.Fatalf("unverify: %+v", unverified.Report)
	}
	_, err = f.svc.ReviewReport(ctx, 2, report.Report.ID, ReviewInput{Decision: "verify", Appreciation: "a", Note: "b"})
	wantStatus(t, err, 403)

	// after the 15th the mentor can no longer edit responses
	f.now = time.Date(2027, 1, 16, 10, 0, 0, 0, WIB)
	_, err = f.svc.Respond(ctx, 2, j.ID, RespondInput{Appreciation: "a", Text: "b", Decision: "accepted"})
	wantStatus(t, err, 409)
	if regexp.MustCompile(`^perintis/journals/\d+/`).FindString(key) == "" {
		t.Fatalf("journal file key: %s", key)
	}
}

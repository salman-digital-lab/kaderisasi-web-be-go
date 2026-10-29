package perintis

// Participant workflows (mentee, mentor, koordinator) served by
// kaderisasi-web-be-go under /v2/perintis. Only this repository has this file.

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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/storage"
)

// Service runs the participant workflows. Now is the clock (tests and the
// optional PERINTIS_CLOCK override); nil means time.Now.
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

// read loads the world outside a transaction.
func (s *Service) read(ctx context.Context) (*World, error) {
	return Load(ctx, dbgen.New(s.Pool), s.now())
}

// write runs fn in a transaction with a world loaded inside it.
func (s *Service) write(ctx context.Context, fn func(*World) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	w, err := Load(ctx, dbgen.New(s.Pool).WithTx(tx), s.now())
	if err != nil {
		return err
	}
	if err := fn(w); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *World) requireMonth(month string) error {
	for _, m := range ReportMonths(w.Now) {
		if m == month {
			return nil
		}
	}
	return fail(404, "Bulan laporan tidak tersedia.")
}

// ---- me & profiles ---------------------------------------------------------------

func (s *Service) Me(ctx context.Context, user int32) (Me, error) {
	w, err := s.read(ctx)
	if err != nil {
		return Me{}, err
	}
	p, err := w.Participant(user)
	if err != nil {
		return Me{}, err
	}
	if err := w.LoadDetails(ctx, []int32{user}); err != nil {
		return Me{}, err
	}
	markers, err := w.Markers(ctx, p)
	if err != nil {
		return Me{}, err
	}
	return Me{Profile: w.Profile(user), Participant: w.ParticipantView(p), Roles: Roles(p),
		Mentor: w.Person(p.MentorUserID), Coordinator: w.Person(w.CoordinatorOfMentor(p.MentorUserID)), Markers: markers}, nil
}

func (s *Service) MyProfile(ctx context.Context, user int32) (FullProfile, error) {
	w, err := s.read(ctx)
	if err != nil {
		return FullProfile{}, err
	}
	if _, err := w.Participant(user); err != nil {
		return FullProfile{}, err
	}
	return w.FullProfile(ctx, user, false)
}

// MenteeProfile is open to the mentee's mentor (full) and koordinator
// (without economic data).
func (s *Service) MenteeProfile(ctx context.Context, viewer, mentee int32) (FullProfile, error) {
	w, err := s.read(ctx)
	if err != nil {
		return FullProfile{}, err
	}
	me, err := w.Participant(viewer)
	if err != nil {
		return FullProfile{}, err
	}
	m, ok := w.Participants[mentee]
	if !ok || !m.IsMentee {
		return FullProfile{}, fail(404, "Mentee tidak ditemukan.")
	}
	isMentor := HasRole(me, "mentor") && m.MentorUserID != nil && *m.MentorUserID == viewer
	coordinator := w.CoordinatorOfMentor(m.MentorUserID)
	isCoordinator := HasRole(me, "coordinator") && coordinator != nil && *coordinator == viewer
	if !isMentor && !isCoordinator {
		return FullProfile{}, fail(403, "Mentee ini bukan bagian dari kelompokmu.")
	}
	return w.FullProfile(ctx, mentee, !isMentor)
}

// ---- mentee: lapbul ------------------------------------------------------------------

func (s *Service) MyJournals(ctx context.Context, user int32) ([]JournalListItem, error) {
	w, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := w.RequireRole(user, "mentee"); err != nil {
		return nil, err
	}
	return w.JournalList(ctx, user)
}

func revisionDueAt(j *dbgen.PerintisJournal) *time.Time {
	if j == nil || j.Status != "revision_requested" || !j.RespondedAt.Valid {
		return nil
	}
	due := j.RespondedAt.Time.AddDate(0, 0, 7)
	return &due
}

func editability(j *dbgen.PerintisJournal, month string, now time.Time) (bool, *string) {
	if j != nil && j.Status == "revision_requested" {
		if due := revisionDueAt(j); due != nil && now.After(*due) {
			return false, strPtr("Batas perbaikan 7 hari sudah lewat.")
		}
		return true, nil
	}
	if j != nil && j.Status != "draft" {
		return false, strPtr("Sudah terkirim. Isian hanya bisa dibaca kecuali mentor meminta perbaikan.")
	}
	if j != nil && j.ReopenReason != nil {
		return true, nil
	}
	switch Window(month, now) {
	case "upcoming":
		return false, strPtr("Belum dibuka. Lapbul dibuka tanggal 1.")
	case "locked":
		return false, strPtr("Terkunci sejak tanggal 12. Hubungi pengelola bila perlu dibuka kembali.")
	}
	return true, nil
}

func (w *World) autoData(ctx context.Context, user int32, month string, j *dbgen.PerintisJournal) (AutoData, error) {
	p := w.Participants[user]
	if err := w.LoadDetails(ctx, []int32{user}); err != nil {
		return AutoData{}, err
	}
	person := w.People[user]
	mentor := p.MentorUserID
	category := deref(p.Category)
	if j != nil {
		mentor, category = j.MentorUserID, j.Category
	}
	if category == "" {
		category = "dasar"
	}
	intake := int32(0)
	if person.IntakeYear != nil {
		intake = *person.IntakeYear
	}
	profile := w.Profile(user)
	out := AutoData{Name: person.Name, Nim: p.Nim, University: deref(person.University), Major: deref(person.Major),
		IntakeYear: intake, Tingkat: w.TingkatOf(user), Category: category, MentorName: w.NamePtr(mentor),
		CoordinatorName: w.NamePtr(w.CoordinatorOfMentor(mentor)), Activities: []string{},
		SSCStatus: profile.SSCStatus, LMDStatus: profile.LMDStatus, Clubs: []string{}}
	if mentor != nil {
		r, err := w.ReportByMonth(ctx, *mentor, month)
		if err != nil {
			return out, err
		}
		if r != nil && r.SessionDate.Valid {
			out.Session = &SessionInfo{Date: r.SessionDate.Time.Format("2006-01-02"), Theme: CurriculumFor(w.GroupTingkat(*mentor), month).Theme}
		}
	}
	start, _ := MonthStart(month)
	for _, a := range w.activities[user] {
		if !a.ActivityStart.Valid {
			continue
		}
		d := time.Date(a.ActivityStart.Time.Year(), a.ActivityStart.Time.Month(), a.ActivityStart.Time.Day(), 0, 0, 0, 0, WIB)
		if !d.Before(start) && d.Before(start.AddDate(0, 1, 0)) {
			out.Activities = append(out.Activities, fmt.Sprintf("%s (%s)", a.Name, titleStatus(a.Status)))
		}
	}
	for _, c := range w.clubs[user] {
		out.Clubs = append(out.Clubs, c.Name+" · "+c.Role)
	}
	return out, nil
}

func (w *World) journalDetail(ctx context.Context, user int32, month string) (JournalDetail, error) {
	p := w.Participants[user]
	j, err := w.JournalByMonth(ctx, user, month)
	if err != nil {
		return JournalDetail{}, err
	}
	auto, err := w.autoData(ctx, user, month, j)
	if err != nil {
		return JournalDetail{}, err
	}
	previous, err := w.PreviousTargets(ctx, user, month)
	if err != nil {
		return JournalDetail{}, err
	}
	canEdit, reason := editability(j, month, w.Now)
	if j == nil && canEdit && p.Category == nil {
		canEdit, reason = false, strPtr(missingCategory)
	}
	out := JournalDetail{Month: month, Category: auto.Category, Window: Window(month, w.Now), AutoData: auto,
		PreviousTargets: previous, CanEdit: canEdit, LockedReason: reason}
	if p.Category != nil && j == nil {
		out.Category = *p.Category
	}
	if j != nil {
		view, err := w.JournalView(ctx, *j)
		if err != nil {
			return out, err
		}
		out.Journal = &view
		if j.ReopenReason != nil {
			out.Window = "open"
		}
		if due := revisionDueAt(j); due != nil {
			s := due.UTC().Format(time.RFC3339Nano)
			out.RevisionDueAt = &s
		}
	}
	return out, nil
}

func (s *Service) MyJournal(ctx context.Context, user int32, month string) (JournalDetail, error) {
	w, err := s.read(ctx)
	if err != nil {
		return JournalDetail{}, err
	}
	if _, err := w.RequireRole(user, "mentee"); err != nil {
		return JournalDetail{}, err
	}
	if err := w.requireMonth(month); err != nil {
		return JournalDetail{}, err
	}
	return w.journalDetail(ctx, user, month)
}

const missingCategory = "Data kepesertaanmu belum lengkap: kategori beasiswa belum diisi. Hubungi pengelola agar bisa mengisi Catatan Perjalanan."

// editableJournal locks (or creates) the mentee's row for the month when it
// may still be changed.
func (w *World) editableJournal(ctx context.Context, user int32, month string) (dbgen.PerintisJournal, error) {
	p, err := w.RequireRole(user, "mentee")
	if err != nil {
		return dbgen.PerintisJournal{}, err
	}
	if err := w.requireMonth(month); err != nil {
		return dbgen.PerintisJournal{}, err
	}
	j, err := w.Q.PerintisJournalByMonthForUpdate(ctx, dbgen.PerintisJournalByMonthForUpdateParams{MenteeUserID: user, Month: DateOf(month)})
	var current *dbgen.PerintisJournal
	if err == nil {
		current = &j
	} else if !notFound(err) {
		return j, err
	}
	if ok, reason := editability(current, month, w.Now); !ok {
		return j, fail(409, "%s", deref(reason))
	}
	if current != nil {
		return j, nil
	}
	// The kategori picks the lapbul form, so a mentee enrolled before it was
	// known starts only after the pengelola sets it.
	category := deref(p.Category)
	if category == "" {
		return j, fail(409, "%s", missingCategory)
	}
	j, err = w.Q.InsertPerintisJournal(ctx, dbgen.InsertPerintisJournalParams{MenteeUserID: user, Month: DateOf(month),
		MentorUserID: p.MentorUserID, Category: category, FormVersion: FormVersion})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return j, fail(409, "Draf sedang disimpan dari perangkat lain. Coba lagi.")
	}
	return j, err
}

// SaveJournalInput is the draft autosave body.
type SaveJournalInput struct {
	Revision         int32   `json:"revision"`
	Answers          Answers `json:"answers"`
	Pulse            Pulse   `json:"pulse"`
	WantsContact     bool    `json:"wantsContact"`
	HonestyConfirmed bool    `json:"honestyConfirmed"`
}

func (s *Service) SaveJournal(ctx context.Context, user int32, month string, in SaveJournalInput) (JournalDetail, error) {
	var out JournalDetail
	err := s.write(ctx, func(w *World) error {
		j, err := w.editableJournal(ctx, user, month)
		if err != nil {
			return err
		}
		if in.Revision != j.Revision {
			return fail(409, "Draf ini sudah diubah dari perangkat lain. Muat ulang untuk melihat versi terbaru.")
		}
		answers, err := in.Answers.Clean()
		if err != nil {
			return err
		}
		if !in.Pulse.Valid() {
			return fail(422, "Skala pulse harus 1–5.")
		}
		if j.Answers, err = json.Marshal(answers); err != nil {
			return err
		}
		j.PulseEnergy, j.PulseAcademic, j.PulseFinance, j.PulseRelation = in.Pulse.Energy, in.Pulse.Academic, in.Pulse.Finance, in.Pulse.Relation
		j.WantsContact = in.WantsContact
		if in.HonestyConfirmed && !j.HonestyConfirmedAt.Valid {
			j.HonestyConfirmedAt = timestamptz(w.Now)
		}
		j.Revision++
		if _, err := SaveJournal(ctx, w.Q, j); err != nil {
			return err
		}
		out, err = w.journalDetail(ctx, user, month)
		return err
	})
	return out, err
}

// FilePayload is an upload sent as a data URL (the app compresses photos first).
type FilePayload struct {
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Size    int64  `json:"size"`
	DataURL string `json:"dataUrl"`
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9.-]`)

func randomHex() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// decode checks the payload and returns its bytes and sniffed content type.
func (f FilePayload) decode(allowPDF bool) ([]byte, string, error) {
	_, encoded, ok := strings.Cut(f.DataURL, ";base64,")
	if !ok || !strings.HasPrefix(f.DataURL, "data:") {
		return nil, "", fail(422, "Berkas tidak terbaca.")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fail(422, "Berkas tidak terbaca.")
	}
	if len(data) > MaxFile {
		return nil, "", fail(413, "Ukuran berkas maksimal 5 MB.")
	}
	mime := http.DetectContentType(data)
	switch {
	case mime == "image/jpeg" || mime == "image/png" || mime == "image/webp" || mime == "image/gif":
	case allowPDF && mime == "application/pdf":
	case allowPDF:
		return nil, "", fail(422, "Hanya foto atau PDF.")
	default:
		return nil, "", fail(422, "Hanya foto.")
	}
	return data, mime, nil
}

func safeName(name string) string {
	name = unsafeName.ReplaceAllString(strings.TrimSpace(name), "_")
	if len(name) > 80 {
		name = name[len(name)-80:]
	}
	if name == "" {
		name = "berkas"
	}
	return name
}

func (s *Service) UploadJournalFile(ctx context.Context, user int32, month, field string, in FilePayload) (JournalDetail, error) {
	if field != "s3b" && field != "s7khs" {
		return JournalDetail{}, fail(422, "Isian unggahan tidak dikenal.")
	}
	data, mime, err := in.decode(true)
	if err != nil {
		return JournalDetail{}, err
	}
	var out JournalDetail
	var stored string
	err = s.write(ctx, func(w *World) error {
		j, err := w.editableJournal(ctx, user, month)
		if err != nil {
			return err
		}
		files := ParseFiles(j.Files)
		if len(files) >= 10 {
			return fail(422, "Paling banyak 10 berkas.")
		}
		key := fmt.Sprintf("perintis/journals/%d/%s-%s", j.ID, randomHex(), safeName(in.Name))
		if err := s.Storage.Put(ctx, key, data, storage.Object{ContentType: mime, CacheControl: "private, no-store", Private: true}); err != nil {
			return err
		}
		stored = key
		files = append(files, File{Field: field, Key: key, Name: strings.TrimSpace(in.Name), Mime: mime, Size: int64(len(data))})
		if j.Files, err = json.Marshal(files); err != nil {
			return err
		}
		j.Revision++
		if _, err := SaveJournal(ctx, w.Q, j); err != nil {
			return err
		}
		out, err = w.journalDetail(ctx, user, month)
		return err
	})
	if err != nil && stored != "" {
		_ = s.Storage.Delete(context.WithoutCancel(ctx), stored)
	}
	return out, err
}

func (s *Service) DeleteJournalFile(ctx context.Context, user int32, month, key string) (JournalDetail, error) {
	var out JournalDetail
	var removed string
	err := s.write(ctx, func(w *World) error {
		j, err := w.editableJournal(ctx, user, month)
		if err != nil {
			return err
		}
		kept := []File{}
		for _, f := range ParseFiles(j.Files) {
			if f.Key == key {
				removed = key
				continue
			}
			kept = append(kept, f)
		}
		if j.Files, err = json.Marshal(kept); err != nil {
			return err
		}
		j.Revision++
		if _, err := SaveJournal(ctx, w.Q, j); err != nil {
			return err
		}
		out, err = w.journalDetail(ctx, user, month)
		return err
	})
	if err == nil && removed != "" {
		_ = s.Storage.Delete(context.WithoutCancel(ctx), removed)
	}
	return out, err
}

func (s *Service) SubmitJournal(ctx context.Context, user int32, month string) (JournalDetail, error) {
	var out JournalDetail
	err := s.write(ctx, func(w *World) error {
		j, err := w.editableJournal(ctx, user, month)
		if err != nil {
			return err
		}
		if !j.HonestyConfirmedAt.Valid {
			return fail(422, "Centang pernyataan kejujuran terlebih dulu.")
		}
		previous, err := w.PreviousTargets(ctx, user, month)
		if err != nil {
			return err
		}
		pulse := JournalPulse(j)
		if missing := MissingItems(j.Category, month, ParseAnswers(j.Answers), pulse, ParseFiles(j.Files), previous); len(missing) > 0 {
			return fail(422, "Masih ada isian wajib: %s.", strings.Join(missing, ", "))
		}
		if j.Status == "revision_requested" {
			j.Status = "revised"
		} else {
			j.Status = "submitted"
			j.SubmittedAt = timestamptz(w.Now)
			j.IsLate = j.ReopenReason == nil && Window(month, w.Now) == "late"
			if trigger := FlagTrigger(pulse, j.WantsContact); trigger != "" {
				j.FlagTrigger, j.FlagStatus = strPtr(trigger), strPtr("open")
				j.FlagDueAt = timestamptz(w.Now.AddDate(0, 0, 3))
			}
		}
		j.Revision++
		if _, err := SaveJournal(ctx, w.Q, j); err != nil {
			return err
		}
		out, err = w.journalDetail(ctx, user, month)
		return err
	})
	return out, err
}

func (s *Service) ReadResponse(ctx context.Context, user int32, month string) error {
	return s.write(ctx, func(w *World) error {
		j, err := w.Q.PerintisJournalByMonthForUpdate(ctx, dbgen.PerintisJournalByMonthForUpdateParams{MenteeUserID: user, Month: DateOf(month)})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if j.RespondedAt.Valid && !j.ResponseReadAt.Valid {
			j.ResponseReadAt = timestamptz(w.Now)
			_, err = SaveJournal(ctx, w.Q, j)
		}
		return err
	})
}

// ---- mentor ------------------------------------------------------------------------

func (w *World) monthOrCurrent(month string) (string, error) {
	if month == "" {
		month = CurrentReportMonth(w.Now)
	}
	if month == "" {
		return "", fail(404, "Belum ada bulan laporan.")
	}
	if _, ok := MonthStart(month); !ok || month < FirstMonth {
		return "", fail(404, "Bulan laporan tidak tersedia.")
	}
	return month, nil
}

func (s *Service) MentorDashboard(ctx context.Context, user int32, month string) (MentorDashboard, error) {
	w, err := s.read(ctx)
	if err != nil {
		return MentorDashboard{}, err
	}
	if _, err := w.RequireRole(user, "mentor"); err != nil {
		return MentorDashboard{}, err
	}
	if month, err = w.monthOrCurrent(month); err != nil {
		return MentorDashboard{}, err
	}
	// Lapbul answered by this mentor stay with them after a move (PRD §8.7),
	// so the rows are the current mentees plus anyone whose lapbul for the
	// month is held here.
	frozen, err := w.JournalsOfMentorsInMonth(ctx, []int32{user}, month)
	if err != nil {
		return MentorDashboard{}, err
	}
	mentees := w.MenteesOf(user)
	current := map[int32]bool{}
	ids := make([]int32, 0, len(mentees)+len(frozen))
	for _, m := range mentees {
		current[m.UserID] = true
		ids = append(ids, m.UserID)
	}
	former := []int32{}
	for _, j := range frozen {
		if !current[j.MenteeUserID] && j.Status != "draft" {
			if _, ok := w.Participants[j.MenteeUserID]; ok {
				former = append(former, j.MenteeUserID)
				ids = append(ids, j.MenteeUserID)
			}
		}
	}
	all, err := w.Q.PerintisJournalsOfMentees(ctx, ids)
	if err != nil {
		return MentorDashboard{}, err
	}
	byMentee := map[int32]map[string]dbgen.PerintisJournal{}
	for _, j := range all {
		if byMentee[j.MenteeUserID] == nil {
			byMentee[j.MenteeUserID] = map[string]dbgen.PerintisJournal{}
		}
		byMentee[j.MenteeUserID][monthOf(j.Month)] = j
	}
	rows := []DashboardRow{}
	addRow := func(id int32, formerMentee bool) error {
		m := w.Participants[id]
		row := DashboardRow{Mentee: DashboardMentee{PersonSummary: w.MustPerson(id), Category: deref(m.Category), Tingkat: w.TingkatOf(id)},
			Status: "not_started", FormerMentee: formerMentee}
		if !formerMentee {
			row.InactiveStreak = InactiveStreak(month, byMentee[id])
		}
		j, ok := byMentee[id][month]
		if !ok {
			rows = append(rows, row)
			return nil
		}
		row.Status, row.IsLate, row.SubmittedAt = j.Status, j.IsLate, iso(j.SubmittedAt)
		if j.MentorUserID == nil || *j.MentorUserID != user {
			if j.Status != "draft" {
				row.HeldBy = w.NamePtr(j.MentorUserID)
			}
			rows = append(rows, row)
			return nil
		}
		jid := j.ID
		row.JournalID = &jid
		if j.FlagTrigger != nil && deref(j.FlagStatus) != "resolved" {
			view, err := w.JournalView(ctx, j)
			if err != nil {
				return err
			}
			row.Flag = &DashboardFlag{JournalFlag: *view.Flag, JournalID: j.ID}
		}
		rows = append(rows, row)
		return nil
	}
	for _, m := range mentees {
		if err := addRow(m.UserID, false); err != nil {
			return MentorDashboard{}, err
		}
	}
	for _, id := range former {
		if err := addRow(id, true); err != nil {
			return MentorDashboard{}, err
		}
	}
	rank := func(r DashboardRow) int {
		switch {
		case r.Flag != nil && (r.Flag.Status == "open" || r.Flag.Status == "escalated"):
			return 0
		case Waiting(r.Status):
			return 1
		case r.Status == "not_started" || r.Status == "draft":
			return 2
		}
		return 3
	}
	sort.SliceStable(rows, func(a, b int) bool {
		ra, rb := rank(rows[a]), rank(rows[b])
		if ra != rb {
			return ra < rb
		}
		sa, sb := deref(rows[a].SubmittedAt), deref(rows[b].SubmittedAt)
		if sa != sb {
			return sa < sb
		}
		return rows[a].Mentee.Name < rows[b].Mentee.Name
	})
	return MentorDashboard{Month: month, Tingkat: w.GroupTingkat(user), Rows: rows, ToRespond: CountWaiting(frozen)}, nil
}

// mentorJournal is a submitted lapbul frozen to this mentor.
func (w *World) mentorJournal(ctx context.Context, user, id int32, lock bool) (dbgen.PerintisJournal, error) {
	if _, err := w.RequireRole(user, "mentor"); err != nil {
		return dbgen.PerintisJournal{}, err
	}
	var j dbgen.PerintisJournal
	var err error
	if lock {
		j, err = w.Q.PerintisJournalForUpdate(ctx, id)
	} else {
		j, err = w.Q.PerintisJournal(ctx, id)
	}
	if notFound(err) {
		return j, fail(404, "Catatan Perjalanan tidak ditemukan.")
	}
	if err != nil {
		return j, err
	}
	if j.MentorUserID == nil || *j.MentorUserID != user {
		return j, fail(403, "Lapbul ini bukan milik mentee di kelompokmu.")
	}
	if j.Status == "draft" {
		return j, fail(404, "Lapbul belum dikirim mentee.")
	}
	return j, nil
}

func (s *Service) MentorJournal(ctx context.Context, user, id int32) (JournalForMentor, error) {
	w, err := s.read(ctx)
	if err != nil {
		return JournalForMentor{}, err
	}
	j, err := w.mentorJournal(ctx, user, id, false)
	if err != nil {
		return JournalForMentor{}, err
	}
	lock := RespondLock(monthOf(j.Month), w.Now)
	return w.mentorReaderView(ctx, user, j, lock == nil, lock)
}

// mentorReaderView is ReaderView for the mentor holding the lapbul.
func (w *World) mentorReaderView(ctx context.Context, user int32, j dbgen.PerintisJournal, canRespond bool, reason *string) (JournalForMentor, error) {
	out, err := w.ReaderView(ctx, j, canRespond, reason)
	if mentor := w.Participants[j.MenteeUserID].MentorUserID; mentor == nil || *mentor != user {
		out.MovedAway = true
	}
	return out, err
}

// RespondInput is the mentor's response and decision.
type RespondInput struct {
	Appreciation    string `json:"appreciation"`
	Text            string `json:"text"`
	Decision        string `json:"decision"`
	RevisionRequest string `json:"revisionRequest"`
	FlagReason      string `json:"flagReason"`
}

func (s *Service) Respond(ctx context.Context, user, id int32, in RespondInput) (JournalForMentor, error) {
	var out JournalForMentor
	err := s.write(ctx, func(w *World) error {
		j, err := w.mentorJournal(ctx, user, id, true)
		if err != nil {
			return err
		}
		if lock := RespondLock(monthOf(j.Month), w.Now); lock != nil {
			return fail(409, "%s", *lock)
		}
		appreciation, err := RequireText(in.Appreciation, "Apresiasi", MaxLong)
		if err != nil {
			return err
		}
		text, err := RequireText(in.Text, "Tanggapan", MaxLong)
		if err != nil {
			return err
		}
		if !Responded(in.Decision) {
			return fail(422, "Pilih keputusan.")
		}
		j.RevisionRequest, j.FlagReason = nil, nil
		if in.Decision == "revision_requested" {
			request, err := RequireText(in.RevisionRequest, "Permintaan perbaikan", MaxLong)
			if err != nil {
				return err
			}
			j.RevisionRequest = &request
		}
		if in.Decision == "flagged" {
			reason, err := RequireText(in.FlagReason, "Alasan ditandai", MaxLong)
			if err != nil {
				return err
			}
			j.FlagReason = &reason
			// A new flag to the pengelola opens a fresh follow-up, unless the
			// wellbeing flag's resolution still depends on the old one.
			if deref(j.FlagStatus) != "resolved" {
				j.FollowupBy, j.FollowupAt, j.FollowupNote = nil, pgtype.Timestamptz{}, nil
			}
		}
		j.ResponseAppreciation, j.ResponseText = &appreciation, &text
		j.RespondedBy, j.RespondedAt, j.ResponseReadAt = &user, timestamptz(w.Now), pgtype.Timestamptz{}
		j.Status = in.Decision
		j, err = SaveJournal(ctx, w.Q, j)
		if err != nil {
			return err
		}
		out, err = w.mentorReaderView(ctx, user, j, true, nil)
		return err
	})
	return out, err
}

func (s *Service) HandleFlag(ctx context.Context, user, id int32, note string) (JournalForMentor, error) {
	var out JournalForMentor
	err := s.write(ctx, func(w *World) error {
		j, err := w.mentorJournal(ctx, user, id, true)
		if err != nil {
			return err
		}
		status := deref(j.FlagStatus)
		if status != "open" && status != "escalated" {
			return fail(409, "Tidak ada bendera yang perlu ditangani.")
		}
		text, err := RequireText(note, "Catatan privat", 500)
		if err != nil {
			return err
		}
		j.FlagHandledBy, j.FlagPrivateNote = &user, &text
		// An escalated flag stays with the pengelola; only an open one becomes handled.
		if status == "open" {
			j.FlagStatus, j.FlagHandledAt = strPtr("handled"), timestamptz(w.Now)
		}
		if j, err = SaveJournal(ctx, w.Q, j); err != nil {
			return err
		}
		lock := RespondLock(monthOf(j.Month), w.Now)
		out, err = w.mentorReaderView(ctx, user, j, lock == nil, lock)
		return err
	})
	return out, err
}

// ---- laporan mentoring -------------------------------------------------------------

func (s *Service) MyReports(ctx context.Context, user int32) ([]ReportListItem, error) {
	w, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := w.RequireRole(user, "mentor"); err != nil {
		return nil, err
	}
	reports, err := w.Q.PerintisReportsOfMentor(ctx, user)
	if err != nil {
		return nil, err
	}
	byMonth := map[string]dbgen.PerintisSessionReport{}
	for _, r := range reports {
		byMonth[monthOf(r.Month)] = r
	}
	items := []ReportListItem{}
	for _, m := range ReportMonths(w.Now) {
		item := ReportListItem{Month: m, Status: "not_started"}
		if r, ok := byMonth[m]; ok {
			item.Status, item.IsLate, item.VerifiedAt = r.Status, r.IsLate, iso(r.VerifiedAt)
			item.ReviewUnread = r.ReviewedAt.Valid && !r.ReviewReadAt.Valid
		}
		items = append(items, item)
	}
	return items, nil
}

func (w *World) reportDetail(ctx context.Context, viewer, mentor int32, month string) (ReportDetail, error) {
	me, err := w.Participant(viewer)
	if err != nil {
		return ReportDetail{}, err
	}
	m, ok := w.Participants[mentor]
	if !ok || !m.IsMentor {
		return ReportDetail{}, fail(404, "Mentor tidak ditemukan.")
	}
	isSelf := viewer == mentor && HasRole(me, "mentor")
	isCoordinator := HasRole(me, "coordinator") && m.CoordinatorUserID != nil && *m.CoordinatorUserID == viewer
	if !isSelf && !isCoordinator {
		return ReportDetail{}, fail(403, "Laporan ini di luar cakupanmu.")
	}
	r, err := w.ReportByMonth(ctx, mentor, month)
	if err != nil {
		return ReportDetail{}, err
	}
	journals, err := w.JournalsOfMentorsInMonth(ctx, []int32{mentor}, month)
	if err != nil {
		return ReportDetail{}, err
	}
	out := ReportDetail{Month: month, Mentor: w.MustPerson(mentor), Coordinator: w.Person(m.CoordinatorUserID),
		Curriculum: CurriculumFor(w.GroupTingkat(mentor), month), Tasks: Tasks(month, r, journals),
		CanEdit: isSelf && (r == nil || r.Status == "draft" || r.Status == "returned"), UnrespondedJournals: CountWaiting(journals)}
	if r != nil {
		view := w.ReportView(ctx, *r, !isSelf)
		out.Report = &view
		out.CanUnverify = isCoordinator && r.Status == "verified" && !w.Now.After(DeadlinesFor(month).UnverifyUntil)
	}
	return out, nil
}

func (s *Service) Report(ctx context.Context, viewer, mentor int32, month string) (ReportDetail, error) {
	w, err := s.read(ctx)
	if err != nil {
		return ReportDetail{}, err
	}
	if err := w.requireMonth(month); err != nil {
		return ReportDetail{}, err
	}
	return w.reportDetail(ctx, viewer, mentor, month)
}

// editableReport locks (or creates) the mentor's draft for the month.
func (w *World) editableReport(ctx context.Context, user int32, month string) (dbgen.PerintisSessionReport, error) {
	if _, err := w.RequireRole(user, "mentor"); err != nil {
		return dbgen.PerintisSessionReport{}, err
	}
	if err := w.requireMonth(month); err != nil {
		return dbgen.PerintisSessionReport{}, err
	}
	r, err := w.Q.PerintisReportByMonthForUpdate(ctx, dbgen.PerintisReportByMonthForUpdateParams{MentorUserID: user, Month: DateOf(month)})
	if err == nil {
		if r.Status != "draft" && r.Status != "returned" {
			return r, fail(409, "Laporan sudah diajukan dan tidak bisa diubah.")
		}
		return r, nil
	}
	if !notFound(err) {
		return r, err
	}
	code := CurriculumFor(w.GroupTingkat(user), month).Code
	r, err = w.Q.InsertPerintisReport(ctx, dbgen.InsertPerintisReportParams{MentorUserID: user, Month: DateOf(month), CurriculumCode: &code})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return r, fail(409, "Laporan sedang disimpan dari perangkat lain. Coba lagi.")
	}
	return r, err
}

// SaveReportInput is the mentor's report draft.
type SaveReportInput struct {
	SessionDate       *string `json:"sessionDate"`
	Mode              string  `json:"mode"`
	LocationOrLink    string  `json:"locationOrLink"`
	FollowsCurriculum bool    `json:"followsCurriculum"`
	DeviationReason   *string `json:"deviationReason"`
	Summary           string  `json:"summary"`
	Photos            []File  `json:"photos"`
	CaseNote          *string `json:"caseNote"`
}

func photoPrefix(mentor int32) string { return fmt.Sprintf("perintis/reports/%d/", mentor) }

func (s *Service) SaveReport(ctx context.Context, user int32, month string, in SaveReportInput) (ReportDetail, error) {
	if in.Mode != "luring" && in.Mode != "daring" {
		return ReportDetail{}, fail(422, "Pilih luring atau daring.")
	}
	if len([]rune(strings.TrimSpace(in.LocationOrLink))) > 500 {
		return ReportDetail{}, fail(422, "Tempat atau tautan maksimal 500 karakter.")
	}
	for _, text := range []string{in.Summary, deref(in.DeviationReason), deref(in.CaseNote)} {
		if len([]rune(strings.TrimSpace(text))) > MaxLong {
			return ReportDetail{}, fail(422, "Isian maksimal %d karakter.", MaxLong)
		}
	}
	if len(in.Photos) > 10 {
		return ReportDetail{}, fail(422, "Paling banyak 10 foto.")
	}
	var date pgtype.Date
	if in.SessionDate != nil && *in.SessionDate != "" {
		t, err := time.Parse("2006-01-02", *in.SessionDate)
		if err != nil {
			return ReportDetail{}, fail(422, "Tanggal sesi tidak valid.")
		}
		if start, ok := MonthStart(month); ok && t.Format("2006-01") != start.Format("2006-01") {
			return ReportDetail{}, fail(422, "Tanggal sesi harus di bulan %s.", MonthLabel(month))
		}
		date = pgtype.Date{Time: t, Valid: true}
	}
	var out ReportDetail
	err := s.write(ctx, func(w *World) error {
		r, err := w.editableReport(ctx, user, month)
		if err != nil {
			return err
		}
		photos := []File{}
		for _, p := range in.Photos {
			if !strings.HasPrefix(p.Key, photoPrefix(user)) {
				return fail(422, "Foto tidak dikenal. Unggah ulang fotonya.")
			}
			photos = append(photos, File{Key: p.Key, Name: p.Name, Mime: p.Mime, Size: p.Size})
		}
		r.SessionDate, r.Mode = date, in.Mode
		r.LocationOrLink = strPtr(strings.TrimSpace(in.LocationOrLink))
		code := CurriculumFor(w.GroupTingkat(user), month).Code
		r.CurriculumCode = &code
		r.DeviationReason = nil
		if !in.FollowsCurriculum {
			r.DeviationReason = nilIfEmpty(deref(in.DeviationReason))
		}
		r.Summary = strPtr(strings.TrimSpace(in.Summary))
		if r.Photos, err = json.Marshal(photos); err != nil {
			return err
		}
		note := nilIfEmpty(deref(in.CaseNote))
		if deref(note) != deref(r.CaseNote) {
			r.CaseNote, r.CaseStatus = note, nil
			r.FollowupBy, r.FollowupAt, r.FollowupNote = nil, pgtype.Timestamptz{}, nil
			if note != nil {
				r.CaseStatus = strPtr("open")
			}
		}
		if _, err := SaveReport(ctx, w.Q, r); err != nil {
			return err
		}
		out, err = w.reportDetail(ctx, user, user, month)
		return err
	})
	return out, err
}

func (s *Service) SubmitReport(ctx context.Context, user int32, month string) (ReportDetail, error) {
	var out ReportDetail
	err := s.write(ctx, func(w *World) error {
		r, err := w.editableReport(ctx, user, month)
		if err != nil {
			return err
		}
		switch {
		case !r.SessionDate.Valid:
			return fail(422, "Isi tanggal sesi terlebih dulu.")
		case len(ParseFiles(r.Photos)) == 0:
			return fail(422, "Unggah minimal satu foto kelompok.")
		case deref(r.Summary) == "":
			return fail(422, "Isi catatan jalannya sesi.")
		case deref(r.LocationOrLink) == "":
			return fail(422, "Isi tempat atau tautan sesi.")
		}
		if !r.SubmittedAt.Valid {
			r.SubmittedAt = timestamptz(w.Now)
			r.IsLate = w.Now.After(DeadlinesFor(month).ReportDue)
		}
		r.Status = "submitted"
		if _, err := SaveReport(ctx, w.Q, r); err != nil {
			return err
		}
		out, err = w.reportDetail(ctx, user, user, month)
		return err
	})
	return out, err
}

func (s *Service) ReadReview(ctx context.Context, user int32, month string) error {
	return s.write(ctx, func(w *World) error {
		r, err := w.Q.PerintisReportByMonthForUpdate(ctx, dbgen.PerintisReportByMonthForUpdateParams{MentorUserID: user, Month: DateOf(month)})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if r.ReviewedAt.Valid && !r.ReviewReadAt.Valid {
			r.ReviewReadAt = timestamptz(w.Now)
			_, err = SaveReport(ctx, w.Q, r)
		}
		return err
	})
}

// UploadPhoto stores a group photo before the report is saved; the key is
// scoped to the mentor so only they can attach it.
func (s *Service) UploadPhoto(ctx context.Context, user int32, in FilePayload) (File, error) {
	w, err := s.read(ctx)
	if err != nil {
		return File{}, err
	}
	if _, err := w.RequireRole(user, "mentor"); err != nil {
		return File{}, err
	}
	data, mime, err := in.decode(false)
	if err != nil {
		return File{}, err
	}
	key := photoPrefix(user) + randomHex() + "-" + safeName(in.Name)
	if err := s.Storage.Put(ctx, key, data, storage.Object{ContentType: mime, CacheControl: "private, no-store", Private: true}); err != nil {
		return File{}, err
	}
	return File{Key: key, Name: strings.TrimSpace(in.Name), Mime: mime, Size: int64(len(data))}, nil
}

// ---- koordinator -------------------------------------------------------------------

func (s *Service) CoordinatorOverview(ctx context.Context, user int32, month string) (CoordinatorOverview, error) {
	w, err := s.read(ctx)
	if err != nil {
		return CoordinatorOverview{}, err
	}
	if _, err := w.RequireRole(user, "coordinator"); err != nil {
		return CoordinatorOverview{}, err
	}
	if month, err = w.monthOrCurrent(month); err != nil {
		return CoordinatorOverview{}, err
	}
	mentors := w.MentorsOf(user)
	mentorIDs, menteeIDs := []int32{}, []int32{}
	for _, m := range mentors {
		mentorIDs = append(mentorIDs, m.UserID)
		for _, mt := range w.MenteesOf(m.UserID) {
			menteeIDs = append(menteeIDs, mt.UserID)
		}
	}
	frozen, err := w.JournalsOfMentorsInMonth(ctx, mentorIDs, month)
	if err != nil {
		return CoordinatorOverview{}, err
	}
	menteeJournals, err := w.Q.PerintisJournalsOfMentees(ctx, menteeIDs)
	if err != nil {
		return CoordinatorOverview{}, err
	}
	reports, err := w.Q.PerintisReportsInMonth(ctx, DateOf(month))
	if err != nil {
		return CoordinatorOverview{}, err
	}
	reportOf := map[int32]dbgen.PerintisSessionReport{}
	for _, r := range reports {
		reportOf[r.MentorUserID] = r
	}
	lowPulse := map[string]bool{}
	for _, j := range menteeJournals {
		if JournalPulse(j).lowCount() >= 2 {
			lowPulse[strconv.Itoa(int(j.MenteeUserID))+monthOf(j.Month)] = true
		}
	}
	prev := PrevMonth(month)
	rows := []CoordinatorRow{}
	for _, m := range mentors {
		journals := []dbgen.PerintisJournal{}
		for _, j := range frozen {
			if j.MentorUserID != nil && *j.MentorUserID == m.UserID {
				journals = append(journals, j)
			}
		}
		row := CoordinatorRow{Mentor: w.MustPerson(m.UserID), MenteeCount: len(w.MenteesOf(m.UserID)), Tingkat: w.GroupTingkat(m.UserID)}
		var report *dbgen.PerintisSessionReport
		if r, ok := reportOf[m.UserID]; ok {
			report = &r
			row.Report = &CoordinatorReport{ID: r.ID, Status: r.Status, IsLate: r.IsLate, SubmittedAt: iso(r.SubmittedAt),
				VerifiedAt: iso(r.VerifiedAt), HasCase: r.CaseNote != nil}
		}
		for _, j := range journals {
			if j.Status != "draft" {
				row.Journals.Submitted++
			}
			if Responded(j.Status) {
				row.Journals.Responded++
			}
			if j.IsLate {
				row.Journals.Late++
			}
			if deref(j.FlagStatus) == "escalated" {
				row.EscalatedFlags++
			}
		}
		for _, mt := range w.MenteesOf(m.UserID) {
			key := strconv.Itoa(int(mt.UserID))
			if lowPulse[key+month] && lowPulse[key+prev] {
				row.PatternFlags++
			}
		}
		row.Tasks = Tasks(month, report, journals)
		rows = append(rows, row)
	}
	return CoordinatorOverview{Month: month, Rows: rows}, nil
}

// ReviewInput is the koordinator's decision on a report.
type ReviewInput struct {
	Decision     string `json:"decision"`
	Appreciation string `json:"appreciation"`
	Note         string `json:"note"`
}

func (s *Service) ReviewReport(ctx context.Context, user, id int32, in ReviewInput) (ReportDetail, error) {
	var out ReportDetail
	err := s.write(ctx, func(w *World) error {
		if _, err := w.RequireRole(user, "coordinator"); err != nil {
			return err
		}
		r, err := w.Q.PerintisReportForUpdate(ctx, id)
		if notFound(err) {
			return fail(404, "Laporan tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		if c := w.Participants[r.MentorUserID].CoordinatorUserID; c == nil || *c != user {
			return fail(403, "Laporan ini di luar cakupanmu.")
		}
		month := monthOf(r.Month)
		switch in.Decision {
		case "unverify":
			if r.Status != "verified" {
				return fail(409, "Laporan belum diverifikasi.")
			}
			if w.Now.After(DeadlinesFor(month).UnverifyUntil) {
				return fail(409, "Verifikasi hanya bisa dibatalkan sampai tanggal 20.")
			}
			r.Status, r.VerifiedAt = "submitted", pgtype.Timestamptz{}
		case "verify", "return":
			if r.Status != "submitted" {
				return fail(409, "Hanya laporan yang diajukan yang bisa diperiksa.")
			}
			appreciation, err := RequireText(in.Appreciation, "Apresiasi", MaxLong)
			if err != nil {
				return err
			}
			note, err := RequireText(in.Note, "Catatan", MaxLong)
			if err != nil {
				return err
			}
			if in.Decision == "verify" {
				journals, err := w.JournalsOfMentorsInMonth(ctx, []int32{r.MentorUserID}, month)
				if err != nil {
					return err
				}
				if pending := CountWaiting(journals); pending > 0 {
					return fail(422, "Belum bisa diverifikasi: masih ada %d lapbul terkirim yang belum ditanggapi mentor.", pending)
				}
				r.Status, r.VerifiedAt = "verified", timestamptz(w.Now)
			} else {
				r.Status = "returned"
			}
			r.ReviewAppreciation, r.ReviewNote, r.ReviewerKind = &appreciation, &note, strPtr("public")
			r.ReviewedBy, r.ReviewedAt, r.ReviewReadAt = &user, timestamptz(w.Now), pgtype.Timestamptz{}
		default:
			return fail(422, "Pilih keputusan.")
		}
		if _, err := SaveReport(ctx, w.Q, r); err != nil {
			return err
		}
		out, err = w.reportDetail(ctx, user, r.MentorUserID, month)
		return err
	})
	return out, err
}

// ---- files -----------------------------------------------------------------------------

var journalKey = regexp.MustCompile(`^perintis/journals/(\d+)/[^/]+$`)
var reportKey = regexp.MustCompile(`^perintis/reports/(\d+)/[^/]+$`)

func hasKey(files []File, key string) bool {
	for _, f := range files {
		if f.Key == key {
			return true
		}
	}
	return false
}

// File streams an uploaded file to someone allowed to see it: a lapbul file
// to the mentee and their mentor; a group photo to the mentor and their
// koordinator.
func (s *Service) File(ctx context.Context, viewer int32, key string) ([]byte, string, error) {
	w, err := s.read(ctx)
	if err != nil {
		return nil, "", err
	}
	me, err := w.Participant(viewer)
	if err != nil {
		return nil, "", err
	}
	denied := fail(404, "Berkas tidak ditemukan.")
	if m := journalKey.FindStringSubmatch(key); m != nil {
		id, _ := strconv.Atoi(m[1])
		j, err := w.Q.PerintisJournal(ctx, int32(id))
		if notFound(err) || (err == nil && !hasKey(ParseFiles(j.Files), key)) {
			return nil, "", denied
		}
		if err != nil {
			return nil, "", err
		}
		isMentor := HasRole(me, "mentor") && j.MentorUserID != nil && *j.MentorUserID == viewer && j.Status != "draft"
		if j.MenteeUserID != viewer && !isMentor {
			return nil, "", denied
		}
	} else if m := reportKey.FindStringSubmatch(key); m != nil {
		mentor, _ := strconv.Atoi(m[1])
		owner := w.Participants[int32(mentor)]
		isCoordinator := HasRole(me, "coordinator") && owner.CoordinatorUserID != nil && *owner.CoordinatorUserID == viewer
		if int32(mentor) != viewer && !isCoordinator {
			return nil, "", denied
		}
	} else {
		return nil, "", denied
	}
	data, mime, err := s.Storage.Get(ctx, key)
	if err != nil {
		return nil, "", denied
	}
	return data, mime, nil
}

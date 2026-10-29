package perintis

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/dbgen"
)

// World is a request-scoped view of the programme: every participant row
// (a few hundred at most) and their kaderisasi profile data, loaded once.
// Journals and reports are read on demand.
type World struct {
	Q            *dbgen.Queries
	Now          time.Time
	Participants map[int32]dbgen.PerintisParticipant
	People       map[int32]dbgen.PerintisPeopleRow
	activities   map[int32][]dbgen.PerintisActivitiesRow
	clubs        map[int32][]dbgen.PerintisClubsRow
	adminNames   map[int32]string
}

// Load reads all participants and their people rows, escalating overdue
// wellbeing flags first (an open flag becomes escalated after 3 days).
func Load(ctx context.Context, q *dbgen.Queries, now time.Time) (*World, error) {
	if _, err := q.EscalatePerintisFlags(ctx, timestamptz(now)); err != nil {
		return nil, err
	}
	rows, err := q.PerintisParticipants(ctx)
	if err != nil {
		return nil, err
	}
	w := &World{Q: q, Now: now, Participants: map[int32]dbgen.PerintisParticipant{}, People: map[int32]dbgen.PerintisPeopleRow{}}
	ids := make([]int32, 0, len(rows))
	for _, p := range rows {
		w.Participants[p.UserID] = p
		ids = append(ids, p.UserID)
	}
	if err := w.loadPeople(ctx, ids); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *World) loadPeople(ctx context.Context, ids []int32) error {
	missing := []int32{}
	for _, id := range ids {
		if _, ok := w.People[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	people, err := w.Q.PerintisPeople(ctx, missing)
	if err != nil {
		return err
	}
	for _, p := range people {
		// The member site and admin keep kampus, prodi and angkatan in the
		// education history; the older profile columns are the fallback.
		if inst, major, intake, ok := CurrentEducation(p.EducationHistory); ok {
			if inst != "" {
				p.University = &inst
			}
			if major != "" {
				p.Major = &major
			}
			if intake > 0 {
				p.IntakeYear = &intake
			}
		}
		w.People[p.UserID] = p
	}
	return nil
}

// CurrentEducation reads the latest entry of a profile's education_history
// (the last one, as the admin member list shows it). Entries may be null or
// carry the year as a string; ok is false when there is no usable entry.
func CurrentEducation(raw []byte) (institution, major string, intake int32, ok bool) {
	var entries []*struct {
		Institution string          `json:"institution"`
		Major       string          `json:"major"`
		IntakeYear  json.RawMessage `json:"intake_year"`
	}
	if json.Unmarshal(raw, &entries) != nil {
		return "", "", 0, false
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e == nil {
			continue
		}
		institution, major = strings.TrimSpace(e.Institution), strings.TrimSpace(e.Major)
		year := strings.Trim(string(e.IntakeYear), `" `)
		if n, err := strconv.Atoi(year); err == nil && n > 1900 && n < 3000 {
			intake = int32(n)
		}
		if institution != "" || major != "" || intake > 0 {
			return institution, major, intake, true
		}
	}
	return "", "", 0, false
}

// LoadDetails reads activities and clubs for the given people (profiles,
// lapbul Bagian 0, exports).
func (w *World) LoadDetails(ctx context.Context, ids []int32) error {
	if w.activities == nil {
		w.activities, w.clubs = map[int32][]dbgen.PerintisActivitiesRow{}, map[int32][]dbgen.PerintisClubsRow{}
	}
	need := []int32{}
	for _, id := range ids {
		if _, ok := w.activities[id]; !ok {
			need = append(need, id)
			w.activities[id], w.clubs[id] = nil, nil
		}
	}
	if len(need) == 0 {
		return nil
	}
	activities, err := w.Q.PerintisActivities(ctx, need)
	if err != nil {
		return err
	}
	for _, a := range activities {
		w.activities[deref32(a.UserID)] = append(w.activities[deref32(a.UserID)], a)
	}
	clubs, err := w.Q.PerintisClubs(ctx, need)
	if err != nil {
		return err
	}
	for _, c := range clubs {
		w.clubs[deref32(c.UserID)] = append(w.clubs[deref32(c.UserID)], c)
	}
	return nil
}

// AdminNames resolves staff names for follow-ups and on-behalf reviews.
func (w *World) AdminName(ctx context.Context, id *int32) string {
	if id == nil {
		return ""
	}
	if w.adminNames == nil {
		w.adminNames = map[int32]string{}
	}
	if name, ok := w.adminNames[*id]; ok {
		return name
	}
	rows, err := w.Q.PerintisAdminNames(ctx, []int32{*id})
	name := "Pengelola"
	if err == nil && len(rows) == 1 {
		name = rows[0].Name
	}
	w.adminNames[*id] = name
	return name
}

// ---- participants and roles ----------------------------------------------------

// Participant returns the row, or a 403 telling the person they are not enrolled.
func (w *World) Participant(id int32) (dbgen.PerintisParticipant, error) {
	p, ok := w.Participants[id]
	if !ok {
		return p, fail(403, "Akunmu belum terdaftar sebagai peserta pembinaan. Hubungi mentor.")
	}
	return p, nil
}

func (w *World) Find(id int32) (dbgen.PerintisParticipant, error) {
	p, ok := w.Participants[id]
	if !ok {
		return p, fail(404, "Peserta tidak ditemukan.")
	}
	return p, nil
}

func Roles(p dbgen.PerintisParticipant) []string {
	roles := []string{}
	if p.Status == "inactive" {
		return roles
	}
	if p.IsMentee {
		roles = append(roles, "mentee")
	}
	if p.IsMentor {
		roles = append(roles, "mentor")
	}
	if p.IsCoordinator {
		roles = append(roles, "coordinator")
	}
	return roles
}

func HasRole(p dbgen.PerintisParticipant, role string) bool {
	for _, r := range Roles(p) {
		if r == role {
			return true
		}
	}
	return false
}

func (w *World) RequireRole(id int32, role string) (dbgen.PerintisParticipant, error) {
	p, err := w.Participant(id)
	if err != nil {
		return p, err
	}
	if !HasRole(p, role) {
		return p, fail(403, "Halaman ini tidak tersedia untuk peranmu.")
	}
	return p, nil
}

func sortedByName(w *World, list []dbgen.PerintisParticipant) []dbgen.PerintisParticipant {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := w.Name(list[i].UserID), w.Name(list[j].UserID)
		if a != b {
			return a < b
		}
		return list[i].UserID < list[j].UserID
	})
	return list
}

// MenteesOf lists a mentor's active mentees.
func (w *World) MenteesOf(mentor int32) []dbgen.PerintisParticipant {
	list := []dbgen.PerintisParticipant{}
	for _, p := range w.Participants {
		if p.IsMentee && p.MentorUserID != nil && *p.MentorUserID == mentor && p.Status == "active" {
			list = append(list, p)
		}
	}
	return sortedByName(w, list)
}

// MentorsOf lists a koordinator's mentors that are not inactive.
func (w *World) MentorsOf(coordinator int32) []dbgen.PerintisParticipant {
	list := []dbgen.PerintisParticipant{}
	for _, p := range w.Participants {
		if p.IsMentor && p.CoordinatorUserID != nil && *p.CoordinatorUserID == coordinator && p.Status != "inactive" {
			list = append(list, p)
		}
	}
	return sortedByName(w, list)
}

func (w *World) All() []dbgen.PerintisParticipant {
	list := make([]dbgen.PerintisParticipant, 0, len(w.Participants))
	for _, p := range w.Participants {
		list = append(list, p)
	}
	return sortedByName(w, list)
}

func (w *World) CoordinatorOfMentor(mentor *int32) *int32 {
	if mentor == nil {
		return nil
	}
	if m, ok := w.Participants[*mentor]; ok {
		return m.CoordinatorUserID
	}
	return nil
}

// ---- people --------------------------------------------------------------------

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (w *World) Name(id int32) string {
	if p, ok := w.People[id]; ok {
		return p.Name
	}
	return ""
}

func (w *World) NamePtr(id *int32) *string {
	if id == nil {
		return nil
	}
	if p, ok := w.People[*id]; ok {
		name := p.Name
		return &name
	}
	return nil
}

func (w *World) Person(id *int32) *PersonSummary {
	if id == nil {
		return nil
	}
	p, ok := w.People[*id]
	if !ok {
		return nil
	}
	return &PersonSummary{UserID: p.UserID, Name: p.Name, Phone: p.Whatsapp}
}

func (w *World) MustPerson(id int32) PersonSummary {
	if p := w.Person(&id); p != nil {
		return *p
	}
	return PersonSummary{UserID: id}
}

func (w *World) TingkatOf(id int32) int {
	if p, ok := w.People[id]; ok && p.IntakeYear != nil {
		return Tingkat(*p.IntakeYear, w.Now)
	}
	return 1
}

// GroupTingkat is the majority tingkat of a mentor's active mentees; ties go
// to the lower tingkat (PRD §9.3).
func (w *World) GroupTingkat(mentor int32) int {
	counts := map[int]int{}
	for _, m := range w.MenteesOf(mentor) {
		counts[w.TingkatOf(m.UserID)]++
	}
	best, bestCount := 1, 0
	for t := 1; t <= 4; t++ {
		if counts[t] > bestCount {
			best, bestCount = t, counts[t]
		}
	}
	return best
}

func levelLabel(level int32) string {
	switch {
	case level >= 10:
		return "Kader Lanjut"
	case level >= 6:
		return "Kader"
	case level >= 3:
		return "Aktivis"
	}
	return "Jamaah"
}

func titleStatus(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// cadreStatus reads SSC (activity_type 2) or LMD (3) from registrations,
// falling back to profile badges such as "SSC-4".
func (w *World) cadreStatus(id int32, activityType int32, badge string) string {
	latest := ""
	for _, a := range w.activities[id] {
		if a.ActivityType == nil || *a.ActivityType != activityType {
			continue
		}
		if a.Status == "LULUS KEGIATAN" {
			return "Lulus"
		}
		if latest == "" && a.Status != "" {
			latest = titleStatus(a.Status)
		}
	}
	if latest != "" {
		return latest
	}
	var badges []string
	if p, ok := w.People[id]; ok {
		_ = json.Unmarshal(p.Badges, &badges)
	}
	for _, b := range badges {
		if strings.HasPrefix(strings.ToUpper(b), badge) {
			return "Lulus"
		}
	}
	return "Belum"
}

// Profile needs LoadDetails for the person when SSC/LMD statuses matter.
func (w *World) Profile(id int32) Profile {
	p := w.People[id]
	gender := ""
	switch deref(p.Gender) {
	case "M":
		gender = "L"
	case "F":
		gender = "P"
	}
	domicile := strings.Trim(strings.Join([]string{deref(p.City), deref(p.Province)}, ", "), ", ")
	intake := int32(0)
	if p.IntakeYear != nil {
		intake = *p.IntakeYear
	}
	return Profile{UserID: id, Name: p.Name, Email: deref(p.Email), Gender: gender, Phone: deref(p.Whatsapp),
		Instagram: p.Instagram, Linkedin: p.Linkedin, University: deref(p.University), Major: deref(p.Major),
		IntakeYear: intake, Domicile: domicile, Level: levelLabel(p.Level),
		SSCStatus: w.cadreStatus(id, 2, "SSC"), LMDStatus: w.cadreStatus(id, 3, "LMD")}
}

func (w *World) HasAccount(id int32) bool {
	p, ok := w.People[id]
	return ok && p.AccountStatus != "no_account"
}

func (w *World) ParticipantView(p dbgen.PerintisParticipant) Participant {
	return Participant{UserID: p.UserID, IsMentee: p.IsMentee, IsMentor: p.IsMentor, IsCoordinator: p.IsCoordinator,
		Nim: p.Nim, KipK: p.KipK, Category: p.Category, KaderisasiOverride: p.KaderisasiOverride,
		KaderisasiOverrideNote: p.KaderisasiOverrideNote, MentorUserID: p.MentorUserID,
		CoordinatorUserID: p.CoordinatorUserID, Status: p.Status, InactiveReason: p.InactiveReason,
		HasAccount: w.HasAccount(p.UserID)}
}

var activityCategories = map[int32]string{0: "Pelatihan", 1: "Keasramaan", 2: "Kaderisasi", 3: "Aktualisasi Diri", 4: "Kealumnian"}
var achievementTypes = map[int32]string{0: "Kompetisi", 1: "Organisasi", 2: "Akademik"}
var achievementStatuses = map[int32]string{0: "Menunggu verifikasi", 1: "Disetujui", 2: "Ditolak"}

func (w *World) Activities(id int32) []ActivityItem {
	list := []ActivityItem{}
	for _, a := range w.activities[id] {
		item := ActivityItem{Name: a.Name, Status: titleStatus(a.Status), Certificate: a.HasCertificate}
		if a.ActivityCategory != nil {
			item.Category = activityCategories[*a.ActivityCategory]
		}
		if a.ActivityStart.Valid {
			item.Date = a.ActivityStart.Time.Format("2006-01-02")
		}
		list = append(list, item)
	}
	return list
}

func (w *World) Clubs(id int32) []ClubItem {
	list := []ClubItem{}
	for _, c := range w.clubs[id] {
		list = append(list, ClubItem{Name: c.Name, Role: c.Role, Status: "Aktif"})
	}
	return list
}

func (w *World) HasClub(id int32) bool { return len(w.clubs[id]) > 0 }

func (w *World) Achievements(ctx context.Context, id int32) ([]Achievement, error) {
	rows, err := w.Q.PerintisAchievements(ctx, []int32{id})
	if err != nil {
		return nil, err
	}
	list := []Achievement{}
	for _, a := range rows {
		item := Achievement{Name: a.Name, Status: achievementStatuses[a.Status]}
		if a.Type != nil {
			item.Level = achievementTypes[*a.Type]
		}
		if a.AchievementDate.Valid {
			item.Year = a.AchievementDate.Time.Year()
		}
		list = append(list, item)
	}
	return list, nil
}

// ---- journals and reports --------------------------------------------------------

func timestamptz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func DateOf(month string) pgtype.Date {
	t, _ := time.Parse("2006-01-02", month)
	return pgtype.Date{Time: t, Valid: true}
}

func monthOf(d pgtype.Date) string { return d.Time.Format("2006-01-02") }

func iso(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339Nano)
	return &s
}

func isoValue(t pgtype.Timestamptz) string {
	if s := iso(t); s != nil {
		return *s
	}
	return ""
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// JournalByMonth returns nil when the mentee has no row for the month yet.
func (w *World) JournalByMonth(ctx context.Context, mentee int32, month string) (*dbgen.PerintisJournal, error) {
	j, err := w.Q.PerintisJournalByMonth(ctx, dbgen.PerintisJournalByMonthParams{MenteeUserID: mentee, Month: DateOf(month)})
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (w *World) ReportByMonth(ctx context.Context, mentor int32, month string) (*dbgen.PerintisSessionReport, error) {
	r, err := w.Q.PerintisReportByMonth(ctx, dbgen.PerintisReportByMonthParams{MentorUserID: mentor, Month: DateOf(month)})
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func ParseFiles(raw []byte) []File {
	files := []File{}
	_ = json.Unmarshal(raw, &files)
	for i := range files {
		files[i].PreviewURL = nil
	}
	return files
}

func JournalPulse(j dbgen.PerintisJournal) Pulse {
	return Pulse{Energy: j.PulseEnergy, Academic: j.PulseAcademic, Finance: j.PulseFinance, Relation: j.PulseRelation}
}

// JournalView converts a row. Names of the responding mentor and of the
// staff who followed up are resolved through the world.
func (w *World) JournalView(ctx context.Context, j dbgen.PerintisJournal) (Journal, error) {
	view := Journal{ID: j.ID, MenteeUserID: j.MenteeUserID, Month: monthOf(j.Month), MentorUserID: j.MentorUserID,
		Category: j.Category, FormVersion: j.FormVersion, Status: j.Status, Answers: ParseAnswers(j.Answers),
		Files: ParseFiles(j.Files), Pulse: JournalPulse(j), WantsContact: j.WantsContact,
		HonestyConfirmedAt: iso(j.HonestyConfirmedAt), SubmittedAt: iso(j.SubmittedAt), IsLate: j.IsLate,
		Revision: j.Revision, ReopenReason: j.ReopenReason, UpdatedAt: isoValue(j.UpdatedAt)}
	ids := []int32{}
	for _, id := range []*int32{j.RespondedBy, j.FlagHandledBy} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if err := w.loadPeople(ctx, ids); err != nil {
		return view, err
	}
	if j.RespondedAt.Valid {
		view.Response = &JournalResponse{Appreciation: deref(j.ResponseAppreciation), Text: deref(j.ResponseText),
			RevisionRequest: j.RevisionRequest, FlagReason: j.FlagReason, RespondedBy: deref(w.NamePtr(j.RespondedBy)),
			RespondedAt: isoValue(j.RespondedAt), ReadAt: iso(j.ResponseReadAt)}
	}
	if j.FlagTrigger != nil {
		view.Flag = &JournalFlag{Trigger: *j.FlagTrigger, Status: deref(j.FlagStatus), DueAt: isoValue(j.FlagDueAt),
			HandledBy: w.NamePtr(j.FlagHandledBy), HandledAt: iso(j.FlagHandledAt), PrivateNote: j.FlagPrivateNote}
	}
	if j.FollowupAt.Valid {
		view.Followup = &Followup{By: w.AdminName(ctx, j.FollowupBy), At: isoValue(j.FollowupAt), Note: deref(j.FollowupNote)}
	}
	return view, nil
}

// ReportView converts a row; hideCase hides the case note from koordinator.
func (w *World) ReportView(ctx context.Context, r dbgen.PerintisSessionReport, hideCase bool) SessionReport {
	view := SessionReport{ID: r.ID, MentorUserID: r.MentorUserID, Month: monthOf(r.Month), Mode: r.Mode,
		LocationOrLink: deref(r.LocationOrLink), CurriculumCode: deref(r.CurriculumCode),
		DeviationReason: r.DeviationReason, Summary: deref(r.Summary), Photos: ParseFiles(r.Photos),
		CaseNote: r.CaseNote, HasCase: r.CaseNote != nil, CaseStatus: r.CaseStatus, Status: r.Status,
		SubmittedAt: iso(r.SubmittedAt), IsLate: r.IsLate, VerifiedAt: iso(r.VerifiedAt)}
	if hideCase {
		view.CaseNote = nil
	}
	if r.SessionDate.Valid {
		date := r.SessionDate.Time.Format("2006-01-02")
		view.SessionDate = &date
	}
	if r.ReviewedAt.Valid {
		reviewer := ""
		if deref(r.ReviewerKind) == "admin" {
			reviewer = w.AdminName(ctx, r.ReviewedBy)
		} else {
			_ = w.loadPeople(ctx, []int32{deref32(r.ReviewedBy)})
			reviewer = deref(w.NamePtr(r.ReviewedBy))
		}
		view.Review = &ReportReview{Appreciation: deref(r.ReviewAppreciation), Note: deref(r.ReviewNote),
			ReviewerKind: deref(r.ReviewerKind), ReviewedBy: reviewer, ReviewedAt: isoValue(r.ReviewedAt),
			ReadAt: iso(r.ReviewReadAt)}
	}
	return view
}

func deref32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

// SaveJournal writes every mutable column of the row back.
func SaveJournal(ctx context.Context, q *dbgen.Queries, j dbgen.PerintisJournal) (dbgen.PerintisJournal, error) {
	return q.SavePerintisJournal(ctx, dbgen.SavePerintisJournalParams{
		ID: j.ID, MentorUserID: j.MentorUserID, Status: j.Status, Answers: j.Answers, Files: j.Files,
		PulseEnergy: j.PulseEnergy, PulseAcademic: j.PulseAcademic, PulseFinance: j.PulseFinance,
		PulseRelation: j.PulseRelation, WantsContact: j.WantsContact, HonestyConfirmedAt: j.HonestyConfirmedAt,
		SubmittedAt: j.SubmittedAt, IsLate: j.IsLate, Revision: j.Revision, ReopenedBy: j.ReopenedBy,
		ReopenReason: j.ReopenReason, ResponseAppreciation: j.ResponseAppreciation, ResponseText: j.ResponseText,
		RevisionRequest: j.RevisionRequest, FlagReason: j.FlagReason, RespondedBy: j.RespondedBy,
		RespondedAt: j.RespondedAt, ResponseReadAt: j.ResponseReadAt, FlagTrigger: j.FlagTrigger,
		FlagStatus: j.FlagStatus, FlagDueAt: j.FlagDueAt, FlagHandledBy: j.FlagHandledBy,
		FlagHandledAt: j.FlagHandledAt, FlagPrivateNote: j.FlagPrivateNote, FollowupBy: j.FollowupBy,
		FollowupAt: j.FollowupAt, FollowupNote: j.FollowupNote,
	})
}

func SaveReport(ctx context.Context, q *dbgen.Queries, r dbgen.PerintisSessionReport) (dbgen.PerintisSessionReport, error) {
	return q.SavePerintisReport(ctx, dbgen.SavePerintisReportParams{
		ID: r.ID, SessionDate: r.SessionDate, Mode: r.Mode, LocationOrLink: r.LocationOrLink,
		CurriculumCode: r.CurriculumCode, DeviationReason: r.DeviationReason, Summary: r.Summary, Photos: r.Photos,
		CaseNote: r.CaseNote, CaseStatus: r.CaseStatus, FollowupBy: r.FollowupBy, FollowupAt: r.FollowupAt,
		FollowupNote: r.FollowupNote, Status: r.Status, SubmittedAt: r.SubmittedAt, IsLate: r.IsLate,
		ReviewAppreciation: r.ReviewAppreciation, ReviewNote: r.ReviewNote, ReviewerKind: r.ReviewerKind,
		ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt, ReviewReadAt: r.ReviewReadAt, VerifiedAt: r.VerifiedAt,
	})
}

func SaveParticipant(ctx context.Context, q *dbgen.Queries, p dbgen.PerintisParticipant) (dbgen.PerintisParticipant, error) {
	return q.SavePerintisParticipant(ctx, dbgen.SavePerintisParticipantParams{
		UserID: p.UserID, IsMentee: p.IsMentee, IsMentor: p.IsMentor, IsCoordinator: p.IsCoordinator, Nim: p.Nim,
		UniversityID: p.UniversityID, KipK: p.KipK, Category: p.Category, KaderisasiOverride: p.KaderisasiOverride,
		KaderisasiOverrideNote: p.KaderisasiOverrideNote, MentorUserID: p.MentorUserID,
		CoordinatorUserID: p.CoordinatorUserID, Status: p.Status, InactiveReason: p.InactiveReason, UpdatedBy: p.UpdatedBy,
	})
}

func strPtr(s string) *string { return &s }

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := strings.TrimSpace(s)
	return &v
}

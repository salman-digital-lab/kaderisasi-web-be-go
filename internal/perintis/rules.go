// Package perintis holds the Pembinaan Beasiswa Perintis rules and data
// access shared by kaderisasi-web-be-go (participants) and
// kaderisasi-admin-be-go (pengelola). The files rules.go, curriculum.go,
// views.go, data.go and logic.go are kept identical in both repositories
// apart from the module import path; see docs/perintis in the workspace.
package perintis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// WIB is Asia/Jakarta (no daylight saving), used for every programme date.
var WIB = time.FixedZone("WIB", 7*60*60)

// FirstMonth is the first reporting month; its lapbul opens 1 Nov 2026.
const FirstMonth = "2026-10-01"

// FormVersion is the version of the lapbul form defined in code.
const FormVersion = 1

const (
	MaxShort = 300
	MaxLong  = 1500
	MaxFile  = 5 << 20
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

// ---- months and deadlines -------------------------------------------------

// MonthStart parses a YYYY-MM-DD month key as midnight WIB on the 1st.
func MonthStart(month string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", month, WIB)
	if err != nil || t.Day() != 1 {
		return time.Time{}, false
	}
	return t, true
}

func MonthKey(t time.Time) string {
	t = t.In(WIB)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, WIB).Format("2006-01-02")
}

func addMonths(month string, n int) string {
	t, _ := MonthStart(month)
	return MonthKey(t.AddDate(0, n, 0))
}

func PrevMonth(month string) string { return addMonths(month, -1) }

// CurrentReportMonth is the month whose lapbul is open now (last month), or
// "" before the programme starts.
func CurrentReportMonth(now time.Time) string {
	month := MonthKey(time.Date(now.In(WIB).Year(), now.In(WIB).Month(), 1, 0, 0, 0, 0, WIB).AddDate(0, -1, 0))
	if month < FirstMonth {
		return ""
	}
	return month
}

// ReportMonths lists the report months so far, newest first.
func ReportMonths(now time.Time) []string {
	months := []string{}
	for m := CurrentReportMonth(now); m != "" && m >= FirstMonth; m = PrevMonth(m) {
		months = append(months, m)
	}
	return months
}

func dayEnd(next time.Time, day int) time.Time {
	return time.Date(next.Year(), next.Month(), day+1, 0, 0, 0, 0, WIB).Add(-time.Nanosecond)
}

// Deadlines are the fixed monthly dates (PRD §5.1), all 23.59 WIB in the
// month after the report month.
type Deadlines struct {
	OpensAt, MenteeDue, LockAt, MentorDue, ReportDue, ResponseEditableUntil, VerifyDue, UnverifyUntil time.Time
}

func DeadlinesFor(month string) Deadlines {
	start, _ := MonthStart(month)
	next := start.AddDate(0, 1, 0)
	return Deadlines{
		OpensAt: next, MenteeDue: dayEnd(next, 5), LockAt: dayEnd(next, 12), MentorDue: dayEnd(next, 12),
		ReportDue: dayEnd(next, 12), ResponseEditableUntil: dayEnd(next, 15), VerifyDue: dayEnd(next, 15),
		UnverifyUntil: dayEnd(next, 20),
	}
}

// Window is where a month's lapbul stands: upcoming, open, late or locked.
func Window(month string, at time.Time) string {
	d := DeadlinesFor(month)
	switch {
	case at.Before(d.OpensAt):
		return "upcoming"
	case !at.After(d.MenteeDue):
		return "open"
	case !at.After(d.LockAt):
		return "late"
	default:
		return "locked"
	}
}

// AsksForGrades reports whether the month's lapbul has section 7 (IP & KHS).
func AsksForGrades(month string) bool {
	t, _ := MonthStart(month)
	return t.Month() == time.January || t.Month() == time.July
}

// Tingkat is the study year 1–4 from the intake year; the academic year
// starts in August.
func Tingkat(intakeYear int32, at time.Time) int {
	if intakeYear <= 0 {
		return 1
	}
	at = at.In(WIB)
	start := at.Year()
	if at.Month() < time.August {
		start--
	}
	return min(4, max(1, start-int(intakeYear)+1))
}

var monthNames = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func MonthLabel(month string) string {
	t, ok := MonthStart(month)
	if !ok {
		return month
	}
	return fmt.Sprintf("%s %d", monthNames[t.Month()-1], t.Year())
}

// FormatDate renders D MMM YYYY in Indonesian (exports).
func FormatDate(t time.Time) string {
	t = t.In(WIB)
	return fmt.Sprintf("%d %s %d", t.Day(), monthNames[t.Month()-1][:3], t.Year())
}

// ---- labels ---------------------------------------------------------------

var CategoryLabel = map[string]string{"dasar": "Beswan dasar", "biaya_hidup": "Penerima biaya hidup"}

var JournalStatusLabel = map[string]string{
	"not_started": "Belum mengisi", "draft": "Draf", "submitted": "Terkirim", "accepted": "Diterima",
	"revision_requested": "Perlu perbaikan", "revised": "Direvisi", "flagged": "Ditandai ke pengelola",
}

var ReportStatusLabel = map[string]string{
	"not_started": "Belum dibuat", "draft": "Draf", "submitted": "Diajukan", "returned": "Dikembalikan", "verified": "Diverifikasi",
}

var FlagTriggerLabel = map[string]string{
	"low_pulse": "Dua skala pulse ≤ 2", "contact_request": "Ingin dihubungi", "both": "Pulse rendah & ingin dihubungi",
}

// Responded statuses carry a mentor response; waiting statuses need one.
func Responded(status string) bool {
	return status == "accepted" || status == "revision_requested" || status == "flagged"
}

func Waiting(status string) bool { return status == "submitted" || status == "revised" }

// ---- lapbul form ------------------------------------------------------------

// Answers are the lapbul answers kept in perintis_journals.answers. Text may
// be multiline; line breaks are stored as typed.
type Answers struct {
	S1b        *string  `json:"s1b,omitempty"`
	S1c        *string  `json:"s1c,omitempty"`
	S1d        *string  `json:"s1d,omitempty"`
	S2a        []string `json:"s2a,omitempty"`
	S2b        *string  `json:"s2b,omitempty"`
	Targets    []string `json:"targets,omitempty"`
	S3a        *string  `json:"s3a,omitempty"`
	S5aPages   *float64 `json:"s5aPages,omitempty"`
	S5aVerse   *string  `json:"s5aVerse,omitempty"`
	S5b        *string  `json:"s5b,omitempty"`
	S5cKinds   []string `json:"s5cKinds,omitempty"`
	S5cText    *string  `json:"s5cText,omitempty"`
	S5d        *float64 `json:"s5d,omitempty"`
	S6         *string  `json:"s6,omitempty"`
	S7ip       *float64 `json:"s7ip,omitempty"`
	S8aChanged *bool    `json:"s8aChanged,omitempty"`
	S8aText    *string  `json:"s8aText,omitempty"`
	S8b        []string `json:"s8b,omitempty"`
}

var targetResults = map[string]bool{"tercapai": true, "sebagian": true, "belum": true}
var sharingKinds = map[string]bool{"Waktu": true, "Tenaga": true, "Ilmu": true, "Harta": true}
var livingCostUses = map[string]bool{"Makan": true, "Transportasi": true, "Kebutuhan kuliah": true, "Tempat tinggal": true, "Membantu keluarga": true, "Lainnya": true}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func filled(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

// Clean trims text answers and rejects values outside the form.
func (a Answers) Clean() (Answers, error) {
	for _, p := range []**string{&a.S1b, &a.S1c, &a.S1d, &a.S2b, &a.S3a, &a.S5aVerse, &a.S5b, &a.S5cText, &a.S6, &a.S8aText} {
		*p = trimPtr(*p)
	}
	for _, s := range []*string{a.S1b, a.S5aVerse, a.S5b} {
		if s != nil && utf8.RuneCountInString(*s) > MaxShort {
			return a, fail(422, "Isian pendek maksimal %d karakter.", MaxShort)
		}
	}
	for _, s := range []*string{a.S1c, a.S1d, a.S2b, a.S3a, a.S5cText, a.S6, a.S8aText} {
		if s != nil && utf8.RuneCountInString(*s) > MaxLong {
			return a, fail(422, "Isian panjang maksimal %d karakter.", MaxLong)
		}
	}
	if len(a.Targets) > 3 || len(a.S2a) > 3 {
		return a, fail(422, "Target jika–maka paling banyak tiga.")
	}
	for i, t := range a.Targets {
		a.Targets[i] = strings.TrimSpace(t)
		if utf8.RuneCountInString(a.Targets[i]) > MaxShort {
			return a, fail(422, "Target maksimal %d karakter.", MaxShort)
		}
	}
	for _, v := range a.S2a {
		if v != "" && !targetResults[v] {
			return a, fail(422, "Status target tidak dikenal.")
		}
	}
	for _, v := range a.S5cKinds {
		if !sharingKinds[v] {
			return a, fail(422, "Bentuk berbagi tidak dikenal.")
		}
	}
	for _, v := range a.S8b {
		if !livingCostUses[v] {
			return a, fail(422, "Pemakaian bantuan tidak dikenal.")
		}
	}
	for _, n := range []*float64{a.S5aPages, a.S5d} {
		if n != nil && (*n < 0 || *n > 100000) {
			return a, fail(422, "Angka tidak valid.")
		}
	}
	if a.S7ip != nil && (*a.S7ip < 0 || *a.S7ip > 4) {
		return a, fail(422, "IP semester harus 0–4.")
	}
	return a, nil
}

// ParseAnswers reads the stored answers; unknown keys from newer forms are ignored.
func ParseAnswers(raw []byte) Answers {
	var a Answers
	_ = json.Unmarshal(raw, &a)
	return a
}

// Pulse is the four 1–5 wellbeing scales.
type Pulse struct {
	Energy   *int16 `json:"energy"`
	Academic *int16 `json:"academic"`
	Finance  *int16 `json:"finance"`
	Relation *int16 `json:"relation"`
}

func (p Pulse) values() []*int16 { return []*int16{p.Energy, p.Academic, p.Finance, p.Relation} }

func (p Pulse) Valid() bool {
	for _, v := range p.values() {
		if v != nil && (*v < 1 || *v > 5) {
			return false
		}
	}
	return true
}

func (p Pulse) lowCount() int {
	n := 0
	for _, v := range p.values() {
		if v != nil && *v <= 2 {
			n++
		}
	}
	return n
}

// FlagTrigger is the wellbeing rule (PRD §8.6): two scales ≤ 2 and/or 1e.
func FlagTrigger(p Pulse, wantsContact bool) string {
	low := p.lowCount() >= 2
	switch {
	case low && wantsContact:
		return "both"
	case low:
		return "low_pulse"
	case wantsContact:
		return "contact_request"
	}
	return ""
}

// MissingItems lists required answers still empty (PRD §8.3 per category).
func MissingItems(category, month string, a Answers, p Pulse, files []File, previousTargets []string) []string {
	missing := []string{}
	add := func(cond bool, label string) {
		if cond {
			missing = append(missing, label)
		}
	}
	for _, v := range p.values() {
		if v == nil {
			missing = append(missing, "1a Pulse (4 skala)")
			break
		}
	}
	add(!filled(a.S1b), "1b Satu kalimat tentang bulan ini")
	add(!filled(a.S1c), "1c Kabar keluarga")
	answered := 0
	for _, v := range a.S2a {
		if v != "" {
			answered++
		}
	}
	add(len(previousTargets) > 0 && answered < len(previousTargets), "2a Status target bulan lalu")
	add(!filled(a.S2b), "2b Yang membantu atau menghambat")
	targets := 0
	for _, t := range a.Targets {
		if strings.TrimSpace(t) != "" {
			targets++
		}
	}
	add(targets < 3, "2c Tiga target bulan depan")
	add(!filled(a.S3a), "3a Insight terbesar")
	add(a.S5aPages == nil, "5a Jumlah bacaan")
	add(!filled(a.S5aVerse), "5a Ayat/pesan berkesan")
	add(!filled(a.S5b), "5b Amalan yang dijaga")
	add(len(a.S5cKinds) == 0, "5c Bentuk berbagi")
	add(!filled(a.S6), "6 Teman Perintis")
	if AsksForGrades(month) {
		add(a.S7ip == nil, "7 IP semester")
		hasKHS := false
		for _, f := range files {
			hasKHS = hasKHS || f.Field == "s7khs"
		}
		add(!hasKHS, "7 Unggah KHS")
	}
	if category == "biaya_hidup" {
		add(a.S8aChanged == nil, "8a Perubahan kondisi ekonomi")
		add(a.S8aChanged != nil && *a.S8aChanged && !filled(a.S8aText), "8a Penjelasan")
		add(len(a.S8b) == 0, "8b Pemakaian bantuan")
	}
	return missing
}

// RequireText trims a required free-text input and checks its length.
func RequireText(value, label string, maxLen int) (string, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", fail(422, "%s wajib diisi.", label)
	}
	if utf8.RuneCountInString(text) > maxLen {
		return "", fail(422, "%s maksimal %d karakter.", label, maxLen)
	}
	return text, nil
}

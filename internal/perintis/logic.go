package perintis

import (
	"context"
	"sort"
	"time"

	"kaderisasi/web/internal/dbgen"
)

// JournalsOfMentorsInMonth reads the month's lapbul frozen to these mentors.
func (w *World) JournalsOfMentorsInMonth(ctx context.Context, mentors []int32, month string) ([]dbgen.PerintisJournal, error) {
	if len(mentors) == 0 {
		return []dbgen.PerintisJournal{}, nil
	}
	return w.Q.PerintisJournalsOfMentorsInMonth(ctx, dbgen.PerintisJournalsOfMentorsInMonthParams{MentorIds: mentors, Month: DateOf(month)})
}

func CountWaiting(journals []dbgen.PerintisJournal) int {
	n := 0
	for _, j := range journals {
		if Waiting(j.Status) {
			n++
		}
	}
	return n
}

// Tasks are T1–T3, computed and never stored (PRD §9.4): T1 report verified,
// T2 every submitted lapbul answered by the 12th, T3 report submitted by the 12th.
func Tasks(month string, report *dbgen.PerintisSessionReport, journals []dbgen.PerintisJournal) MentorTasks {
	due := DeadlinesFor(month)
	t2 := true
	for _, j := range journals {
		if j.Status == "draft" {
			continue
		}
		if !Responded(j.Status) || !j.RespondedAt.Valid || j.RespondedAt.Time.After(due.MentorDue) {
			t2 = false
		}
	}
	return MentorTasks{
		T1: report != nil && report.Status == "verified",
		T2: t2,
		T3: report != nil && report.SubmittedAt.Valid && !report.SubmittedAt.Time.After(due.ReportDue),
	}
}

func journalIndex(journals []dbgen.PerintisJournal) map[string]dbgen.PerintisJournal {
	index := map[string]dbgen.PerintisJournal{}
	for _, j := range journals {
		index[monthOf(j.Month)] = j
	}
	return index
}

// ListItem is one month in a mentee's lapbul list.
func (w *World) ListItem(month string, j *dbgen.PerintisJournal) JournalListItem {
	item := JournalListItem{Month: month, Status: "not_started", Window: Window(month, w.Now)}
	if j == nil {
		return item
	}
	id := j.ID
	item.JournalID, item.Status, item.IsLate, item.SubmittedAt = &id, j.Status, j.IsLate, iso(j.SubmittedAt)
	item.ResponseUnread = j.RespondedAt.Valid && !j.ResponseReadAt.Valid
	if j.ReopenReason != nil {
		item.Window = "open"
	}
	return item
}

// JournalList lists every report month for a mentee, newest first.
func (w *World) JournalList(ctx context.Context, mentee int32) ([]JournalListItem, error) {
	journals, err := w.Q.PerintisJournalsOfMentees(ctx, []int32{mentee})
	if err != nil {
		return nil, err
	}
	index := journalIndex(journals)
	items := []JournalListItem{}
	for _, m := range ReportMonths(w.Now) {
		if j, ok := index[m]; ok {
			items = append(items, w.ListItem(m, &j))
		} else {
			items = append(items, w.ListItem(m, nil))
		}
	}
	return items, nil
}

func submitted(j *dbgen.PerintisJournal) bool { return j != nil && j.Status != "draft" }

// InactiveStreak is two report months in a row without a submitted lapbul.
func InactiveStreak(month string, index map[string]dbgen.PerintisJournal) bool {
	prev := PrevMonth(month)
	if prev < FirstMonth {
		return false
	}
	cur, okCur := index[month]
	old, okOld := index[prev]
	return !(okCur && submitted(&cur)) && !(okOld && submitted(&old))
}

// PreviousTargets are last month's three targets, asked about in 2a.
func (w *World) PreviousTargets(ctx context.Context, mentee int32, month string) ([]string, error) {
	prev := PrevMonth(month)
	if prev < FirstMonth {
		return nil, nil
	}
	j, err := w.JournalByMonth(ctx, mentee, prev)
	if err != nil || !submitted(j) {
		return nil, err
	}
	targets := ParseAnswers(j.Answers).Targets
	for _, t := range targets {
		if t != "" {
			return targets, nil
		}
	}
	return nil, nil
}

// Markers are the attention dots for the person's roles in the open month.
func (w *World) Markers(ctx context.Context, p dbgen.PerintisParticipant) (Markers, error) {
	m := Markers{}
	month := CurrentReportMonth(w.Now)
	if month == "" {
		return m, nil
	}
	if HasRole(p, "mentee") {
		journals, err := w.Q.PerintisJournalsOfMentees(ctx, []int32{p.UserID})
		if err != nil {
			return m, err
		}
		index := journalIndex(journals)
		j, ok := index[month]
		window := Window(month, w.Now)
		m.LapbulPending = (window != "upcoming" && window != "locked" && (!ok || j.Status == "draft")) ||
			(ok && j.Status == "revision_requested")
		for _, x := range journals {
			m.ResponseUnread = m.ResponseUnread || (x.RespondedAt.Valid && !x.ResponseReadAt.Valid)
		}
	}
	if HasRole(p, "mentor") {
		journals, err := w.JournalsOfMentorsInMonth(ctx, []int32{p.UserID}, month)
		if err != nil {
			return m, err
		}
		m.JournalsToRespond = CountWaiting(journals)
		for _, j := range journals {
			if s := deref(j.FlagStatus); s == "open" || s == "escalated" {
				m.OpenFlags++
			}
		}
		r, err := w.ReportByMonth(ctx, p.UserID, month)
		if err != nil {
			return m, err
		}
		m.ReportPending = len(w.MenteesOf(p.UserID)) > 0 && (r == nil || r.Status == "draft" || r.Status == "returned")
		unread, err := w.Q.PerintisUnreadReviews(ctx, p.UserID)
		if err != nil {
			return m, err
		}
		m.ReportReviewUnread = unread > 0
	}
	if HasRole(p, "coordinator") {
		reports, err := w.Q.PerintisReportsInMonth(ctx, DateOf(month))
		if err != nil {
			return m, err
		}
		mine := map[int32]bool{}
		for _, mentor := range w.MentorsOf(p.UserID) {
			mine[mentor.UserID] = true
		}
		for _, r := range reports {
			if mine[r.MentorUserID] && r.Status == "submitted" {
				m.ReportsToVerify++
			}
		}
	}
	return m, nil
}

// FullProfile is the combined profile; sensitiveHidden hides KIP-K and
// category from koordinator (economic data).
func (w *World) FullProfile(ctx context.Context, id int32, sensitiveHidden bool) (FullProfile, error) {
	p, err := w.Find(id)
	if err != nil {
		return FullProfile{}, err
	}
	if err := w.LoadDetails(ctx, []int32{id}); err != nil {
		return FullProfile{}, err
	}
	achievements, err := w.Achievements(ctx, id)
	if err != nil {
		return FullProfile{}, err
	}
	view := w.ParticipantView(p)
	if sensitiveHidden {
		view.KipK, view.Category = nil, nil
	}
	out := FullProfile{Profile: w.Profile(id), Participant: view, Tingkat: w.TingkatOf(id),
		Mentor: w.Person(p.MentorUserID), Coordinator: w.Person(w.CoordinatorOfMentor(p.MentorUserID)),
		Activities: w.Activities(id), Clubs: w.Clubs(id), Achievements: achievements, SensitiveHidden: sensitiveHidden}
	month := CurrentReportMonth(w.Now)
	if month == "" {
		return out, nil
	}
	out.Summary.Month = &month
	journals, err := w.Q.PerintisJournalsOfMentees(ctx, []int32{id})
	if err != nil {
		return out, err
	}
	if p.IsMentee {
		status := "not_started"
		for _, j := range journals {
			if monthOf(j.Month) == month {
				status = j.Status
			}
		}
		out.Summary.JournalStatus = &status
	}
	for i := len(journals) - 1; i >= 0; i-- {
		if journals[i].RespondedAt.Valid {
			out.Summary.LastResponse = journals[i].ResponseText
			break
		}
	}
	if p.MentorUserID != nil {
		r, err := w.ReportByMonth(ctx, *p.MentorUserID, month)
		if err != nil {
			return out, err
		}
		if r != nil && r.SessionDate.Valid {
			date := r.SessionDate.Time.Format("2006-01-02")
			out.Summary.SessionDate = &date
		}
	}
	return out, nil
}

// ReaderView is a submitted lapbul with the mentee's context and history,
// for the mentor (who may respond) and the pengelola (read only).
func (w *World) ReaderView(ctx context.Context, j dbgen.PerintisJournal, canRespond bool, reason *string) (JournalForMentor, error) {
	view, err := w.JournalView(ctx, j)
	if err != nil {
		return JournalForMentor{}, err
	}
	p := w.Participants[j.MenteeUserID]
	if err := w.loadPeople(ctx, []int32{j.MenteeUserID}); err != nil {
		return JournalForMentor{}, err
	}
	person := w.People[j.MenteeUserID]
	previous, err := w.PreviousTargets(ctx, j.MenteeUserID, monthOf(j.Month))
	if err != nil {
		return JournalForMentor{}, err
	}
	journals, err := w.Q.PerintisJournalsOfMentees(ctx, []int32{j.MenteeUserID})
	if err != nil {
		return JournalForMentor{}, err
	}
	history := []HistoryItem{}
	sort.Slice(journals, func(a, b int) bool { return monthOf(journals[a].Month) > monthOf(journals[b].Month) })
	for _, h := range journals {
		if monthOf(h.Month) >= monthOf(j.Month) || h.Status == "draft" {
			continue
		}
		history = append(history, HistoryItem{Month: monthOf(h.Month), Status: h.Status, Pulse: JournalPulse(h),
			Appreciation: h.ResponseAppreciation, ResponseText: h.ResponseText})
	}
	return JournalForMentor{Journal: view,
		Mentee: ReaderMentee{PersonSummary: w.MustPerson(j.MenteeUserID), Category: j.Category,
			Tingkat: w.TingkatOf(j.MenteeUserID), University: deref(person.University), Major: deref(person.Major), KipK: p.KipK},
		PreviousTargets: previous, History: history, CanRespond: canRespond, RespondLockedReason: reason}, nil
}

// RespondLock is why a mentor can no longer edit the response (after the 15th).
func RespondLock(month string, now time.Time) *string {
	if now.After(DeadlinesFor(month).ResponseEditableUntil) {
		return strPtr("Tanggapan hanya bisa disunting sampai tanggal 15.")
	}
	return nil
}

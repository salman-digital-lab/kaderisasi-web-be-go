package perintis

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestMonthsAndWindows(t *testing.T) {
	if got := CurrentReportMonth(at("2026-10-31T23:00:00+07:00")); got != "" {
		t.Fatalf("before the programme: %q", got)
	}
	if got := CurrentReportMonth(at("2026-11-01T00:00:00+07:00")); got != "2026-10-01" {
		t.Fatalf("first month: %q", got)
	}
	if got := ReportMonths(at("2027-01-04T10:00:00+07:00")); len(got) != 3 || got[0] != "2026-12-01" || got[2] != "2026-10-01" {
		t.Fatalf("months: %v", got)
	}
	for when, want := range map[string]string{
		"2026-12-31T23:59:00+07:00": "upcoming",
		"2027-01-01T00:00:00+07:00": "open",
		"2027-01-05T23:59:59+07:00": "open",
		"2027-01-06T00:00:00+07:00": "late",
		"2027-01-12T23:59:59+07:00": "late",
		"2027-01-13T00:00:00+07:00": "locked",
		"2027-01-05T17:30:00Z":      "late", // 00:30 WIB on the 6th
	} {
		if got := Window("2026-12-01", at(when)); got != want {
			t.Errorf("%s: got %s want %s", when, got, want)
		}
	}
	if !AsksForGrades("2027-01-01") || AsksForGrades("2026-12-01") {
		t.Fatal("grades are asked in January and July")
	}
	if MonthLabel("2026-12-01") != "Desember 2026" {
		t.Fatal(MonthLabel("2026-12-01"))
	}
}

func TestTingkat(t *testing.T) {
	for _, c := range []struct {
		intake int32
		at     string
		want   int
	}{{2025, "2027-01-04T10:00:00+07:00", 2}, {2026, "2026-07-31T10:00:00+07:00", 1}, {2026, "2026-08-01T10:00:00+07:00", 1}, {2020, "2027-01-04T10:00:00+07:00", 4}, {0, "2027-01-04T10:00:00+07:00", 1}} {
		if got := Tingkat(c.intake, at(c.at)); got != c.want {
			t.Errorf("Tingkat(%d, %s) = %d, want %d", c.intake, c.at, got, c.want)
		}
	}
}

func p(v int16) *int16 { return &v }

func TestFlagTrigger(t *testing.T) {
	low := Pulse{Energy: p(2), Academic: p(1), Finance: p(4), Relation: p(5)}
	one := Pulse{Energy: p(2), Academic: p(3), Finance: p(4), Relation: p(5)}
	if FlagTrigger(low, false) != "low_pulse" || FlagTrigger(low, true) != "both" || FlagTrigger(one, true) != "contact_request" || FlagTrigger(one, false) != "" {
		t.Fatal("wellbeing rule")
	}
}

func TestMissingItemsPerCategoryAndMonth(t *testing.T) {
	text := func(s string) *string { return &s }
	pages, ip, changed := 10.0, 3.5, true
	full := Answers{S1b: text("a"), S1c: text("b\nc"), S2b: text("d"), Targets: []string{"x", "y", "z"}, S3a: text("e"),
		S5aPages: &pages, S5aVerse: text("f"), S5b: text("g"), S5cKinds: []string{"Waktu"}, S6: text("h")}
	pulse := Pulse{Energy: p(3), Academic: p(3), Finance: p(3), Relation: p(3)}
	if m := MissingItems("dasar", "2026-12-01", full, pulse, nil, nil); len(m) != 0 {
		t.Fatalf("complete dasar lapbul: %v", m)
	}
	if m := MissingItems("dasar", "2026-12-01", full, pulse, nil, []string{"t1", "t2", "t3"}); len(m) != 1 {
		t.Fatalf("last month's targets must be rated: %v", m)
	}
	if m := MissingItems("biaya_hidup", "2027-01-01", full, pulse, nil, nil); len(m) != 4 {
		t.Fatalf("January biaya hidup asks IP, KHS and section 8: %v", m)
	}
	full.S7ip, full.S8aChanged, full.S8b = &ip, &changed, []string{"Makan"}
	if m := MissingItems("biaya_hidup", "2027-01-01", full, pulse, []File{{Field: "s7khs"}}, nil); len(m) != 1 || m[0] != "8a Penjelasan" {
		t.Fatalf("a changed economy needs an explanation: %v", m)
	}
}

func TestCleanKeepsLineBreaksAndRejectsUnknownChoices(t *testing.T) {
	text := "  baris satu\nbaris dua  "
	a, err := Answers{S1c: &text}.Clean()
	if err != nil || *a.S1c != "baris satu\nbaris dua" {
		t.Fatalf("%q %v", deref(a.S1c), err)
	}
	if _, err := (Answers{S5cKinds: []string{"Uang"}}).Clean(); err == nil {
		t.Fatal("unknown sharing kind accepted")
	}
	ip := 4.5
	if _, err := (Answers{S7ip: &ip}).Clean(); err == nil {
		t.Fatal("IP above 4 accepted")
	}
}

func TestCurriculumFallsBackForMissingEntries(t *testing.T) {
	if c := CurriculumFor(2, "2026-11-01"); c.Code != "T2-11" || c.Theme == "" || len(c.Flow) == 0 {
		t.Fatalf("%+v", c)
	}
	if c := CurriculumFor(3, "2027-03-01"); c.Code != "T3-03" || c.Theme != "Materi tingkat 3 · Maret 2027" {
		t.Fatalf("%+v", c)
	}
}

func TestCurrentEducationReadsTheLatestEntry(t *testing.T) {
	inst, major, intake, ok := CurrentEducation([]byte(`[{"institution":"SMA 3"},null,{"institution":"ITB","major":"Fisika","intake_year":"2024"}]`))
	if !ok || inst != "ITB" || major != "Fisika" || intake != 2024 {
		t.Fatalf("latest entry: %q %q %d %v", inst, major, intake, ok)
	}
	if _, _, intake, ok := CurrentEducation([]byte(`[{"institution":"UNPAD","intake_year":2023},{}]`)); !ok || intake != 2023 {
		t.Fatalf("empty last entry is skipped: %d %v", intake, ok)
	}
	for _, raw := range []string{``, `null`, `[]`, `"x"`, `[null]`} {
		if _, _, _, ok := CurrentEducation([]byte(raw)); ok {
			t.Fatalf("%s: no usable entry", raw)
		}
	}
}

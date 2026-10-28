package perintis

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// curriculum.json is the curriculum per tingkat × month (PRD §10.5), keyed
// T{tingkat}-{MM}, exported from the frontend's content/curriculum.ts.
//
//go:embed curriculum.json
var curriculumJSON []byte

type curriculumEntry struct {
	Theme  string   `json:"theme"`
	Goal   string   `json:"goal"`
	Flow   []string `json:"flow"`
	Output string   `json:"output"`
}

var curriculum = func() map[string]curriculumEntry {
	entries := map[string]curriculumEntry{}
	if err := json.Unmarshal(curriculumJSON, &entries); err != nil {
		panic(err)
	}
	return entries
}()

// CurriculumItem is the session material for one group in one month.
type CurriculumItem struct {
	Code     string   `json:"code"`
	Tingkat  int      `json:"tingkat"`
	Month    string   `json:"month"`
	Theme    string   `json:"theme"`
	Goal     string   `json:"goal"`
	Flow     []string `json:"flow"`
	Output   string   `json:"output"`
	SheetURL string   `json:"sheetUrl"`
}

// SheetBaseURL is where the printable Lembar Mentor files live.
var SheetBaseURL = "https://example.org/lembar-mentor/"

func CurriculumFor(tingkat int, month string) CurriculumItem {
	start, _ := MonthStart(month)
	code := fmt.Sprintf("T%d-%02d", tingkat, int(start.Month()))
	entry, ok := curriculum[code]
	if !ok {
		entry = curriculumEntry{
			Theme:  fmt.Sprintf("Materi tingkat %d · %s", tingkat, MonthLabel(month)),
			Goal:   "Materi lengkap ada di Lembar Mentor bulan ini.",
			Flow:   []string{"Pembuka & kabar (15')", "Materi inti (65')", "Target jika–maka (10')"},
			Output: "Sesuai Lembar Mentor",
		}
	}
	return CurriculumItem{Code: code, Tingkat: tingkat, Month: month, Theme: entry.Theme, Goal: entry.Goal,
		Flow: entry.Flow, Output: entry.Output, SheetURL: SheetBaseURL + code + ".pdf"}
}

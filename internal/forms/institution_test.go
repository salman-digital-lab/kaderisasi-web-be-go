package forms

import (
	"testing"

	"kaderisasi/web/internal/js"
)

func parse(t *testing.T, raw string) js.Value {
	t.Helper()
	v, err := js.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSubmissionEducationNeedsInstitution(t *testing.T) {
	schema := parse(t, `{"fields":[{"id":"a","section_name":"A","fields":[
		{"key":"history","label":"Riwayat","type":"education_history","required":false},
		{"key":"current","label":"Pendidikan","type":"current_education","required":false}]}]}`)
	for raw, want := range map[string]bool{
		`{}`: true,
		`{"history":[{"degree":"bachelor","institution":" ITB ","major":"Fisika"}],"current":{"institution":"ITB"}}`: true,
		`{"current":{"degree":"bachelor","institution":"","faculty":"","major":"","intake_year":2026}}`:              true,
		`{"history":[{"institution":"ITB"},{"degree":"bachelor","major":"Fisika"}]}`:                                 false,
		`{"history":[{"institution":"   "}]}`:             false,
		`{"current":{"institution":"","major":"Fisika"}}`: false,
	} {
		valid, _, errs, err := ValidateSubmission(schema, parse(t, raw))
		if err != nil || valid != want {
			t.Errorf("%s: valid=%v errs=%+v err=%v", raw, valid, errs, err)
		}
	}
	if errs := GuestEducationErrors(parse(t, `{"name":"A","education_history":[{"major":"Fisika"}],"current_education":{"faculty":"FMIPA"}}`)); len(errs) != 2 {
		t.Errorf("guest: %+v", errs)
	}
	if errs := GuestEducationErrors(parse(t, `{"name":"A","education_history":[{"institution":"ITB"}]}`)); len(errs) != 0 {
		t.Errorf("guest: %+v", errs)
	}
	if errs := GuestEducationErrors(js.Value{}); len(errs) != 0 {
		t.Errorf("guest: %+v", errs)
	}
}

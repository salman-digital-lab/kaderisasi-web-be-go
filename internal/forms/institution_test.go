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

func TestInstitutionsMustBeListed(t *testing.T) {
	schema := parse(t, `{"fields":[{"id":"a","section_name":"A","fields":[
		{"key":"history","label":"Riwayat","type":"education_history","required":false},
		{"key":"current","label":"Pendidikan","type":"current_education","required":false},
		{"key":"note","label":"Catatan","type":"text","required":false}]}]}`)
	data := parse(t, `{"history":[{"degree":"high_school","institution":"SMA 3"},{"degree":"bachelor","institution":" ITB "},{"institution":"Kampus Lama"}],
		"current":{"degree":"master","institution":"asal ketik"},"note":"UI"}`)
	guest := parse(t, `{"name":"A","current_education":{"institution":"UGM"}}`)
	names := SubmissionInstitutions(schema, data, guest)
	if len(names) != 4 || names[0] != "ITB" || names[1] != "Kampus Lama" || names[2] != "asal ketik" || names[3] != "UGM" {
		t.Fatalf("names: %q", names)
	}
	errs := UnlistedInstitutionErrors(schema, data, guest, map[string]bool{"ITB": true, "Kampus Lama": true, "UGM": true})
	if len(errs) != 1 || *errs[0].Field != "current" {
		t.Fatalf("errs: %+v", errs)
	}
	if errs := UnlistedInstitutionErrors(schema, data, guest, map[string]bool{"ITB": true, "Kampus Lama": true, "asal ketik": true}); len(errs) != 1 || *errs[0].Field != "current_education" {
		t.Fatalf("guest errs: %+v", errs)
	}
	if names := SubmissionInstitutions(schema, parse(t, `{"note":"x"}`), js.Value{}); len(names) != 0 {
		t.Fatalf("names: %q", names)
	}
}

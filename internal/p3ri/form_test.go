package p3ri

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

var knownTypes = []string{"short", "long", "date", "radio", "dropdown", "checkbox", "scale", "grid", "file"}

func TestFormDefinitionIsConsistent(t *testing.T) {
	f := Current()
	if f.Code != "1448h" || f.MaxChoices != 3 || len(f.Divisions) != 42 || len(f.ChoiceQuestions) != 2 {
		t.Fatalf("unexpected form header %s %d %d %d", f.Code, f.MaxChoices, len(f.Divisions), len(f.ChoiceQuestions))
	}
	if !f.OpensAt.Before(f.ClosesAt) {
		t.Fatal("opens_at must be before closes_at")
	}
	check := func(where string, questions []Question) {
		ids := map[string]bool{}
		for _, q := range questions {
			if q.ID == "" || ids[q.ID] {
				t.Errorf("%s: missing or duplicate id %q", where, q.ID)
			}
			ids[q.ID] = true
			if !slices.Contains(knownTypes, q.Type) || q.Label == "" {
				t.Errorf("%s/%s: bad type %q or empty label", where, q.ID, q.Type)
			}
			switch q.Type {
			case "radio", "dropdown", "checkbox":
				if len(q.Options) == 0 {
					t.Errorf("%s/%s: no options", where, q.ID)
				}
			case "grid":
				if len(q.Rows) == 0 || len(q.Columns) == 0 {
					t.Errorf("%s/%s: empty grid", where, q.ID)
				}
			case "scale":
				if q.Min != 1 || q.Max != 5 {
					t.Errorf("%s/%s: scale %d-%d", where, q.ID, q.Min, q.Max)
				}
			}
			if strings.Contains(q.Label, "1447") {
				t.Errorf("%s/%s: label still says 1447", where, q.ID)
			}
		}
	}
	var common []Question
	for _, s := range f.Sections {
		common = append(common, s.Questions...)
	}
	check("common", common)
	codes := map[string]bool{}
	for _, d := range f.Divisions {
		if codes[d.Code] {
			t.Errorf("duplicate division %s", d.Code)
		}
		codes[d.Code] = true
		check(d.Code, slices.Concat(f.ChoiceQuestions, d.Questions))
	}
}

// fill answers every question with a valid value.
func fill(q Question) any {
	switch q.Type {
	case "date":
		return "2005-03-14"
	case "radio", "dropdown":
		return q.Options[0]
	case "checkbox":
		return []string{q.Options[0]}
	case "scale":
		return 4
	case "grid":
		grid := map[string]string{}
		for _, row := range q.Rows {
			grid[row] = q.Columns[0]
		}
		return grid
	case "file":
		return File{Key: "p3ri/1448h/42/abc-" + q.ID + ".jpg", Name: q.ID + ".jpg", Mime: "image/jpeg", Size: 10}
	}
	return "Jawaban " + q.ID
}

func raw(t *testing.T, v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validInput(t *testing.T, choices ...string) Input {
	f := Current()
	in := Input{Choices: choices}
	in.Answers.Common = map[string]json.RawMessage{}
	in.Answers.Divisions = map[string]map[string]json.RawMessage{}
	for _, s := range f.Sections {
		for _, q := range s.Questions {
			in.Answers.Common[q.ID] = raw(t, fill(q))
		}
	}
	for _, d := range f.Divisions {
		answers := map[string]json.RawMessage{}
		for _, q := range slices.Concat(f.ChoiceQuestions, d.Questions) {
			answers[q.ID] = raw(t, fill(q))
		}
		in.Answers.Divisions[d.Code] = answers
	}
	return in
}

func owns(key string) bool { return OwnsKey(42, key) }

func TestValidateKeepsOnlyChosenDivisions(t *testing.T) {
	answers, files, err := Current().Validate(validInput(t, "website", "berbuka", "grafis"), owns)
	if err != nil {
		t.Fatal(err)
	}
	if len(answers.Divisions) != 3 || answers.Divisions["website"]["q2"] == nil || answers.Divisions["sahur"] != nil {
		t.Fatalf("divisions %v", answers.Divisions)
	}
	if len(files) != 4 || files[0].Field != "foto_diri" || answers.Common["foto_diri"] != files[0].Key {
		t.Fatalf("files %v", files)
	}
	if answers.Common["semangat"] != 4 {
		t.Fatalf("scale kept as %v", answers.Common["semangat"])
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	cases := map[string]func(*Input){
		"no choice":        func(in *Input) { in.Choices = nil },
		"four choices":     func(in *Input) { in.Choices = []string{"rnd", "sahur", "grafis", "website"} },
		"duplicate choice": func(in *Input) { in.Choices = []string{"sahur", "sahur"} },
		"unknown choice":   func(in *Input) { in.Choices = []string{"astronot"} },
		"missing required": func(in *Input) { delete(in.Answers.Common, "nama_lengkap") },
		"blank required":   func(in *Input) { in.Answers.Common["nama_lengkap"] = json.RawMessage(`"  "`) },
		"unknown option":   func(in *Input) { in.Answers.Common["golongan_darah"] = json.RawMessage(`"Z"`) },
		"scale too high":   func(in *Input) { in.Answers.Common["semangat"] = json.RawMessage(`6`) },
		"bad date":         func(in *Input) { in.Answers.Common["tanggal_lahir"] = json.RawMessage(`"14/03/2005"`) },
		"grid row missing": func(in *Input) { in.Answers.Common["ketersediaan"] = json.RawMessage(`{}`) },
		"foreign file": func(in *Input) {
			in.Answers.Common["twibbon"] = json.RawMessage(`{"key":"p3ri/1448h/7/x.jpg"}`)
		},
		"too long": func(in *Input) {
			in.Answers.Common["nama_lengkap"] = json.RawMessage(`"` + strings.Repeat("a", MaxShort+1) + `"`)
		},
		"division answer missing": func(in *Input) { delete(in.Answers.Divisions["sahur"], "komitmen") },
	}
	for name, mutate := range cases {
		in := validInput(t, "sahur")
		mutate(&in)
		_, _, err := Current().Validate(in, owns)
		var failure *Error
		if !errors.As(err, &failure) || failure.Status != 422 {
			t.Errorf("%s: got %v, want a 422", name, err)
		}
	}
}

func TestValidateAcceptsOtherAndOptionalAnswers(t *testing.T) {
	in := validInput(t, "website")
	in.Answers.Common["hubungan_darurat"] = json.RawMessage(`"Kakak"`)
	delete(in.Answers.Common, "line")
	in.Answers.Divisions["website"]["q2"] = json.RawMessage(`["Frontend","DevOps"]`)
	answers, _, err := Current().Validate(in, owns)
	if err != nil {
		t.Fatal(err)
	}
	if answers.Common["hubungan_darurat"] != "Kakak" || answers.Common["line"] != nil {
		t.Fatalf("common %v", answers.Common)
	}
}

func TestOpenWindowAndKeys(t *testing.T) {
	f := Current()
	if f.Open(f.OpensAt.Add(-time.Second)) || !f.Open(f.OpensAt) || f.Open(f.ClosesAt) {
		t.Fatal("open window must be [opens_at, closes_at)")
	}
	if !OwnsKey(42, "p3ri/1448h/42/abc-a.jpg") || OwnsKey(42, "p3ri/1448h/421/a.jpg") || OwnsKey(42, "p3ri/1448h/42/x/y.jpg") || OwnsKey(42, "p3ri/1448h/42/") {
		t.Fatal("key ownership")
	}
}

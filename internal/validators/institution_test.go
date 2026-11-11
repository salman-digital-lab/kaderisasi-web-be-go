package validators

import (
	"errors"
	"testing"

	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/vine"
)

func TestEducationHistoryNeedsInstitution(t *testing.T) {
	for raw, want := range map[string]string{
		`{"name":"A"}`:             "",
		`{"education_history":[]}`: "",
		`{"education_history":[{"degree":"bachelor","institution":" ITB "}]}`: "",
		`{"education_history":[{"institution":"ITB"},{"major":"Fisika"}]}`:    "education_history.1.institution",
		`{"education_history":[{"institution":"","major":"Fisika"}]}`:         "education_history.0.institution",
		`{"education_history":[{"institution":"   ","degree":"bachelor"}]}`:   "education_history.0.institution",
		`{"education_history":[{"institution":null,"faculty":"FMIPA"}]}`:      "education_history.0.institution",
	} {
		body, err := js.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		for name, schema := range map[string]vine.Obj{"update": UpdateProfile, "submit": SelfSubmit} {
			if name == "submit" {
				body.Keys, body.Fields["name"] = append(body.Keys, "name"), js.Value{Kind: 's', String: "A"}
			}
			payload, err := vine.ValidateObject(schema, body)
			if err != nil {
				t.Fatalf("%s %s: %v", name, raw, err)
			}
			got := ""
			var invalid *vine.Error
			if errors.As(EducationInstitutions(payload), &invalid) {
				got = invalid.Issues[0].Field
			}
			if got != want {
				t.Errorf("%s %s: got %q, want %q", name, raw, got, want)
			}
		}
	}
}

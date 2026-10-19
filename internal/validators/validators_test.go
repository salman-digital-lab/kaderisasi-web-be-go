package validators

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"testing"

	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/vine"
)

// Values that exercise coercion, nesting, missing/null handling and the
// custom history-year rules.
var atoms = []any{nil, "", " ", "x", "  trimmed  ", "M", "F", "m", "bachelor", "phd", "professional", "true", "false", "12", "1.5", "0x10", "1e3",
	" 2020 ", "2020", 2020, 1899, 1900, 2036, 2037, 2045, 2020.5, 0, -1, 7, 1.5, true, false, []any{}, []any{"a", 1}, []any{"x", nil}, map[string]any{},
	map[string]any{"a": 1}, "ab12cd", "a3f5b9c1d2e4f60718293a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d", "user@example.com", "bad@", []any{2021}}

func randomValue(r *rand.Rand, depth int) any {
	if depth > 0 && r.Intn(4) == 0 {
		if r.Intn(2) == 0 {
			n := r.Intn(4)
			list := make([]any, n)
			for i := range list {
				list[i] = randomObject(r, depth-1)
			}
			return list
		}
		return randomObject(r, depth-1)
	}
	return atoms[r.Intn(len(atoms))]
}

var keys = []string{"name", "gender", "whatsapp", "province_id", "city_id", "birth_date", "education_history", "work_history", "extra_data",
	"degree", "institution", "intake_year", "job_title", "company", "start_year", "end_year", "preferred_name", "salman_activity_history",
	"current_activity_focus", "kaderisasi_path", "ssc", "lmd", "spectra", "description", "achievement_date", "type", "problem_ownership",
	"owner_name", "problem_category", "problem_description", "handling_technic", "counselor_gender", "cursor", "unread", "cutoff",
	"session_token", "questionnaire_answer", "guest_data", "email", "additional_data", "completed", "1", "extra", "proof", "file"}

func randomObject(r *rand.Rand, depth int) map[string]any {
	o := map[string]any{}
	for i := 0; i < r.Intn(7); i++ {
		o[keys[r.Intn(len(keys))]] = randomValue(r, depth)
	}
	return o
}

func TestValidatorsMatchWebBe(t *testing.T) {
	webBe := "../../../kaderisasi-web-be"
	if _, err := os.Stat(webBe + "/app/validators"); err != nil {
		t.Skip("kaderisasi-web-be checkout not present")
	}
	names := make([]string, 0, len(All))
	for name := range All {
		names = append(names, name)
	}
	sort.Strings(names)
	type testCase struct {
		Validator string `json:"validator"`
		Data      any    `json:"data"`
	}
	r := rand.New(rand.NewSource(5))
	var cases []testCase
	for _, name := range names {
		cases = append(cases, testCase{name, map[string]any{}})
		for i := 0; i < 400; i++ {
			cases = append(cases, testCase{name, randomObject(r, 3)})
		}
	}
	input, _ := json.Marshal(cases)
	script, _ := os.Getwd()
	command := exec.Command("node", "--import", "ts-node-maintained/register/esm", script+"/../../scripts/webbe-validators-fixture.mjs")
	command.Dir = webBe
	command.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	raw, err := command.Output()
	if err != nil {
		t.Fatalf("fixture: %v %s", err, stderr.String())
	}
	var want []json.RawMessage
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	mismatches := 0
	for i, c := range cases {
		data, _ := json.Marshal(c.Data)
		parsed, _ := js.Parse(data)
		value, err := vine.ValidateObject(All[c.Validator], parsed)
		var got []byte
		if invalid, ok := err.(*vine.Error); ok {
			got, _ = json.Marshal(map[string]any{"issues": invalid.Issues})
		} else {
			got, _ = json.Marshal(map[string]any{"value": value})
		}
		if !bytes.Equal(got, want[i]) {
			mismatches++
			if mismatches <= 15 {
				t.Errorf("%s %s\n got  %s\n want %s", c.Validator, data, got, want[i])
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d mismatches", mismatches, len(cases))
	}
}

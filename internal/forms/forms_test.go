package forms

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"testing"

	"kaderisasi/web/internal/js"
)

type gen struct{ r *rand.Rand }

func (g gen) pick(items ...any) any { return items[g.r.Intn(len(items))] }

func (g gen) target(ids []string) any {
	switch g.r.Intn(6) {
	case 0:
		return map[string]any{"type": "next"}
	case 1:
		return map[string]any{"type": "submit"}
	case 2:
		return nil
	case 3:
		return "bogus"
	}
	return map[string]any{"type": "section", "sectionId": g.pick(append([]any{"missing", nil}, toAny(ids)...)...)}
}

func toAny(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func (g gen) schema() map[string]any {
	n := 1 + g.r.Intn(4)
	ids := []string{}
	sections := []any{}
	keys := []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7"}
	if g.r.Intn(3) == 0 {
		sections = append(sections, map[string]any{"section_name": "profile_data", "fields": []any{}})
	}
	for i := 0; i < n; i++ {
		id := []string{"a", "b", "c", "d", "e"}[i]
		section := map[string]any{"section_name": "Bagian " + id, "fields": []any{}}
		if g.r.Intn(5) != 0 {
			section["id"] = id
			ids = append(ids, id)
		}
		fields := []any{}
		for j := 0; j < 1+g.r.Intn(3); j++ {
			kind := g.pick("text", "number", "radio", "select", "checkbox", "multiselect", "file", "education_history", "current_education", "textarea").(string)
			field := map[string]any{"key": keys[g.r.Intn(len(keys))], "label": "Label " + kind, "required": g.r.Intn(2) == 0, "type": kind}
			if g.r.Intn(2) == 0 {
				field["options"] = []any{map[string]any{"label": "Ya"}, map[string]any{"label": "Tidak", "value": g.pick(nil, "no", 0, "", 1.5, false)},
					map[string]any{"label": "Mati", "disabled": true}}
			}
			if g.r.Intn(3) == 0 {
				field["validation"] = map[string]any{"min": g.pick(nil, 1, "3", 5), "max": g.pick(10, nil), "minLength": g.pick(2, nil, "4"), "maxLength": g.pick(5, nil),
					"pattern": g.pick(nil, `^\d+$`, `(?=a)a+`, `[`, "^(a|b)$", `\p{L}+`), "customMessage": g.pick(nil, "", "Pesan khusus")}
			}
			if g.r.Intn(8) == 0 {
				field["hidden"] = true
			}
			if g.r.Intn(8) == 0 {
				field["disabled"] = 1
			}
			fields = append(fields, field)
		}
		section["fields"] = fields
		if g.r.Intn(2) == 0 {
			nav := map[string]any{"defaultTarget": g.target(ids)}
			if g.r.Intn(2) == 0 {
				nav["questionKey"] = g.pick(keys[g.r.Intn(len(keys))], 5, "")
				nav["routes"] = g.pick([]any{map[string]any{"optionValue": "Ya", "target": g.target([]string{"b", "c", "d", "e"})},
					map[string]any{"optionValue": g.pick("no", "Tidak", "0", 3), "target": g.target([]string{"c", "e"})}}, "nope", nil, []any{nil})
			}
			section["navigation"] = g.pick(nav, nav, nav, nil, "x", []any{})
		}
		sections = append(sections, section)
	}
	schema := map[string]any{"fields": sections}
	if g.r.Intn(3) == 0 {
		schema["version"] = g.pick(2, 1, "2")
	}
	return schema
}

func (g gen) value() any {
	return g.pick(nil, "", " ", "Ya", "Tidak", "no", "0", 0, 1.5, 7, 12, true, false, []any{}, []any{"Ya"}, []any{"Ya", "x"}, []any{1},
		[]any{"3f2a1b4c-1111-2222-3333-444455556666"}, []any{"bad"}, map[string]any{"institution": " ITB ", "degree": "bachelor", "intake_year": 2020},
		map[string]any{"institution": ""}, map[string]any{"major": 5}, []any{map[string]any{"degree": "phd"}}, "aaaa", "12345", "ab")
}

func (g gen) submission(schema map[string]any) any {
	if g.r.Intn(20) == 0 {
		return g.pick("text", []any{1}, nil)
	}
	out := map[string]any{}
	for _, section := range schema["fields"].([]any) {
		for _, f := range section.(map[string]any)["fields"].([]any) {
			if g.r.Intn(4) != 0 {
				out[f.(map[string]any)["key"].(string)] = g.value()
			}
		}
	}
	if g.r.Intn(10) == 0 {
		out["unknown"] = "x"
	}
	return out
}

func TestMatchesWebBe(t *testing.T) {
	webBe := "../../../kaderisasi-web-be"
	if _, err := os.Stat(webBe + "/app/services/form_routing.ts"); err != nil {
		t.Skip("kaderisasi-web-be checkout not present")
	}
	g := gen{rand.New(rand.NewSource(3))}
	type testCase struct {
		Schema     any `json:"schema"`
		Submission any `json:"submission"`
	}
	cases := []testCase{}
	if raw, err := os.ReadFile(webBe + "/tests/unit/form-routing.fixtures.json"); err == nil {
		var fixture map[string]json.RawMessage
		if json.Unmarshal(raw, &fixture) == nil {
			var schema any
			_ = json.Unmarshal(fixture["schema"], &schema)
			cases = append(cases, testCase{schema, map[string]any{}})
		}
	}
	for i := 0; i < 3000; i++ {
		schema := g.schema()
		cases = append(cases, testCase{schema, g.submission(schema)})
	}
	input, _ := json.Marshal(cases)
	dir, _ := os.Getwd()
	command := exec.Command("node", "--import", "ts-node-maintained/register/esm", dir+"/../../scripts/forms-fixture.mjs")
	command.Dir = webBe
	command.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		t.Fatalf("fixture: %v %s", err, stderr.String())
	}
	var want []map[string]json.RawMessage
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatal(err)
	}
	mismatches := 0
	report := func(i int, what string, got, expected []byte) {
		mismatches++
		if mismatches <= 12 {
			c, _ := json.Marshal(cases[i])
			t.Errorf("%s case %s\n got  %s\n want %s", what, c, got, expected)
		}
	}
	for i, c := range cases {
		schemaRaw, _ := json.Marshal(c.Schema)
		submissionRaw, _ := json.Marshal(c.Submission)
		schema, _ := js.Parse(schemaRaw)
		submission, _ := js.Parse(submissionRaw)

		valid, data, errs, err := Validate(schema, submission)
		var got []byte
		switch {
		case err != nil:
			got, _ = json.Marshal(map[string]any{"error": err.Error()})
		case valid:
			d, _ := json.Marshal(data)
			got = []byte(`{"value":{"valid":true,"data":` + string(d) + `}}`)
		default:
			e, _ := json.Marshal(errs)
			got = []byte(`{"value":{"valid":false,"errors":` + string(e) + `}}`)
		}
		if !bytes.Equal(got, want[i]["validation"]) {
			report(i, "validate", got, want[i]["validation"])
			continue
		}
		if !isSchema(schema) || submission.Kind != 'o' || len(ValidateRouting(schema)) > 0 {
			continue // the TypeScript route helpers assume a valid schema
		}
		route, err := Route(schema, submission)
		if err != nil {
			got, _ = json.Marshal(map[string]any{"error": err.Error()})
		} else {
			got, _ = json.Marshal(map[string]any{"value": route})
		}
		if !bytes.Equal(got, want[i]["route"]) {
			report(i, "route", got, want[i]["route"])
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d mismatches", mismatches, len(cases))
	}
}

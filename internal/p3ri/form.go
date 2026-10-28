// Package p3ri is the P3RI 1448 H committee registration (Open Recruitment).
// The form is hardcoded in form_1448h.json, extracted once from the Google
// Form; kaderisasi-admin-be-go keeps a byte-identical copy for its labels.
// Change the two copies together (docs/P3RI.md).
package p3ri

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed form_1448h.json
var formJSON []byte

// Form is the registration form. Sections hold the common questions; each
// chosen division adds ChoiceQuestions plus its own Questions.
type Form struct {
	Code            string     `json:"code"`
	Title           string     `json:"title"`
	OpensAt         time.Time  `json:"opens_at"`
	ClosesAt        time.Time  `json:"closes_at"`
	Intro           []string   `json:"intro"`
	Contacts        []Contact  `json:"contacts"`
	GuidebookURL    *string    `json:"guidebook_url"`
	FAQURL          *string    `json:"faq_url"`
	MaxChoices      int        `json:"max_choices"`
	Sections        []Section  `json:"sections"`
	ChoiceQuestions []Question `json:"choice_questions"`
	Divisions       []Division `json:"divisions"`
}

type Contact struct {
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	WhatsApp *string `json:"whatsapp"`
}

type Section struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	Questions   []Question `json:"questions"`
}

type Division struct {
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Questions []Question `json:"questions"`
}

// Question types: short, long, date, radio, dropdown, checkbox, scale, grid, file.
type Question struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Other    bool     `json:"other,omitempty"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
	Rows     []string `json:"rows,omitempty"`
	Columns  []string `json:"columns,omitempty"`
	Accept   []string `json:"accept,omitempty"`
}

const (
	MaxShort = 300
	MaxLong  = 3000
	MaxFile  = 5 << 20
)

var form = mustForm()

func mustForm() Form {
	var f Form
	if err := json.Unmarshal(formJSON, &f); err != nil {
		panic(fmt.Sprintf("p3ri: invalid form_1448h.json: %v", err))
	}
	return f
}

// Current returns the hardcoded 1448 H form.
func Current() Form { return form }

// Open reports whether registrations are accepted at now.
func (f Form) Open(now time.Time) bool {
	return !now.Before(f.OpensAt) && now.Before(f.ClosesAt)
}

func (f Form) Division(code string) (Division, bool) {
	i := slices.IndexFunc(f.Divisions, func(d Division) bool { return d.Code == code })
	if i < 0 {
		return Division{}, false
	}
	return f.Divisions[i], true
}

// Answers is the stored answers object: common answers keyed by question id,
// and per chosen division its choice and division answers keyed by question id.
type Answers struct {
	Common    map[string]any            `json:"common"`
	Divisions map[string]map[string]any `json:"divisions"`
}

// File is an uploaded file referenced by a file question.
type File struct {
	Field string `json:"field"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Mime  string `json:"mime"`
	Size  int64  `json:"size"`
}

// Input is what the applicant sends. File questions carry a File object.
type Input struct {
	Choices []string `json:"choices"`
	Answers struct {
		Common    map[string]json.RawMessage            `json:"common"`
		Divisions map[string]map[string]json.RawMessage `json:"divisions"`
	} `json:"answers"`
}

// Validate checks the input against the form and returns the cleaned answers
// (only known questions, only chosen divisions) and the referenced files.
// ownsKey reports whether a file key was uploaded by this applicant.
func (f Form) Validate(in Input, ownsKey func(string) bool) (Answers, []File, error) {
	out := Answers{Common: map[string]any{}, Divisions: map[string]map[string]any{}}
	files := []File{}

	if len(in.Choices) == 0 {
		return out, nil, fail(422, "Pilih minimal satu bidang/divisi.")
	}
	if len(in.Choices) > f.MaxChoices {
		return out, nil, fail(422, "Paling banyak %d pilihan.", f.MaxChoices)
	}
	for i, code := range in.Choices {
		if _, ok := f.Division(code); !ok {
			return out, nil, fail(422, "Pilihan %d tidak dikenal.", i+1)
		}
		if slices.Contains(in.Choices[:i], code) {
			return out, nil, fail(422, "Setiap pilihan harus berbeda.")
		}
	}

	for _, section := range f.Sections {
		for _, q := range section.Questions {
			v, file, err := q.value(in.Answers.Common[q.ID], ownsKey)
			if err != nil {
				return out, nil, fail(422, "%s: %s", q.Label, err.Error())
			}
			if v != nil {
				out.Common[q.ID] = v
			}
			if file != nil {
				file.Field = q.ID
				files = append(files, *file)
			}
		}
	}

	for n, code := range in.Choices {
		division, _ := f.Division(code)
		raw := in.Answers.Divisions[code]
		answers := map[string]any{}
		for _, q := range slices.Concat(f.ChoiceQuestions, division.Questions) {
			v, _, err := q.value(raw[q.ID], ownsKey)
			if err != nil {
				return out, nil, fail(422, "Pilihan %d (%s), %s: %s", n+1, division.Name, q.Label, err.Error())
			}
			if v != nil {
				answers[q.ID] = v
			}
		}
		out.Divisions[code] = answers
	}
	return out, files, nil
}

type valueError string

func (e valueError) Error() string { return string(e) }

const (
	errRequired valueError = "wajib diisi."
	errInvalid  valueError = "jawaban tidak valid."
)

// value parses one answer. Missing optional answers return nil.
func (q Question) value(raw json.RawMessage, ownsKey func(string) bool) (any, *File, error) {
	empty := len(raw) == 0 || string(raw) == "null"
	switch q.Type {
	case "short", "long", "date", "radio", "dropdown":
		var s string
		if !empty && json.Unmarshal(raw, &s) != nil {
			return nil, nil, errInvalid
		}
		s = strings.TrimSpace(s)
		if s == "" {
			if q.Required {
				return nil, nil, errRequired
			}
			return nil, nil, nil
		}
		limit := MaxShort
		if q.Type == "long" {
			limit = MaxLong
		}
		if utf8.RuneCountInString(s) > limit {
			return nil, nil, valueError(fmt.Sprintf("maksimal %d karakter.", limit))
		}
		if q.Type == "date" {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return nil, nil, errInvalid
			}
		}
		if (q.Type == "radio" || q.Type == "dropdown") && !q.Other && !slices.Contains(q.Options, s) {
			return nil, nil, errInvalid
		}
		return s, nil, nil
	case "checkbox":
		var list []string
		if !empty && json.Unmarshal(raw, &list) != nil {
			return nil, nil, errInvalid
		}
		kept := []string{}
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" || slices.Contains(kept, s) {
				continue
			}
			if utf8.RuneCountInString(s) > MaxShort || (!q.Other && !slices.Contains(q.Options, s)) {
				return nil, nil, errInvalid
			}
			kept = append(kept, s)
		}
		if len(kept) == 0 {
			if q.Required {
				return nil, nil, errRequired
			}
			return nil, nil, nil
		}
		return kept, nil, nil
	case "scale":
		var n *int
		if !empty && json.Unmarshal(raw, &n) != nil {
			return nil, nil, errInvalid
		}
		if n == nil {
			if q.Required {
				return nil, nil, errRequired
			}
			return nil, nil, nil
		}
		if *n < q.Min || *n > q.Max {
			return nil, nil, errInvalid
		}
		return *n, nil, nil
	case "grid":
		var grid map[string]string
		if !empty && json.Unmarshal(raw, &grid) != nil {
			return nil, nil, errInvalid
		}
		kept := map[string]string{}
		for _, row := range q.Rows {
			col := grid[row]
			if col == "" {
				if q.Required {
					return nil, nil, valueError(fmt.Sprintf("baris \"%s\" wajib diisi.", row))
				}
				continue
			}
			if !slices.Contains(q.Columns, col) {
				return nil, nil, errInvalid
			}
			kept[row] = col
		}
		if len(kept) == 0 {
			return nil, nil, nil
		}
		return kept, nil, nil
	case "file":
		var file *File
		if !empty && json.Unmarshal(raw, &file) != nil {
			return nil, nil, errInvalid
		}
		if file == nil || file.Key == "" {
			if q.Required {
				return nil, nil, errRequired
			}
			return nil, nil, nil
		}
		if !ownsKey(file.Key) {
			return nil, nil, valueError("berkas tidak ditemukan, unggah ulang.")
		}
		clean := File{Key: file.Key, Name: safeLabel(file.Name), Mime: file.Mime, Size: file.Size}
		return clean.Key, &clean, nil
	}
	return nil, nil, errInvalid
}

func safeLabel(name string) string {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 120 {
		name = string([]rune(name)[:120])
	}
	return name
}

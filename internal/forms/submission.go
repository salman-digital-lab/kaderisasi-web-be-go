package forms

import (
	"math"
	"regexp"
	"strconv"
	"time"
	"unicode/utf16"

	"github.com/dlclark/regexp2"

	"kaderisasi/web/internal/js"
)

// SubmissionError is a CustomFormSubmissionError.
type SubmissionError struct {
	Field   *string `json:"field,omitempty"`
	Message string  `json:"message"`
}

// Now is the clock for intake-year checks (new Date().getFullYear()).
var Now = time.Now

func units(s string) int { return len(utf16.Encode([]rune(s))) }

func isRecord(v js.Value) bool { return v.Kind == 'o' }

// isSchema is isCustomFormSchema.
func isSchema(v js.Value) bool {
	sections, ok := get(v, "fields")
	if !isRecord(v) || !ok || sections.Kind != 'a' {
		return false
	}
	for _, section := range sections.Items {
		if !isRecord(section) {
			return false
		}
		if id, ok := get(section, "id"); ok && id.Kind != 's' {
			return false
		}
		if name, _ := get(section, "section_name"); name.Kind != 's' {
			return false
		}
		fields, ok := get(section, "fields")
		if !ok || fields.Kind != 'a' {
			return false
		}
		for _, field := range fields.Items {
			if !isRecord(field) {
				return false
			}
			k, _ := get(field, "key")
			l, _ := get(field, "label")
			r, _ := get(field, "required")
			t, _ := get(field, "type")
			if k.Kind != 's' || l.Kind != 's' || r.Kind != 'b' || t.Kind != 's' {
				return false
			}
			if options, ok := get(field, "options"); ok {
				if options.Kind != 'a' {
					return false
				}
				for _, o := range options.Items {
					label, _ := get(o, "label")
					if !isRecord(o) || label.Kind != 's' {
						return false
					}
				}
			}
			if validation, ok := get(field, "validation"); ok && !isRecord(validation) {
				return false
			}
		}
	}
	return true
}

func isEmpty(v js.Value) bool {
	return v.Kind == 0 || v.Kind == 'n' || (v.Kind == 's' && js.Trim(v.String) == "") || (v.Kind == 'a' && len(v.Items) == 0)
}

var fileID = regexp.MustCompile(`^[a-f0-9-]{36}$`)

func validEducation(v js.Value) bool {
	if !isRecord(v) {
		return false
	}
	for _, key := range v.Keys {
		item := v.Fields[key]
		if item.Kind == 'n' || (item.Kind == 's' && item.String == "") {
			continue
		}
		switch key {
		case "degree":
			s := String(item)
			if s != "high_school" && s != "diploma" && s != "bachelor" && s != "master" && s != "doctoral" {
				return false
			}
		case "intake_year":
			if item.Kind != 'f' || item.Number != math.Trunc(item.Number) || math.IsInf(item.Number, 0) || item.Number < 1900 || item.Number > float64(Now().Year()+10) {
				return false
			}
		case "institution", "faculty", "major":
			if item.Kind != 's' || units(item.String) > 10000 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func allStrings(v js.Value) bool {
	for _, item := range v.Items {
		if item.Kind != 's' {
			return false
		}
	}
	return true
}

func validType(field, v js.Value) bool {
	t, _ := get(field, "type")
	switch t.String {
	case "education_history":
		if v.Kind != 'a' || len(v.Items) > 50 {
			return false
		}
		for _, item := range v.Items {
			if !validEducation(item) {
				return false
			}
		}
		return true
	case "current_education":
		return validEducation(v)
	case "file":
		if v.Kind != 'a' || len(v.Items) > 5 {
			return false
		}
		for _, item := range v.Items {
			if item.Kind != 's' || !fileID.MatchString(item.String) {
				return false
			}
		}
		return true
	case "number":
		return v.Kind == 'f' && !math.IsInf(v.Number, 0) && !math.IsNaN(v.Number)
	case "multiselect":
		return v.Kind == 'a' && allStrings(v)
	case "checkbox":
		if options, _ := get(field, "options"); options.Kind == 'a' && len(options.Items) > 0 {
			return v.Kind == 'a' && allStrings(v)
		}
		return v.Kind == 'b'
	}
	return v.Kind == 's'
}

func allowedOptions(field, v js.Value) bool {
	options, _ := get(field, "options")
	if options.Kind != 'a' || len(options.Items) == 0 {
		return true
	}
	allowed := map[string]bool{}
	for _, o := range options.Items {
		if d, _ := get(o, "disabled"); truthy(d) {
			continue
		}
		allowed[OptionValue(o)] = true
	}
	submitted := []js.Value{v}
	if v.Kind == 'a' {
		submitted = v.Items
	}
	for _, s := range submitted {
		if s.Kind != 's' || !allowed[s.String] {
			return false
		}
	}
	return true
}

func number(v js.Value) (float64, bool) {
	if v.Kind == 'f' {
		return v.Number, true
	}
	return 0, false
}

// jsTemplate is `${value}` for a validation rule value.
func jsTemplate(v js.Value) string { return String(v) }

func fieldRules(field, v js.Value) string {
	label, _ := get(field, "label")
	required, _ := get(field, "required")
	kind, _ := get(field, "type")
	name := label.String
	if required.Bool && kind.String == "current_education" {
		institution, ok := get(v, "institution")
		if !isRecord(v) || !ok || institution.Kind != 's' || js.Trim(institution.String) == "" {
			return name + " wajib diisi."
		}
	}
	if required.Bool && (isEmpty(v) || (kind.String == "checkbox" && v.Kind == 'b' && !v.Bool)) {
		return name + " wajib diisi."
	}
	if isEmpty(v) {
		return ""
	}
	if !validType(field, v) {
		return "Format " + name + " tidak valid."
	}
	if !allowedOptions(field, v) {
		return "Pilihan " + name + " tidak valid."
	}
	if v.Kind == 's' && units(v.String) > 10000 {
		return name + " terlalu panjang."
	}
	rules, ok := get(field, "validation")
	if !ok {
		return ""
	}
	custom, _ := get(rules, "customMessage")
	or := func(fallback string) string {
		if truthy(custom) {
			return String(custom)
		}
		return fallback
	}
	compare := func(key string) (js.Value, bool) {
		r, ok := get(rules, key)
		return r, ok
	}
	if n, isNumber := number(v); isNumber {
		if min, ok := compare("min"); ok && lessThan(n, min) {
			return or("Nilai minimum " + name + " adalah " + jsTemplate(min) + ".")
		}
		if max, ok := compare("max"); ok && greaterThan(n, max) {
			return or("Nilai maksimum " + name + " adalah " + jsTemplate(max) + ".")
		}
	}
	if v.Kind == 's' {
		length := float64(units(v.String))
		if min, ok := compare("minLength"); ok && lessThan(length, min) {
			return or(name + " minimal " + jsTemplate(min) + " karakter.")
		}
		if max, ok := compare("maxLength"); ok && greaterThan(length, max) {
			return or(name + " maksimal " + jsTemplate(max) + " karakter.")
		}
		if pattern, ok := compare("pattern"); ok && truthy(pattern) {
			re, err := regexp2.Compile(String(pattern), regexp2.ECMAScript)
			if err != nil {
				return "Konfigurasi validasi " + name + " tidak valid."
			}
			re.MatchTimeout = time.Second
			matched, err := re.MatchString(v.String)
			if err == nil && !matched {
				return or("Format " + name + " tidak valid.")
			}
		}
	}
	return ""
}

// lessThan and greaterThan are JavaScript's relational comparison of a
// number with a JSON value (ToPrimitive / ToNumber; NaN compares false).
func lessThan(n float64, v js.Value) bool {
	if v.Kind == 0 {
		return false
	}
	return n < toNumber(v)
}

func greaterThan(n float64, v js.Value) bool {
	if v.Kind == 0 {
		return false
	}
	return n > toNumber(v)
}

func toNumber(v js.Value) float64 {
	switch v.Kind {
	case 'n':
		return 0
	case 'b':
		if v.Bool {
			return 1
		}
		return 0
	case 'f':
		return v.Number
	case 's':
		return js.Number(v.String)
	case 'a':
		return js.Number(String(v))
	}
	return math.NaN()
}

// Validate is validateCustomFormSubmission. A routing error (an Error the
// TypeScript throws) is returned as err.
func Validate(schema, submission js.Value) (valid bool, data js.Value, errs []SubmissionError, err error) {
	if !isSchema(schema) {
		return false, js.Value{}, []SubmissionError{{Message: "Konfigurasi formulir tidak valid."}}, nil
	}
	if len(ValidateRouting(schema)) > 0 {
		return false, js.Value{}, []SubmissionError{{Message: "Konfigurasi alur formulir tidak valid."}}, nil
	}
	if !isRecord(submission) {
		return false, js.Value{}, []SubmissionError{{Message: "Data formulir tidak valid."}}, nil
	}
	raw, _ := submission.MarshalJSON()
	if units(string(raw)) > 100_000 {
		return false, js.Value{}, []SubmissionError{{Message: "Data formulir terlalu besar."}}, nil
	}
	route, err := Route(schema, submission)
	if err != nil {
		return false, js.Value{}, nil, err
	}
	sections, _ := get(schema, "fields")
	keys := map[string]bool{}
	for _, section := range sections.Items {
		for _, field := range fieldsOf(section) {
			k, _ := get(field, "key")
			if keys[k.String] {
				key := k.String
				return false, js.Value{}, []SubmissionError{{Field: &key, Message: "Kunci formulir " + key + " digunakan lebih dari sekali."}}, nil
			}
			keys[k.String] = true
		}
	}
	for _, k := range submission.Keys {
		if !keys[k] {
			key := k
			return false, js.Value{}, []SubmissionError{{Field: &key, Message: "Formulir telah berubah. Muat ulang lalu isi kembali."}}, nil
		}
	}
	var fields []js.Value
	for _, s := range CustomSections(schema) {
		reached := false
		for _, r := range route {
			reached = reached || strictEqual(r, s.ID)
		}
		if !reached {
			continue
		}
		for _, f := range fieldsOf(s.Value) {
			hidden, _ := get(f, "hidden")
			disabled, _ := get(f, "disabled")
			if !truthy(hidden) && !truthy(disabled) {
				fields = append(fields, f)
			}
		}
	}
	for _, f := range fields {
		k, _ := get(f, "key")
		value, _ := submission.Get(k.String)
		if message := fieldRules(f, value); message != "" {
			key := k.String
			errs = append(errs, SubmissionError{Field: &key, Message: message})
		}
	}
	if len(errs) > 0 {
		return false, js.Value{}, errs, nil
	}
	data = js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	for _, f := range fields {
		k, _ := get(f, "key")
		if v, ok := submission.Get(k.String); ok {
			if _, seen := data.Fields[k.String]; !seen {
				data.Keys = append(data.Keys, k.String)
			}
			data.Fields[k.String] = v
		}
	}
	data.Keys = js.ObjectKeyOrder(data.Keys)
	return true, data, nil, nil
}

var _ = strconv.Itoa

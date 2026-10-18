// Package vine reproduces the subset of VineJS 3 validation web-be uses:
// error messages, rule names, meta, field order and transforms. Interop tests
// compare it with the real library (tests/interop).
package vine

import (
	"encoding/json"
	"strings"
	"unicode/utf16"
)

// Issue is one entry of E_VALIDATION_ERROR.messages (SimpleErrorReporter).
type Issue struct {
	Message string         `json:"message"`
	Rule    string         `json:"rule"`
	Field   string         `json:"field"`
	Meta    map[string]int `json:"meta,omitempty"`
}

// Error is Vine's E_VALIDATION_ERROR; its message is always "Validation failure".
type Error struct{ Issues []Issue }

func (*Error) Error() string { return "Validation failure" }

// Input is request.all(): JSON values by field name.
type Input map[string]json.RawMessage

// String describes `vine.string()` with its chained rules, in chain order:
// Trim, then Lower, then Email/MinLength/MaxLength.
type String struct {
	Trim, Lower, Email bool
	MinLength          int // 0 means unset
	MaxLength          int // 0 means unset
}

// Field is one property of a `vine.object` schema.
type Field struct {
	Name string
	Rule String
}

// Validate runs the fields in schema order. Like Vine it stops at the first
// failing rule of each field and reports every failing field.
func Validate(input Input, fields ...Field) (map[string]string, error) {
	out := map[string]string{}
	var issues []Issue
	for _, f := range fields {
		value, issue := f.Rule.validate(f.Name, input[f.Name])
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		out[f.Name] = value
	}
	if len(issues) > 0 {
		return nil, &Error{Issues: issues}
	}
	return out, nil
}

func (r String) validate(field string, raw json.RawMessage) (string, *Issue) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", &Issue{Message: "The " + field + " field must be defined", Rule: "required", Field: field}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", &Issue{Message: "The " + field + " field must be a string", Rule: "string", Field: field}
	}
	if r.Trim {
		value = TrimJS(value)
	}
	if r.Lower {
		value = strings.ToLower(value)
	}
	if r.Email && !IsEmail(value) {
		return "", &Issue{Message: "The " + field + " field must be a valid email address", Rule: "email", Field: field}
	}
	length := len(utf16.Encode([]rune(value)))
	if r.MinLength > 0 && length < r.MinLength {
		return "", &Issue{Message: "The " + field + " field must have at least " + itoa(r.MinLength) + " characters", Rule: "minLength", Field: field, Meta: map[string]int{"min": r.MinLength}}
	}
	if r.MaxLength > 0 && length > r.MaxLength {
		return "", &Issue{Message: "The " + field + " field must not be greater than " + itoa(r.MaxLength) + " characters", Rule: "maxLength", Field: field, Meta: map[string]int{"max": r.MaxLength}}
	}
	return value, nil
}

// TrimJS is String.prototype.trim: JavaScript whitespace and line terminators.
func TrimJS(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return r <= 0xFFFF && jsWhitespace(uint16(r)) })
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

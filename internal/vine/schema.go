package vine

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"kaderisasi/web/internal/js"
)

// Schema is a VineJS schema node. Values flow as js.Value so output keeps
// JavaScript's key order and number formatting.
type Schema interface {
	validate(f *field) (js.Value, bool)
	modifiers() (optional, nullable bool)
}

// field is Vine's FieldContext.
type field struct {
	value         js.Value
	present       bool // false for undefined
	path, name    string
	isArrayMember bool
	parent        js.Value
	reporter      *reporter
}

type reporter struct{ issues []Issue }

func (r *reporter) report(f *field, message, rule string, meta map[string]any) {
	var m any
	if meta != nil {
		m = meta
	}
	r.reportMeta(f, message, rule, m)
}

func (r *reporter) reportMeta(f *field, message, rule string, meta any) {
	issue := Issue{Message: strings.ReplaceAll(message, "{{ field }}", f.name), Rule: rule, Field: f.path, Meta: meta}
	if f.isArrayMember {
		index, _ := strconv.Atoi(f.name)
		issue.Index = &index
	}
	r.issues = append(r.issues, issue)
}

type base struct{ Optional, Nullable bool }

func (b base) modifiers() (bool, bool) { return b.Optional, b.Nullable }

// Rule is a custom `vine.createRule` run after the type check, while the
// field is still valid; it returns a message and rule name to report.
type Rule func(value js.Value, parent js.Value) (message, rule string, failed bool)

// Str is vine.string().
type Str struct {
	base
	Trim, Lower, Email bool
	MinLength          int
	MaxLength          int
	Regex              *regexp.Regexp
}

// Num is vine.number(); Parse runs first (Vine's .parse()).
type Num struct {
	base
	WithoutDecimals, Positive bool
	Parse                     func(js.Value, bool) (js.Value, bool)
	Rules                     []Rule
}

// Enum is vine.enum(choices).
type Enum struct {
	base
	Choices []string
}

// Bool is vine.boolean({ strict }).
type Bool struct {
	base
	Strict bool
}

// Arr is vine.array(of).
type Arr struct {
	base
	Of Schema
}

// Prop is one property of an Obj.
type Prop struct {
	Name   string
	Schema Schema
}

// Obj is vine.object(props) (allowUnknownProperties with AllowUnknown).
type Obj struct {
	base
	Props        []Prop
	AllowUnknown bool
}

// Rec is vine.record(of).
type Rec struct {
	base
	Of Schema
}

// Any is vine.any().
type Any struct{ base }

func missing(f *field) bool { return !f.present || f.value.Kind == 'n' }

// run applies the shared required / optional / nullable handling, then the
// type validation. ok=false with no issue means "omit from the output".
func run(s Schema, f *field) (js.Value, bool) {
	if n, ok := s.(Num); ok && n.Parse != nil {
		f.value, f.present = n.Parse(f.value, f.present)
	}
	optional, nullable := s.modifiers()
	if missing(f) {
		if nullable && f.present && f.value.Kind == 'n' {
			return f.value, true
		}
		if optional {
			return js.Value{}, false
		}
		f.reporter.report(f, "The {{ field }} field must be defined", "required", nil)
		return js.Value{}, false
	}
	return s.validate(f)
}

func (s Str) validate(f *field) (js.Value, bool) {
	if f.value.Kind != 's' {
		f.reporter.report(f, "The {{ field }} field must be a string", "string", nil)
		return js.Value{}, false
	}
	v := f.value.String
	if s.Trim {
		v = js.Trim(v)
	}
	if s.Lower {
		v = strings.ToLower(v)
	}
	if s.Email && !IsEmail(v) {
		f.reporter.report(f, "The {{ field }} field must be a valid email address", "email", nil)
		return js.Value{}, false
	}
	if s.Regex != nil && !s.Regex.MatchString(v) {
		f.reporter.report(f, "The {{ field }} field format is invalid", "regex", nil)
		return js.Value{}, false
	}
	length := len(utf16.Encode([]rune(v)))
	if s.MinLength > 0 && length < s.MinLength {
		f.reporter.report(f, "The {{ field }} field must have at least "+strconv.Itoa(s.MinLength)+" characters", "minLength", map[string]any{"min": s.MinLength})
		return js.Value{}, false
	}
	if s.MaxLength > 0 && length > s.MaxLength {
		f.reporter.report(f, "The {{ field }} field must not be greater than "+strconv.Itoa(s.MaxLength)+" characters", "maxLength", map[string]any{"max": s.MaxLength})
		return js.Value{}, false
	}
	return js.Value{Kind: 's', String: v}, true
}

// ToNumber is Number(value) for a parsed JSON value.
func ToNumber(v js.Value) float64 {
	switch v.Kind {
	case 'n':
		return math.NaN() // helpers.asNumber(null)
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
		parts := make([]string, len(v.Items))
		for i, item := range v.Items {
			if item.Kind == 'n' {
				continue
			}
			parts[i] = ToString(item)
		}
		return js.Number(strings.Join(parts, ","))
	}
	return math.NaN()
}

// ToString is String(value) for primitives and arrays.
func ToString(v js.Value) string {
	switch v.Kind {
	case 'n':
		return "null"
	case 'b':
		return strconv.FormatBool(v.Bool)
	case 'f':
		return js.ToString(v.Number)
	case 's':
		return v.String
	case 'a':
		parts := make([]string, len(v.Items))
		for i, item := range v.Items {
			if item.Kind != 'n' {
				parts[i] = ToString(item)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

func (s Num) validate(f *field) (js.Value, bool) {
	n := ToNumber(f.value)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		f.reporter.report(f, "The {{ field }} field must be a number", "number", nil)
		return js.Value{}, false
	}
	out := js.Value{Kind: 'f', Number: n}
	if s.WithoutDecimals && n != math.Trunc(n) {
		f.reporter.report(f, "The {{ field }} field must be an integer", "withoutDecimals", nil)
		return js.Value{}, false
	}
	if s.Positive && n < 0 {
		f.reporter.report(f, "The {{ field }} field must be positive", "positive", nil)
		return js.Value{}, false
	}
	for _, rule := range s.Rules {
		if message, name, failed := rule(out, f.parent); failed {
			f.reporter.report(f, message, name, nil)
			return js.Value{}, false
		}
	}
	return out, true
}

func (s Enum) validate(f *field) (js.Value, bool) {
	for _, c := range s.Choices {
		if f.value.Kind == 's' && f.value.String == c {
			return f.value, true
		}
	}
	f.reporter.report(f, "The selected {{ field }} is invalid", "enum", map[string]any{"choices": s.Choices})
	return js.Value{}, false
}

func (s Bool) validate(f *field) (js.Value, bool) {
	v := f.value
	if !s.Strict {
		switch {
		case v.Kind == 'b':
		case v.Kind == 'f' && v.Number == 1, v.Kind == 's' && (v.String == "1" || v.String == "true" || v.String == "on"):
			v = js.Value{Kind: 'b', Bool: true}
		case v.Kind == 'f' && v.Number == 0, v.Kind == 's' && (v.String == "0" || v.String == "false"):
			v = js.Value{Kind: 'b', Bool: false}
		}
	}
	if v.Kind != 'b' {
		f.reporter.report(f, "The value must be a boolean", "boolean", nil)
		return js.Value{}, false
	}
	return v, true
}

func (s Any) validate(f *field) (js.Value, bool) { return f.value, true }

func child(f *field, name string, v js.Value, present, arrayMember bool) *field {
	path := name
	if f.path != "" {
		path = f.path + "." + name
	}
	return &field{value: v, present: present, path: path, name: name, isArrayMember: arrayMember, parent: f.value, reporter: f.reporter}
}

func (s Arr) validate(f *field) (js.Value, bool) {
	if f.value.Kind != 'a' {
		f.reporter.report(f, "The {{ field }} field must be an array", "array", nil)
		return js.Value{}, false
	}
	before := len(f.reporter.issues)
	out := js.Value{Kind: 'a', Items: []js.Value{}}
	for i, item := range f.value.Items {
		v, ok := run(s.Of, child(f, strconv.Itoa(i), item, true, true))
		if !ok {
			v = js.Value{} // undefined in an array serializes as null
		}
		out.Items = append(out.Items, v)
	}
	return out, len(f.reporter.issues) == before
}

func (s Obj) validate(f *field) (js.Value, bool) {
	if f.value.Kind != 'o' {
		f.reporter.report(f, "The {{ field }} field must be an object", "object", nil)
		return js.Value{}, false
	}
	before := len(f.reporter.issues)
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	set := func(k string, v js.Value) {
		if _, seen := out.Fields[k]; !seen {
			out.Keys = append(out.Keys, k)
		}
		out.Fields[k] = v
	}
	if s.AllowUnknown {
		for _, k := range f.value.Keys {
			set(k, f.value.Fields[k])
		}
	}
	for _, p := range s.Props {
		item, present := f.value.Fields[p.Name]
		v, ok := run(p.Schema, child(f, p.Name, item, present, false))
		if ok {
			set(p.Name, v)
		} else if s.AllowUnknown {
			if _, seen := out.Fields[p.Name]; seen {
				delete(out.Fields, p.Name)
				keys := out.Keys[:0]
				for _, k := range out.Keys {
					if k != p.Name {
						keys = append(keys, k)
					}
				}
				out.Keys = keys
			}
		}
	}
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out, len(f.reporter.issues) == before
}

func (s Rec) validate(f *field) (js.Value, bool) {
	if f.value.Kind != 'o' {
		f.reporter.report(f, "The {{ field }} field must be an object", "object", nil)
		return js.Value{}, false
	}
	before := len(f.reporter.issues)
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	for _, k := range f.value.Keys {
		if v, ok := run(s.Of, child(f, k, f.value.Fields[k], true, false)); ok {
			out.Keys = append(out.Keys, k)
			out.Fields[k] = v
		}
	}
	return out, len(f.reporter.issues) == before
}

// ValidateObject runs a top-level vine.object against request.all().
func ValidateObject(schema Obj, value js.Value) (js.Value, error) {
	r := &reporter{}
	out, ok := run(schema, &field{value: value, present: true, reporter: r})
	if len(r.issues) > 0 || !ok {
		return js.Value{}, &Error{Issues: r.issues}
	}
	return out, nil
}

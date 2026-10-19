// Package forms ports web-be's custom form logic (app/services/form_routing.ts
// and custom_form_submission_service.ts) over parsed JSON values, keeping
// JavaScript's comparison and coercion rules. TestMatchesWebBe runs the
// original TypeScript functions on the same inputs.
package forms

import (
	"errors"
	"strconv"
	"strings"

	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/vine"
)

// RouteError is an Error thrown by the routing functions.
type RouteError struct{ Message string }

func (e *RouteError) Error() string { return e.Message }

func get(v js.Value, key string) (js.Value, bool) {
	if v.Kind != 'o' {
		return js.Value{}, false
	}
	return v.Get(key)
}

func undefined() js.Value { return js.Value{} }

// strictEqual is === between two values that may be undefined (Kind 0).
func strictEqual(a, b js.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case 0, 'n':
		return true
	case 'b':
		return a.Bool == b.Bool
	case 'f':
		return a.Number == b.Number
	case 's':
		return a.String == b.String
	}
	return false // distinct objects are never ===
}

// String is String(value).
func String(v js.Value) string {
	if v.Kind == 0 {
		return "undefined"
	}
	return vine.ToString(v)
}

// OptionValue is optionValue / getOptionValue.
func OptionValue(option js.Value) string {
	label, _ := get(option, "label")
	value, ok := get(option, "value")
	if !ok || value.Kind == 'n' {
		return String(label)
	}
	if s := String(value); s != "" {
		return s
	}
	return String(label)
}

// Section is a section with its effective id (customSections).
type Section struct {
	ID    js.Value
	Value js.Value
}

// CustomSections is customSections: legacy sections get generated ids and
// the profile_data section is left out.
func CustomSections(schema js.Value) []Section {
	sections, _ := get(schema, "fields")
	reserved := map[string]bool{}
	var reservedUndefined bool
	for _, s := range sections.Items {
		id, ok := get(s, "id")
		if !ok {
			reservedUndefined = true
		} else {
			reserved[String(id)+"\x00"+string(id.Kind)] = true
		}
	}
	_ = reservedUndefined
	out := []Section{}
	for index, s := range sections.Items {
		id, ok := get(s, "id")
		if !ok {
			name := "legacy_section_" + strconv.Itoa(index)
			for reserved[name+"\x00s"] {
				name += "_"
			}
			reserved[name+"\x00s"] = true
			id = js.Value{Kind: 's', String: name}
		}
		if n, _ := get(s, "section_name"); n.Kind == 's' && n.String == "profile_data" {
			continue
		}
		out = append(out, Section{ID: id, Value: s})
	}
	return out
}

func truthy(v js.Value) bool { return v.Kind != 0 && v.Truthy() }

func fieldsOf(section js.Value) []js.Value {
	f, _ := get(section, "fields")
	return f.Items
}

// Issue is a RoutingIssue.
type Issue struct {
	SectionID js.Value
	Message   string
}

// ValidateRouting is validateFormRouting.
func ValidateRouting(schema js.Value) []Issue {
	var issues []Issue
	ids := map[string]bool{}
	keys := map[string]bool{}
	sections := CustomSections(schema)
	all, _ := get(schema, "fields")
	profiles := 0
	for _, s := range all.Items {
		if n, _ := get(s, "section_name"); n.String == "profile_data" {
			profiles++
		}
	}
	firstIsProfile := false
	if len(all.Items) > 0 {
		n, _ := get(all.Items[0], "section_name")
		firstIsProfile = n.Kind == 's' && n.String == "profile_data"
	}
	if profiles > 1 || (profiles == 1 && !firstIsProfile) {
		issues = append(issues, Issue{Message: "Data diri harus menjadi bagian pertama dan hanya boleh ada satu."})
	}
	version, hasVersion := get(schema, "version")
	if hasVersion && !(version.Kind == 'f' && version.Number == 2) {
		issues = append(issues, Issue{Message: "Versi formulir tidak didukung."})
	}
	for _, section := range all.Items {
		id, hasID := get(section, "id")
		add := func(message string) { issues = append(issues, Issue{SectionID: id, Message: message}) }
		if hasVersion && version.Kind == 'f' && version.Number == 2 && (id.Kind != 's' || js.Trim(id.String) == "") {
			add("Bagian harus memiliki ID.")
		}
		if hasID {
			key := String(id)
			if id.Kind != 's' || js.Trim(id.String) == "" || ids[key] {
				add("ID bagian harus unik dan tidak kosong.")
			}
			ids[key] = true
		}
		for _, field := range fieldsOf(section) {
			k, _ := get(field, "key")
			if js.Trim(k.String) == "" || keys[k.String] {
				add("Kunci pertanyaan harus unik dan tidak kosong.")
			}
			keys[k.String] = true
		}
		navigation, hasNavigation := get(section, "navigation")
		if !hasNavigation {
			continue
		}
		if !truthy(navigation) || (navigation.Kind != 'o' && navigation.Kind != 'a') {
			add("Pengaturan alur tidak valid.")
			continue
		}
		if n, _ := get(section, "section_name"); n.String == "profile_data" {
			add("Data diri tidak dapat mengatur percabangan.")
			continue
		}
		index := -1
		for i, item := range sections {
			if strictEqual(item.ID, id) || (!hasID && false) {
				index = i
				break
			}
		}
		if !hasID {
			// findIndex(item => item.id === undefined) never matches a generated id.
			index = -1
		}
		if !truthy(id) || index < 0 {
			add("Bagian dengan alur harus memiliki ID.")
		}
		checkTarget := func(target js.Value) {
			if !truthy(target) || (target.Kind != 'o' && target.Kind != 'a') {
				add("Tujuan bagian tidak valid.")
				return
			}
			kind, _ := get(target, "type")
			if kind.Kind == 's' && (kind.String == "next" || kind.String == "submit") {
				return
			}
			sectionID, _ := get(target, "sectionId")
			found := false
			for i, item := range sections {
				if i > index && strictEqual(item.ID, sectionID) {
					found = true
				}
			}
			if !(kind.Kind == 's' && kind.String == "section") || !truthy(sectionID) || !found {
				add("Tujuan harus berupa bagian setelah bagian ini. Perbaiki alur yang terputus.")
			}
		}
		defaultTarget, _ := get(navigation, "defaultTarget")
		checkTarget(defaultTarget)
		routes, hasRoutes := get(navigation, "routes")
		if hasRoutes && routes.Kind != 'a' {
			add("Daftar aturan alur tidak valid.")
			continue
		}
		questionKey, hasQuestion := get(navigation, "questionKey")
		var question *js.Value
		for _, field := range fieldsOf(section) {
			k, _ := get(field, "key")
			if strictEqual(k, questionKeyOrUndefined(questionKey, hasQuestion)) {
				f := field
				question = &f
				break
			}
		}
		if hasQuestion {
			bad := question == nil
			if question != nil {
				t, _ := get(*question, "type")
				hidden, _ := get(*question, "hidden")
				disabled, _ := get(*question, "disabled")
				bad = !(t.Kind == 's' && (t.String == "radio" || t.String == "select")) || truthy(hidden) || truthy(disabled)
			}
			if bad {
				add("Pilih satu pertanyaan pilihan ganda atau dropdown yang aktif di bagian ini.")
			}
		}
		allowed := []string{}
		if question != nil {
			if options, ok := get(*question, "options"); ok && options.Kind == 'a' {
				for _, option := range options.Items {
					if d, _ := get(option, "disabled"); truthy(d) {
						continue
					}
					allowed = append(allowed, OptionValue(option))
				}
			}
		}
		if truthy(questionKey) {
			unique := map[string]bool{}
			for _, a := range allowed {
				unique[a] = true
			}
			if len(unique) != len(allowed) {
				add("Nilai pilihan untuk percabangan harus unik.")
			}
		}
		values := map[string]bool{}
		for _, route := range routes.Items {
			optionValue, _ := get(route, "optionValue")
			contains := false
			for _, a := range allowed {
				contains = contains || (optionValue.Kind == 's' && a == optionValue.String)
			}
			if !truthy(route) || optionValue.Kind != 's' || !truthy(questionKey) || !contains || values[optionValue.String] {
				add("Pilihan pada aturan alur tidak tersedia atau digunakan lebih dari sekali.")
			} else {
				values[optionValue.String] = true
			}
			target, _ := get(route, "target")
			checkTarget(target)
		}
	}
	return issues
}

func questionKeyOrUndefined(v js.Value, ok bool) js.Value {
	if !ok {
		return undefined()
	}
	return v
}

// NextSectionID is nextSectionId; ok is false for null (submit).
func NextSectionID(schema js.Value, sectionID js.Value, answers js.Value) (js.Value, bool, error) {
	sections := CustomSections(schema)
	index := -1
	for i, s := range sections {
		if strictEqual(s.ID, sectionID) {
			index = i
			break
		}
	}
	if index < 0 {
		return js.Value{}, false, &RouteError{"Bagian formulir tidak tersedia."}
	}
	navigation, hasNavigation := get(sections[index].Value, "navigation")
	var target js.Value
	found := false
	if hasNavigation && navigation.Kind != 'n' {
		questionKey, _ := get(navigation, "questionKey")
		if truthy(questionKey) {
			answer, ok := get(answers, String(questionKey))
			if ok && answer.Kind != 'n' {
				routes, _ := get(navigation, "routes")
				if routes.Kind == 'a' {
					for _, route := range routes.Items {
						option, _ := get(route, "optionValue")
						if option.Kind == 's' && option.String == String(answer) {
							if t, ok := get(route, "target"); ok && t.Kind != 'n' {
								target, found = t, true
							}
							break
						}
					}
				}
			}
		}
		if !found {
			if t, ok := get(navigation, "defaultTarget"); ok && t.Kind != 'n' {
				target, found = t, true
			}
		}
	}
	if !found {
		target = js.Value{Kind: 'o', Keys: []string{"type"}, Fields: map[string]js.Value{"type": {Kind: 's', String: "next"}}}
	}
	if target.Kind == 'n' || target.Kind == 0 {
		return js.Value{}, false, errors.New("Cannot read properties of null (reading 'type')")
	}
	kind, _ := get(target, "type")
	if kind.Kind == 's' && kind.String == "submit" {
		return js.Value{}, false, nil
	}
	if kind.Kind == 's' && kind.String == "next" {
		if index+1 < len(sections) {
			return sections[index+1].ID, true, nil
		}
		return js.Value{}, false, nil
	}
	sectionIDTarget, _ := get(target, "sectionId")
	for i, item := range sections {
		if i > index && strictEqual(item.ID, sectionIDTarget) {
			return sectionIDTarget, true, nil
		}
	}
	return js.Value{}, false, &RouteError{"Tujuan formulir tidak valid."}
}

// Route is formRoute: the ids of the sections the answers reach.
func Route(schema js.Value, answers js.Value) ([]js.Value, error) {
	route := []js.Value{}
	sections := CustomSections(schema)
	if len(sections) == 0 {
		return route, nil
	}
	current, ok := sections[0].ID, true
	for ok {
		for _, seen := range route {
			if strictEqual(seen, current) {
				return nil, &RouteError{"Alur formulir berulang."}
			}
		}
		route = append(route, current)
		var err error
		current, ok, err = NextSectionID(schema, current, answers)
		if err != nil {
			return nil, err
		}
	}
	return route, nil
}

// Prune is pruneFormAnswers: answers to skipped sections are dropped.
func Prune(schema js.Value, answers js.Value) (js.Value, error) {
	route, err := Route(schema, answers)
	if err != nil {
		return js.Value{}, err
	}
	skipped := map[string]bool{}
	for _, s := range CustomSections(schema) {
		reached := false
		for _, r := range route {
			reached = reached || strictEqual(r, s.ID)
		}
		if reached {
			continue
		}
		for _, f := range fieldsOf(s.Value) {
			k, _ := get(f, "key")
			skipped[String(k)] = true
		}
	}
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	for _, k := range answers.Keys {
		if !skipped[k] {
			out.Keys = append(out.Keys, k)
			out.Fields[k] = answers.Fields[k]
		}
	}
	return out, nil
}

var _ = strings.TrimSpace

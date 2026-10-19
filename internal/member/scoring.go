package member

import (
	"fmt"

	"kaderisasi/web/internal/js"
)

// typeError is a V8 TypeError message.
type typeError string

func (e typeError) Error() string { return string(e) }

func kindName(v js.Value) string {
	if v.Kind == 'n' {
		return "null"
	}
	return "undefined"
}

// prop is `v.key` with JavaScript's TypeError on null/undefined.
func prop(v js.Value, key string) (js.Value, error) {
	if v.Kind == 0 || v.Kind == 'n' {
		return js.Value{}, typeError(fmt.Sprintf("Cannot read properties of %s (reading '%s')", kindName(v), key))
	}
	f, _ := v.Get(key)
	return f, nil
}

// destructure is `({ a, b }) => …` applied to v.
func destructure(v js.Value, keys ...string) ([]js.Value, error) {
	if v.Kind == 0 || v.Kind == 'n' {
		name := kindName(v)
		return nil, typeError(fmt.Sprintf("Cannot destructure property '%s' of '%s' as it is %s.", keys[0], name, name))
	}
	out := make([]js.Value, len(keys))
	for i, k := range keys {
		out[i], _ = v.Get(k)
	}
	return out, nil
}

// mapArray is `expr.map(fn)`.
func mapArray(v js.Value, expr string, fn func(js.Value) (js.Value, error)) (js.Value, error) {
	if v.Kind == 0 || v.Kind == 'n' {
		return js.Value{}, typeError(fmt.Sprintf("Cannot read properties of %s (reading 'map')", kindName(v)))
	}
	if v.Kind != 'a' {
		return js.Value{}, typeError(expr + ".map is not a function")
	}
	out := js.Value{Kind: 'a', Items: []js.Value{}}
	for _, item := range v.Items {
		mapped, err := fn(item)
		if err != nil {
			return js.Value{}, err
		}
		out.Items = append(out.Items, mapped)
	}
	return out, nil
}

// object builds an ordered object literal; undefined values are left out
// (JSON.stringify drops them).
func object(pairs ...any) js.Value {
	o := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	for i := 0; i < len(pairs); i += 2 {
		k, v := pairs[i].(string), pairs[i+1].(js.Value)
		if v.Kind == 0 {
			continue
		}
		o.Keys = append(o.Keys, k)
		o.Fields[k] = v
	}
	return o
}

func isNumber(v js.Value, n int32) bool { return v.Kind == 'f' && v.Number == float64(n) }

// PublishedScore is activity_scoring.ts publishedActivityScore. It returns
// a null value when nothing is published.
func PublishedScore(raw []byte, registrationID, activityID int32) (js.Value, error) {
	null := js.Value{Kind: 'n'}
	data := js.Value{Kind: 'n'}
	if raw != nil {
		if v, err := js.Parse(raw); err == nil {
			data = v
		}
	}
	version, _ := data.Get("schema_version")
	if !isNumber(version, 1) {
		return null, nil
	}
	published, _ := data.Get("published")
	if !published.Truthy() {
		return null, nil
	}
	pv, _ := published.Get("schema_version")
	reg, _ := published.Get("registration_id")
	act, _ := published.Get("activity_id")
	if !isNumber(pv, 1) || !isNumber(reg, registrationID) || !isNumber(act, activityID) {
		return null, nil
	}
	result, _ := published.Get("result")
	complete, err := prop(result, "complete")
	if err != nil {
		return null, err
	}
	if !complete.Truthy() {
		return null, nil
	}
	revision, _ := published.Get("revision")
	publishedAt, _ := published.Get("published_at")
	draft, _ := published.Get("draft")
	note, err := prop(draft, "note")
	if err != nil {
		return null, err
	}
	rubric, _ := published.Get("rubric")
	rubricNote, err := prop(rubric, "note")
	if err != nil {
		return null, err
	}
	grades, _ := rubric.Get("grades")
	if grades.Kind == 0 || grades.Kind == 'n' {
		grades = js.Value{Kind: 'a'}
	}
	gradeList, err := mapArray(grades, "(published.rubric.grades ?? [])", func(g js.Value) (js.Value, error) {
		f, err := destructure(g, "label", "minimum")
		if err != nil {
			return js.Value{}, err
		}
		return object("label", f[0], "minimum", f[1]), nil
	})
	if err != nil {
		return null, err
	}
	groups, _ := rubric.Get("groups")
	groupList, err := mapArray(groups, "published.rubric.groups", func(g js.Value) (js.Value, error) {
		f, err := destructure(g, "id", "name", "criteria")
		if err != nil {
			return js.Value{}, err
		}
		criteria, err := mapArray(f[2], "criteria", func(c js.Value) (js.Value, error) {
			cf, err := destructure(c, "id", "name", "maximum", "weight")
			if err != nil {
				return js.Value{}, err
			}
			return object("id", cf[0], "name", cf[1], "maximum", cf[2], "weight", cf[3]), nil
		})
		if err != nil {
			return js.Value{}, err
		}
		return object("id", f[0], "name", f[1], "criteria", criteria), nil
	})
	if err != nil {
		return null, err
	}
	total, _ := result.Get("total")
	grade, _ := result.Get("grade")
	resultCriteria, _ := result.Get("criteria")
	criteriaList, err := mapArray(resultCriteria, "published.result.criteria", func(c js.Value) (js.Value, error) {
		f, err := destructure(c, "criterion_id", "score", "normalized", "grade")
		if err != nil {
			return js.Value{}, err
		}
		return object("criterion_id", f[0], "score", f[1], "normalized", f[2], "grade", f[3]), nil
	})
	if err != nil {
		return null, err
	}
	return object("revision", revision, "published_at", publishedAt, "note", note,
		"rubric", object("note", rubricNote, "grades", gradeList, "groups", groupList),
		"result", object("total", total, "grade", grade, "complete", js.Value{Kind: 'b', Bool: true}, "criteria", criteriaList)), nil
}

package member

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
)

// Kaderisasi is helpers/kaderisasi_profile.ts for a validated
// extra_data.kaderisasi_path object: participation means "present", even null.
func Kaderisasi(path js.Value) (level int, badges []string) {
	has := func(k string) (js.Value, bool) { return path.Get(k) }
	badges = []string{}
	for _, item := range []struct {
		key, label string
		level      int
	}{{"ssc", "SSC", 3}, {"lmd", "LMD", 6}, {"spectra", "SPECTRA", 10}} {
		if v, ok := has(item.key); ok {
			if v.Truthy() {
				badges = append(badges, item.label+"-"+js.ToString(v.Number))
			} else {
				badges = append(badges, item.label)
			}
			level = item.level
		}
	}
	return level, badges
}

// spread is `{...value}` for a parsed JSON value.
func spread(v js.Value) js.Value {
	out := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	add := func(k string, item js.Value) {
		if _, seen := out.Fields[k]; !seen {
			out.Keys = append(out.Keys, k)
		}
		out.Fields[k] = item
	}
	switch v.Kind {
	case 'o':
		for _, k := range v.Keys {
			add(k, v.Fields[k])
		}
	case 'a':
		for i, item := range v.Items {
			add(strconv.Itoa(i), item)
		}
	case 's':
		i := 0
		for _, r := range v.String {
			add(strconv.Itoa(i), js.Value{Kind: 's', String: string(r)})
			i++
		}
	}
	return out
}

func merge(base, over js.Value) js.Value {
	out := spread(base)
	for _, k := range over.Keys {
		if _, seen := out.Fields[k]; !seen {
			out.Keys = append(out.Keys, k)
		}
		out.Fields[k] = over.Fields[k]
	}
	out.Keys = js.ObjectKeyOrder(out.Keys)
	return out
}

// ValueEqual is fast-deep-equal on parsed JSON values (key order ignored).
func ValueEqual(a, b js.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case 'n':
		return true
	case 'b':
		return a.Bool == b.Bool
	case 'f':
		return a.Number == b.Number
	case 's':
		return a.String == b.String
	case 'a':
		if len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !ValueEqual(a.Items[i], b.Items[i]) {
				return false
			}
		}
		return true
	case 'o':
		if len(a.Keys) != len(b.Keys) {
			return false
		}
		for _, k := range a.Keys {
			other, ok := b.Fields[k]
			if !ok || !ValueEqual(a.Fields[k], other) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func normalizedEducation(v js.Value) js.Value {
	raw, _ := v.MarshalJSON()
	out, _ := js.Parse(mustJSON(EducationHistory(raw)))
	return out
}

func mustJSON(v any) []byte {
	raw, _ := marshal(v)
	return raw
}

// profileColumns maps a profile attribute (as serialized) to its SQL type
// for the cast of a bound text value.
var profileColumns = map[string]string{
	"name": "varchar", "gender": "varchar", "personal_id": "varchar", "picture": "varchar", "whatsapp": "varchar", "line": "varchar",
	"instagram": "varchar", "tiktok": "varchar", "linkedin": "varchar", "province_id": "integer", "city_id": "integer",
	"university_id": "integer", "major": "varchar", "intake_year": "integer", "level": "integer", "badges": "jsonb",
	"updated_at": "timestamptz", "birth_date": "date", "origin_province_id": "integer", "origin_city_id": "integer",
	"country": "varchar", "education_history": "jsonb", "work_history": "jsonb", "extra_data": "jsonb",
}

// UpdateProfile is ProfilesController.update after validation: it merges
// the payload into the stored profile like Lucid's merge + save (only dirty
// columns are written, updated_at only moves when something changed) and
// returns the serialized model.
func UpdateProfile(ctx context.Context, db dbgen.DBTX, profile dbgen.Profile, payload js.Value, zone *time.Location) (Object, error) {
	current := Profile(profile, zone)
	updates := js.Value{Kind: 'o', Fields: map[string]js.Value{}}
	set := func(k string, v js.Value) {
		if _, seen := updates.Fields[k]; !seen {
			updates.Keys = append(updates.Keys, k)
		}
		updates.Fields[k] = v
	}
	for _, k := range payload.Keys {
		set(k, payload.Fields[k])
	}
	if extra, ok := payload.Get("extra_data"); ok && extra.Truthy() {
		stored := js.Value{Kind: 'n'}
		if v, ok := current.Get("extra_data"); ok && v != nil {
			stored = v.(js.Value)
		}
		set("extra_data", merge(stored, extra))
	}
	if education, ok := payload.Get("education_history"); ok && education.Truthy() {
		set("education_history", normalizedEducation(education))
	}
	if extra, ok := payload.Get("extra_data"); ok {
		if path, ok := extra.Get("kaderisasi_path"); ok && path.Truthy() {
			level, badges := Kaderisasi(path)
			set("level", js.Value{Kind: 'f', Number: float64(level)})
			items := make([]js.Value, len(badges))
			for i, b := range badges {
				items[i] = js.Value{Kind: 's', String: b}
			}
			set("badges", js.Value{Kind: 'a', Items: items})
		}
	}

	original := current.Value()
	dirty := map[string]js.Value{}
	for _, k := range updates.Keys {
		v := updates.Fields[k]
		old, _ := original.Get(k)
		// birth_date is a JavaScript Date before the merge, so any value differs.
		if k == "birth_date" || !ValueEqual(old, v) {
			dirty[k] = v
		}
		current = current.Set(k, v)
	}
	if len(dirty) == 0 {
		return current, nil
	}
	updated := Now()
	current = current.Set("updated_at", ISO(updated, zone))
	dirty["updated_at"] = js.Value{Kind: 's', String: updated.In(zone).Format("2006-01-02T15:04:05.000-07:00")}

	prepared := map[string]string{}
	for k, v := range dirty {
		if text := prepareProfile(k, v); text != nil {
			prepared[k] = text.(string)
		} else {
			prepared[k] = "\x00null"
		}
	}
	if err := update(ctx, db, "profiles", profileColumns, current, prepared, profile.ID); err != nil {
		return nil, err
	}
	return current, nil
}

// prepareProfile is the column's prepare function and the pg driver's
// value conversion, as text.
func prepareProfile(key string, v js.Value) any {
	switch key {
	case "badges":
		raw, _ := v.MarshalJSON()
		return string(mustJSON(StringArray(raw)))
	case "education_history", "work_history", "extra_data":
		if v.Kind == 'n' {
			return nil
		}
		raw, _ := v.MarshalJSON()
		return string(raw)
	}
	switch v.Kind {
	case 'n':
		return nil
	case 's':
		return v.String
	case 'f':
		return js.ToString(v.Number)
	case 'b':
		return strconv.FormatBool(v.Bool)
	}
	raw, _ := v.MarshalJSON()
	return string(raw)
}

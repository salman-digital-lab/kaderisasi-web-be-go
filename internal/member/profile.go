package member

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"kaderisasi/web/internal/dbgen"
)

// User serializes a PublicUser loaded from the database. Lucid emits columns in
// row order with snake_case names and hides `password`.
func User(u dbgen.PublicUser, zone *time.Location) Object {
	return Object{{"id", u.ID}, {"email", u.Email}, {"created_at", DateTime(u.CreatedAt, zone)},
		{"updated_at", DateTime(u.UpdatedAt, zone)}, {"member_id", u.MemberID}, {"account_status", u.AccountStatus}}
}

// CreatedUser serializes a PublicUser created in this request: Lucid emits the
// attributes in the order they were set (email, password, account_status,
// created_at, updated_at, then the returned id and the member id saved after).
func CreatedUser(u dbgen.PublicUser, zone *time.Location) Object {
	return Object{{"email", u.Email}, {"account_status", u.AccountStatus}, {"created_at", DateTime(u.CreatedAt, zone)},
		{"updated_at", DateTime(u.UpdatedAt, zone)}, {"id", u.ID}, {"member_id", u.MemberID}}
}

// Profile serializes web-be's Profile model loaded from the database,
// including the column consumers in app/models/profile.ts.
func Profile(p dbgen.Profile, zone *time.Location) Object {
	return Object{{"id", p.ID}, {"user_id", p.UserID}, {"name", p.Name}, {"gender", p.Gender}, {"personal_id", p.PersonalID},
		{"picture", p.Picture}, {"whatsapp", p.Whatsapp}, {"line", p.Line}, {"instagram", p.Instagram}, {"tiktok", p.Tiktok},
		{"linkedin", p.Linkedin}, {"province_id", p.ProvinceID}, {"city_id", p.CityID}, {"university_id", p.UniversityID},
		{"major", p.Major}, {"intake_year", p.IntakeYear}, {"level", p.Level}, {"badges", StringArray(p.Badges)},
		{"created_at", DateTime(p.CreatedAt, zone)}, {"updated_at", DateTime(p.UpdatedAt, zone)}, {"birth_date", Date(p.BirthDate, zone)},
		{"origin_province_id", p.OriginProvinceID}, {"origin_city_id", p.OriginCityID}, {"country", p.Country},
		{"education_history", EducationHistory(p.EducationHistory)}, {"work_history", WorkHistory(p.WorkHistory)},
		{"extra_data", jsonb(p.ExtraData)}}
}

// StringArray is normalizeStringArray from app/models/profile.ts.
func StringArray(raw []byte) []string {
	out := []string{}
	var value any
	if raw == nil || json.Unmarshal(raw, &value) != nil {
		return out
	}
	switch v := value.(type) {
	case []any:
		return onlyStrings(v)
	case string:
		trimmed := trimJS(v)
		if trimmed == "" {
			return out
		}
		var parsed any
		if json.Unmarshal([]byte(trimmed), &parsed) != nil {
			return []string{trimmed}
		}
		if list, ok := parsed.([]any); ok {
			return onlyStrings(list)
		}
		return []string{trimmed}
	}
	return out
}

func onlyStrings(list []any) []string {
	out := []string{}
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// historyEntries is app/helpers/education_history.ts historyEntries.
func historyEntries(raw []byte) []map[string]any {
	var value any
	if raw == nil || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	if s, ok := value.(string); ok {
		if json.Unmarshal([]byte(s), &value) != nil {
			return nil
		}
	}
	list, _ := value.([]any)
	entries := []map[string]any{}
	for _, entry := range list {
		if object, ok := entry.(map[string]any); ok {
			entries = append(entries, object)
		}
	}
	return entries
}

func text(v any) string {
	if s, ok := v.(string); ok {
		return trimJS(s)
	}
	return ""
}

// year is normalizeYearValue: a finite Number(value), or undefined.
func year(v any) (float64, bool) {
	var n float64
	switch value := v.(type) {
	case float64:
		n = value
	case string:
		if trimJS(value) == "" {
			return 0, false
		}
		n = jsNumber(value)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}

func withYear(o Object, key string, v any) Object {
	if n, ok := year(v); ok {
		return append(o, Pair{key, n})
	}
	return o
}

// EducationHistory is normalizeEducationHistory.
func EducationHistory(raw []byte) []Object {
	out := []Object{}
	for _, e := range historyEntries(raw) {
		o := Object{}
		switch e["degree"] {
		case "high_school", "diploma", "bachelor", "master", "doctoral":
			o = append(o, Pair{"degree", e["degree"]})
		}
		o = append(o, Pair{"institution", text(e["institution"])}, Pair{"faculty", text(e["faculty"])}, Pair{"major", text(e["major"])})
		out = append(out, withYear(o, "intake_year", e["intake_year"]))
	}
	return out
}

// WorkHistory is normalizeWorkHistory.
func WorkHistory(raw []byte) []Object {
	out := []Object{}
	for _, e := range historyEntries(raw) {
		o := Object{{"job_title", text(e["job_title"])}, {"company", text(e["company"])}}
		o = withYear(o, "start_year", e["start_year"])
		out = append(out, withYear(o, "end_year", e["end_year"]))
	}
	return out
}

// MemberID is generateMemberId: the user id zero-padded to eight digits.
func MemberID(id int32) string { return fmt.Sprintf("%08d", id) }

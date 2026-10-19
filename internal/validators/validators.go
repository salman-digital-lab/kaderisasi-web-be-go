// Package validators holds the Go equivalents of kaderisasi-web-be's Vine
// validators (app/validators/*.ts). TestValidatorsMatchWebBe runs them and
// the original modules on the same inputs.
package validators

import (
	"regexp"
	"time"

	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/vine"
)

func str() vine.Str { return vine.Str{} }

func optStr() vine.Str { s := vine.Str{}; s.Optional = true; return s }
func optNum() vine.Num { s := vine.Num{}; s.Optional = true; return s }

func optEnum(choices ...string) vine.Enum {
	e := vine.Enum{Choices: choices}
	e.Optional = true
	return e
}

// Now is the clock historyYear uses (new Date().getFullYear()).
var Now = time.Now

// yearRule: an integer between 1900 and ten years from now.
func yearRule(value js.Value, _ js.Value) (string, string, bool) {
	n := value.Number
	if n != float64(int64(n)) || n < 1900 || n > float64(Now().Year()+10) {
		return "Tahun tidak valid", "historyYear", true
	}
	return "", "", false
}

// endYearRule: not before the sibling start_year.
func endYearRule(value js.Value, parent js.Value) (string, string, bool) {
	start, ok := parent.Get("start_year")
	if !ok || start.Kind == 'n' || (start.Kind == 's' && start.String == "") {
		return "", "", false
	}
	if value.Number < vine.ToNumber(start) {
		return "Tahun selesai tidak boleh lebih kecil dari tahun mulai", "workYearRange", true
	}
	return "", "", false
}

// historyYearParse is `.parse(v => typeof v === 'string' && v.trim() === ” ? undefined : v)`.
func historyYearParse(v js.Value, present bool) (js.Value, bool) {
	if present && v.Kind == 's' && js.Trim(v.String) == "" {
		return js.Value{}, false
	}
	return v, present
}

func historyYear(rules ...vine.Rule) vine.Num {
	n := vine.Num{Parse: historyYearParse, Rules: append([]vine.Rule{yearRule}, rules...)}
	n.Optional = true
	return n
}

func trimmed(min int) vine.Str {
	s := vine.Str{Trim: true, MinLength: min}
	if min == 0 {
		s.Optional = true
	}
	return s
}

var EducationHistory = func() vine.Arr {
	a := vine.Arr{Of: vine.Obj{Props: []vine.Prop{
		{Name: "degree", Schema: optEnum("high_school", "diploma", "bachelor", "master", "doctoral")},
		{Name: "institution", Schema: trimmed(0)},
		{Name: "faculty", Schema: trimmed(0)},
		{Name: "major", Schema: trimmed(0)},
		{Name: "intake_year", Schema: historyYear()},
	}}}
	a.Optional = true
	return a
}()

var WorkHistory = func() vine.Arr {
	a := vine.Arr{Of: vine.Obj{Props: []vine.Prop{
		{Name: "job_title", Schema: trimmed(1)},
		{Name: "company", Schema: trimmed(1)},
		{Name: "start_year", Schema: historyYear()},
		{Name: "end_year", Schema: historyYear(endYearRule)},
	}}}
	a.Optional = true
	return a
}()

func nullableOptNum() vine.Num {
	n := vine.Num{}
	n.Optional, n.Nullable = true, true
	return n
}

var extraData = func() vine.Obj {
	strings := vine.Arr{Of: str()}
	strings.Optional = true
	focus := vine.Arr{Of: vine.Enum{Choices: []string{"professional", "academic", "social", "entrepreneur", "politics", "other"}}}
	focus.Optional = true
	path := vine.Obj{Props: []vine.Prop{{Name: "ssc", Schema: nullableOptNum()}, {Name: "lmd", Schema: nullableOptNum()}, {Name: "spectra", Schema: nullableOptNum()}}}
	path.Optional = true
	o := vine.Obj{Props: []vine.Prop{{Name: "preferred_name", Schema: optStr()}, {Name: "salman_activity_history", Schema: strings},
		{Name: "current_activity_focus", Schema: focus}, {Name: "kaderisasi_path", Schema: path}}}
	o.Optional = true
	return o
}()

// UpdateProfile is profile_validator.ts updateProfileValidator.
var UpdateProfile = vine.Obj{Props: []vine.Prop{
	{Name: "name", Schema: optStr()}, {Name: "gender", Schema: optEnum("M", "F")}, {Name: "whatsapp", Schema: optStr()},
	{Name: "line", Schema: optStr()}, {Name: "instagram", Schema: optStr()}, {Name: "province_id", Schema: optNum()},
	{Name: "city_id", Schema: optNum()}, {Name: "linkedin", Schema: optStr()}, {Name: "personal_id", Schema: optStr()},
	{Name: "tiktok", Schema: optStr()}, {Name: "birth_date", Schema: optStr()}, {Name: "origin_province_id", Schema: optNum()},
	{Name: "origin_city_id", Schema: optNum()}, {Name: "country", Schema: optStr()}, {Name: "education_history", Schema: EducationHistory},
	{Name: "work_history", Schema: WorkHistory}, {Name: "extra_data", Schema: extraData},
}}

// SelfSubmit is member_validator.ts selfSubmitValidator.
var SelfSubmit = vine.Obj{Props: []vine.Prop{
	{Name: "name", Schema: str()}, {Name: "gender", Schema: optEnum("M", "F")}, {Name: "personal_id", Schema: optStr()},
	{Name: "whatsapp", Schema: optStr()}, {Name: "line", Schema: optStr()}, {Name: "instagram", Schema: optStr()},
	{Name: "tiktok", Schema: optStr()}, {Name: "linkedin", Schema: optStr()}, {Name: "province_id", Schema: optNum()},
	{Name: "city_id", Schema: optNum()}, {Name: "country", Schema: optStr()}, {Name: "birth_date", Schema: optStr()},
	{Name: "origin_province_id", Schema: optNum()}, {Name: "origin_city_id", Schema: optNum()},
	{Name: "education_history", Schema: EducationHistory}, {Name: "work_history", Schema: WorkHistory}, {Name: "extra_data", Schema: extraData},
}}

func integer(optional bool) vine.Num {
	n := vine.Num{WithoutDecimals: true}
	n.Optional = optional
	return n
}

var proofExtnames = []string{"jpg", "png", "jpeg", "pdf", "doc", "docx", "PNG", "JPG", "JPEG", "PDF", "DOC", "DOCX"}

// Achievement and UpdateAchievement are achievement_validator.ts.
var Achievement = vine.Obj{Props: []vine.Prop{{Name: "name", Schema: str()}, {Name: "description", Schema: str()},
	{Name: "achievement_date", Schema: str()}, {Name: "type", Schema: integer(false)},
	{Name: "proof", Schema: vine.File{Size: "5mb", Extnames: proofExtnames}}}}

var UpdateAchievement = vine.Obj{Props: []vine.Prop{{Name: "name", Schema: optStr()}, {Name: "description", Schema: optStr()},
	{Name: "achievement_date", Schema: optStr()}, {Name: "type", Schema: integer(true)},
	{Name: "proof", Schema: vine.OptionalFile(vine.File{Size: "5mb", Extnames: proofExtnames})}}}

// Image is profile_validator.ts imageValidator.
var Image = vine.Obj{Props: []vine.Prop{{Name: "file", Schema: vine.File{Size: "2mb", Extnames: []string{"jpg", "png", "jpeg", "PNG", "JPG", "JPEG"}}}}}

// StoreRuangCurhat is ruang_curhat_validator.ts.
var StoreRuangCurhat = vine.Obj{Props: []vine.Prop{{Name: "problem_ownership", Schema: vine.Num{}}, {Name: "owner_name", Schema: optStr()},
	{Name: "problem_category", Schema: str()}, {Name: "problem_description", Schema: str()}, {Name: "handling_technic", Schema: str()},
	{Name: "counselor_gender", Schema: str()}}}

// NotificationList and NotificationReadAll are notification_validator.ts.
var NotificationList = vine.Obj{Props: []vine.Prop{
	{Name: "cursor", Schema: func() vine.Str { s := vine.Str{MaxLength: 500}; s.Optional = true; return s }()},
	{Name: "unread", Schema: optEnum("true", "false")}}}

var NotificationReadAll = vine.Obj{Props: []vine.Prop{{Name: "cutoff", Schema: vine.Str{MaxLength: 50}}}}

var sessionToken = func() vine.Str {
	s := vine.Str{Regex: regexp.MustCompile(`^[a-f0-9]{64}$`)}
	s.Optional = true
	return s
}()

// ActivityRegistration and GuestActivityRegistration are activity_validator.ts.
var ActivityRegistration = vine.Obj{Props: []vine.Prop{{Name: "session_token", Schema: sessionToken},
	{Name: "questionnaire_answer", Schema: vine.Obj{AllowUnknown: true}}}}

var GuestActivityRegistration = vine.Obj{Props: []vine.Prop{{Name: "session_token", Schema: sessionToken},
	{Name: "guest_data", Schema: vine.Obj{AllowUnknown: true, Props: []vine.Prop{{Name: "name", Schema: trimmed(1)}, {Name: "email", Schema: vine.Str{Email: true}}}}},
	{Name: "questionnaire_answer", Schema: vine.Obj{AllowUnknown: true}}}}

// UpdateClubRegistration is club_registration_validator.ts.
var UpdateClubRegistration = vine.Obj{Props: []vine.Prop{{Name: "additional_data", Schema: func() vine.Rec {
	r := vine.Rec{Of: vine.Any{}}
	r.Optional = true
	return r
}()}}}

// CourseCompletion is course_validator.ts.
var CourseCompletion = vine.Obj{Props: []vine.Prop{{Name: "completed", Schema: vine.Bool{Strict: true}}}}

// All maps the web-be export names to their schemas (for the interop test).
var All = map[string]vine.Obj{
	"profile_validator.updateProfileValidator":                    UpdateProfile,
	"member_validator.selfSubmitValidator":                        SelfSubmit,
	"achievement_validator.achievementValidator":                  Achievement,
	"achievement_validator.updateAchievementValidator":            UpdateAchievement,
	"ruang_curhat_validator.storeRuangCurhatValidator":            StoreRuangCurhat,
	"notification_validator.notificationListValidator":            NotificationList,
	"notification_validator.notificationReadAllValidator":         NotificationReadAll,
	"activity_validator.activityRegistrationValidator":            ActivityRegistration,
	"activity_validator.guestActivityRegistrationValidator":       GuestActivityRegistration,
	"club_registration_validator.updateClubRegistrationValidator": UpdateClubRegistration,
	"course_validator.courseCompletionValidator":                  CourseCompletion,
	"profile_validator.imageValidator":                            Image,
}

// FormSubmission and FormSession are form_responses_controller.ts's
// submissionValidator and sessionValidator.
var FormSubmission = vine.Obj{Props: []vine.Prop{
	{Name: "session_token", Schema: vine.Str{Regex: regexp.MustCompile(`^[a-f0-9]{64}$`)}},
	{Name: "answers", Schema: vine.Rec{Of: func() vine.Any {
		a := vine.Any{}
		a.Optional, a.Nullable = true, true
		return a
	}()}}}}

var FormSession = vine.Obj{Props: []vine.Prop{{Name: "schema_hash", Schema: vine.Str{Regex: regexp.MustCompile(`^[a-f0-9]{64}$`)}}}}

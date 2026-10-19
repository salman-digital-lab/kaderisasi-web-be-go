package member

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
)

// LucidDate is a Lucid `@column.date()` value: DateTime#toISODate().
func LucidDate(d pgtype.Date) any {
	if !d.Valid || d.InfinityModifier != pgtype.Finite {
		return nil
	}
	return d.Time.Format("2006-01-02")
}

// JSONB is a jsonb column as JSON.parse returns it.
func JSONB(raw []byte) any { return jsonb(raw) }

// Activity serializes the Activity model (badge is not a declared column).
func Activity(a dbgen.Activity, zone *time.Location) Object {
	return KV("id", a.ID, "name", a.Name, "slug", a.Slug, "description", a.Description,
		"activity_start", LucidDate(a.ActivityStart), "activity_end", LucidDate(a.ActivityEnd), "registration_start", LucidDate(a.RegistrationStart),
		"registration_end", LucidDate(a.RegistrationEnd), "selection_start", LucidDate(a.SelectionStart), "selection_end", LucidDate(a.SelectionEnd),
		"activity_type", a.ActivityType, "activity_category", a.ActivityCategory, "additional_config", jsonb(a.AdditionalConfig),
		"minimum_level", a.MinimumLevel, "is_published", a.IsPublished, "created_at", DateTime(a.CreatedAt, zone),
		"updated_at", DateTime(a.UpdatedAt, zone), "is_registration_open", a.IsRegistrationOpen, "club_id", a.ClubID,
		"certificate_template_id", a.CertificateTemplateID)
}

// Registration serializes ActivityRegistration (scoring_data is hidden).
func Registration(r dbgen.ActivityRegistration, zone *time.Location) Object {
	return KV("id", r.ID, "user_id", r.UserID, "activity_id", r.ActivityID, "status", r.Status, "questionnaire_answer", jsonb(r.QuestionnaireAnswer),
		"created_at", DateTime(r.CreatedAt, zone), "updated_at", DateTime(r.UpdatedAt, zone), "guest_data", jsonb(r.GuestData),
		"certificate_group", r.CertificateGroup)
}

// Achievement serializes the Achievement model.
func Achievement(a dbgen.Achievement, zone *time.Location) Object {
	return KV("id", a.ID, "user_id", a.UserID, "name", a.Name, "description", a.Description, "achievement_date", LucidDate(a.AchievementDate),
		"type", a.Type, "score", a.Score, "proof", a.Proof, "is_proof_deleted", a.IsProofDeleted, "status", a.Status, "remark", a.Remark,
		"approver_id", a.ApproverID, "approved_at", LucidDate(a.ApprovedAt), "created_at", DateTime(a.CreatedAt, zone), "updated_at", DateTime(a.UpdatedAt, zone))
}

// RuangCurhat serializes the RuangCurhat model.
func RuangCurhat(r dbgen.RuangCurhat, zone *time.Location) Object {
	return KV("id", r.ID, "user_id", r.UserID, "problem_ownership", r.ProblemOwnership, "owner_name", r.OwnerName, "problem_category", r.ProblemCategory,
		"problem_description", r.ProblemDescription, "handling_technic", r.HandlingTechnic, "counselor_gender", r.CounselorGender,
		"counselor_id", r.CounselorID, "status", r.Status, "additional_notes", r.AdditionalNotes, "created_at", DateTime(r.CreatedAt, zone),
		"updated_at", DateTime(r.UpdatedAt, zone))
}

// AdminUser serializes the AdminUser model (password hidden; is_active and
// additional_role_codes are not declared columns).
func AdminUser(u dbgen.AdminUser, zone *time.Location) Object {
	return KV("id", u.ID, "email", u.Email, "display_name", u.DisplayName, "created_at", DateTime(u.CreatedAt, zone),
		"updated_at", DateTime(u.UpdatedAt, zone), "normalized_email", u.NormalizedEmail, "role_code", u.RoleCode)
}

// Set replaces a key's value in place, or appends it (JavaScript property
// assignment on an object).
func (o Object) Set(key string, value any) Object {
	for i := range o {
		if o[i].Key == key {
			o[i].Value = value
			return o
		}
	}
	return append(o, Pair{key, value})
}

// Get returns a key's value.
func (o Object) Get(key string) (any, bool) {
	for _, p := range o {
		if p.Key == key {
			return p.Value, true
		}
	}
	return nil, false
}

// Value converts a serialized object to an ordered js.Value.
func (o Object) Value() js.Value {
	raw, _ := o.MarshalJSON()
	v, _ := js.Parse(raw)
	return v
}

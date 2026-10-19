// Package certificate serves the public certificate lookups of
// kaderisasi-web-be/app/services/certificate_service.ts and
// certificate_approval.ts from issued_certificates snapshots.
package certificate

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
)

const codeMaxLength = 96

var codePattern = regexp.MustCompile(`^[A-Z0-9-]+$`)

// NormalizeCode is normalizeCertificateCode; ok is false for an invalid code.
func NormalizeCode(code string) (string, bool) {
	normalized := strings.ToUpper(js.Trim(code))
	if normalized == "" || len(utf16.Encode([]rune(normalized))) > codeMaxLength || !codePattern.MatchString(normalized) {
		return "", false
	}
	return normalized, true
}

// errTypeError stands for a JavaScript TypeError while reading a malformed
// snapshot; the controllers answer 500 GENERAL_ERROR.
var errTypeError = errors.New("malformed certificate snapshot")

func parse(raw []byte) js.Value {
	v, err := js.Parse(raw)
	if err != nil {
		return js.Value{Kind: 'n'}
	}
	return v
}

// field reads value[key] like JavaScript property access on a non-null
// value; ok is false for undefined.
func field(v js.Value, key string) (js.Value, bool) {
	if v.Kind == 'o' {
		return v.Get(key)
	}
	return js.Value{}, false
}

func nullish(v js.Value, ok bool) bool { return !ok || v.Kind == 'n' }

// HasApproval is hasCertificateApproval.
func HasApproval(issued dbgen.IssuedCertificate) (bool, error) {
	template := parse(issued.TemplateSnapshot)
	if template.Kind == 'n' {
		return true, nil
	}
	data, ok := field(template, "template_data")
	if nullish(data, ok) {
		return false, errTypeError // undefined.elements
	}
	elements, ok := field(data, "elements")
	if !ok || elements.Kind != 'a' {
		return false, errTypeError // elements.some is not a function
	}
	required := false
	for _, element := range elements.Items {
		if element.Kind == 'n' {
			return false, errTypeError
		}
		kind, _ := field(element, "type")
		if kind.Kind != 's' || kind.String != "variable-text" {
			continue
		}
		variable, ok := field(element, "variable")
		if nullish(variable, ok) {
			continue
		}
		if variable.Kind != 's' {
			return false, errTypeError // variable.replace is not a function
		}
		if js.Trim(strings.NewReplacer("{", "", "}", "").Replace(variable.String)) == "approval" {
			required = true
			break
		}
	}
	if !required {
		return true, nil
	}
	approval := parse(issued.ApprovalSnapshot)
	for _, key := range []string{"signer_name", "signer_title", "approved_at"} {
		v, ok := field(approval, key)
		if !ok || !v.Truthy() {
			return false, nil
		}
	}
	return true, nil
}

// put appends key: value unless the value is undefined.
func put(o member.Object, key string, v js.Value, ok bool) member.Object {
	if !ok {
		return o
	}
	return append(o, member.Pair{Key: key, Value: v})
}

func publicApproval(raw []byte) (member.Object, bool) {
	approval := parse(raw)
	if !approval.Truthy() {
		return nil, false
	}
	o := member.Object{}
	for _, key := range []string{"signer_name", "signer_title", "approved_at"} {
		v, ok := field(approval, key)
		o = put(o, key, v, ok)
	}
	return o, true
}

type snapshots struct {
	activity, participant, template js.Value
	gender                          js.Value
}

// responseData is the part of buildIssuedResponseData both payloads read.
func responseData(issued dbgen.IssuedCertificate) (snapshots, error) {
	participant := parse(issued.ParticipantSnapshot)
	activity := parse(issued.ActivitySnapshot)
	if activity.Kind == 'n' {
		if participant.Kind == 'n' {
			return snapshots{}, errTypeError
		}
		name, ok := field(participant, "activity_name")
		activity = js.Value{Kind: 'o', Fields: map[string]js.Value{"id": {Kind: 'f', Number: float64(issued.ActivityID)}, "activity_start": {Kind: 'n'}}}
		activity.Keys = []string{"id"}
		if ok {
			activity.Keys = append(activity.Keys, "name")
			activity.Fields["name"] = name
		}
		activity.Keys = append(activity.Keys, "activity_start")
	}
	if participant.Kind == 'n' {
		return snapshots{}, errTypeError
	}
	gender, ok := field(participant, "gender")
	if nullish(gender, ok) {
		gender = js.Value{Kind: 's'}
	}
	return snapshots{activity: activity, participant: participant, template: parse(issued.TemplateSnapshot), gender: gender}, nil
}

func (s snapshots) participantField(key string) (js.Value, bool) {
	if key == "gender" {
		return s.gender, true
	}
	return field(s.participant, key)
}

func iso(t time.Time, zone *time.Location) string { return member.ISO(t, zone) }

func revokedAt(issued dbgen.IssuedCertificate, zone *time.Location) any {
	if !issued.RevokedAt.Valid {
		return nil
	}
	return iso(issued.RevokedAt.Time, zone)
}

func state(issued dbgen.IssuedCertificate) string {
	if issued.RevokedAt.Valid {
		return "issued_revoked"
	}
	return "issued_active"
}

// Public is serializePublicCertificate(buildIssuedResponseData(issued)).
func Public(issued dbgen.IssuedCertificate, zone *time.Location) (member.Object, error) {
	s, err := responseData(issued)
	if err != nil {
		return nil, err
	}
	if s.template.Kind == 'n' {
		return nil, errTypeError // data.template.name
	}
	activity := member.Object{}
	v, ok := field(s.activity, "name")
	activity = put(activity, "name", v, ok)
	v, ok = field(s.activity, "activity_start")
	activity = put(activity, "activity_start", v, ok)
	v, ok = s.participantField("activity_date")
	activity = put(activity, "activity_date", v, ok)
	v, ok = field(s.activity, "certificate_settings")
	activity = put(activity, "certificate_settings", v, ok)

	template := member.Object{}
	for _, key := range []string{"name", "background_image", "template_data"} {
		v, ok := field(s.template, key)
		template = put(template, key, v, ok)
	}
	participant := member.Object{}
	for _, key := range []string{"name", "university", "gender", "activity_name", "activity_date"} {
		v, ok := s.participantField(key)
		participant = put(participant, key, v, ok)
	}
	certificate := member.Object{}
	if approval, ok := publicApproval(issued.ApprovalSnapshot); ok {
		certificate = append(certificate, member.Pair{Key: "approval", Value: approval})
	}
	certificate = append(certificate, member.KV("certificate_code", issued.CertificateCode, "issued_at", iso(issued.IssuedAt.Time, zone),
		"revoked_at", revokedAt(issued, zone), "revoked_reason", issued.RevokedReason)...)
	return member.KV("state", state(issued), "activity", activity, "template", template, "participant", participant, "certificate", certificate), nil
}

// Verification is serializeCertificateVerification(buildIssuedResponseData(issued)).
func Verification(issued dbgen.IssuedCertificate, zone *time.Location) (member.Object, error) {
	s, err := responseData(issued)
	if err != nil {
		return nil, err
	}
	o := member.KV("valid", !issued.RevokedAt.Valid)
	if approval, ok := publicApproval(issued.ApprovalSnapshot); ok {
		o = append(o, member.Pair{Key: "approval", Value: approval})
	}
	o = append(o, member.KV("state", state(issued), "certificate_code", issued.CertificateCode)...)
	v, ok := s.participantField("name")
	o = put(o, "participant_name", v, ok)
	v, ok = s.participantField("activity_name")
	o = put(o, "activity_name", v, ok)
	v, ok = s.participantField("activity_date")
	o = put(o, "activity_date", v, ok)
	return append(o, member.KV("issued_at", iso(issued.IssuedAt.Time, zone), "revoked_at", revokedAt(issued, zone),
		"revoked_reason", issued.RevokedReason)...), nil
}

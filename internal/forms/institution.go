package forms

import "kaderisasi/web/internal/js"

// The checks in this file are deliberately stricter than the retired web-be,
// which accepted an education entry without an institution. They run after
// Validate so that Validate itself stays identical to web-be.

func blankText(entry js.Value, key string) bool {
	item, ok := get(entry, key)
	return !ok || item.Kind != 's' || js.Trim(item.String) == ""
}

// lacksInstitution reports an education answer that names no institution.
// A current_education answer left entirely untouched counts as unanswered.
func lacksInstitution(kind string, v js.Value) bool {
	switch kind {
	case "education_history":
		for _, entry := range v.Items {
			if isRecord(entry) && blankText(entry, "institution") {
				return true
			}
		}
	case "current_education":
		return isRecord(v) && blankText(v, "institution") && !(blankText(v, "faculty") && blankText(v, "major"))
	}
	return false
}

// ValidateSubmission is Validate plus the rule that every education answer
// names its institution.
func ValidateSubmission(schema, submission js.Value) (valid bool, data js.Value, errs []SubmissionError, err error) {
	valid, data, errs, err = Validate(schema, submission)
	if err != nil || !valid {
		return valid, data, errs, err
	}
	sections, _ := get(schema, "fields")
	for _, section := range sections.Items {
		for _, field := range fieldsOf(section) {
			k, _ := get(field, "key")
			kind, _ := get(field, "type")
			label, _ := get(field, "label")
			if value, ok := data.Get(k.String); ok && lacksInstitution(kind.String, value) {
				key := k.String
				errs = append(errs, SubmissionError{Field: &key, Message: "Institusi pada " + label.String + " wajib diisi."})
			}
		}
	}
	if len(errs) > 0 {
		return false, js.Value{}, errs, nil
	}
	return true, data, nil, nil
}

// GuestEducationErrors applies the same rule to the education a guest sends
// with a registration (guest_data is otherwise free-form).
func GuestEducationErrors(guest js.Value) []SubmissionError {
	var errs []SubmissionError
	for _, field := range []struct{ key, label string }{
		{"education_history", "riwayat pendidikan"},
		{"current_education", "pendidikan saat ini"},
	} {
		if value, ok := guest.Get(field.key); ok && lacksInstitution(field.key, value) {
			key := field.key
			errs = append(errs, SubmissionError{Field: &key, Message: "Institusi pada " + field.label + " wajib diisi."})
		}
	}
	return errs
}

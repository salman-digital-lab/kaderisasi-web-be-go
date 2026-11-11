package webapi

import (
	"context"
	"strconv"

	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/forms"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/vine"
)

// The retired web-be stored any text as an education institution. Here it
// must be a name from the universities list, or one the member's profile
// already holds (so older profiles stay editable). SMA/SMK entries are exempt.

// knownInstitutions returns which of names are acceptable for the member.
func (s *Server) knownInstitutions(ctx context.Context, names []string, stored []byte) (map[string]bool, error) {
	known := member.StoredInstitutions(stored)
	if len(names) == 0 {
		return known, nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT name FROM universities WHERE name = ANY($1::text[])`, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		known[name] = true
	}
	return known, rows.Err()
}

// unlistedProfileInstitutions checks a validated profile payload; a failure
// is a Vine-style issue on education_history.N.institution.
func (s *Server) unlistedProfileInstitutions(ctx context.Context, payload js.Value, stored []byte) error {
	history, _ := payload.Get("education_history")
	known, err := s.knownInstitutions(ctx, forms.ListedInstitutions(history), stored)
	if err != nil {
		return err
	}
	var issues []vine.Issue
	for i, entry := range history.Items {
		for _, name := range forms.ListedInstitutions(entry) {
			if !known[name] {
				issues = append(issues, vine.Issue{Message: "The selected institution is not in the universities list", Rule: "database.exists",
					Field: "education_history." + strconv.Itoa(i) + ".institution"})
			}
		}
	}
	if len(issues) > 0 {
		return &vine.Error{Issues: issues}
	}
	return nil
}

// unlistedAnswerInstitutions checks the education answers of a validated
// form submission, plus the guest data sent with it.
func (s *Server) unlistedAnswerInstitutions(ctx context.Context, schema, data, guest js.Value, userID *int32) ([]forms.SubmissionError, error) {
	names := forms.SubmissionInstitutions(schema, data, guest)
	if len(names) == 0 {
		return nil, nil
	}
	var stored []byte
	if userID != nil {
		profile, err := member.ProfileFor(ctx, dbgen.New(s.Pool), *userID)
		if err != nil {
			return nil, err
		}
		if profile != nil {
			stored = profile.EducationHistory
		}
	}
	known, err := s.knownInstitutions(ctx, names, stored)
	if err != nil {
		return nil, err
	}
	return forms.UnlistedInstitutionErrors(schema, data, guest, known), nil
}

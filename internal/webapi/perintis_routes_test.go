package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPerintisRoutesRequireLearner(t *testing.T) {
	_, h := newTestServer(t)
	for _, route := range [][2]string{
		{"GET", "/v2/perintis/me"},
		{"GET", "/v2/perintis/me/profile"},
		{"GET", "/v2/perintis/mentees/3/profile"},
		{"GET", "/v2/perintis/me/journals"},
		{"GET", "/v2/perintis/me/journals/2026-12-01"},
		{"PUT", "/v2/perintis/me/journals/2026-12-01"},
		{"POST", "/v2/perintis/me/journals/2026-12-01/files/s3b"},
		{"DELETE", "/v2/perintis/me/journals/2026-12-01/files/perintis%2Fjournals%2F1%2Fa.jpg"},
		{"POST", "/v2/perintis/me/journals/2026-12-01/submit"},
		{"POST", "/v2/perintis/me/journals/2026-12-01/response/read"},
		{"GET", "/v2/perintis/mentor/dashboard"},
		{"GET", "/v2/perintis/journals/7"},
		{"PUT", "/v2/perintis/journals/7/response"},
		{"PUT", "/v2/perintis/journals/7/flag"},
		{"GET", "/v2/perintis/reports/me"},
		{"PUT", "/v2/perintis/reports/me/2026-12-01"},
		{"POST", "/v2/perintis/reports/me/2026-12-01/submit"},
		{"POST", "/v2/perintis/reports/me/2026-12-01/review/read"},
		{"GET", "/v2/perintis/reports/9/2026-12-01"},
		{"POST", "/v2/perintis/uploads"},
		{"GET", "/v2/perintis/files?key=perintis/reports/9/a.jpg"},
		{"GET", "/v2/perintis/coordinator/overview"},
		{"POST", "/v2/perintis/reports/4/review"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(route[0], route[1], nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d, want 401", route[0], route[1], rec.Code)
		}
	}
}

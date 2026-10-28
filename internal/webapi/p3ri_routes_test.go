package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestP3riRoutesRequireLearner(t *testing.T) {
	_, h := newTestServer(t)
	for _, route := range [][2]string{
		{"GET", "/v2/p3ri/registration"},
		{"PUT", "/v2/p3ri/registration"},
		{"POST", "/v2/p3ri/uploads"},
		{"GET", "/v2/p3ri/files?key=p3ri/1448h/42/a.jpg"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(route[0], route[1], nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d, want 401", route[0], route[1], rec.Code)
		}
	}
}

func TestP3riFormIsPublic(t *testing.T) {
	_, h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v2/p3ri/form", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Message string `json:"message"`
		Data    struct {
			Code      string `json:"code"`
			IsOpen    bool   `json:"is_open"`
			Divisions []any  `json:"divisions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// The test clock (2023) is before the 1448 H registration window.
	if body.Message != "OK" || body.Data.Code != "1448h" || body.Data.IsOpen || len(body.Data.Divisions) != 42 {
		t.Fatalf("unexpected form %+v", body)
	}
}

func TestP3riClosedRegistrationRejectsWrites(t *testing.T) {
	_, h := newTestServer(t)
	for _, route := range [][2]string{{"PUT", "/v2/p3ri/registration"}, {"POST", "/v2/p3ri/uploads"}} {
		req := httptest.NewRequest(route[0], route[1], strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+learnerToken(t, 42))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Pendaftaran sudah ditutup.") {
			t.Errorf("%s %s: got %d %s, want 409", route[0], route[1], rec.Code, rec.Body.String())
		}
	}
}

func TestP3riFilesOnlyServeOwnUploads(t *testing.T) {
	_, h := newTestServer(t)
	req := httptest.NewRequest("GET", "/v2/p3ri/files?key=p3ri/1448h/7/a.jpg", nil)
	req.Header.Set("Authorization", "Bearer "+learnerToken(t, 42))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %s, want 404", rec.Code, rec.Body.String())
	}
}

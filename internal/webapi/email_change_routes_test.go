package webapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"kaderisasi/web/internal/config"
)

func TestEmailChangeURLUsesResetHost(t *testing.T) {
	for base, want := range map[string]string{
		"kaderisasi.salmanitb.com/reset":         "https://kaderisasi.salmanitb.com/confirm-email?token=a%2Bb",
		"localhost:3000/reset-password":          "http://localhost:3000/confirm-email?token=a%2Bb",
		"https://staging.example.test/reset?x=1": "https://staging.example.test/confirm-email?token=a%2Bb",
		"http://127.0.0.1:3000":                  "http://127.0.0.1:3000/confirm-email?token=a%2Bb",
	} {
		s := &Server{Config: config.Config{ResetPasswordURL: base}}
		if got := s.emailChangeURL("a+b"); got != want {
			t.Errorf("%s: got %s want %s", base, got, want)
		}
	}
}

func TestEmailChangeRoutesCheckInputBeforeTheDatabase(t *testing.T) {
	_, h := newTestServer(t)
	token := learnerToken(t, 42)
	for _, c := range []struct{ method, path, body, want string }{
		{"POST", "/v2/profiles/email-change", `{"email":"bukan email"}`, `{"message":"INVALID_EMAIL"}`},
		{"POST", "/v2/profiles/email-change", `{"email":"baru@example.test","password":12}`, `{"message":"INVALID_EMAIL"}`},
		{"POST", "/v2/profiles/email-change/verify-code", `{"code":"12ab56"}`, `{"message":"INVALID_CODE"}`},
		{"POST", "/v2/profiles/email-change/verify-code", `{}`, `{"message":"INVALID_CODE"}`},
		{"PUT", "/v2/auth/confirm-email-change", ``, `{"message":"INVALID_TOKEN"}`},
		{"PUT", "/v2/auth/confirm-email-change?token=a&token=b", ``, `{"message":"INVALID_TOKEN"}`},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 400 || strings.TrimSpace(rec.Body.String()) != c.want || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s %s: %d %s", c.method, c.path, c.body, rec.Code, rec.Body.String())
		}
	}
}

func TestEmailChangeRoutesNeedALearner(t *testing.T) {
	_, h := newTestServer(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/v2/profiles/email-change"}, {"POST", "/v2/profiles/email-change"},
		{"DELETE", "/v2/profiles/email-change"}, {"POST", "/v2/profiles/email-change/verify-code"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(`{}`)))
		if rec.Code != 401 {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
}

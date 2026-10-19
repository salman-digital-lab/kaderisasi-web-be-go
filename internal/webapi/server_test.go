package webapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/dbgen"
)

const testKey = "synthetic-test-key-never-use-outside-tests"

var testNow = time.Unix(1700000100, 0)

type users map[int32]dbgen.PublicUser

func (u users) PublicUserByID(_ context.Context, id int32) (dbgen.PublicUser, error) {
	user, ok := u[id]
	if !ok {
		return dbgen.PublicUser{}, pgx.ErrNoRows
	}
	return user, nil
}

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s := &Server{Config: config.Config{AppKey: testKey}, Users: users{42: {ID: 42, AccountStatus: "active"}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return testNow }}
	h := s.Handler()
	learner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l, ok := LearnerFrom(r.Context())
		writeJSON(w, 200, map[string]any{"authenticated": ok, "id": l.User.ID})
	})
	s.mux.Handle("GET /v2/test/auth", s.RequireAuth(learner))
	s.mux.Handle("GET /v2/test/silent", s.SilentAuth(learner))
	s.mux.Handle("GET /v2/test/course", s.RequireAuth(s.CourseAudience(learner)))
	return s, h
}

func learnerToken(t *testing.T, id int32) string {
	token, err := auth.SignPublicAccess(testKey, id, nil, testNow.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestHealthAndRequestIDAreOwned(t *testing.T) {
	_, h := newTestServer(t)
	for _, id := range []string{"", "bad id with spaces", strings.Repeat("a", 129)} {
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("X-Request-Id", id)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
			t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Request-Id"); len(got) != 36 {
			t.Fatalf("invalid request id %q was not replaced: %q", id, got)
		}
	}
}

func TestPreflightMatchesAdonisCors(t *testing.T) {
	_, h := newTestServer(t)
	req := httptest.NewRequest("OPTIONS", "/v2/profiles", nil)
	req.Header.Set("Origin", "https://web.example")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	want := map[string]string{"Access-Control-Allow-Origin": "https://web.example", "Access-Control-Allow-Credentials": "true",
		"Access-Control-Allow-Methods": "GET,HEAD,POST,PUT,DELETE", "Access-Control-Allow-Headers": "authorization, content-type", "Access-Control-Max-Age": "90"}
	if rec.Code != 204 {
		t.Fatalf("status %d", rec.Code)
	}
	for k, v := range want {
		if rec.Header().Get(k) != v {
			t.Errorf("%s = %q, want %q", k, rec.Header().Get(k), v)
		}
	}
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed method preflight: %d %v", rec.Code, rec.Header())
	}
	get := httptest.NewRequest("GET", "/health", nil)
	get.Header.Set("Origin", "https://web.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, get)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://web.example" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("owned response CORS headers: %v", rec.Header())
	}
}

func TestUnknownRoutesAnswerRouteNotFound(t *testing.T) {
	_, h := newTestServer(t)
	for _, c := range []struct{ method, path string }{{"GET", "/v2/unknown"}, {"DELETE", "/v2/courses"}, {"POST", "/"}, {"GET", "/v2//activities"}, {"GET", "/v2/./x"}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		want := `{"message":"Cannot ` + c.method + `:` + c.path + `","name":"Exception","status":404}`
		if rec.Code != 404 || rec.Body.String() != want {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
}

func TestLearnerAuthentication(t *testing.T) {
	_, h := newTestServer(t)
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "exp": testNow.Unix() + 60}).SignedString([]byte(testKey))
	noExpiry, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": auth.PublicAudience}).SignedString([]byte(testKey))
	admin, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": "kaderisasi-admin", "exp": testNow.Unix() + 900}).SignedString([]byte(testKey))
	call := func(path, header, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if cookie != "" {
			req.Header.Set("Cookie", "token="+cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	valid := learnerToken(t, 42)
	for name, c := range map[string]struct {
		path, header, cookie string
		status               int
		body                 string
	}{
		"bearer":                 {"/v2/test/auth", "Bearer " + valid, "", 200, `{"authenticated":true,"id":42}`},
		"cookie":                 {"/v2/test/auth", "", valid, 200, `{"authenticated":true,"id":42}`},
		"legacy token":           {"/v2/test/auth", "Bearer " + legacy, "", 200, `{"authenticated":true,"id":42}`},
		"missing":                {"/v2/test/auth", "", "", 401, "Unauthorized access"},
		"unknown member":         {"/v2/test/auth", "Bearer " + learnerToken(t, 7), "", 401, "Unauthorized access"},
		"administrator token":    {"/v2/test/auth", "Bearer " + admin, "", 401, "Unauthorized access"},
		"silent anonymous":       {"/v2/test/silent", "Bearer junk", "", 200, `{"authenticated":false,"id":0}`},
		"silent member":          {"/v2/test/silent", "Bearer " + valid, "", 200, `{"authenticated":true,"id":42}`},
		"course":                 {"/v2/test/course", "Bearer " + valid, "", 200, `{"authenticated":true,"id":42}`},
		"course legacy token":    {"/v2/test/course", "Bearer " + legacy, "", 401, `{"message":"UNAUTHORIZED"}`},
		"course without expiry":  {"/v2/test/course", "Bearer " + noExpiry, "", 401, `{"message":"UNAUTHORIZED"}`},
		"course cookie only":     {"/v2/test/course", "", valid, 401, `{"message":"UNAUTHORIZED"}`},
		"course no learner auth": {"/v2/test/course", "", "", 401, "Unauthorized access"},
	} {
		rec := call(c.path, c.header, c.cookie)
		if rec.Code != c.status || strings.TrimSpace(rec.Body.String()) != c.body {
			t.Errorf("%s: %d %q, want %d %q", name, rec.Code, rec.Body.String(), c.status, c.body)
		}
		if c.status == 401 && c.body == "Unauthorized access" && rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("%s: content type %q", name, rec.Header().Get("Content-Type"))
		}
		if strings.HasPrefix(c.path, "/v2/test/course") && c.body != "Unauthorized access" && rec.Header().Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: cache control %q", name, rec.Header().Get("Cache-Control"))
		}
	}
}

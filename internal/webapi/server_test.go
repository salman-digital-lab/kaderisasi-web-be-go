package webapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type seen struct {
	method, path, query, body, auth, cookie, requestID, forwardedFor string
}

func newTestServer(t *testing.T, upstream http.Handler) (*Server, http.Handler) {
	t.Helper()
	legacy := httptest.NewServer(upstream)
	t.Cleanup(legacy.Close)
	target, _ := url.Parse(legacy.URL)
	s := &Server{Config: config.Config{AppKey: testKey, LegacyURL: target}, Users: users{42: {ID: 42, AccountStatus: "active"}},
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

func TestProxyForwardsUnownedRoutesUnchanged(t *testing.T) {
	var got seen
	_, h := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = seen{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Get("Authorization"), r.Header.Get("Cookie"), r.Header.Get("X-Request-Id"), r.Header.Get("X-Forwarded-For")}
		w.Header().Set("X-Request-Id", r.Header.Get("X-Request-Id"))
		w.Header().Set("Access-Control-Allow-Origin", "https://web.example")
		w.Header().Set("Set-Cookie", "a=b; Path=/")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"errors":[{"message":"invalid"}]}`))
	}))
	req := httptest.NewRequest("PUT", "/v2/activities/abc/registration?x=1&y=%20", strings.NewReader(`{"a":1}`))
	req.RemoteAddr = "10.0.0.9:5555"
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Cookie", "token=t")
	req.Header.Set("X-Request-Id", "req-1")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("Origin", "https://web.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	want := seen{"PUT", "/v2/activities/abc/registration", "x=1&y=%20", `{"a":1}`, "Bearer abc", "token=t", "req-1", "203.0.113.7, 10.0.0.9"}
	if got != want {
		t.Fatalf("upstream saw %+v, want %+v", got, want)
	}
	if rec.Code != 422 || rec.Body.String() != `{"errors":[{"message":"invalid"}]}` || rec.Header().Get("Set-Cookie") != "a=b; Path=/" {
		t.Fatalf("response not passed through: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if ids := rec.Header().Values("X-Request-Id"); len(ids) != 1 || ids[0] != "req-1" {
		t.Fatalf("request id header %v", ids)
	}
	if origins := rec.Header().Values("Access-Control-Allow-Origin"); len(origins) != 1 {
		t.Fatalf("CORS origin duplicated on proxied response: %v", origins)
	}
}

func TestHealthAndRequestIDAreOwned(t *testing.T) {
	_, h := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("health proxied") }))
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
	_, h := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("preflight proxied") }))
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

func TestLegacyUnavailable(t *testing.T) {
	s, _ := newTestServer(t, http.NotFoundHandler())
	s.Config.LegacyURL, _ = url.Parse("http://127.0.0.1:1")
	h := s.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v2/club-registrations/my-registrations", nil))
	if rec.Code != 502 || strings.TrimSpace(rec.Body.String()) != `{"message":"LEGACY_API_UNAVAILABLE"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLearnerAuthentication(t *testing.T) {
	_, h := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("test route proxied") }))
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

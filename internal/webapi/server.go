// Package webapi is the public member API that replaced the AdonisJS
// kaderisasi-web-be service, keeping its HTTP contract (docs/MIGRATION.md).
package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/google"
	"kaderisasi/web/internal/mail"
	"kaderisasi/web/internal/storage"
)

// UserStore loads the learner a token names, like the Adonis session user
// provider (`PublicUser.find`).
type UserStore interface {
	PublicUserByID(ctx context.Context, id int32) (dbgen.PublicUser, error)
}

type Server struct {
	Config  config.Config
	Users   UserStore
	Pool    *pgxpool.Pool
	Mail    mail.Sender
	Storage storage.Store
	// Google is nil when Google sign-in is not configured.
	Google *google.Client
	Logger *slog.Logger
	// Now is the clock used for token checks; nil means time.Now.
	Now func() time.Time

	mux      *http.ServeMux
	throttle *certificateThrottle
}

func (s *Server) Handler() http.Handler {
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{"ok"})
	})
	// Route groups ported from web-be (see docs/MIGRATION.md).
	s.registerAuth()
	s.throttle = newCertificateThrottle(s.now)
	s.registerCatalog()
	s.registerProfiles()
	s.registerMember()
	s.registerRegistrations()
	s.registerForms()
	s.registerCourses()
	s.mux.HandleFunc("/", routeNotFound)
	return s.observe(s.cors(adonisPaths(s.mux)))
}

// rawPath is the request target's path exactly as the client sent it.
func rawPath(r *http.Request) string {
	path := r.RequestURI
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return path
}

// routeNotFound is Adonis's E_ROUTE_NOT_FOUND for any method and path no
// route matches (debug stack frames are not reproduced).
func routeNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, struct {
		Message string `json:"message"`
		Name    string `json:"name"`
		Status  int    `json:"status"`
	}{"Cannot " + r.Method + ":" + rawPath(r), "Exception", http.StatusNotFound})
}

// adonisPaths matches paths the way the Adonis router does: one trailing
// slash is ignored and nothing is cleaned, so an empty, "." or ".." segment
// can still fill a route parameter but never a literal segment. net/http's
// mux would clean and redirect such paths, so those segments are handed to
// it with a prefix only wildcards match; handlers read parameters from the
// raw request target (rawSegment).
func adonisPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			path = path[:len(path)-1]
		}
		if path != "/" {
			segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
			for i, segment := range segments {
				if segment == "" || segment == "." || segment == ".." {
					segments[i] = "\x01" + segment
				}
			}
			path = "/" + strings.Join(segments, "/")
		}
		if path != r.URL.Path {
			u := *r.URL
			u.Path, u.RawPath = path, ""
			r2 := r.Clone(r.Context())
			r2.URL = &u
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// writeJSON matches Adonis's response.send for objects: JSON.stringify output
// (no HTML escaping, no trailing newline).
func writeJSON(w http.ResponseWriter, status int, data any) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"GENERAL_ERROR"}`))
		return
	}
	body := rawLineSeparators(bytes.TrimRight(b.Bytes(), "\n"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Like Adonis, send the length instead of a chunked body (net/http only
	// does so by itself for small bodies).
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// rawLineSeparators undoes encoding/json's \u2028 and \u2029 escapes, which
// JSON.stringify does not produce.
func rawLineSeparators(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u202`)) {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		if b[i+1] == 'u' && i+5 < len(b) && (string(b[i+2:i+6]) == "2028" || string(b[i+2:i+6]) == "2029") {
			if b[i+5] == '8' {
				out = append(out, "\u2028"...)
			} else {
				out = append(out, "\u2029"...)
			}
			i += 5
			continue
		}
		out = append(out, b[i], b[i+1])
		i++
	}
	return out
}

type message struct {
	Message string `json:"message"`
}

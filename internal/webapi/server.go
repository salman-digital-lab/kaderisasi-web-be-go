// Package webapi is the Go replacement for the AdonisJS public member API
// (kaderisasi-web-be). Routes move here group by group; every request for a
// route this package does not own yet is reverse-proxied unchanged to the
// Adonis service at WEB_BE_LEGACY_URL. See docs/WEB_API.md.
package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/google"
	"kaderisasi/web/internal/mail"
)

// UserStore loads the learner a token names, like the Adonis session user
// provider (`PublicUser.find`).
type UserStore interface {
	PublicUserByID(ctx context.Context, id int32) (dbgen.PublicUser, error)
}

type Server struct {
	Config config.Config
	Users  UserStore
	Pool   *pgxpool.Pool
	Mail   mail.Sender
	// Google is nil when Google sign-in is not configured.
	Google *google.Client
	Logger *slog.Logger
	// Now is the clock used for token checks; nil means time.Now.
	Now func() time.Time

	mux   *http.ServeMux
	proxy *httputil.ReverseProxy
}

func (s *Server) Handler() http.Handler {
	s.mux = http.NewServeMux()
	s.proxy = newLegacyProxy(s.Config.LegacyURL, s.logger())
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{"ok"})
	})
	// Route groups ported from web-be (see docs/MIGRATION.md).
	s.registerAuth()
	s.mux.Handle("/", s.proxy)
	return s.observe(s.cors(s.mux))
}

// Owned reports whether the Go service answers the request itself rather than
// proxying it to Adonis.
func (s *Server) Owned(r *http.Request) bool {
	_, pattern := s.mux.Handler(r)
	return pattern != "/"
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(bytes.TrimRight(b.Bytes(), "\n"))
}

type message struct {
	Message string `json:"message"`
}

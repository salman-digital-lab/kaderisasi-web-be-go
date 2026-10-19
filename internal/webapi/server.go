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
	proxy    *httputil.ReverseProxy
	throttle *certificateThrottle
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
	s.throttle = newCertificateThrottle(s.now)
	s.registerCatalog()
	s.registerProfiles()
	s.registerMember()
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
	_, _ = w.Write(rawLineSeparators(bytes.TrimRight(b.Bytes(), "\n")))
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

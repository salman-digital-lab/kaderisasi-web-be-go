package webapi

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type requestIDKey struct{}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// requestID matches web-be's request_id_middleware: a valid caller-supplied
// X-Request-Id (at most 128 characters) is kept, otherwise a UUID is issued.
// The value is forwarded to Adonis, which then reuses it.
func requestID(r *http.Request) string {
	if supplied := strings.TrimSpace(r.Header.Get("X-Request-Id")); supplied != "" && len(supplied) <= 128 && requestIDPattern.MatchString(supplied) {
		return supplied
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type observation struct {
	status  int
	actorID int32
}

type recorder struct {
	http.ResponseWriter
	observed *observation
}

func (w *recorder) WriteHeader(status int) {
	if w.observed.status == 0 {
		w.observed.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *recorder) Write(body []byte) (int, error) {
	if w.observed.status == 0 {
		w.observed.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController flush streamed proxy responses.
func (w *recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// observe assigns the request id, recovers panics and logs rejected or failed
// requests the way web-be's logger_middleware does.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := requestID(r)
		r.Header.Set("X-Request-Id", id)
		w.Header().Set("X-Request-Id", id)
		observed := &observation{}
		r = r.WithContext(contextWithObservation(r.Context(), observed))
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				s.logger().Error("request panic", "request_id", id, "path", r.URL.Path, "panic", fmt.Sprint(recovered))
				if observed.status == 0 {
					writeJSON(&recorder{w, observed}, http.StatusInternalServerError, message{"GENERAL_ERROR"})
				}
				observed.status = http.StatusInternalServerError
			}
			status := observed.status
			if status == 0 {
				status = http.StatusOK
			}
			attributes := []any{"request_id", id, "method", r.Method, "path", r.URL.RequestURI(), "status", status,
				"duration_ms", float64(time.Since(started).Microseconds()) / 1000}
			if observed.actorID != 0 {
				attributes = append(attributes, "actor_user_id", observed.actorID)
			}
			switch {
			case status >= 500:
				s.logger().Error("HTTP request failed", attributes...)
			case status >= 400:
				s.logger().Warn("HTTP request rejected", attributes...)
			default:
				s.logger().Debug("HTTP request completed", attributes...)
			}
		}()
		next.ServeHTTP(&recorder{w, observed}, r)
	})
}

func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Origin") != ""
}

// corsMethods and corsMaxAge mirror kaderisasi-web-be/config/cors.ts
// (origin '*', credentials, any requested header, no extra exposed headers).
var corsMethods = []string{"GET", "HEAD", "POST", "PUT", "DELETE"}

const corsMaxAge = "90"

// cors reproduces @adonisjs/cors 2.2 for that config. Preflights are answered
// here for every path.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			next.ServeHTTP(w, r)
			return
		}
		method := r.Header.Get("Access-Control-Request-Method")
		allowed := false
		for _, m := range corsMethods {
			allowed = allowed || m == method
		}
		if allowed {
			// Adonis splits on "," without trimming and echoes the lower-cased
			// list, so the header comes back exactly as requested, lower-cased.
			headers := strings.ToLower(r.Header.Get("Access-Control-Request-Headers"))
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(corsMethods, ","))
			w.Header().Set("Access-Control-Allow-Headers", headers)
			w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

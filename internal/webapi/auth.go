package webapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/dbgen"
)

type observationKey struct{}
type learnerKey struct{}

// Learner is the authenticated member for a request.
type Learner struct {
	User   dbgen.PublicUser
	Claims auth.PublicClaims
	Token  string
}

func contextWithObservation(ctx context.Context, o *observation) context.Context {
	return context.WithValue(ctx, observationKey{}, o)
}

// LearnerFrom returns the member set by RequireAuth or SilentAuth.
func LearnerFrom(ctx context.Context) (Learner, bool) {
	learner, ok := ctx.Value(learnerKey{}).(Learner)
	return learner, ok
}

var errUnauthorized = errors.New("unauthorized access")

// tokenFrom follows the Adonis JWT guard: a `token` cookie wins when the
// request has cookies, otherwise the text after "Bearer " in Authorization.
func tokenFrom(r *http.Request) string {
	if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	_, token, _ := strings.Cut(r.Header.Get("Authorization"), "Bearer ")
	return token
}

func (s *Server) authenticate(r *http.Request) (Learner, error) {
	token := tokenFrom(r)
	if token == "" {
		return Learner{}, errUnauthorized
	}
	claims, err := auth.VerifyPublicAccess(s.Config.AppKey, token, s.now())
	if err != nil {
		return Learner{}, errUnauthorized
	}
	user, err := s.Users.PublicUserByID(r.Context(), claims.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Learner{}, errUnauthorized
	}
	if err != nil {
		return Learner{}, err
	}
	return Learner{User: user, Claims: claims, Token: token}, nil
}

func (s *Server) withLearner(r *http.Request, learner Learner) *http.Request {
	if o, ok := r.Context().Value(observationKey{}).(*observation); ok {
		o.actorID = learner.User.ID
	}
	return r.WithContext(context.WithValue(r.Context(), learnerKey{}, learner))
}

// RequireAuth is web-be's `auth` middleware. Adonis renders the JWT guard's
// E_UNAUTHORIZED_ACCESS as a plain-text 401 because the guard has no JSON
// renderer.
func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		learner, err := s.authenticate(r)
		if errors.Is(err, errUnauthorized) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized access"))
			return
		}
		if err != nil {
			s.logger().Error("learner lookup failed", "error", err.Error())
			writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
			return
		}
		next.ServeHTTP(w, s.withLearner(r, learner))
	})
}

// SilentAuth is web-be's `silentAuth`: it attaches the member when the token
// is valid and continues anonymously otherwise.
func (s *Server) SilentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if learner, err := s.authenticate(r); err == nil {
			r = s.withLearner(r, learner)
		}
		next.ServeHTTP(w, r)
	})
}

// CourseAudience is web-be's course_audience_middleware, applied after
// RequireAuth: the Authorization bearer token itself must carry the learner
// audience and an expiry, and must name the authenticated member.
func (s *Server) CourseAudience(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		learner, ok := LearnerFrom(r.Context())
		header := r.Header.Get("Authorization")
		if ok && strings.HasPrefix(header, "Bearer ") {
			claims, err := auth.VerifyPublicAccess(s.Config.AppKey, header[len("Bearer "):], s.now())
			if err == nil && claims.HasAudience && claims.HasExpiry && claims.UserID == learner.User.ID {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, message{"UNAUTHORIZED"})
	})
}

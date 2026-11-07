package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"kaderisasi/web/internal/emailchange"
	"kaderisasi/web/internal/mail"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/vine"
)

// Self-service email change. New in the Go API (no web-be equivalent); see
// docs/EMAIL_CHANGE.md.
func (s *Server) registerEmailChange() {
	auth := func(h http.HandlerFunc) http.Handler { return s.RequireAuth(noStore(h)) }
	s.mux.Handle("GET /v2/profiles/email-change", auth(s.pendingEmailChange))
	s.mux.Handle("POST /v2/profiles/email-change", auth(s.startEmailChange))
	s.mux.Handle("DELETE /v2/profiles/email-change", auth(s.cancelEmailChange))
	s.mux.Handle("POST /v2/profiles/email-change/verify-code", auth(s.verifyEmailChangeCode))
	s.mux.Handle("PUT /v2/auth/confirm-email-change", noStore(s.confirmEmailChange))
}

func noStore(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	}
}

func (s *Server) emailChanges() emailchange.Service {
	return emailchange.Service{Pool: s.Pool, Key: s.Config.AppKey, Now: s.Now}
}

func isoTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }

type emailChangeStep struct {
	Step      string `json:"step"`
	Email     string `json:"email"`
	ExpiresAt string `json:"expires_at"`
}

// emailChangeFailure answers a refusal as {message: CODE}, plus the attempts
// left for a wrong code; anything else is GENERAL_ERROR.
func (s *Server) emailChangeFailure(w http.ResponseWriter, err error) {
	var refusal *emailchange.Error
	if !errors.As(err, &refusal) {
		s.logger().Error("email change failed", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
		return
	}
	if refusal.Code == "INVALID_CODE" {
		writeJSON(w, refusal.Status, member.KV("message", refusal.Code, "data", member.KV("remaining_attempts", refusal.Remaining)))
		return
	}
	writeJSON(w, refusal.Status, message{refusal.Code})
}

// optionalString reads a JSON string field; ok is false for any other type.
func optionalString(input vine.Input, name string) (value *string, ok bool) {
	raw, present := input[name]
	if !present || string(raw) == "null" {
		return nil, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return nil, false
	}
	return &text, true
}

func (s *Server) pendingEmailChange(w http.ResponseWriter, r *http.Request) {
	user := learner(r)
	pending, err := s.emailChanges().PendingFor(r.Context(), user.ID)
	if err != nil {
		s.emailChangeFailure(w, err)
		return
	}
	var step any
	if pending != nil {
		step = emailChangeStep{pending.Step, pending.NewEmail, isoTime(pending.ExpiresAt)}
	}
	// password_required tells the form whether to ask for the password
	// (otherwise a code is sent to the current email).
	writeJSON(w, http.StatusOK, member.KV("message", "GET_DATA_SUCCESS", "data", member.KV(
		"password_required", user.Password != nil && *user.Password != "", "pending", step)))
}

func (s *Server) startEmailChange(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, vine.Field{Name: "email", Rule: emailRule})
	password, passwordOK := optionalString(input, "password")
	if err != nil || !passwordOK || len(v["email"]) > 255 {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_EMAIL"})
		return
	}
	user := learner(r)
	started, err := s.emailChanges().Start(r.Context(), user.ID, v["email"], password)
	if err != nil {
		s.emailChangeFailure(w, err)
		return
	}
	s.sendEmailChangeStep(w, r, user.ID, started)
}

func (s *Server) verifyEmailChangeCode(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	code, codeOK := optionalString(input, "code")
	if !codeOK || code == nil || !sixDigits.MatchString(strings.TrimSpace(*code)) {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_CODE"})
		return
	}
	user := learner(r)
	started, err := s.emailChanges().VerifyCode(r.Context(), user.ID, *code)
	if err != nil {
		s.emailChangeFailure(w, err)
		return
	}
	s.sendEmailChangeStep(w, r, user.ID, started)
}

var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

// sendEmailChangeStep emails the code (to the current email) or the
// confirmation link (to the new email). When sending fails the request is
// withdrawn so the member can try again at once.
func (s *Server) sendEmailChangeStep(w http.ResponseWriter, r *http.Request, userID int32, started emailchange.Started) {
	m := mail.Message{FromAddress: s.Config.MailFrom, FromName: "Kaderisasi Masjid Salman ITB"}
	result := "EMAIL_CHANGE_LINK_SENT"
	if started.Step == emailchange.StepCode {
		result = "EMAIL_CHANGE_CODE_SENT"
		m.To, m.Subject, m.HTML = started.OldEmail, "Kode perubahan email akun Kaderisasi Masjid Salman ITB", mail.EmailChangeCodeHTML(started.Code, started.NewEmail)
	} else {
		m.To, m.Subject, m.HTML = started.NewEmail, "Konfirmasi email baru akun Kaderisasi Masjid Salman ITB", mail.EmailChangeConfirmHTML(s.emailChangeURL(started.Token))
	}
	if err := s.Mail.Send(r.Context(), m); err != nil {
		s.logger().Error("email change email failed", "step", started.Step, "error", err.Error())
		if err := s.emailChanges().Cancel(r.Context(), userID); err != nil {
			s.logger().Error("email change cancel failed", "error", err.Error())
		}
		writeJSON(w, http.StatusInternalServerError, message{"SEND_EMAIL_FAILED"})
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", result, "data", emailChangeStep{started.Step, started.NewEmail, isoTime(started.ExpiresAt)}))
}

func (s *Server) cancelEmailChange(w http.ResponseWriter, r *http.Request) {
	if err := s.emailChanges().Cancel(r.Context(), learner(r).ID); err != nil {
		s.emailChangeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, message{"EMAIL_CHANGE_CANCELLED"})
}

func (s *Server) confirmEmailChange(w http.ResponseWriter, r *http.Request) {
	tokens := r.URL.Query()["token"]
	if len(tokens) != 1 {
		writeJSON(w, http.StatusBadRequest, message{"INVALID_TOKEN"})
		return
	}
	confirmed, err := s.emailChanges().Confirm(r.Context(), tokens[0])
	if err != nil {
		s.emailChangeFailure(w, err)
		return
	}
	// The change is committed. The notice to the old email goes out in the
	// background so a slow mail server does not hold the answer; a failure is
	// only logged.
	notice := mail.Message{FromAddress: s.Config.MailFrom, FromName: "Kaderisasi Masjid Salman ITB", To: confirmed.OldEmail,
		Subject: "Email akun Kaderisasi Masjid Salman ITB telah diubah", HTML: mail.EmailChangedHTML(*confirmed.User.Email)}
	go func(ctx context.Context) {
		// A panic here would stop the whole API, unlike one in a handler.
		defer func() {
			if p := recover(); p != nil {
				s.logger().Error("email changed notice panicked", "panic", fmt.Sprint(p))
			}
		}()
		if s.Mail == nil {
			return
		}
		if err := s.Mail.Send(ctx, notice); err != nil {
			s.logger().Error("email changed notice failed", "error", err.Error())
		}
	}(context.WithoutCancel(r.Context()))
	userJSON := member.User(confirmed.User, s.Config.Location)
	if err := s.sessionSuccess(w, r, "EMAIL_CHANGE_SUCCESS", confirmed.User, userJSON); err != nil {
		// Still a success: the member signs in again with the new email.
		s.logger().Error("email change session failed", "error", err.Error())
		writeJSON(w, http.StatusOK, member.KV("message", "EMAIL_CHANGE_SUCCESS", "data", member.KV("user", userJSON)))
	}
}

// emailChangeURL is the web frontend's /confirm-email page on the host of
// RESET_PASSWORD_URL. html/template only keeps http(s) links, so a value
// without a scheme gets one (http for localhost).
func (s *Server) emailChangeURL(token string) string {
	base := s.Config.ResetPasswordURL
	scheme := "https://"
	if i := strings.Index(base, "://"); i >= 0 {
		scheme, base = base[:i+3], base[i+3:]
	} else if strings.HasPrefix(base, "localhost") || strings.HasPrefix(base, "127.0.0.1") {
		scheme = "http://"
	}
	host, _, _ := strings.Cut(base, "/")
	return scheme + host + "/confirm-email?token=" + url.QueryEscape(token)
}

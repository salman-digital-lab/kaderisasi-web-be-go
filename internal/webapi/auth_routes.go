package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"kaderisasi/web/internal/auth"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/google"
	"kaderisasi/web/internal/mail"
	"kaderisasi/web/internal/member"
	"kaderisasi/web/internal/vine"
)

// Phase 1: kaderisasi-web-be/app/controllers/auth_controller.ts and
// google_auth_controller.ts.
func (s *Server) registerAuth() {
	s.mux.HandleFunc("POST /v2/auth/register", s.register)
	s.mux.HandleFunc("POST /v2/auth/login", s.login)
	s.mux.HandleFunc("POST /v2/auth/google", s.googleLogin)
	s.mux.HandleFunc("POST /v2/auth/check-email", s.checkEmail)
	s.mux.HandleFunc("POST /v2/auth/forgot-password", s.forgotPassword)
	s.mux.HandleFunc("PUT /v2/auth/reset-password", s.resetPassword)
	s.mux.Handle("PUT /v2/auth/logout", s.RequireAuth(http.HandlerFunc(s.logout)))
}

var emailRule = vine.String{Trim: true, Lower: true, Email: true}

// The auth controllers answer every failure with 500 and this envelope:
// Vine errors carry the first message and the list, anything else the error message.
func failure(w http.ResponseWriter, err error) {
	var invalid *vine.Error
	if errors.As(err, &invalid) {
		writeJSON(w, http.StatusInternalServerError, struct {
			Message string       `json:"message"`
			Error   []vine.Issue `json:"error"`
		}{invalid.Issues[0].Message, invalid.Issues})
		return
	}
	writeJSON(w, http.StatusInternalServerError, struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}{"GENERAL_ERROR", err.Error()})
}

// input is request.all(), or a response for a body the bodyparser rejects.
func (s *Server) input(w http.ResponseWriter, r *http.Request) (vine.Input, bool) {
	input, err := all(r)
	var invalid *bodyError
	if errors.As(err, &invalid) {
		// Adonis renders bodyparser exceptions through its error handler:
		// message, name and status (debug builds add stack frames, not reproduced).
		writeJSON(w, invalid.status, struct {
			Message string `json:"message"`
			Name    string `json:"name"`
			Status  int    `json:"status"`
		}{invalid.message, invalid.name, invalid.status})
		return nil, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, message{"GENERAL_ERROR"})
		return nil, false
	}
	return input, true
}

type tokenJSON struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	ExpiresIn string `json:"expiresIn"`
}

// loginSuccess is {message: 'LOGIN_SUCCESS', data: {user, data, token}}.
func (s *Server) loginSuccess(w http.ResponseWriter, r *http.Request, user dbgen.PublicUser, userJSON member.Object) error {
	profile, err := member.ProfileFor(r.Context(), dbgen.New(s.Pool), user.ID)
	if err != nil {
		return err
	}
	token, err := auth.SignPublicAccess(s.Config.AppKey, user.ID, user.Email, s.now())
	if err != nil {
		return err
	}
	var data any
	if profile != nil {
		data = member.Profile(*profile, s.Config.Location)
	}
	writeJSON(w, http.StatusOK, member.KV("message", "LOGIN_SUCCESS", "data", member.KV(
		"user", userJSON, "data", data, "token", tokenJSON{"bearer", token, "1d"})))
	return nil
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, vine.Field{Name: "fullname"}, vine.Field{Name: "email", Rule: emailRule}, vine.Field{Name: "password"})
	if err != nil {
		failure(w, err)
		return
	}
	existing, err := member.UserByEmail(r.Context(), dbgen.New(s.Pool), v["email"])
	if err != nil {
		failure(w, err)
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusConflict, message{"EMAIL_ALREADY_REGISTERED"})
		return
	}
	user, err := member.Register(r.Context(), s.Pool, v["email"], v["password"], v["fullname"])
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member.KV("message", "REGISTER_SUCCESS", "data", member.CreatedUser(user, s.Config.Location)))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, vine.Field{Name: "email", Rule: emailRule}, vine.Field{Name: "password"})
	if err != nil {
		failure(w, err)
		return
	}
	user, err := member.UserByEmail(r.Context(), dbgen.New(s.Pool), v["email"])
	if err != nil {
		failure(w, err)
		return
	}
	if user == nil {
		writeJSON(w, http.StatusNotFound, message{"USER_NOT_FOUND"})
		return
	}
	if user.Password == nil || *user.Password == "" {
		writeJSON(w, http.StatusUnauthorized, message{"WRONG_PASSWORD"})
		return
	}
	if !auth.VerifyPassword(*user.Password, v["password"]) {
		writeJSON(w, http.StatusUnauthorized, message{"WRONG_PASSWORD"})
		return
	}
	if err := s.loginSuccess(w, r, *user, member.User(*user, s.Config.Location)); err != nil {
		failure(w, err)
	}
}

func (s *Server) checkEmail(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, vine.Field{Name: "email", Rule: emailRule})
	if err != nil {
		failure(w, err)
		return
	}
	user, err := member.UserByEmail(r.Context(), dbgen.New(s.Pool), v["email"])
	if err != nil {
		failure(w, err)
		return
	}
	result := "EMAIL_AVAILABLE"
	if user != nil {
		result = "EMAIL_ALREADY_REGISTERED"
	}
	writeJSON(w, http.StatusOK, member.KV("message", result, "data", member.KV("exists", user != nil)))
}

const resetLinkLifetime = 30 * time.Minute

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	// This action reports validation failures like any other error.
	v, err := vine.Validate(input, vine.Field{Name: "email", Rule: emailRule})
	if err != nil {
		failure(w, errors.New(err.Error()))
		return
	}
	user, err := member.UserByEmail(r.Context(), dbgen.New(s.Pool), v["email"])
	if err != nil {
		failure(w, err)
		return
	}
	if user == nil {
		writeJSON(w, http.StatusNotFound, message{"EMAIL_NOT_FOUND"})
		return
	}
	token, err := auth.Encrypt(s.Config.AppKey, v["email"], resetLinkLifetime, s.now())
	if err != nil {
		failure(w, err)
		return
	}
	err = s.Mail.Send(r.Context(), mail.Message{FromAddress: s.Config.MailFrom, FromName: "Kaderisasi Masjid Salman ITB", To: v["email"],
		Subject: "Reset kata sandi akun Kaderisasi Masjid Salman ITB", HTML: mail.ResetPasswordHTML(s.Config.ResetPasswordURL + "?token=" + token)})
	if err != nil {
		s.logger().Error("password recovery email failed", "error", err.Error())
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, message{"SEND_EMAIL_SUCCESS"})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, vine.Field{Name: "password"})
	if err != nil {
		failure(w, err)
		return
	}
	// request.qs().token: a repeated parameter is an array, which never decrypts.
	tokens := r.URL.Query()["token"]
	var user *dbgen.PublicUser
	if len(tokens) == 1 {
		if decrypted, err := auth.Decrypt(s.Config.AppKey, tokens[0], s.now()); err == nil {
			var email string
			if json.Unmarshal(decrypted, &email) != nil {
				failure(w, errors.New("decrypted.trim is not a function"))
				return
			}
			user, err = member.UserByEmail(r.Context(), dbgen.New(s.Pool), strings.ToLower(vine.TrimJS(email)))
			if err != nil {
				failure(w, err)
				return
			}
		}
	}
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, message{"INVALID_TOKEN"})
		return
	}
	hash, err := auth.HashPassword(v["password"])
	if err == nil {
		err = dbgen.New(s.Pool).SetPassword(r.Context(), dbgen.SetPasswordParams{ID: user.ID, Password: &hash, UpdatedAt: member.Timestamp(member.Now())})
	}
	if err != nil {
		failure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, message{"RESET_PASSWORD_SUCCESS"})
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, message{"LOGOUT_SUCCESS"})
}

var googleRules = []vine.Field{{Name: "code", Rule: vine.String{MinLength: 1, MaxLength: 4096}},
	{Name: "codeVerifier", Rule: vine.String{MinLength: 43, MaxLength: 128}}, {Name: "nonce", Rule: vine.String{MinLength: 32, MaxLength: 128}}}

func (s *Server) googleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Google == nil {
		writeJSON(w, http.StatusServiceUnavailable, message{"GOOGLE_LOGIN_UNAVAILABLE"})
		return
	}
	input, ok := s.input(w, r)
	if !ok {
		return
	}
	v, err := vine.Validate(input, googleRules...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, message{"GOOGLE_LOGIN_FAILED"})
		return
	}
	refuse := func(err error) {
		code := "GOOGLE_LOGIN_FAILED"
		var refused *google.LoginError
		if errors.As(err, &refused) {
			code = refused.Code
		} else {
			s.logger().Warn("Google sign-in failed", "error", err.Error())
		}
		writeJSON(w, http.StatusUnauthorized, message{code})
	}
	identity, err := s.Google.VerifyCode(r.Context(), v["code"], v["codeVerifier"], v["nonce"])
	if err != nil {
		refuse(err)
		return
	}
	user, created, err := member.ResolveGoogleAccount(r.Context(), s.Pool, identity)
	if err != nil {
		refuse(err)
		return
	}
	userJSON := member.User(user, s.Config.Location)
	if created {
		userJSON = member.CreatedUser(user, s.Config.Location)
	}
	if err := s.loginSuccess(w, r, user, userJSON); err != nil {
		refuse(err)
	}
}

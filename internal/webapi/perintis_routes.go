package webapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"kaderisasi/web/internal/perintis"
)

// Pembinaan Beasiswa Perintis for participants (mentee, mentor,
// koordinator). New routes, not ported from web-be: responses are
// {"message":"OK","data":…} with the camelCase shapes of
// kaderisasi-perintis-fe, and failures are {"message":<Indonesian text>}.
func (s *Server) registerPerintis() {
	service := &perintis.Service{Pool: s.Pool, Storage: s.Storage, Now: s.perintisClock()}
	auth := func(h func(http.ResponseWriter, *http.Request, int32) (any, error)) http.Handler {
		return s.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			learner, _ := LearnerFrom(r.Context())
			data, err := h(w, r, learner.User.ID)
			if err != nil {
				s.perintisFailure(w, err)
				return
			}
			if data != nil {
				writeJSON(w, http.StatusOK, struct {
					Message string `json:"message"`
					Data    any    `json:"data"`
				}{"OK", data})
			}
		}))
	}
	month := func(r *http.Request) string { return r.PathValue("month") }
	id := func(r *http.Request, name string) (int32, error) {
		n, err := strconv.ParseInt(r.PathValue(name), 10, 32)
		if err != nil || n <= 0 {
			return 0, &perintis.Error{Status: http.StatusNotFound, Message: "Data tidak ditemukan."}
		}
		return int32(n), nil
	}
	ok := struct {
		OK bool `json:"ok"`
	}{true}

	s.mux.Handle("GET /v2/perintis/me", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.Me(r.Context(), user)
	}))
	s.mux.Handle("GET /v2/perintis/me/profile", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.MyProfile(r.Context(), user)
	}))
	s.mux.Handle("GET /v2/perintis/mentees/{menteeId}/profile", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		mentee, err := id(r, "menteeId")
		if err != nil {
			return nil, err
		}
		return service.MenteeProfile(r.Context(), user, mentee)
	}))

	// mentee: lapbul
	s.mux.Handle("GET /v2/perintis/me/journals", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.MyJournals(r.Context(), user)
	}))
	s.mux.Handle("GET /v2/perintis/me/journals/{month}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.MyJournal(r.Context(), user, month(r))
	}))
	s.mux.Handle("PUT /v2/perintis/me/journals/{month}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in perintis.SaveJournalInput
		if err := decodePerintis(r, &in, 256<<10); err != nil {
			return nil, err
		}
		return service.SaveJournal(r.Context(), user, month(r), in)
	}))
	s.mux.Handle("POST /v2/perintis/me/journals/{month}/files/{field}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in perintis.FilePayload
		if err := decodePerintis(r, &in, 8<<20); err != nil {
			return nil, err
		}
		return service.UploadJournalFile(r.Context(), user, month(r), r.PathValue("field"), in)
	}))
	s.mux.Handle("DELETE /v2/perintis/me/journals/{month}/files/{key}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		// ServeMux keeps an encoded %2F inside the segment and decodes it here.
		return service.DeleteJournalFile(r.Context(), user, month(r), r.PathValue("key"))
	}))
	s.mux.Handle("POST /v2/perintis/me/journals/{month}/submit", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.SubmitJournal(r.Context(), user, month(r))
	}))
	s.mux.Handle("POST /v2/perintis/me/journals/{month}/response/read", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return ok, service.ReadResponse(r.Context(), user, month(r))
	}))

	// mentor
	s.mux.Handle("GET /v2/perintis/mentor/dashboard", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.MentorDashboard(r.Context(), user, r.URL.Query().Get("month"))
	}))
	s.mux.Handle("GET /v2/perintis/journals/{id}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		journal, err := id(r, "id")
		if err != nil {
			return nil, err
		}
		return service.MentorJournal(r.Context(), user, journal)
	}))
	s.mux.Handle("PUT /v2/perintis/journals/{id}/response", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		journal, err := id(r, "id")
		if err != nil {
			return nil, err
		}
		var in perintis.RespondInput
		if err := decodePerintis(r, &in, 64<<10); err != nil {
			return nil, err
		}
		return service.Respond(r.Context(), user, journal, in)
	}))
	s.mux.Handle("PUT /v2/perintis/journals/{id}/flag", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		journal, err := id(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			Note string `json:"note"`
		}
		if err := decodePerintis(r, &in, 16<<10); err != nil {
			return nil, err
		}
		return service.HandleFlag(r.Context(), user, journal, in.Note)
	}))

	// laporan mentoring
	s.mux.Handle("GET /v2/perintis/reports/me", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.MyReports(r.Context(), user)
	}))
	s.mux.Handle("PUT /v2/perintis/reports/me/{month}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in perintis.SaveReportInput
		if err := decodePerintis(r, &in, 128<<10); err != nil {
			return nil, err
		}
		return service.SaveReport(r.Context(), user, month(r), in)
	}))
	s.mux.Handle("POST /v2/perintis/reports/me/{month}/submit", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.SubmitReport(r.Context(), user, month(r))
	}))
	s.mux.Handle("POST /v2/perintis/reports/me/{month}/review/read", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return ok, service.ReadReview(r.Context(), user, month(r))
	}))
	s.mux.Handle("GET /v2/perintis/reports/{mentorId}/{month}", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		mentor, err := id(r, "mentorId")
		if err != nil {
			return nil, err
		}
		return service.Report(r.Context(), user, mentor, month(r))
	}))
	s.mux.Handle("POST /v2/perintis/uploads", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in perintis.FilePayload
		if err := decodePerintis(r, &in, 8<<20); err != nil {
			return nil, err
		}
		return service.UploadPhoto(r.Context(), user, in)
	}))
	s.mux.Handle("GET /v2/perintis/files", auth(func(w http.ResponseWriter, r *http.Request, user int32) (any, error) {
		data, mime, err := service.File(r.Context(), user, r.URL.Query().Get("key"))
		if err != nil {
			return nil, err
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "inline")
		_, _ = w.Write(data)
		return nil, nil
	}))

	// koordinator
	s.mux.Handle("GET /v2/perintis/coordinator/overview", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.CoordinatorOverview(r.Context(), user, r.URL.Query().Get("month"))
	}))
	s.mux.Handle("POST /v2/perintis/reports/{id}/review", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		report, err := id(r, "id")
		if err != nil {
			return nil, err
		}
		var in perintis.ReviewInput
		if err := decodePerintis(r, &in, 64<<10); err != nil {
			return nil, err
		}
		return service.ReviewReport(r.Context(), user, report, in)
	}))
}

// perintisClock honours PERINTIS_CLOCK by shifting real time so the demo
// clock still moves.
func (s *Server) perintisClock() func() time.Time {
	if s.Config.PerintisClock == "" {
		return s.now
	}
	start, err := time.Parse(time.RFC3339, s.Config.PerintisClock)
	if err != nil {
		s.logger().Warn("ignoring invalid PERINTIS_CLOCK")
		return s.now
	}
	offset := start.Sub(time.Now())
	return func() time.Time { return s.now().Add(offset) }
}

func decodePerintis(r *http.Request, into any, limit int64) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return &perintis.Error{Status: http.StatusRequestEntityTooLarge, Message: "Data terlalu besar."}
	}
	if err := json.Unmarshal(body, into); err != nil {
		return &perintis.Error{Status: http.StatusUnprocessableEntity, Message: "Data tidak terbaca."}
	}
	return nil
}

func (s *Server) perintisFailure(w http.ResponseWriter, err error) {
	var failure *perintis.Error
	if errors.As(err, &failure) {
		writeJSON(w, failure.Status, message{failure.Message})
		return
	}
	s.logger().Error("perintis request failed", "error", err.Error())
	writeJSON(w, http.StatusInternalServerError, message{"Terjadi kesalahan. Coba lagi."})
}

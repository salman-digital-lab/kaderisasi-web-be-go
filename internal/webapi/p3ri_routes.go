package webapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"kaderisasi/web/internal/p3ri"
)

// P3RI 1448 H committee registration for the p3ri.salmanitb.com site. New
// routes, not ported from web-be: responses are {"message":"OK","data":…}
// and failures are {"message":<Indonesian text>}.
func (s *Server) registerP3ri() {
	service := &p3ri.Service{Pool: s.Pool, Storage: s.Storage, Now: s.now}
	reply := func(w http.ResponseWriter, data any) {
		writeJSON(w, http.StatusOK, struct {
			Message string `json:"message"`
			Data    any    `json:"data"`
		}{"OK", data})
	}
	auth := func(h func(http.ResponseWriter, *http.Request, int32) (any, error)) http.Handler {
		return s.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			learner, _ := LearnerFrom(r.Context())
			data, err := h(w, r, learner.User.ID)
			if err != nil {
				s.p3riFailure(w, err)
				return
			}
			if w.Header().Get("Content-Type") == "" {
				reply(w, data)
			}
		}))
	}

	s.mux.HandleFunc("GET /v2/p3ri/form", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		reply(w, service.Form())
	})
	s.mux.Handle("GET /v2/p3ri/registration", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		return service.Mine(r.Context(), user)
	}))
	s.mux.Handle("PUT /v2/p3ri/registration", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in p3ri.Input
		if err := decodeP3ri(r, &in, 512<<10); err != nil {
			return nil, err
		}
		return service.Save(r.Context(), user, in)
	}))
	s.mux.Handle("POST /v2/p3ri/uploads", auth(func(_ http.ResponseWriter, r *http.Request, user int32) (any, error) {
		var in p3ri.FilePayload
		if err := decodeP3ri(r, &in, 8<<20); err != nil {
			return nil, err
		}
		return service.Upload(r.Context(), user, in)
	}))
	s.mux.Handle("GET /v2/p3ri/files", auth(func(w http.ResponseWriter, r *http.Request, user int32) (any, error) {
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
}

func decodeP3ri(r *http.Request, into any, limit int64) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return &p3ri.Error{Status: http.StatusRequestEntityTooLarge, Message: "Data terlalu besar."}
	}
	if err := json.Unmarshal(body, into); err != nil {
		return &p3ri.Error{Status: http.StatusUnprocessableEntity, Message: "Data tidak terbaca."}
	}
	return nil
}

func (s *Server) p3riFailure(w http.ResponseWriter, err error) {
	var failure *p3ri.Error
	if errors.As(err, &failure) {
		writeJSON(w, failure.Status, message{failure.Message})
		return
	}
	s.logger().Error("p3ri request failed", "error", err.Error())
	writeJSON(w, http.StatusInternalServerError, message{"Terjadi kesalahan. Coba lagi."})
}

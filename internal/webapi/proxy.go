package webapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// newLegacyProxy forwards a request the Go service does not own to the Adonis
// web-be service unchanged: method, path, query, body, cookies and
// Authorization. Incoming X-Forwarded-For entries are kept and the caller's
// address is appended, so Adonis sees the same client chain as before.
func newLegacyProxy(target *url.URL, logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			if forwarded, ok := r.In.Header["X-Forwarded-For"]; ok {
				r.Out.Header["X-Forwarded-For"] = append([]string(nil), forwarded...)
			}
			r.SetXForwarded()
		},
		ModifyResponse: func(response *http.Response) error {
			// observe already set the same id, which Adonis reused; drop the
			// upstream copy so the header is not sent twice.
			response.Header.Del("X-Request-Id")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, r.Context().Err()) {
				return
			}
			logger.Error("web-be proxy failed", "request_id", r.Header.Get("X-Request-Id"), "path", r.URL.Path, "error", err.Error())
			writeJSON(w, http.StatusBadGateway, message{"LEGACY_API_UNAVAILABLE"})
		},
	}
}

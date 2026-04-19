package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// BearerAuth returns a middleware that enforces Bearer token authentication.
// Requests without a valid Authorization header receive a 401 response.
// If token is empty the middleware is a no-op (auth disabled).
func BearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	expected := "Bearer " + token

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("Authorization")) != expected {
			slog.Warn("auth failed", "path", r.URL.Path, "method", r.Method, "remote_addr", r.RemoteAddr)
			w.Header().Set("WWW-Authenticate", `Bearer realm="cerbai"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		slog.Debug("auth ok", "path", r.URL.Path, "method", r.Method, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

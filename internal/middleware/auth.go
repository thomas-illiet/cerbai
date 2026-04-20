package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// obfuscate returns the first 4 chars of s followed by "***", or "<empty>" if s is blank.
func obfuscate(s string) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= 4 {
		return "***"
	}
	return s[:4] + "***"
}

// BearerAuth returns a middleware that enforces Bearer token authentication.
// Requests without a valid Authorization header receive a 401 response.
// If token is empty the middleware is a no-op (auth disabled).
func BearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	expected := "Bearer " + token

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(r.Header.Get("Authorization"))
		if got != expected {
			slog.Warn("auth failed",
				"path", r.URL.Path,
				"method", r.Method,
				"remote_addr", r.RemoteAddr,
			)
			slog.Debug("auth failed detail",
				"got_header", obfuscate(got),
				"want_prefix", "Bearer ",
				"want_token", obfuscate(token),
				"header_set", got != "",
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="cerbai"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		slog.Debug("auth ok", "path", r.URL.Path, "method", r.Method, "remote_addr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

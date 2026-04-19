package middleware

import (
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
			w.Header().Set("WWW-Authenticate", `Bearer realm="cerbai"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		requestToken string
		wantStatus   int
		wantBody     string
	}{
		{
			name:         "valid token",
			token:        "my-secret",
			requestToken: "Bearer my-secret",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "missing token",
			token:        "my-secret",
			requestToken: "",
			wantStatus:   http.StatusUnauthorized,
			wantBody:     "unauthorized\n",
		},
		{
			name:         "wrong token",
			token:        "my-secret",
			requestToken: "Bearer wrong-token",
			wantStatus:   http.StatusUnauthorized,
			wantBody:     "unauthorized\n",
		},
		{
			name:         "token without prefix",
			token:        "my-secret",
			requestToken: "my-secret",
			wantStatus:   http.StatusUnauthorized,
			wantBody:     "unauthorized\n",
		},
		{
			name:         "token with extra whitespace",
			token:        "my-secret",
			requestToken: "  Bearer my-secret  ",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			handler := BearerAuth(tt.token, next)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.requestToken != "" {
				req.Header.Set("Authorization", tt.requestToken)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}

			if tt.wantStatus == http.StatusOK && !nextCalled {
				t.Error("next handler not called on valid auth")
			}
		})
	}
}

func TestBearerAuth_EmptyToken_DisablesAuth(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := BearerAuth("", next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if !nextCalled {
		t.Error("next handler not called when auth is disabled")
	}
}

func TestBearerAuth_WWWAuthenticateHeader(t *testing.T) {
	handler := BearerAuth("my-secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("WWW-Authenticate"); got != `Bearer realm="cerbai"` {
		t.Errorf("WWW-Authenticate = %q, want %q", got, `Bearer realm="cerbai"`)
	}
}

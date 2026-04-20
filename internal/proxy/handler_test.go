package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTokenFetcher is a test double for TokenFetcher.
type mockTokenFetcher struct {
	token string
	err   error
}

func (m *mockTokenFetcher) Fetch(ctx context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		cache   TokenFetcher
		cfg     HandlerConfig
		wantErr bool
	}{
		{
			name:   "valid config",
			target: "http://localhost:8085",
			cache:  &mockTokenFetcher{token: "test-token"},
			cfg: HandlerConfig{
				TokenHeader: "Authorization",
				TokenPrefix: "Bearer ",
			},
			wantErr: false,
		},
		{
			name:    "invalid url",
			target:  "://invalid-url",
			cache:   &mockTokenFetcher{token: "test-token"},
			cfg:     HandlerConfig{},
			wantErr: true,
		},
		{
			name:   "custom header",
			target: "http://localhost:8085",
			cache:  &mockTokenFetcher{token: "test-token"},
			cfg: HandlerConfig{
				TokenHeader: "X-Custom-Auth",
				TokenPrefix: "Token ",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := New(tt.target, tt.cache, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && h == nil {
				t.Error("New() returned nil handler")
			}
		})
	}
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		tokenErr   error
		wantStatus int
		wantToken  string
		wantHeader string
		wantBody   string
	}{
		{
			name:       "successful proxy with token",
			token:      "valid-token",
			wantStatus: http.StatusOK,
			wantToken:  "Bearer valid-token",
			wantHeader: "Authorization",
		},
		{
			name:       "token fetch error",
			tokenErr:   context.Canceled,
			wantStatus: http.StatusBadGateway,
			wantBody:   "upstream authentication failed\n",
		},
		{
			name:       "custom header and prefix",
			token:      "my-token",
			wantStatus: http.StatusOK,
			wantToken:  "Token my-token",
			wantHeader: "X-Custom-Auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test backend server
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify token was injected
				if tt.tokenErr == nil {
					if got := r.Header.Get(tt.wantHeader); got != tt.wantToken {
						t.Errorf("header %s = %q, want %q", tt.wantHeader, got, tt.wantToken)
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			cache := &mockTokenFetcher{
				token: tt.token,
				err:   tt.tokenErr,
			}

			cfg := HandlerConfig{
				TokenHeader: "Authorization",
				TokenPrefix: "Bearer ",
			}
			if tt.wantHeader == "X-Custom-Auth" {
				cfg.TokenHeader = "X-Custom-Auth"
				cfg.TokenPrefix = "Token "
			}

			handler, err := New(backend.URL, cache, cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_ServeHTTP_StripsClientAuth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify client-provided auth was stripped
		if r.Header.Get("Authorization") != "Bearer server-token" {
			t.Errorf("Authorization header = %q, want Bearer server-token", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cache := &mockTokenFetcher{token: "server-token"}
	handler, err := New(backend.URL, cache, HandlerConfig{
		TokenHeader: "Authorization",
		TokenPrefix: "Bearer ",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer client-token")
	req.Header.Set("Proxy-Authorization", "Basic abc123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHandler_ModifyResponse_RemovesContentLengthForSSE(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "1234")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cache := &mockTokenFetcher{token: "test-token"}
	handler, err := New(backend.URL, cache, HandlerConfig{
		TokenHeader: "Authorization",
		TokenPrefix: "Bearer ",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Length") != "" {
		t.Errorf("Content-Length = %q, want empty for SSE", rr.Header().Get("Content-Length"))
	}
}

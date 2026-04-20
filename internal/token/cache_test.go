package token

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockFetcher is a test double for tokenFetcher.
type mockFetcher struct {
	token string
	err   error
	mu    sync.Mutex
	calls int
}

func (m *mockFetcher) getToken(_ context.Context, _ time.Duration) (string, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.token, 5 * time.Minute, m.err
}

func TestMemoryCache_Fetch(t *testing.T) {
	tests := []struct {
		name          string
		cachedToken   string
		fetchToken    string
		cacheValid    bool
		err           error
		wantToken     string
		wantErr       bool
		expectedCalls int
	}{
		{
			name:          "cache hit",
			cachedToken:   "cached-token-123",
			fetchToken:    "should-not-be-returned",
			cacheValid:    true,
			wantToken:     "cached-token-123",
			wantErr:       false,
			expectedCalls: 0,
		},
		{
			name:          "cache miss, refresh success",
			cachedToken:   "",
			fetchToken:    "fresh-token-456",
			cacheValid:    false,
			wantToken:     "fresh-token-456",
			wantErr:       false,
			expectedCalls: 1,
		},
		{
			name:          "token refresh fails",
			cachedToken:   "",
			fetchToken:    "",
			cacheValid:    false,
			err:           io.ErrUnexpectedEOF,
			wantToken:     "",
			wantErr:       true,
			expectedCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockFetcher{token: tt.fetchToken, err: tt.err}
			expiry := time.Now().Add(-time.Minute) // expired by default
			if tt.cacheValid {
				expiry = time.Now().Add(10 * time.Minute)
			}
			cache := &MemoryCache{
				fetcher:       mock,
				configuredTTL: 5 * time.Minute,
				cachedToken:   tt.cachedToken,
				expiresAt:     expiry,
			}

			got, err := cache.Fetch(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantToken, got)

			mock.mu.Lock()
			calls := mock.calls
			mock.mu.Unlock()
			assert.Equal(t, tt.expectedCalls, calls)
		})
	}
}

func TestMemoryCache_Fetch_Concurrent(t *testing.T) {
	fetcher := &mockFetcher{token: "shared-token"}

	cache := &MemoryCache{
		fetcher:       fetcher,
		configuredTTL: 5 * time.Minute,
		cachedToken:   "",
		expiresAt:     time.Now().Add(-time.Minute),
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, _ = cache.Fetch(context.Background())
			}
		}()
	}
	wg.Wait()
}

func TestMemoryCache_Fetch_DoubleChecked(t *testing.T) {
	fetcher := &mockFetcher{token: "token"}

	cache := &MemoryCache{
		fetcher:       fetcher,
		configuredTTL: 5 * time.Minute,
		cachedToken:   "",
		expiresAt:     time.Now().Add(-time.Minute),
	}

	ctx := context.Background()

	token1, err1 := cache.Fetch(ctx)
	assert.NoError(t, err1)
	assert.Equal(t, "token", token1)

	// Second call should hit the now-valid cache without calling the fetcher again.
	token2, err2 := cache.Fetch(ctx)
	assert.NoError(t, err2)
	assert.Equal(t, "token", token2)

	fetcher.mu.Lock()
	calls := fetcher.calls
	fetcher.mu.Unlock()
	assert.Equal(t, 1, calls, "fetch should only be called once due to double-check")
}

func TestFetcher_getToken(t *testing.T) {
	tests := []struct {
		name      string
		clientID  string
		secret    string
		httpCode  int
		body      string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "success with 300s expires_in",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusOK,
			body:      `{"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token", "expires_in": 300, "token_type": "Bearer"}`,
			wantToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token",
		},
		{
			name:      "success with short expires_in",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusOK,
			body:      `{"access_token": "short-token", "expires_in": 300, "token_type": "Bearer"}`,
			wantToken: "short-token",
		},
		{
			name:      "http error",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusUnauthorized,
			body:      "",
			wantToken: "",
			wantErr:   true,
		},
		{
			name:      "json decode error",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusOK,
			body:      `{invalid json}`,
			wantToken: "",
			wantErr:   true,
		},
		{
			name:      "oauth error response",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusOK,
			body:      `{"error": "invalid_client", "error_description": "Client credentials invalid"}`,
			wantToken: "",
			wantErr:   true,
		},
		{
			name:      "empty access_token",
			clientID:  "test-client",
			secret:    "test-secret",
			httpCode:  http.StatusOK,
			body:      `{"access_token": "", "expires_in": 3600, "token_type": "Bearer"}`,
			wantToken: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.httpCode)
				if tt.body != "" {
					w.Write([]byte(tt.body))
				}
			}))
			defer server.Close()

			f := &fetcher{
				httpClient:    &http.Client{Timeout: 10 * time.Second},
				tokenEndpoint: server.URL,
				clientID:      tt.clientID,
				clientSecret:  tt.secret,
			}

			gotToken, gotTTL, err := f.getToken(context.Background(), 5*time.Minute)

			if (err != nil) != tt.wantErr {
				t.Errorf("getToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantToken, gotToken)
			if tt.wantErr {
				assert.Equal(t, time.Duration(0), gotTTL)
			} else {
				assert.NotZero(t, gotTTL)
			}
		})
	}
}

func TestFetcher_getToken_WithNilTLS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token": "test-token", "expires_in": 3600}`))
	}))
	defer server.Close()

	f := newFetcher(nil, server.URL, "client", "secret", "basic")
	tok, ttl, err := f.getToken(context.Background(), 5*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "test-token", tok)
	assert.Greater(t, ttl, time.Duration(0))
}

func TestFetcher_getToken_WithCustomTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token": "tls-token", "expires_in": 3600}`))
	}))
	defer server.Close()

	tlsCfg := server.Client().Transport.(*http.Transport).TLSClientConfig
	f := newFetcher(tlsCfg, server.URL, "client", "secret", "basic")
	tok, ttl, err := f.getToken(context.Background(), 5*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "tls-token", tok)
	assert.Greater(t, ttl, time.Duration(0))
}

func TestFetcher_getToken_MinTTLFiveSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token": "test-token", "expires_in": 1}`))
	}))
	defer server.Close()

	f := &fetcher{
		httpClient:    &http.Client{},
		tokenEndpoint: server.URL,
		clientID:      "test-client",
		clientSecret:  "test-secret",
	}

	_, ttl, err := f.getToken(context.Background(), 5*time.Minute)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, ttl, 5*time.Second, "TTL should be at least 5 seconds")
}

func TestFetcher_getToken_BuildsRequestCorrectly(t *testing.T) {
	tests := []struct {
		name             string
		authMethod       string
		wantBodyClientID bool
		wantBasicAuth    bool
	}{
		{"form", "form", true, false},
		{"basic", "basic", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedMethod, capturedContentType, capturedBody, capturedAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedContentType = r.Header.Get("Content-Type")
				capturedAuth = r.Header.Get("Authorization")
				b, _ := io.ReadAll(r.Body)
				capturedBody = string(b)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"access_token": "test-token", "expires_in": 3600}`))
			}))
			defer server.Close()

			f := &fetcher{
				httpClient:       &http.Client{},
				tokenEndpoint:    server.URL,
				clientID:         "my-client-id",
				clientSecret:     "my-client-secret",
				clientAuthMethod: tt.authMethod,
			}

			_, _, err := f.getToken(context.Background(), 5*time.Minute)
			assert.NoError(t, err)
			assert.Equal(t, http.MethodPost, capturedMethod)
			assert.Equal(t, "application/x-www-form-urlencoded", capturedContentType)
			assert.Contains(t, capturedBody, "grant_type=client_credentials")
			if tt.wantBodyClientID {
				assert.Contains(t, capturedBody, "client_id=my-client-id")
				assert.Contains(t, capturedBody, "client_secret=my-client-secret")
				assert.Empty(t, capturedAuth)
			} else {
				assert.NotContains(t, capturedBody, "client_id")
				assert.NotContains(t, capturedBody, "client_secret")
				assert.Contains(t, capturedAuth, "Basic ")
			}
		})
	}
}

func TestFetcher_getToken_HTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	f := &fetcher{
		httpClient:    &http.Client{Timeout: 100 * time.Millisecond},
		tokenEndpoint: server.URL,
		clientID:      "test-client",
		clientSecret:  "test-secret",
	}

	_, _, err := f.getToken(context.Background(), 5*time.Minute)
	assert.Error(t, err, "expected timeout error")
}

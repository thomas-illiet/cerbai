package token

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// fetcher handles the OAuth2 client_credentials HTTP call over mTLS.
// It is shared between MemoryCache and RedisCache.
type fetcher struct {
	httpClient    *http.Client
	tokenEndpoint string
	clientID      string
	clientSecret  string
}

func newFetcher(tlsCfg *tls.Config, endpoint, clientID, secret string) *fetcher {
	// tlsCfg may be nil (standard HTTPS) or non-nil (custom CA / mTLS).
	client := &http.Client{Timeout: 10 * time.Second}
	if tlsCfg != nil {
		client.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	}
	return &fetcher{
		httpClient:    client,
		tokenEndpoint: endpoint,
		clientID:      clientID,
		clientSecret:  secret,
	}
}

// getToken calls the token endpoint and returns the access token along with
// the effective TTL to cache it for (min of configuredTTL and expires_in-30s).
func (f *fetcher) getToken(ctx context.Context, configuredTTL time.Duration) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", f.clientID)
	form.Set("client_secret", f.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", 0, fmt.Errorf("decode token response: %w", err)
	}
	if tr.Error != "" {
		return "", 0, fmt.Errorf("token endpoint error %q: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("token endpoint returned empty access_token")
	}

	ttl := configuredTTL
	if tr.ExpiresIn > 0 {
		// Subtract 30s buffer to avoid using a token right as it expires.
		fromServer := time.Duration(tr.ExpiresIn)*time.Second - 30*time.Second
		if fromServer < ttl {
			ttl = fromServer
		}
	}
	// Never cache for less than 5 seconds.
	if ttl < 5*time.Second {
		ttl = 5 * time.Second
	}

	return tr.AccessToken, ttl, nil
}

// tokenFetcher is the interface for obtaining OAuth2 tokens from the endpoint.
type tokenFetcher interface {
	getToken(ctx context.Context, configuredTTL time.Duration) (string, time.Duration, error)
}

// MemoryCache is a thread-safe in-memory JWT token cache.
type MemoryCache struct {
	fetcher       tokenFetcher
	mu            sync.RWMutex
	configuredTTL time.Duration
	cachedToken   string
	expiresAt     time.Time
}

// NewMemory creates an in-memory token cache backed by an mTLS token endpoint.
func NewMemory(tlsCfg *tls.Config, endpoint, clientID, secret string, ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		fetcher:       newFetcher(tlsCfg, endpoint, clientID, secret),
		configuredTTL: ttl,
	}
}

// Fetch returns a valid access token, refreshing if the cache has expired.
func (c *MemoryCache) Fetch(ctx context.Context) (string, error) {
	// Fast path: read-lock, return cached token if still valid.
	c.mu.RLock()
	if c.cachedToken != "" && time.Now().Before(c.expiresAt) {
		c.mu.RUnlock()
		return c.cachedToken, nil
	}
	c.mu.RUnlock()

	// Slow path: acquire write-lock, double-check before refreshing.
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.expiresAt) {
		return c.cachedToken, nil
	}

	slog.Info("memory cache miss, refreshing token")
	start := time.Now()
	tok, effectiveTTL, err := c.fetcher.getToken(ctx, c.configuredTTL)
	if err != nil {
		return "", fmt.Errorf("token refresh: %w", err)
	}
	c.cachedToken = tok
	c.expiresAt = time.Now().Add(effectiveTTL)
	slog.Info("token refreshed", "expires_at", c.expiresAt.Format(time.RFC3339), "backend", "memory", "duration_ms", time.Since(start).Milliseconds())
	return tok, nil
}

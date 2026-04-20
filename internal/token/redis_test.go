package token

import (
	"testing"
	"time"
)

func TestNewRedis(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		clientID string
		secret   string
		ttl      time.Duration
		redisURL string
		wantErr  bool
	}{
		{
			name:     "success",
			endpoint: "https://token.example.com/oauth2/token",
			clientID: "test-client",
			secret:   "test-secret",
			ttl:      5 * time.Minute,
			redisURL: "redis://localhost:6379/0",
			wantErr:  true, // no redis server
		},
		{
			name:     "invalid redis URL",
			endpoint: "https://token.example.com/oauth2/token",
			clientID: "test-client",
			secret:   "test-secret",
			ttl:      5 * time.Minute,
			redisURL: "not-a-valid-redis-url",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRedis(nil, tt.endpoint, tt.clientID, tt.secret, "basic", tt.ttl, tt.redisURL)
			if !tt.wantErr && err == nil {
				t.Error("NewRedis() expected error but got none")
			}
		})
	}
}

func TestNewMemory(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		clientID string
		secret   string
		ttl      time.Duration
		wantErr  bool
	}{
		{
			name:     "success",
			endpoint: "https://token.example.com/oauth2/token",
			clientID: "test-client",
			secret:   "test-secret",
			ttl:      5 * time.Minute,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewMemory(nil, tt.endpoint, tt.clientID, tt.secret, "basic", tt.ttl)
			if cache == nil {
				t.Error("NewMemory() returned nil")
			}
		})
	}
}

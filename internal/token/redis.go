package token

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisTokenKey = "cerbai:token"

// RedisCache is a JWT token cache backed by Redis, suitable for multi-replica
// Kubernetes deployments where all pods share the same cached token.
type RedisCache struct {
	*fetcher
	client        *redis.Client
	configuredTTL time.Duration
}

// NewRedis creates a Redis-backed token cache. redisURL must be a valid Redis
// URL (e.g. "redis://localhost:6379/0" or "rediss://..." for TLS).
func NewRedis(tlsCfg *tls.Config, endpoint, clientID, secret string, ttl time.Duration, redisURL string) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify connectivity at startup.
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisCache{
		fetcher:       newFetcher(tlsCfg, endpoint, clientID, secret),
		client:        client,
		configuredTTL: ttl,
	}, nil
}

// Fetch returns a valid access token from Redis, refreshing via the token
// endpoint on a cache miss. Redis SET is atomic so no mutex is needed.
func (c *RedisCache) Fetch(ctx context.Context) (string, error) {
	tok, err := c.client.Get(ctx, redisTokenKey).Result()
	if err == nil {
		return tok, nil
	}
	if !errors.Is(err, redis.Nil) {
		return "", fmt.Errorf("redis get: %w", err)
	}

	slog.Info("redis cache miss, refreshing token")
	tok, ttl, err := c.getToken(ctx, c.configuredTTL)
	if err != nil {
		return "", fmt.Errorf("token refresh: %w", err)
	}

	if err := c.client.Set(ctx, redisTokenKey, tok, ttl).Err(); err != nil {
		// Non-fatal: return the token even if caching fails.
		slog.Warn("redis set failed, token will not be cached", "error", err)
	} else {
		slog.Info("token refreshed", "ttl", ttl, "backend", "redis")
	}

	return tok, nil
}

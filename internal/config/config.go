package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all proxy runtime configuration.
type Config struct {
	ListenAddr       string
	LLMURL           string
	TokenEndpoint    string
	ClientID         string
	ClientSecret     string
	TLSCertFile      string
	TLSKeyFile       string
	TLSCAFile        string
	TokenCacheTTL    time.Duration
	TokenHeader      string
	TokenPrefix      string
	RedisURL         string // optional — empty means in-memory cache
	ProxyToken       string // optional — empty disables proxy auth
	LogLevel         string // debug, info, warn, error
	ClientAuthMethod string // "basic" (default) or "form"
}

// RegisterFlags registers all CLI flags on cmd and binds them to viper.
// Env vars follow the pattern CERBAI_<FLAG_NAME_UPPERCASED_UNDERSCORED>.
func RegisterFlags(cmd *cobra.Command, v *viper.Viper) {
	f := cmd.Flags()

	f.String("listen-addr", ":8085", "Address to listen on (env: CERBAI_LISTEN_ADDR)")
	f.String("llm-url", "", "Upstream LLM base URL (env: CERBAI_LLM_URL)")
	f.String("token-endpoint", "", "OAuth2 token endpoint URL (env: CERBAI_TOKEN_ENDPOINT)")
	f.String("client-id", "", "OAuth2 client ID (env: CERBAI_CLIENT_ID)")
	f.String("client-secret", "", "OAuth2 client secret (env: CERBAI_CLIENT_SECRET)")
	f.String("tls-cert-file", "", "Client certificate file for mTLS, optional (env: CERBAI_TLS_CERT_FILE)")
	f.String("tls-key-file", "", "Client key file for mTLS, optional — must be set with --tls-cert-file (env: CERBAI_TLS_KEY_FILE)")
	f.String("tls-ca-file", "", "Custom CA certificate file, optional (env: CERBAI_TLS_CA_FILE)")
	f.Duration("token-cache-ttl", 5*time.Minute, "Token cache TTL (env: CERBAI_TOKEN_CACHE_TTL)")
	f.String("token-header", "Authorization", "Header name to inject the token into (env: CERBAI_TOKEN_HEADER)")
	f.String("token-prefix", "Bearer ", "Token value prefix, e.g. 'Bearer ' (env: CERBAI_TOKEN_PREFIX)")
	f.String("redis-url", "", "Redis URL for shared token cache, optional (env: CERBAI_REDIS_URL)")
	f.String("proxy-token", "", "Bearer token required to use the proxy, optional — omit to disable auth (env: CERBAI_PROXY_TOKEN)")
	f.String("log-level", "info", "Log level: debug, info, warn, error (env: CERBAI_LOG_LEVEL)")
	f.String("client-auth-method", "basic", "OAuth2 client auth method: basic or form (env: CERBAI_CLIENT_AUTH_METHOD)")

	v.SetEnvPrefix("CERBAI")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	_ = v.BindPFlags(f)
}

// Load builds a Config from the bound viper instance.
func Load(v *viper.Viper) (*Config, error) {
	cfg := &Config{
		ListenAddr:       v.GetString("listen-addr"),
		LLMURL:           v.GetString("llm-url"),
		TokenEndpoint:    v.GetString("token-endpoint"),
		ClientID:         v.GetString("client-id"),
		ClientSecret:     v.GetString("client-secret"),
		TLSCertFile:      v.GetString("tls-cert-file"),
		TLSKeyFile:       v.GetString("tls-key-file"),
		TLSCAFile:        v.GetString("tls-ca-file"),
		TokenCacheTTL:    v.GetDuration("token-cache-ttl"),
		TokenHeader:      v.GetString("token-header"),
		TokenPrefix:      v.GetString("token-prefix"),
		RedisURL:         v.GetString("redis-url"),
		ProxyToken:       v.GetString("proxy-token"),
		LogLevel:         v.GetString("log-level"),
		ClientAuthMethod: v.GetString("client-auth-method"),
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validate(cfg *Config) error {
	required := []struct {
		name  string
		value string
	}{
		{"--llm-url / CERBAI_LLM_URL", cfg.LLMURL},
		{"--token-endpoint / CERBAI_TOKEN_ENDPOINT", cfg.TokenEndpoint},
		{"--client-id / CERBAI_CLIENT_ID", cfg.ClientID},
		{"--client-secret / CERBAI_CLIENT_SECRET", cfg.ClientSecret},
	}

	if cfg.ClientAuthMethod != "basic" && cfg.ClientAuthMethod != "form" {
		return fmt.Errorf("--client-auth-method must be \"basic\" or \"form\", got %q", cfg.ClientAuthMethod)
	}

	// cert and key must be provided together if either one is set.
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return fmt.Errorf("--tls-cert-file and --tls-key-file must both be set or both be omitted")
	}
	var missing []string
	for _, r := range required {
		if r.value == "" {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration:\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}

// BuildTLSConfig constructs a *tls.Config for the token HTTP client.
// Returns nil when no cert/key are configured, which means the client will
// use standard HTTPS with system CAs (no mutual TLS).
func (c *Config) BuildTLSConfig() (*tls.Config, error) {
	if c.TLSCertFile == "" {
		// No client cert — plain HTTPS, optionally with a custom CA.
		if c.TLSCAFile == "" {
			return nil, nil
		}
		caBytes, err := os.ReadFile(c.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("no valid CA certs found in %s", c.TLSCAFile)
		}
		return &tls.Config{RootCAs: pool}, nil
	}

	// mTLS: load client certificate pair.
	cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	if c.TLSCAFile != "" {
		caBytes, err := os.ReadFile(c.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("no valid CA certs found in %s", c.TLSCAFile)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

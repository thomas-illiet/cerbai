package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// generateSelfSignedCert creates a valid self-signed certificate and key in PEM form.
func generateSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func TestRegisterFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	RegisterFlags(cmd, v)

	tests := []struct {
		name     string
		key      string
		expected any
	}{
		{"listen-addr", "listen-addr", ":8085"},
		{"llm-url", "llm-url", ""},
		{"token-endpoint", "token-endpoint", ""},
		{"client-id", "client-id", ""},
		{"client-secret", "client-secret", ""},
		{"tls-cert-file", "tls-cert-file", ""},
		{"tls-key-file", "tls-key-file", ""},
		{"tls-ca-file", "tls-ca-file", ""},
		// v.Get() returns the string form of duration flags when bound via BindPFlags.
		{"token-cache-ttl", "token-cache-ttl", "5m0s"},
		{"token-header", "token-header", "Authorization"},
		{"token-prefix", "token-prefix", "Bearer "},
		{"redis-url", "redis-url", ""},
		{"proxy-token", "proxy-token", ""},
		{"log-level", "log-level", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := v.Get(tt.key)
			if got != tt.expected {
				t.Errorf("flag %s: expected %v, got %v", tt.name, tt.expected, got)
			}
		})
	}

	// Separate typed check for the duration default.
	if d := v.GetDuration("token-cache-ttl"); d != 5*time.Minute {
		t.Errorf("GetDuration(token-cache-ttl) = %v, want 5m0s", d)
	}
}

func TestLoad(t *testing.T) {
	v := viper.New()
	v.Set("listen-addr", ":9090")
	v.Set("llm-url", "https://llm.example.com")
	v.Set("token-endpoint", "https://auth.example.com/oauth2/token")
	v.Set("client-id", "test-client")
	v.Set("client-secret", "test-secret")
	v.Set("tls-cert-file", "/path/to/cert.pem")
	v.Set("tls-key-file", "/path/to/key.pem")
	v.Set("tls-ca-file", "/path/to/ca.pem")
	v.Set("token-cache-ttl", "10m")
	v.Set("token-header", "X-Auth-Token")
	v.Set("token-prefix", "Token ")
	v.Set("redis-url", "redis://localhost:6379")
	v.Set("proxy-token", "my-token")
	v.Set("log-level", "debug")
	v.Set("client-auth-method", "basic")

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %v, want :9090", cfg.ListenAddr)
	}
	if cfg.LLMURL != "https://llm.example.com" {
		t.Errorf("LLMURL = %v, want https://llm.example.com", cfg.LLMURL)
	}
	if cfg.TokenEndpoint != "https://auth.example.com/oauth2/token" {
		t.Errorf("TokenEndpoint = %v, want https://auth.example.com/oauth2/token", cfg.TokenEndpoint)
	}
	if cfg.ClientID != "test-client" {
		t.Errorf("ClientID = %v, want test-client", cfg.ClientID)
	}
	if cfg.ClientSecret != "test-secret" {
		t.Errorf("ClientSecret = %v, want test-secret", cfg.ClientSecret)
	}
	if cfg.TLSCertFile != "/path/to/cert.pem" {
		t.Errorf("TLSCertFile = %v, want /path/to/cert.pem", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/path/to/key.pem" {
		t.Errorf("TLSKeyFile = %v, want /path/to/key.pem", cfg.TLSKeyFile)
	}
	if cfg.TLSCAFile != "/path/to/ca.pem" {
		t.Errorf("TLSCAFile = %v, want /path/to/ca.pem", cfg.TLSCAFile)
	}
	if cfg.TokenCacheTTL != 10*time.Minute {
		t.Errorf("TokenCacheTTL = %v, want 10m", cfg.TokenCacheTTL)
	}
	if cfg.TokenHeader != "X-Auth-Token" {
		t.Errorf("TokenHeader = %v, want X-Auth-Token", cfg.TokenHeader)
	}
	if cfg.TokenPrefix != "Token " {
		t.Errorf("TokenPrefix = %v, want Token ", cfg.TokenPrefix)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("RedisURL = %v, want redis://localhost:6379", cfg.RedisURL)
	}
	if cfg.ProxyToken != "my-token" {
		t.Errorf("ProxyToken = %v, want my-token", cfg.ProxyToken)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				LLMURL:           "https://llm.example.com",
				TokenEndpoint:    "https://auth.example.com/token",
				ClientID:         "client",
				ClientSecret:     "secret",
				ClientAuthMethod: "basic",
			},
			wantErr: false,
		},
		{
			name: "missing llm-url",
			cfg: &Config{
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
			},
			wantErr: true,
		},
		{
			name: "missing token-endpoint",
			cfg: &Config{
				LLMURL:       "https://llm.example.com",
				ClientID:     "client",
				ClientSecret: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing client-id",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientSecret:  "secret",
			},
			wantErr: true,
		},
		{
			name: "missing client-secret",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
			},
			wantErr: true,
		},
		{
			name: "tls cert without key",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCertFile:   "/path/to/cert.pem",
			},
			wantErr: true,
		},
		{
			name: "tls key without cert",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSKeyFile:    "/path/to/key.pem",
			},
			wantErr: true,
		},
		{
			name: "tls cert and key both provided",
			cfg: &Config{
				LLMURL:           "https://llm.example.com",
				TokenEndpoint:    "https://auth.example.com/token",
				ClientID:         "client",
				ClientSecret:     "secret",
				TLSCertFile:      "/path/to/cert.pem",
				TLSKeyFile:       "/path/to/key.pem",
				ClientAuthMethod: "basic",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	tmpDir := t.TempDir()

	certPEM, keyPEM := generateSelfSignedCert(t)
	caPEM := certPEM // self-signed: cert is its own CA

	certPath := tmpDir + "/cert.pem"
	keyPath := tmpDir + "/key.pem"
	caPath := tmpDir + "/ca.pem"

	for _, pair := range []struct {
		path string
		data []byte
	}{
		{certPath, certPEM},
		{keyPath, keyPEM},
		{caPath, caPEM},
	} {
		if err := os.WriteFile(pair.path, pair.data, 0o600); err != nil {
			t.Fatalf("write %s: %v", pair.path, err)
		}
	}

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		wantNil bool
	}{
		{
			name: "no tls config",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
			},
			wantErr: false,
			wantNil: true,
		},
		{
			name: "custom ca only",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCAFile:     caPath,
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "mtls with cert and key",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCertFile:   certPath,
				TLSKeyFile:    keyPath,
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "mtls with cert, key and ca",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCertFile:   certPath,
				TLSKeyFile:    keyPath,
				TLSCAFile:     caPath,
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "invalid ca file",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCAFile:     "/nonexistent/ca.pem",
			},
			wantErr: true,
			wantNil: true, // BuildTLSConfig returns nil on error
		},
		{
			name: "invalid cert file",
			cfg: &Config{
				LLMURL:        "https://llm.example.com",
				TokenEndpoint: "https://auth.example.com/token",
				ClientID:      "client",
				ClientSecret:  "secret",
				TLSCertFile:   "/nonexistent/cert.pem",
				TLSKeyFile:    keyPath,
			},
			wantErr: true,
			wantNil: true, // BuildTLSConfig returns nil on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.BuildTLSConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Errorf("BuildTLSConfig() = %v, want nil", got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("BuildTLSConfig() = nil, want non-nil")
			}
		})
	}
}

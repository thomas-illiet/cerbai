package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thomas-illiet/cerbai/internal/config"
	"github.com/thomas-illiet/cerbai/internal/middleware"
	"github.com/thomas-illiet/cerbai/internal/proxy"
	"github.com/thomas-illiet/cerbai/internal/token"
)

// Build-time variables injected via -ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	v := viper.New()

	rootCmd := &cobra.Command{
		Use:   "cerbai",
		Short: "CerbAI — LLM reverse proxy with mTLS JWT authentication",
		Long: `CerbAI proxies requests to an internal LLM service, handling OAuth2
client_credentials authentication over mTLS and caching the resulting JWT.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(v)
		},
	}

	config.RegisterFlags(rootCmd, v)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run(v *viper.Viper) error {
	// Parse log level from config.
	level := slog.LevelInfo
	switch strings.ToLower(v.GetString("log-level")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("starting CerbAI", "version", version, "commit", commit, "build_date", buildDate)

	cfg, err := config.Load(v)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	slog.Info("config loaded",
		"listen_addr", cfg.ListenAddr,
		"llm_url", cfg.LLMURL,
		"token_cache_ttl", cfg.TokenCacheTTL,
		"mtls", cfg.TLSCertFile != "",
		"redis", cfg.RedisURL != "",
		"proxy_auth", cfg.ProxyToken != "",
	)

	tlsCfg, err := cfg.BuildTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS config: %w", err)
	}

	// Build token cache: Redis if configured, otherwise in-memory.
	var tokenCache proxy.TokenFetcher
	if cfg.RedisURL != "" {
		slog.Info("using Redis token cache", "url", cfg.RedisURL)
		tokenCache, err = token.NewRedis(tlsCfg, cfg.TokenEndpoint, cfg.ClientID, cfg.ClientSecret, cfg.TokenCacheTTL, cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("redis token cache: %w", err)
		}
	} else {
		slog.Info("using in-memory token cache")
		tokenCache = token.NewMemory(tlsCfg, cfg.TokenEndpoint, cfg.ClientID, cfg.ClientSecret, cfg.TokenCacheTTL)
	}

	// Warm up the token cache at startup; non-fatal so the proxy can still start.
	if _, err := tokenCache.Fetch(context.Background()); err != nil {
		slog.Warn("initial token fetch failed, will retry on first request", "error", err)
	}

	proxyHandler, err := proxy.New(cfg.LLMURL, tokenCache, proxy.HandlerConfig{
		TokenHeader: cfg.TokenHeader,
		TokenPrefix: cfg.TokenPrefix,
	})
	if err != nil {
		return fmt.Errorf("proxy handler: %w", err)
	}

	mux := http.NewServeMux()
	// /healthz is always public — exempt from proxy auth.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("health check", "remote_addr", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", middleware.BearerAuth(cfg.ProxyToken, proxyHandler))

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
		// WriteTimeout is disabled because SSE streaming responses have no
		// fixed end time — a deadline would kill long completions mid-stream.
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("starting proxy server", "addr", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	slog.Info("server stopped")
	return nil
}

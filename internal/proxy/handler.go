package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// TokenFetcher returns a valid JWT access token.
type TokenFetcher interface {
	Fetch(ctx context.Context) (string, error)
}

// HandlerConfig holds header injection settings.
type HandlerConfig struct {
	TokenHeader string
	TokenPrefix string
}

// Handler is an HTTP reverse proxy that injects a JWT into every request.
type Handler struct {
	rp    *httputil.ReverseProxy
	cache TokenFetcher
	cfg   HandlerConfig
}

// New builds a Handler that proxies to targetURL, injecting tokens from cache.
func New(targetURL string, cache TokenFetcher, cfg HandlerConfig) (*Handler, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	h := &Handler{cache: cache, cfg: cfg}

	rp := &httputil.ReverseProxy{
		// FlushInterval -1 means flush every write immediately, which is
		// required for OpenAI SSE streaming (data: {...}\n\n chunks).
		FlushInterval: -1,

		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			if target.RawQuery == "" || req.URL.RawQuery == "" {
				req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
			} else {
				req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
			}

			// Strip any auth the client sent to prevent token injection.
			req.Header.Del(cfg.TokenHeader)
			req.Header.Del("Proxy-Authorization")

			// Fetch is a fast cache hit here since ServeHTTP pre-warmed it.
			tok, err := cache.Fetch(req.Context())
			if err != nil {
				slog.Error("token fetch in director failed", "error", err)
				return
			}
			req.Header.Set(cfg.TokenHeader, cfg.TokenPrefix+tok)
		},

		ModifyResponse: func(resp *http.Response) error {
			slog.Debug("upstream response", "status", resp.StatusCode, "path", resp.Request.URL.Path)
			// Remove Content-Length for streamed responses so the client
			// reads until EOF rather than a fixed byte count.
			if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
				resp.Header.Del("Content-Length")
			}
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("reverse proxy error", "error", err, "path", r.URL.Path)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	h.rp = rp
	return h, nil
}

// ServeHTTP pre-fetches the token so auth errors surface as 502 before the
// reverse proxy Director runs (Director has no error return value).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := h.cache.Fetch(r.Context()); err != nil {
		slog.Error("token fetch failed, refusing to proxy", "error", err)
		http.Error(w, "upstream authentication failed", http.StatusBadGateway)
		return
	}
	h.rp.ServeHTTP(w, r)
}

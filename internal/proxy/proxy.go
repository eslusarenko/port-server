package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/eslusarenko/port-server/internal/httputil"
	"github.com/eslusarenko/port-server/internal/protocol"
)

// TunnelForwarder forwards an HTTP request through a tunnel and returns the
// response headers plus a streaming body.
type TunnelForwarder interface {
	ForwardRequest(ctx context.Context, meta protocol.HttpRequestMeta, body []byte) (protocol.HttpResponseMeta, io.ReadCloser, error)
}

// TunnelLookup finds a tunnel by subdomain.
type TunnelLookup interface {
	Lookup(subdomain string) (TunnelForwarder, bool)
}

// Proxy is an HTTP handler that routes requests to tunnels based on the Host header.
type Proxy struct {
	lookup            TunnelLookup
	baseDomain        string
	logger            *slog.Logger
	maxBody           int64
	trustProxyHeaders bool
}

// NewProxy creates a new reverse proxy handler.
func NewProxy(lookup TunnelLookup, baseDomain string, logger *slog.Logger, maxBody int64) *Proxy {
	return &Proxy{
		lookup:     lookup,
		baseDomain: baseDomain,
		logger:     logger,
		maxBody:    maxBody,
	}
}

// NewProxyWithOptions creates a new reverse proxy handler with additional options.
func NewProxyWithOptions(lookup TunnelLookup, baseDomain string, logger *slog.Logger, maxBody int64, trustProxyHeaders bool) *Proxy {
	return &Proxy{
		lookup:            lookup,
		baseDomain:        baseDomain,
		logger:            logger,
		maxBody:           maxBody,
		trustProxyHeaders: trustProxyHeaders,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	suffix := "." + p.baseDomain
	if !strings.HasSuffix(host, suffix) {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}
	subdomain := strings.TrimSuffix(host, suffix)
	if subdomain == "" {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}

	tun, ok := p.lookup.Lookup(subdomain)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, p.maxBody+1))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > p.maxBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	meta := protocol.HttpRequestMeta{
		Method:        r.Method,
		Path:          r.URL.RequestURI(),
		Host:          r.Host,
		Headers:       r.Header,
		ContentLength: r.ContentLength,
	}

	start := time.Now()
	respMeta, respBody, err := tun.ForwardRequest(r.Context(), meta, body)
	if err != nil {
		p.logger.Error("forward_request_failed", "subdomain", subdomain, "error", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer func() { _ = respBody.Close() }()

	// Write response headers (Content-Length forwarded verbatim from origin).
	for k, vs := range respMeta.Headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(respMeta.StatusCode)

	n, copyErr := io.Copy(w, respBody)

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	p.logger.Info("http_request",
		"subdomain", subdomain,
		"method", r.Method,
		"path", r.URL.RequestURI(),
		"status", respMeta.StatusCode,
		"duration_ms", durationMs,
		"bytes_in", int64(len(body)),
		"bytes_out", n,
		"client_ip", httputil.ClientIP(r, p.trustProxyHeaders),
	)
	if copyErr != nil {
		// Origin/tunnel ended the body early; the visitor already sees a broken
		// stream. Correct behavior, logged for visibility.
		p.logger.Warn("response_stream_incomplete", "subdomain", subdomain, "bytes_out", n, "error", copyErr)
	}
}

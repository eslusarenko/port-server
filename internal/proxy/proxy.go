package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eslusarenko/port-server/internal/protocol"
)

// TunnelForwarder forwards an HTTP request through a tunnel and returns the response.
type TunnelForwarder interface {
	ForwardRequest(ctx context.Context, meta protocol.HttpRequestMeta, body []byte) (protocol.HttpResponseMeta, []byte, error)
}

// TunnelLookup finds a tunnel by subdomain.
type TunnelLookup interface {
	Lookup(subdomain string) (TunnelForwarder, bool)
}

// Proxy is an HTTP handler that routes requests to tunnels based on the Host header.
type Proxy struct {
	lookup     TunnelLookup
	baseDomain string
	logger     *slog.Logger
	maxBody    int64
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

	respMeta, respBody, err := tun.ForwardRequest(r.Context(), meta, body)
	if err != nil {
		p.logger.Error("forward request failed", "subdomain", subdomain, "error", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	// Write response headers.
	for k, vs := range respMeta.Headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(respMeta.StatusCode)
	_, _ = w.Write(respBody)
}

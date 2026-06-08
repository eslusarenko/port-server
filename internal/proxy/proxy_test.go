package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eslusarenko/port-server/internal/protocol"

	"log/slog"
)

type mockForwarder struct {
	statusCode int
	headers    map[string][]string
	body       []byte
	err        error
}

func (m *mockForwarder) ForwardRequest(_ context.Context, _ protocol.HttpRequestMeta, _ []byte) (protocol.HttpResponseMeta, io.ReadCloser, error) {
	if m.err != nil {
		return protocol.HttpResponseMeta{}, nil, m.err
	}
	return protocol.HttpResponseMeta{
		StatusCode: m.statusCode,
		Headers:    m.headers,
	}, io.NopCloser(bytes.NewReader(m.body)), nil
}

type mockLookup struct {
	tunnels map[string]TunnelForwarder
}

func (m *mockLookup) Lookup(subdomain string) (TunnelForwarder, bool) {
	t, ok := m.tunnels[subdomain]
	return t, ok
}

func TestProxyForwardsRequest(t *testing.T) {
	fwd := &mockForwarder{
		statusCode: 200,
		headers:    map[string][]string{"Content-Type": {"text/plain"}},
		body:       []byte("hello from tunnel"),
	}
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{"abc123": fwd}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10<<20)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "abc123.tunnel.localhost"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from tunnel" {
		t.Errorf("body = %q, want %q", body, "hello from tunnel")
	}
	if resp.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", resp.Header.Get("Content-Type"))
	}
}

func TestProxyTunnelNotFound(t *testing.T) {
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10<<20)

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "unknown.tunnel.localhost"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestProxyNoSubdomain(t *testing.T) {
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10<<20)

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "other.example.com"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestProxyForwardError(t *testing.T) {
	fwd := &mockForwarder{err: fmt.Errorf("connection refused")}
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{"abc": fwd}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10<<20)

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "abc.tunnel.localhost"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestProxyWithPort(t *testing.T) {
	fwd := &mockForwarder{statusCode: 200, body: []byte("ok")}
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{"abc": fwd}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10<<20)

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "abc.tunnel.localhost:8080"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestProxyBodyTooLarge(t *testing.T) {
	fwd := &mockForwarder{statusCode: 200}
	lookup := &mockLookup{tunnels: map[string]TunnelForwarder{"abc": fwd}}
	p := NewProxy(lookup, "tunnel.localhost", slog.Default(), 10) // 10 bytes max

	body := strings.NewReader("this body is way too large for the limit")
	req := httptest.NewRequest("POST", "/", body)
	req.Host = "abc.tunnel.localhost"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

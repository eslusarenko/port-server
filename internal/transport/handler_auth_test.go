package transport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eslusarenko/port-server/internal/protocol"
	"github.com/eslusarenko/port-server/internal/tunnel"
)

var mockDriverCounter atomic.Int64

type mockQueryFunc func(query string, args []driver.NamedValue) (driver.Rows, error)
type mockExecFunc func(query string, args []driver.NamedValue) (driver.Result, error)

type mockDriver struct {
	queryFn mockQueryFunc
	execFn  mockExecFunc
}

func (d *mockDriver) Open(string) (driver.Conn, error) {
	return &mockConn{queryFn: d.queryFn, execFn: d.execFn}, nil
}

type mockConn struct {
	queryFn mockQueryFunc
	execFn  mockExecFunc
}

func (c *mockConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *mockConn) Close() error                        { return nil }
func (c *mockConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

func (c *mockConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.queryFn == nil {
		return &mockRows{columns: []string{"id", "user_id"}}, nil
	}
	return c.queryFn(query, args)
}

func (c *mockConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.execFn == nil {
		return driver.RowsAffected(1), nil
	}
	return c.execFn(query, args)
}

var _ driver.QueryerContext = (*mockConn)(nil)
var _ driver.ExecerContext = (*mockConn)(nil)

type mockRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *mockRows) Columns() []string { return r.columns }
func (r *mockRows) Close() error      { return nil }
func (r *mockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func newMockDB(t *testing.T, queryFn mockQueryFunc, execFn mockExecFunc) *sql.DB {
	t.Helper()
	name := "port_server_mockdb_" + strconvI64(mockDriverCounter.Add(1))
	sql.Register(name, &mockDriver{queryFn: queryFn, execFn: execFn})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	return db
}

func strconvI64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func newTestWSURL(t *testing.T, h *Handler) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /tunnel/connect", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/tunnel/connect"
}

func hashForTest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func TestServeHTTPAuthHandshakeValidKey(t *testing.T) {
	const rawKey = "test-api-key"
	db := newMockDB(t,
		func(query string, args []driver.NamedValue) (driver.Rows, error) {
			if !strings.Contains(query, "FROM api_keys") {
				t.Fatalf("unexpected query: %s", query)
			}
			if len(args) != 1 {
				t.Fatalf("expected one query arg, got %d", len(args))
			}
			if got, _ := args[0].Value.(string); got != hashForTest(rawKey) {
				t.Fatalf("unexpected hash argument: got %q", got)
			}
			return &mockRows{
				columns: []string{"id", "user_id"},
				rows:    [][]driver.Value{{int64(1001), int64(42)}},
			}, nil
		},
		nil,
	)
	defer func() { _ = db.Close() }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, db, false, false, time.Hour, 2*time.Hour, nil, 10<<20, false)

	wsURL := newTestWSURL(t, h)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawKey)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read tunnel ready: %v", err)
	}
	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode ready message: %v", err)
	}
	if msgType != protocol.TypeTunnelReady {
		t.Fatalf("message type = %d, want %d", msgType, protocol.TypeTunnelReady)
	}

	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("json.Unmarshal ready: %v", err)
	}
	tun, ok := mgr.Lookup(ready.Subdomain)
	if !ok {
		t.Fatalf("expected tunnel to be registered")
	}
	if !tun.Authed {
		t.Fatalf("expected tunnel.Authed=true")
	}
	if tun.UserID != 42 {
		t.Fatalf("tunnel.UserID = %d, want 42", tun.UserID)
	}
}

func TestServeHTTPAuthHandshakeRejectsBadKey(t *testing.T) {
	db := newMockDB(t,
		func(_ string, _ []driver.NamedValue) (driver.Rows, error) {
			return &mockRows{columns: []string{"id", "user_id"}, rows: nil}, nil
		},
		nil,
	)
	defer func() { _ = db.Close() }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, db, false, false, time.Hour, 2*time.Hour, nil, 10<<20, false)
	wsURL := newTestWSURL(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer bad-key")
	_, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		t.Fatalf("expected dial to fail for bad key")
	}
	if resp == nil {
		t.Fatalf("expected HTTP response on unauthorized handshake")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestServeHTTPLegacyUnauthedModePassesThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, nil, true, false, time.Hour, 2*time.Hour, nil, 10<<20, false)
	wsURL := newTestWSURL(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read tunnel ready: %v", err)
	}
	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode ready message: %v", err)
	}
	if msgType != protocol.TypeTunnelReady {
		t.Fatalf("message type = %d, want %d", msgType, protocol.TypeTunnelReady)
	}

	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("json.Unmarshal ready: %v", err)
	}
	tun, ok := mgr.Lookup(ready.Subdomain)
	if !ok {
		t.Fatalf("expected tunnel to be registered")
	}
	if tun.Authed {
		t.Fatalf("expected tunnel.Authed=false")
	}
	if tun.UserID != 0 {
		t.Fatalf("tunnel.UserID = %d, want 0", tun.UserID)
	}
}

// TestServeHTTPDBModeNoTokenRejects verifies that DB mode (db != nil) with no
// Authorization header and allowUnauthed=false returns 401.
func TestServeHTTPDBModeNoTokenRejects(t *testing.T) {
	db := newMockDB(t, nil, nil)
	defer func() { _ = db.Close() }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, db, false, false, time.Hour, 2*time.Hour, nil, 10<<20, false)
	wsURL := newTestWSURL(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatalf("expected dial to fail with no token in DB mode")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

// TestServeHTTPAllowUnauthedNoToken verifies that allowUnauthed=true accepts a
// connection with no token, and the tunnel is marked authed=false.
func TestServeHTTPAllowUnauthedNoToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, nil, true, false, time.Hour, 2*time.Hour, nil, 10<<20, false)
	wsURL := newTestWSURL(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msgType != protocol.TypeTunnelReady {
		t.Fatalf("msg type = %d, want TypeTunnelReady", msgType)
	}
	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	tun, ok := mgr.Lookup(ready.Subdomain)
	if !ok {
		t.Fatalf("tunnel not found")
	}
	if tun.Authed {
		t.Errorf("expected tunnel.Authed=false for unauthed connection")
	}
}

// TestServeHTTPUnauthedTTLCapped verifies that unauthed tunnels get a TTL capped
// at min(tunnelTTL, unauthedTTL).
func TestServeHTTPUnauthedTTLCapped(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	// tunnelTTL=24h, unauthedTTL=30m: effective should be 30m.
	h := NewHandler(mgr, logger, nil, true, false, 24*time.Hour, 30*time.Minute, nil, 10<<20, false)
	wsURL := newTestWSURL(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	_, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	tun, ok := mgr.Lookup(ready.Subdomain)
	if !ok {
		t.Fatalf("tunnel not found")
	}
	// ExpiresAt should be ~30m from now, not 24h.
	expectedMax := time.Now().Add(31 * time.Minute)
	if tun.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt = %v, expected capped at ~30m (before %v)", tun.ExpiresAt, expectedMax)
	}
}

// TestServeHTTPReservedSubdomainRejected verifies that a reserved --domain is
// rejected and a TunnelError is sent back to the client.
func TestServeHTTPReservedSubdomainRejected(t *testing.T) {
	const rawKey = "test-api-key"
	db := newMockDB(t,
		func(_ string, _ []driver.NamedValue) (driver.Rows, error) {
			return &mockRows{
				columns: []string{"id", "user_id"},
				rows:    [][]driver.Value{{int64(1), int64(1)}},
			}, nil
		},
		nil,
	)
	defer func() { _ = db.Close() }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, db, false, false, time.Hour, 2*time.Hour, []string{"admin"}, 10<<20, false)

	mux := http.NewServeMux()
	mux.Handle("GET /tunnel/connect", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/tunnel/connect?subdomain=admin"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+rawKey)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msgType != protocol.TypeTunnelError {
		t.Fatalf("msg type = %d, want TypeTunnelError (%d)", msgType, protocol.TypeTunnelError)
	}
	var tunErr protocol.TunnelError
	if err := json.Unmarshal(payload, &tunErr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !strings.Contains(tunErr.Error, "reserved") {
		t.Errorf("error = %q, want 'reserved'", tunErr.Error)
	}
}

// TestServeHTTPNoUnauthedRestrictionsAllowsDomain verifies that --no-unauthed-restrictions
// allows an unauthed user to use --domain.
func TestServeHTTPNoUnauthedRestrictionsAllowsDomain(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := tunnel.NewManager("tunnel.test", logger)
	h := NewHandler(mgr, logger, nil, true, true, 24*time.Hour, 30*time.Minute, nil, 10<<20, false)

	mux := http.NewServeMux()
	mux.Handle("GET /tunnel/connect", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/tunnel/connect?subdomain=mything"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msgType != protocol.TypeTunnelReady {
		t.Fatalf("msg type = %d, want TypeTunnelReady", msgType)
	}
	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ready.Subdomain != "mything" {
		t.Errorf("subdomain = %q, want mything", ready.Subdomain)
	}
	// TTL should not be capped: ExpiresAt should be ~24h, not 30m.
	tun, ok := mgr.Lookup(ready.Subdomain)
	if !ok {
		t.Fatalf("tunnel not found")
	}
	// Should be >1h from now (much more than the 30m unauthed cap).
	if !tun.ExpiresAt.After(time.Now().Add(time.Hour)) {
		t.Errorf("ExpiresAt = %v, expected > 1h from now (no restriction)", tun.ExpiresAt)
	}
}

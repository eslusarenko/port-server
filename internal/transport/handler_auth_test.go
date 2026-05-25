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
	mgr := tunnel.NewManager("tunnel.test", time.Hour, logger)
	h := NewHandler(mgr, logger, db, false, 10<<20, false)

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
	mgr := tunnel.NewManager("tunnel.test", time.Hour, logger)
	h := NewHandler(mgr, logger, db, false, 10<<20, false)
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
	mgr := tunnel.NewManager("tunnel.test", time.Hour, logger)
	h := NewHandler(mgr, logger, nil, true, 10<<20, false)
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

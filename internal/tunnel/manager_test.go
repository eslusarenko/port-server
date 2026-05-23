package tunnel

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager("test.localhost", time.Hour, slog.Default())
}

func testWebSocketConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		// Hold the connection open until test ends.
		<-r.Context().Done()
		_ = c.CloseNow()
	}))

	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String(), nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial websocket: %v", err)
	}

	return conn, func() {
		_ = conn.CloseNow()
		srv.Close()
	}
}

func TestManagerRegisterLookupRemove(t *testing.T) {
	m := testManager(t)
	conn, cleanup := testWebSocketConn(t)
	defer cleanup()

	tun, err := m.Register(conn, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tun.ID == "" {
		t.Fatal("tunnel ID is empty")
	}

	got, ok := m.Lookup(tun.ID)
	if !ok {
		t.Fatal("Lookup: tunnel not found")
	}
	if got != tun {
		t.Error("Lookup returned a different tunnel")
	}

	m.Remove(tun.ID)

	_, ok = m.Lookup(tun.ID)
	if ok {
		t.Error("Lookup: tunnel still found after Remove")
	}
}

func TestManagerLookupNotFound(t *testing.T) {
	m := testManager(t)

	_, ok := m.Lookup("nonexistent")
	if ok {
		t.Error("Lookup: found nonexistent tunnel")
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	m := testManager(t)

	const n = 50
	var wg sync.WaitGroup
	tunnelIDs := make(chan string, n)

	// Concurrent registers.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, cleanup := testWebSocketConn(t)
			defer cleanup()
			tun, err := m.Register(conn, "")
			if err != nil {
				t.Errorf("Register: %v", err)
				return
			}
			tunnelIDs <- tun.ID
		}()
	}
	wg.Wait()
	close(tunnelIDs)

	// Verify all were unique.
	seen := make(map[string]bool)
	for id := range tunnelIDs {
		if seen[id] {
			t.Errorf("duplicate tunnel ID: %s", id)
		}
		seen[id] = true
	}

	// Concurrent lookups and removes.
	for id := range seen {
		wg.Add(1)
		go func(sub string) {
			defer wg.Done()
			m.Lookup(sub)
			m.Remove(sub)
		}(id)
	}
	wg.Wait()
}

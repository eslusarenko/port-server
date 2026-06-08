package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
	serverapp "github.com/eslusarenko/port-server/internal/app"
	"github.com/eslusarenko/port-server/internal/config"
	"github.com/eslusarenko/port-server/internal/protocol"
)

// makeBody returns a deterministic body of n bytes.
func makeBody(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('A' + (i % 26))
	}
	return b
}

// startStreamingServer boots port-server with the given MaxBodySize and returns
// its address plus a cancel func.
func startStreamingServer(t *testing.T, maxBody int64) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	cfg := &config.Config{
		Addr:          addr,
		BaseDomain:    "tunnel.test",
		AllowUnauthed: true,
		TunnelTTL:     time.Hour,
		LogLevel:      "error",
		MaxBodySize:   maxBody,
		Ping:          config.PingConfig{Interval: 30 * time.Second, Timeout: 90 * time.Second},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = serverapp.New(cfg, slog.Default()).Run(ctx) }()
	waitForServer(t, addr)
	return addr, cancel
}

// dialTunnel connects a fake client and returns the connection + ready subdomain.
func dialTunnel(t *testing.T, ctx context.Context, addr string) (*websocket.Conn, string) {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, "ws://"+addr+"/tunnel/connect", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.SetReadLimit(64 << 20)
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ready: %v", err)
	}
	mt, _, payload, err := protocol.DecodeMessage(data)
	if err != nil || mt != protocol.TypeTunnelReady {
		t.Fatalf("expected TunnelReady, got type=%d err=%v", mt, err)
	}
	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	return conn, ready.Subdomain
}

func TestStreamingResponseExceedsMaxBody(t *testing.T) {
	const maxBody = 1 << 20 // 1 MB tunnel cap
	const bodyLen = 3 << 20 // 3 MB response — must NOT be capped
	body := makeBody(bodyLen)

	addr, cancelSrv := startStreamingServer(t, maxBody)
	defer cancelSrv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, subdomain := dialTunnel(t, ctx, addr)
	defer func() { _ = conn.CloseNow() }()

	// Fake client: stream the 3 MB body back in 64 KiB chunks with the origin's
	// real Content-Length in the Head frame.
	go func() {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		mt, reqID, _, err := protocol.DecodeMessage(data)
		if err != nil || mt != protocol.TypeHttpRequest {
			return
		}
		head := protocol.HttpResponseMeta{
			StatusCode:    200,
			Headers:       map[string][]string{"Content-Length": {strconv.Itoa(bodyLen)}},
			ContentLength: int64(bodyLen),
		}
		hp, _ := protocol.EncodeHttpMeta(head, nil)
		_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseHead, reqID, hp))
		for off := 0; off < len(body); off += 64 << 10 {
			end := off + (64 << 10)
			if end > len(body) {
				end = len(body)
			}
			_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseChunk, reqID, body[off:end]))
		}
		_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseEnd, reqID, nil))
	}()

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", "http://"+addr+"/big", nil)
	req.Host = subdomain + ".tunnel.test"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(got) != bodyLen {
		t.Fatalf("body length = %d, want %d", len(got), bodyLen)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body content mismatch")
	}
	if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(bodyLen) {
		t.Errorf("Content-Length = %q, want %d", cl, bodyLen)
	}
}

func TestStreamingResponseMidStreamAbort(t *testing.T) {
	const maxBody = 1 << 20
	const declared = 3 << 20 // promised in Content-Length
	const sent = 512 << 10   // actually sent before abort
	body := makeBody(sent)

	addr, cancelSrv := startStreamingServer(t, maxBody)
	defer cancelSrv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, subdomain := dialTunnel(t, ctx, addr)
	defer func() { _ = conn.CloseNow() }()

	go func() {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		mt, reqID, _, err := protocol.DecodeMessage(data)
		if err != nil || mt != protocol.TypeHttpRequest {
			return
		}
		head := protocol.HttpResponseMeta{
			StatusCode:    200,
			Headers:       map[string][]string{"Content-Length": {strconv.Itoa(declared)}},
			ContentLength: int64(declared),
		}
		hp, _ := protocol.EncodeHttpMeta(head, nil)
		_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseHead, reqID, hp))
		_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseChunk, reqID, body))
		// Abort: declared 3 MB, sent 512 KiB.
		_ = conn.Write(ctx, websocket.MessageBinary, protocol.EncodeMessage(protocol.TypeHttpResponseEnd, reqID, []byte("origin connection reset")))
	}()

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", "http://"+addr+"/abort", nil)
	req.Host = subdomain + ".tunnel.test"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Content-Length declared but fewer bytes delivered: the visitor must observe
	// an error, not a clean short "success".
	if _, readErr := io.ReadAll(resp.Body); readErr == nil {
		t.Fatalf("expected a read error on truncated body, got nil")
	}
}

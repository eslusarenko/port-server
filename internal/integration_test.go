package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	serverapp "github.com/eslusarenko/port-server/internal/app"
	"github.com/eslusarenko/port-server/internal/config"
	"github.com/eslusarenko/port-server/internal/protocol"
)

func TestEndToEndTunnel(t *testing.T) {
	// 1. Start a local "application" server.
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "works")
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "echo: %s", body)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "hello from local")
	}))
	defer localSrv.Close()

	// 2. Start port-server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serverAddr := listener.Addr().String()
	_ = listener.Close()

	logger := slog.Default()
	cfg := &config.Config{
		Addr:          serverAddr,
		BaseDomain:    "tunnel.test",
		AllowUnauthed: true,
		TunnelTTL:     time.Hour,
		LogLevel:      "debug",
		MaxBodySize:   10 << 20,
		Ping: config.PingConfig{
			Interval: 30 * time.Second,
			Timeout:  90 * time.Second,
		},
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- serverapp.New(cfg, logger).Run(serverCtx)
	}()

	// Wait for the server to be ready.
	waitForServer(t, serverAddr)

	// 3. Connect a tunnel client via WebSocket.
	wsURL := "ws://" + serverAddr + "/tunnel/connect"
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	conn, _, err := websocket.Dial(clientCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	conn.SetReadLimit(10 << 20)

	// Read TunnelReady.
	_, data, err := conn.Read(clientCtx)
	if err != nil {
		t.Fatalf("read tunnel ready: %v", err)
	}

	msgType, _, payload, err := protocol.DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msgType != protocol.TypeTunnelReady {
		t.Fatalf("expected TunnelReady, got %d", msgType)
	}

	var ready protocol.TunnelReady
	if err := json.Unmarshal(payload, &ready); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	t.Logf("tunnel ready: subdomain=%s url=%s", ready.Subdomain, ready.URL)

	// 4. Start a client read loop that forwards requests to the local server.
	localTarget, _ := url.Parse(localSrv.URL)
	httpClient := &http.Client{Timeout: 5 * time.Second}

	go func() {
		for {
			_, data, err := conn.Read(clientCtx)
			if err != nil {
				return
			}

			mt, reqID, pl, err := protocol.DecodeMessage(data)
			if err != nil || mt != protocol.TypeHttpRequest {
				continue
			}

			meta, body, err := protocol.DecodeHttpRequestMeta(pl)
			if err != nil {
				continue
			}

			// Forward to local server.
			u := *localTarget
			u.Path = meta.Path

			var bodyReader io.Reader
			if len(body) > 0 {
				bodyReader = strings.NewReader(string(body))
			}

			req, _ := http.NewRequestWithContext(clientCtx, meta.Method, u.String(), bodyReader)
			for k, vs := range meta.Headers {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				errPayload, _ := json.Marshal(protocol.RequestError{RequestID: reqID, Error: err.Error()})
				msg := protocol.EncodeMessage(protocol.TypeRequestError, 0, errPayload)
				_ = conn.Write(clientCtx, websocket.MessageBinary, msg)
				continue
			}

			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			respMeta := protocol.HttpResponseMeta{
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
			}
			respPayload, _ := protocol.EncodeHttpMeta(respMeta, respBody)
			msg := protocol.EncodeMessage(protocol.TypeHttpResponse, reqID, respPayload)
			_ = conn.Write(clientCtx, websocket.MessageBinary, msg)
		}
	}()

	// 5. Make HTTP requests to the tunnel endpoint on the server.
	tunnelHost := ready.Subdomain + ".tunnel.test"

	t.Run("GET", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://"+serverAddr+"/test-path", nil)
		req.Host = tunnelHost
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "hello from local" {
			t.Errorf("body = %q, want %q", body, "hello from local")
		}
		if resp.Header.Get("X-Test") != "works" {
			t.Errorf("X-Test = %q, want %q", resp.Header.Get("X-Test"), "works")
		}
	})

	t.Run("POST", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "http://"+serverAddr+"/data", strings.NewReader("test-body"))
		req.Host = tunnelHost
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "echo: test-body" {
			t.Errorf("body = %q, want %q", body, "echo: test-body")
		}
	})

	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get("http://" + serverAddr + "/health")
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://"+serverAddr+"/", nil)
		req.Host = "unknown.tunnel.test"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
		}
	})

	clientCancel()
	serverCancel()
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not start on %s", addr)
}

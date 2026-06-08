package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/eslusarenko/port-server/internal/protocol"
)

// Tunnel represents a single active tunnel to a connected client.
type Tunnel struct {
	ID        string
	CreatedAt time.Time
	Conn      *websocket.Conn
	UserID    int64
	Authed    bool
	ExpiresAt time.Time

	writeMu   sync.Mutex
	nextReqID atomic.Uint32
	bytesIn   atomic.Int64
	bytesOut  atomic.Int64
	mu        sync.Mutex
	pending   map[uint32]*pendingResponse
	closed    bool
}

// pendingResponse tracks one in-flight request awaiting a streamed response.
type pendingResponse struct {
	head chan headResult // buffered(1); ForwardRequest receives exactly one
	pw   *io.PipeWriter  // set when the Head frame arrives; guarded by Tunnel.mu
}

type headResult struct {
	meta protocol.HttpResponseMeta
	body io.ReadCloser
	err  error
}

// NewTunnel creates a Tunnel for the given WebSocket connection.
func NewTunnel(id string, conn *websocket.Conn, userID int64, authed bool, ttl time.Duration) *Tunnel {
	return &Tunnel{
		ID:        id,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		Conn:      conn,
		UserID:    userID,
		Authed:    authed,
		pending:   make(map[uint32]*pendingResponse),
	}
}

// ForwardRequest sends an HTTP request through the tunnel and returns the
// response headers plus a streaming body reader. The body is filled as chunks
// arrive from the client; reading it applies backpressure to the tunnel. The
// ctx controls how long to wait for the response headers.
func (t *Tunnel) ForwardRequest(ctx context.Context, meta protocol.HttpRequestMeta, body []byte) (protocol.HttpResponseMeta, io.ReadCloser, error) {
	reqID := t.nextReqID.Add(1)

	payload, err := protocol.EncodeHttpMeta(meta, body)
	if err != nil {
		return protocol.HttpResponseMeta{}, nil, fmt.Errorf("encode request: %w", err)
	}

	pr := &pendingResponse{head: make(chan headResult, 1)}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return protocol.HttpResponseMeta{}, nil, fmt.Errorf("tunnel closed")
	}
	t.pending[reqID] = pr
	t.mu.Unlock()

	msg := protocol.EncodeMessage(protocol.TypeHttpRequest, reqID, payload)

	t.writeMu.Lock()
	writeErr := t.Conn.Write(ctx, websocket.MessageBinary, msg)
	t.writeMu.Unlock()

	if writeErr != nil {
		t.mu.Lock()
		delete(t.pending, reqID)
		t.mu.Unlock()
		return protocol.HttpResponseMeta{}, nil, fmt.Errorf("write to tunnel: %w", writeErr)
	}
	t.bytesIn.Add(int64(len(body)))

	select {
	case <-ctx.Done():
		// Visitor gone before headers arrived. Drop the entry and, if the Head
		// frame already opened a pipe, close it so the read loop's pending
		// chunk write unblocks instead of deadlocking.
		t.mu.Lock()
		delete(t.pending, reqID)
		w := pr.pw
		t.mu.Unlock()
		if w != nil {
			_ = w.CloseWithError(ctx.Err())
		}
		return protocol.HttpResponseMeta{}, nil, ctx.Err()
	case res := <-pr.head:
		if res.err != nil {
			return protocol.HttpResponseMeta{}, nil, res.err
		}
		return res.meta, res.body, nil
	}
}

// HandleResponseHead delivers response headers and opens the streaming body.
func (t *Tunnel) HandleResponseHead(requestID uint32, meta protocol.HttpResponseMeta) {
	r, w := io.Pipe()
	t.mu.Lock()
	pr, ok := t.pending[requestID]
	if ok {
		pr.pw = w
	}
	t.mu.Unlock()
	if !ok {
		_ = w.Close()
		_ = r.Close()
		return
	}
	pr.head <- headResult{meta: meta, body: r}
}

// HandleResponseChunk writes a body chunk to the streaming response. The write
// blocks until the proxy reads, providing end-to-end backpressure.
func (t *Tunnel) HandleResponseChunk(requestID uint32, data []byte) {
	t.mu.Lock()
	var w *io.PipeWriter
	if pr, ok := t.pending[requestID]; ok {
		w = pr.pw
	}
	t.mu.Unlock()
	if w == nil {
		return
	}
	if _, err := w.Write(data); err != nil {
		return // reader closed (visitor gone); discard until End arrives
	}
	t.bytesOut.Add(int64(len(data)))
}

// HandleResponseEnd finalizes a streamed response. errMsg=="" means clean EOF.
func (t *Tunnel) HandleResponseEnd(requestID uint32, errMsg string) {
	t.mu.Lock()
	var w *io.PipeWriter
	if pr, ok := t.pending[requestID]; ok {
		w = pr.pw
		delete(t.pending, requestID)
	}
	t.mu.Unlock()
	if w == nil {
		return
	}
	if errMsg == "" {
		_ = w.Close()
	} else {
		_ = w.CloseWithError(fmt.Errorf("client: %s", errMsg))
	}
}

// HandleResponse dispatches a legacy one-shot response (backward compatibility
// with clients that predate streaming). The whole body arrives in one frame.
func (t *Tunnel) HandleResponse(requestID uint32, meta protocol.HttpResponseMeta, body []byte) {
	t.mu.Lock()
	pr, ok := t.pending[requestID]
	if ok {
		delete(t.pending, requestID)
	}
	t.mu.Unlock()

	if ok {
		t.bytesOut.Add(int64(len(body)))
		pr.head <- headResult{meta: meta, body: io.NopCloser(bytes.NewReader(body))}
	}
}

// HandleRequestError dispatches a pre-response error from the client.
func (t *Tunnel) HandleRequestError(requestID uint32, errMsg string) {
	t.mu.Lock()
	pr, ok := t.pending[requestID]
	if ok {
		delete(t.pending, requestID)
	}
	t.mu.Unlock()

	if ok {
		pr.head <- headResult{err: fmt.Errorf("client: %s", errMsg)}
	}
}

// BytesIn returns the total bytes received into the tunnel (request bodies).
func (t *Tunnel) BytesIn() int64 {
	return t.bytesIn.Load()
}

// BytesOut returns the total bytes sent out of the tunnel (response bodies).
func (t *Tunnel) BytesOut() int64 {
	return t.bytesOut.Load()
}

// Close tears down the tunnel, failing all pending requests.
func (t *Tunnel) Close() {
	t.mu.Lock()
	t.closed = true
	pending := t.pending
	t.pending = make(map[uint32]*pendingResponse)
	t.mu.Unlock()

	// Detached entries are no longer reachable by the read loop, so reading
	// pr.pw here is race-free (the field is only written under t.mu, before the
	// swap above).
	for _, pr := range pending {
		if pr.pw != nil {
			_ = pr.pw.CloseWithError(fmt.Errorf("tunnel closed"))
		} else {
			pr.head <- headResult{err: fmt.Errorf("tunnel closed")}
		}
	}

	_ = t.Conn.Close(websocket.StatusGoingAway, "tunnel closed")
}

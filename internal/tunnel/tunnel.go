package tunnel

import (
	"context"
	"fmt"
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

	writeMu   sync.Mutex
	nextReqID atomic.Uint32
	mu        sync.Mutex
	pending   map[uint32]chan responseResult
	closed    bool
}

type responseResult struct {
	Meta protocol.HttpResponseMeta
	Body []byte
	Err  error
}

// NewTunnel creates a Tunnel for the given WebSocket connection.
func NewTunnel(id string, conn *websocket.Conn) *Tunnel {
	return &Tunnel{
		ID:        id,
		CreatedAt: time.Now(),
		Conn:      conn,
		pending:   make(map[uint32]chan responseResult),
	}
}

// ForwardRequest sends an HTTP request through the tunnel and waits for the
// client's response. The ctx controls the overall timeout.
func (t *Tunnel) ForwardRequest(ctx context.Context, meta protocol.HttpRequestMeta, body []byte) (protocol.HttpResponseMeta, []byte, error) {
	reqID := t.nextReqID.Add(1)

	payload, err := protocol.EncodeHttpMeta(meta, body)
	if err != nil {
		return protocol.HttpResponseMeta{}, nil, fmt.Errorf("encode request: %w", err)
	}

	ch := make(chan responseResult, 1)
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return protocol.HttpResponseMeta{}, nil, fmt.Errorf("tunnel closed")
	}
	t.pending[reqID] = ch
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

	select {
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, reqID)
		t.mu.Unlock()
		return protocol.HttpResponseMeta{}, nil, ctx.Err()
	case res := <-ch:
		return res.Meta, res.Body, res.Err
	}
}

// HandleResponse dispatches a response from the client to the waiting
// ForwardRequest goroutine.
func (t *Tunnel) HandleResponse(requestID uint32, meta protocol.HttpResponseMeta, body []byte) {
	t.mu.Lock()
	ch, ok := t.pending[requestID]
	if ok {
		delete(t.pending, requestID)
	}
	t.mu.Unlock()

	if ok {
		ch <- responseResult{Meta: meta, Body: body}
	}
}

// HandleRequestError dispatches an error from the client for a specific request.
func (t *Tunnel) HandleRequestError(requestID uint32, errMsg string) {
	t.mu.Lock()
	ch, ok := t.pending[requestID]
	if ok {
		delete(t.pending, requestID)
	}
	t.mu.Unlock()

	if ok {
		ch <- responseResult{Err: fmt.Errorf("client: %s", errMsg)}
	}
}

// Close tears down the tunnel, failing all pending requests.
func (t *Tunnel) Close() {
	t.mu.Lock()
	t.closed = true
	pending := t.pending
	t.pending = make(map[uint32]chan responseResult)
	t.mu.Unlock()

	for _, ch := range pending {
		ch <- responseResult{Err: fmt.Errorf("tunnel closed")}
	}

	_ = t.Conn.Close(websocket.StatusGoingAway, "tunnel closed")
}

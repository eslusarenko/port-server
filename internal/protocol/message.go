// Package protocol defines the binary message format exchanged between
// port-client and port-server over a WebSocket tunnel.
//
// SYNC: keep in sync with client/internal/protocol/message.go
package protocol

// MessageType identifies the kind of frame sent over the WebSocket.
type MessageType uint8

const (
	TypeTunnelReady  MessageType = 0x01
	TypeTunnelError  MessageType = 0x02
	TypeHttpRequest  MessageType = 0x03
	TypeHttpResponse MessageType = 0x04
	TypeRequestError MessageType = 0x05
	TypePing         MessageType = 0x10
	TypePong         MessageType = 0x11
	TypeShutdown     MessageType = 0x12
)

// HeaderSize is the fixed binary header prepended to every WebSocket message.
//
//	byte 0:   MessageType (uint8)
//	bytes 1-4: RequestID  (uint32, big-endian)
//	bytes 5-7: reserved   (zero)
const HeaderSize = 8

// TunnelReady is sent from server to client after a tunnel is established.
type TunnelReady struct {
	Subdomain string `json:"subdomain"`
	URL       string `json:"url"`
}

// TunnelError is sent from server to client when tunnel creation fails.
type TunnelError struct {
	Error string `json:"error"`
}

// RequestError is sent from client to server when the local service is unreachable.
type RequestError struct {
	RequestID uint32 `json:"request_id"`
	Error     string `json:"error"`
}

// Shutdown is sent in either direction to signal a graceful close.
type Shutdown struct {
	Reason string `json:"reason,omitempty"`
}

// HttpRequestMeta carries HTTP request metadata inside an HttpRequest frame.
// The raw body follows immediately after the JSON-encoded metadata.
type HttpRequestMeta struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Host          string              `json:"host"`
	Headers       map[string][]string `json:"headers"`
	ContentLength int64               `json:"content_length"`
}

// HttpResponseMeta carries HTTP response metadata inside an HttpResponse frame.
type HttpResponseMeta struct {
	StatusCode    int                 `json:"status_code"`
	Headers       map[string][]string `json:"headers"`
	ContentLength int64               `json:"content_length"`
}

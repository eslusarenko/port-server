package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/eslusarenko/port-server/internal/protocol"
	"github.com/eslusarenko/port-server/internal/tunnel"
)

// Handler upgrades HTTP connections to WebSocket and manages tunnel lifecycles.
type Handler struct {
	manager *tunnel.Manager
	logger  *slog.Logger
	maxBody int64
	trustProxyHeaders bool
}

// NewHandler creates a new WebSocket tunnel handler.
func NewHandler(manager *tunnel.Manager, logger *slog.Logger, maxBody int64, trustProxyHeaders bool) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
		maxBody: maxBody,
		trustProxyHeaders: trustProxyHeaders,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow all origins since clients connect from various environments.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("websocket accept failed", "error", err)
		return
	}
	conn.SetReadLimit(h.maxBody + protocol.HeaderSize + 4 + 4096) // body + header + meta overhead

	tun, err := h.manager.Register(conn, r.URL.Query().Get("subdomain"))
	if err != nil {
		h.logger.Error("tunnel registration failed", "error", err)
		errPayload, _ := json.Marshal(protocol.TunnelError{Error: err.Error()})
		msg := protocol.EncodeMessage(protocol.TypeTunnelError, 0, errPayload)
		_ = conn.Write(r.Context(), websocket.MessageBinary, msg)
		_ = conn.Close(websocket.StatusInternalError, "registration failed")
		return
	}

	// Send TunnelReady to the client.
	baseDomain := h.manager.BaseDomain()
	ready := protocol.TunnelReady{
		Subdomain: tun.ID,
		URL:       "https://" + tun.ID + "." + baseDomain,
	}
	readyPayload, _ := json.Marshal(ready)
	readyMsg := protocol.EncodeMessage(protocol.TypeTunnelReady, 0, readyPayload)

	if err := conn.Write(r.Context(), websocket.MessageBinary, readyMsg); err != nil {
		h.logger.Error("failed to send TunnelReady", "subdomain", tun.ID, "error", err)
		h.manager.Remove(tun.ID)
		return
	}

	h.logger.Info("tunnel connected", "subdomain", tun.ID, "remote", clientIP(r, h.trustProxyHeaders))

	// Read loop: process messages from the client.
	h.readLoop(r.Context(), tun)

	h.manager.Remove(tun.ID)
	h.logger.Info("tunnel disconnected", "subdomain", tun.ID)
}

func (h *Handler) readLoop(ctx context.Context, tun *tunnel.Tunnel) {
	for {
		_, data, err := tun.Conn.Read(ctx)
		if err != nil {
			return
		}

		msgType, requestID, payload, err := protocol.DecodeMessage(data)
		if err != nil {
			h.logger.Warn("malformed message", "subdomain", tun.ID, "error", err)
			continue
		}

		switch msgType {
		case protocol.TypeHttpResponse:
			meta, body, err := protocol.DecodeHttpResponseMeta(payload)
			if err != nil {
				h.logger.Warn("malformed http response", "subdomain", tun.ID, "error", err)
				continue
			}
			tun.HandleResponse(requestID, meta, body)

		case protocol.TypeRequestError:
			var reqErr protocol.RequestError
			if err := json.Unmarshal(payload, &reqErr); err != nil {
				h.logger.Warn("malformed request error", "subdomain", tun.ID, "error", err)
				continue
			}
			tun.HandleRequestError(reqErr.RequestID, reqErr.Error)

		case protocol.TypePing:
			pong := protocol.EncodeMessage(protocol.TypePong, 0, nil)
			_ = tun.Conn.Write(ctx, websocket.MessageBinary, pong)

		case protocol.TypeShutdown:
			h.logger.Info("client initiated shutdown", "subdomain", tun.ID)
			return

		default:
			h.logger.Warn("unknown message type", "subdomain", tun.ID, "type", msgType)
		}
	}
}

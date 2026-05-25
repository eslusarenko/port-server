package transport

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	dbpkg "github.com/eslusarenko/port-server/internal/db"
	"github.com/eslusarenko/port-server/internal/httputil"
	"github.com/eslusarenko/port-server/internal/protocol"
	"github.com/eslusarenko/port-server/internal/tunnel"
)

// Handler upgrades HTTP connections to WebSocket and manages tunnel lifecycles.
type Handler struct {
	manager           *tunnel.Manager
	logger            *slog.Logger
	db                *sql.DB
	allowUnauthed     bool
	maxBody           int64
	trustProxyHeaders bool
}

// NewHandler creates a new WebSocket tunnel handler.
func NewHandler(manager *tunnel.Manager, logger *slog.Logger, database *sql.DB, allowUnauthed bool, maxBody int64, trustProxyHeaders bool) *Handler {
	return &Handler{
		manager:           manager,
		logger:            logger,
		db:                database,
		allowUnauthed:     allowUnauthed,
		maxBody:           maxBody,
		trustProxyHeaders: trustProxyHeaders,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := httputil.ClientIP(r, h.trustProxyHeaders)
	ua := r.Header.Get("User-Agent")

	rawKey := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		rawKey = strings.TrimPrefix(auth, "Bearer ")
	}

	var (
		userID int64
		authed bool
		keyID  int64
	)

	if rawKey != "" {
		if h.db == nil {
			http.Error(w, "authentication not configured", http.StatusUnauthorized)
			return
		}
		res, err := dbpkg.LookupAPIKey(r.Context(), h.db, rawKey)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID = res.UserID
		keyID = res.KeyID
		authed = true
	} else if !h.allowUnauthed {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow all origins since clients connect from various environments.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("websocket_accept_failed", "error", err)
		return
	}
	conn.SetReadLimit(h.maxBody + protocol.HeaderSize + 4 + 4096) // body + header + meta overhead

	tun, err := h.manager.Register(conn, r.URL.Query().Get("subdomain"), userID, authed)
	if err != nil {
		h.logger.Error("tunnel_register_failed", "error", err)
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
		h.logger.Error("tunnel_ready_send_failed", "subdomain", tun.ID, "error", err)
		h.manager.Remove(tun.ID)
		return
	}

	startTime := time.Now()
	closeReason := "client_disconnect"

	if authed {
		go dbpkg.UpdateLastUsed(h.db, keyID)
	}

	logArgs := []any{
		"subdomain", tun.ID,
		"client_ip", ip,
		"user_agent", ua,
		"authed", authed,
	}
	if authed {
		logArgs = append(logArgs, "user_id", userID)
	}
	h.logger.Info("tunnel_open", logArgs...)

	defer func() {
		h.logger.Info("tunnel_close",
			"subdomain", tun.ID,
			"client_ip", ip,
			"duration_seconds", time.Since(startTime).Seconds(),
			"bytes_in", tun.BytesIn(),
			"bytes_out", tun.BytesOut(),
			"reason", closeReason,
		)
	}()

	// Read loop: process messages from the client.
	h.readLoop(r.Context(), tun, &closeReason)

	h.manager.Remove(tun.ID)
}

func (h *Handler) readLoop(ctx context.Context, tun *tunnel.Tunnel, closeReason *string) {
	for {
		_, data, err := tun.Conn.Read(ctx)
		if err != nil {
			return
		}

		msgType, requestID, payload, err := protocol.DecodeMessage(data)
		if err != nil {
			h.logger.Warn("malformed_message", "subdomain", tun.ID, "error", err)
			continue
		}

		switch msgType {
		case protocol.TypeHttpResponse:
			meta, body, err := protocol.DecodeHttpResponseMeta(payload)
			if err != nil {
				h.logger.Warn("malformed_http_response", "subdomain", tun.ID, "error", err)
				continue
			}
			tun.HandleResponse(requestID, meta, body)

		case protocol.TypeRequestError:
			var reqErr protocol.RequestError
			if err := json.Unmarshal(payload, &reqErr); err != nil {
				h.logger.Warn("malformed_request_error", "subdomain", tun.ID, "error", err)
				continue
			}
			tun.HandleRequestError(reqErr.RequestID, reqErr.Error)

		case protocol.TypePing:
			pong := protocol.EncodeMessage(protocol.TypePong, 0, nil)
			_ = tun.Conn.Write(ctx, websocket.MessageBinary, pong)

		case protocol.TypeShutdown:
			*closeReason = "client_disconnect"
			return

		default:
			h.logger.Warn("unknown_message_type", "subdomain", tun.ID, "type", msgType)
		}
	}
}

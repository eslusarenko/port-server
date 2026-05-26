# Operations

- [Health check](#health-check)
- [Logging](#logging)
  - [Log routing](#log-routing)
  - [Structured log event schema](#structured-log-event-schema)
- [Tunnel lifecycle from the operator's perspective](#tunnel-lifecycle-from-the-operators-perspective)
- [DB schema and migrations](#db-schema-and-migrations)
- [Graceful shutdown](#graceful-shutdown)

## Health check

```
GET /health  →  200 OK  body: ok
```

Used by both the readiness probe and the liveness probe in the Kubernetes deployment. No authentication required. Returns 200 as long as the HTTP server is accepting connections — it does not check DB connectivity.

## Logging

### Log routing

| Setting | Effect |
|---------|--------|
| `PORT_LOG_TYPE=plain` (default) | Human-readable text to stderr (or log file) |
| `PORT_LOG_TYPE=json` | NDJSON to stderr (or log file) — use in log aggregation pipelines |
| `PORT_LOG_TYPE=silent` | All output discarded |
| `PORT_LOG_FILENAME=/path/to/file` | Write to file instead of stderr (append mode, created if absent) |

The slog `msg` field is renamed to `event` in all outputs. Every log line includes `time` and `level` in addition to the event-specific fields listed below.

### Structured log event schema

#### Server lifecycle

| Event | Level | Extra fields |
|-------|-------|-------------|
| `server_starting` | INFO | `addr`, `domain` |
| `server_stopping` | INFO | — |
| `server_failed` | ERROR | `error` |

#### Tunnel lifecycle

| Event | Level | Extra fields |
|-------|-------|-------------|
| `tunnel_open` | INFO | `subdomain`, `client_ip` (port stripped), `user_agent`, `authed` (bool), `user_id` (omitted when unauthed) |
| `tunnel_close` | INFO | `subdomain`, `client_ip`, `duration_seconds`, `bytes_in`, `bytes_out`, `reason` |

`reason` values for `tunnel_close`:

| Value | Meaning |
|-------|---------|
| `client_disconnect` | Client closed the WebSocket connection normally or sent a Shutdown message |
| `expired` | Tunnel's TTL elapsed; server evicted it |
| `shutdown` | Server is shutting down gracefully |

Byte direction convention: **visitor's perspective**.
- `bytes_in` — bytes into the tunnel from the public visitor (request body).
- `bytes_out` — bytes out of the tunnel to the visitor (response body).

#### HTTP traffic

| Event | Level | Extra fields |
|-------|-------|-------------|
| `http_request` | INFO | `subdomain`, `method`, `path`, `status`, `duration_ms`, `bytes_in`, `bytes_out`, `client_ip` |

#### Errors and warnings

| Event | Level | Notes |
|-------|-------|-------|
| `websocket_accept_failed` | ERROR | WebSocket upgrade failed |
| `tunnel_register_failed` | ERROR | Tunnel registration failed (includes reserved subdomain, auth errors) |
| `tunnel_ready_send_failed` | ERROR | Could not deliver TunnelReady to client |
| `forward_request_failed` | ERROR | Proxy could not forward an HTTP request |
| `malformed_message` | WARN | Unparseable binary frame from client |
| `malformed_http_response` | WARN | Client sent a malformed HTTP response payload |
| `malformed_request_error` | WARN | Client sent a malformed RequestError payload |
| `unknown_message_type` | WARN | Client sent an unrecognized message type code |

## Tunnel lifecycle from the operator's perspective

Each tunnel produces exactly one `tunnel_open` and one `tunnel_close` event, regardless of how many HTTP requests it proxies.

```
Client connects via WebSocket
  → Token validated (if DB mode)
  → Subdomain assigned (random or client-requested)
  → tunnel_open logged
  → http_request logged per forwarded request
  → ...
Client disconnects  →  tunnel_close reason=client_disconnect
  OR
TTL elapsed         →  tunnel_close reason=expired       (eviction runs every 60 s)
  OR
SIGINT/SIGTERM      →  tunnel_close reason=shutdown      (after graceful shutdown starts)
```

To distinguish a client that went away unexpectedly from one that shut down cleanly: both currently show `reason=client_disconnect`. The distinction is visible in client-side logs, not server-side.

## DB schema and migrations

Migrations are embedded in the binary and run automatically on startup via `golang-migrate`. No external migration tool is needed. The current schema (migration `000001_init`):

```sql
users (
    id          BIGINT UNSIGNED PK AUTO_INCREMENT,
    email       VARCHAR(255) UNIQUE NOT NULL,
    name        VARCHAR(255) NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    status      VARCHAR(32) DEFAULT 'active'
)

api_keys (
    id           BIGINT UNSIGNED PK AUTO_INCREMENT,
    user_id      BIGINT UNSIGNED FK → users(id),
    key_hash     CHAR(64) UNIQUE NOT NULL,   -- hex SHA-256 of plaintext key
    label        VARCHAR(255) DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME NULL,
    revoked_at   DATETIME NULL
)

subdomain_reservations (
    subdomain    VARCHAR(63) PK,
    user_id      BIGINT UNSIGNED FK → users(id),
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)   -- present in schema; not yet used by the server
```

`key_hash` stores the hex-encoded SHA-256 of the plaintext API key. Plaintext is printed once by `admin create-key` and never stored.

`last_used_at` is updated asynchronously after every successful key lookup; a slow DB write never delays tunnel establishment.

## Graceful shutdown

On `SIGINT` or `SIGTERM`:
1. The server stops accepting new connections.
2. All active tunnels are closed (`reason=shutdown` in `tunnel_close` logs).
3. `server_stopping` is logged.
4. The HTTP server drains in-flight requests before exiting.

---

See [configuration.md](configuration.md) for log-related config keys. See [deployment.md](deployment.md) for Kubernetes probe configuration.

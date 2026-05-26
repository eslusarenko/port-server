# Authentication

- [Three modes](#three-modes)
- [Startup rules](#startup-rules)
- [Authed vs unauthed capabilities](#authed-vs-unauthed-capabilities)
- [Reserved subdomains](#reserved-subdomains)
- [Removing restrictions — the kill switch](#removing-restrictions--the-kill-switch)
- [What clients see on rejection](#what-clients-see-on-rejection)

## Three modes

| Mode | `PORT_DB_DSN` | `--allow-unauthed` | Who can connect |
|------|--------------|-------------------|-----------------|
| **Strict DB** (default with DB) | set | not set | Clients with a valid API key only |
| **DB + guests** | set | set | API-key clients + unauthenticated clients (restrictions apply) |
| **Legacy no-DB** | not set | set | All clients, unauthenticated (restrictions apply unless `--no-unauthed-restrictions`) |

A fourth state — `PORT_DB_DSN` unset and `--allow-unauthed` not set — causes **port-server to refuse to start** with an explicit error. This is intentional: operators must make an active choice.

## Startup rules

```
PORT_DB_DSN set
  → DB mode. Migrations run. Token validation active.
  → If --allow-unauthed is NOT set: connections without Authorization → 401.
  → If --allow-unauthed is set:     connections without Authorization → accepted as unauthed.

PORT_DB_DSN not set + --allow-unauthed set
  → Legacy mode. No DB. All connections accepted as unauthed.

PORT_DB_DSN not set + --allow-unauthed not set
  → Fatal error at startup.
```

## Authed vs unauthed capabilities

### Authenticated tunnels

A tunnel is authenticated when:
- The client sends `Authorization: Bearer <key>` during the WebSocket upgrade.
- The key is found in `api_keys` and `revoked_at IS NULL`.

Authenticated tunnels receive:
- Full `PORT_TUNNEL_TTL` (default 24 h).
- Ability to request a specific subdomain with `--domain <name>` (unless the name is in the reserved list).

### Unauthenticated tunnels

A connection is unauthenticated when no `Authorization` header is present and `--allow-unauthed` is set.

Unauthenticated tunnels receive:
- TTL capped at `PORT_UNAUTHED_TTL` (default 2 h). Effective TTL = `min(PORT_TUNNEL_TTL, PORT_UNAUTHED_TTL)`.
- `--domain` requests rejected with a client-facing error.
- The reserved subdomain list still applies (defense-in-depth).

### Bad / revoked keys

If a client sends a `Bearer` token that is not in `api_keys` or has a non-null `revoked_at`, the server returns `401 unauthorized` immediately. A failed explicit auth attempt is **never** silently downgraded to unauthenticated — even when `--allow-unauthed` is set.

`api_keys.last_used_at` is updated asynchronously after a successful lookup; slow DB writes never delay tunnel establishment.

## Reserved subdomains

The reserved list is purely operator-supplied via `PORT_RESERVED_SUBDOMAINS` (comma-separated). An empty list means no enforcement.

Recommended baseline for public deployments (mirrors the production manifest in `deploy/server/deployment.yaml`):

```
admin, login, dashboard, billing, support, donate, office, api, www,
mail, status, auth, account, accounts, secure, portal, help, docs,
pay, payment, payments, shop, store
```

Matching is case-insensitive. Reserved subdomains apply to `--domain` requests for both authed and unauthed clients (authed clients get a clear error returned over the tunnel protocol, not a silent random subdomain assignment).

## Removing restrictions — the kill switch

```bash
PORT_NO_UNAUTHED_RESTRICTIONS=true
# or
port-server --no-unauthed-restrictions
```

When set, unauthenticated tunnels receive the same TTL as authenticated ones and may use `--domain`. Reserved subdomain checks still apply. This is intended for single-operator setups where the "unauthenticated" client is the operator themselves.

## What clients see on rejection

| Scenario | HTTP status | Response body |
|----------|-------------|---------------|
| No `Authorization` header, strict DB mode | 401 | `unauthorized` |
| `Authorization` header present, no DB configured | 401 | `authentication not configured` |
| `Authorization` header present, key not found or revoked | 401 | `unauthorized` |
| `--domain` requested by unauthed client (no override) | — | TunnelError message: `--domain requires authentication` |
| `--domain` is in the reserved list | — | TunnelError message: `subdomain "x" is reserved and cannot be used` |
| `--domain` is already in use | — | TunnelError message: `subdomain "x" is already in use` |

TunnelError messages are delivered over the WebSocket tunnel protocol before the connection is closed — not as HTTP errors.

---

See [admin.md](admin.md) for how to create users and API keys. See [configuration.md](configuration.md) for all related config keys.

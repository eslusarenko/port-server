# Configuration

- [Priority chain](#priority-chain)
- [Config file lookup](#config-file-lookup)
- [All keys](#all-keys)
- [Minimal config examples](#minimal-config-examples)

## Priority chain

Resolution order (lowest to highest priority):

```
defaults → config file → CLI flags → environment variables
```

Environment variables always win. Orchestration systems (Kubernetes, systemd) can override any setting without touching the config file or binary flags.

## Config file lookup

By default port-server looks for `port-server.conf` in the same directory as the binary. Override with:

```bash
port-server --config /etc/port-server/port-server.conf
```

The file format is `KEY=VALUE`, one per line. Lines starting with `#` and blank lines are ignored. Unknown keys are skipped with a warning to stderr. Bad values for typed keys (durations, integers, booleans) are a fatal error.

Boolean keys accept: `true`, `false`, `1`, `0`, `yes`, `no`.

## All keys

| Env var | CLI flag | Default | Description |
|---------|----------|---------|-------------|
| `PORT_ADDR` | `--addr` | `:8080` | TCP listen address. |
| `PORT_BASE_DOMAIN` | `--base-domain` | `tunnel.localhost` | Base domain. Tunnels are assigned `<subdomain>.<base-domain>`. |
| `PORT_TUNNEL_TTL` | `--tunnel-ttl` | `24h` | How long a tunnel lives before the server closes it. Go duration string (`24h`, `30m`, etc.). |
| `PORT_LOG_LEVEL` | `--log-level` | `info` | Verbosity: `debug`, `info`, `warn`, `error`. |
| `PORT_LOG_TYPE` | `--log-type` | `plain` | Output format: `plain` (text), `json`, `silent`. The `--json-logs` shortcut is equivalent to `--log-type=json`. |
| `PORT_LOG_FILENAME` | `--log-file` | `` (stderr) | Write logs to this file instead of stderr. |
| `PORT_MAX_BODY_SIZE` | `--max-body-size` | `10485760` | Maximum HTTP request body size in bytes (default 10 MiB). |
| `PORT_PING_INTERVAL` | `--ping-interval` | `30s` | WebSocket ping interval. |
| `PORT_PING_TIMEOUT` | `--ping-timeout` | `1m30s` | WebSocket ping timeout. |
| `PORT_TRUST_PROXY_HEADERS` | `--trust-proxy-headers` | `false` | Trust `X-Forwarded-For` / `X-Real-IP`. Enable only when port-server is behind a reverse proxy and NOT directly reachable from the internet. |
| `PORT_DB_DSN` | `--db-dsn` | `` | MySQL DSN. Required unless `--allow-unauthed` is set. Format: `user:pass@tcp(host:3306)/dbname?parseTime=true`. |
| `PORT_ALLOW_UNAUTHED` | `--allow-unauthed` | `false` | Accept tunnel connections without an `Authorization` header. In no-DB mode, this is required. In DB mode, unauthenticated connections are accepted alongside authenticated ones. |
| `PORT_UNAUTHED_TTL` | `--unauthed-ttl` | `2h` | TTL cap for unauthenticated tunnels. Effective TTL = `min(PORT_TUNNEL_TTL, PORT_UNAUTHED_TTL)`. Only enforced when `PORT_ALLOW_UNAUTHED=true` and `PORT_NO_UNAUTHED_RESTRICTIONS` is not set. |
| `PORT_RESERVED_SUBDOMAINS` | `--reserved-subdomains` | `` | Comma-separated subdomains to block from `--domain` requests. Operator-supplied — no built-in defaults. Empty = no enforcement. |
| `PORT_NO_UNAUTHED_RESTRICTIONS` | `--no-unauthed-restrictions` | `false` | Remove the TTL cap and `--domain` block for unauthenticated tunnels. Useful for single-operator deployments where unauthenticated means trusted. |

## Minimal config examples

### No-DB (single operator)

```ini
PORT_BASE_DOMAIN=tunnel.example.com
PORT_ALLOW_UNAUTHED=true
PORT_NO_UNAUTHED_RESTRICTIONS=true
PORT_LOG_TYPE=plain
```

### DB mode — strict

```ini
PORT_BASE_DOMAIN=tunnel.example.com
PORT_DB_DSN=port:secret@tcp(127.0.0.1:3306)/port?parseTime=true
PORT_LOG_TYPE=json
PORT_TRUST_PROXY_HEADERS=true
```

### DB mode — allow unauthenticated guests

```ini
PORT_BASE_DOMAIN=tunnel.example.com
PORT_DB_DSN=port:secret@tcp(127.0.0.1:3306)/port?parseTime=true
PORT_ALLOW_UNAUTHED=true
PORT_UNAUTHED_TTL=1h
PORT_LOG_TYPE=json
PORT_TRUST_PROXY_HEADERS=true
```

---

See [authentication.md](authentication.md) for how these settings combine to control access.

# port-server

[![Go Report Card](https://goreportcard.com/badge/github.com/eslusarenko/port-server)](https://goreportcard.com/report/github.com/eslusarenko/port-server)
[![Release](https://img.shields.io/github/v/release/eslusarenko/port-server)](https://github.com/eslusarenko/port-server/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/eslusarenko/port-server.svg)](https://pkg.go.dev/github.com/eslusarenko/port-server)

Server component of [port](https://github.com/eslusarenko/port) — a self-hosted tunneling tool that exposes local ports over public subdomains.

`port-server` runs on a public host. The `port` client connects to it over WebSocket and registers a tunnel. Public HTTP traffic arriving at `<subdomain>.<base-domain>` is forwarded through the tunnel to the local port on the client machine. Clients can be any language; the protocol is a simple binary framing over WebSocket.

## Key features

- **Language-agnostic clients** — tunnels use a binary-framed WebSocket protocol; any client that speaks WebSocket can implement it.
- **Three auth modes** — full DB-backed authentication, DB mode with unauthenticated guests allowed, or legacy no-DB mode for single-operator use.
- **Per-tunnel TTL** — authed tunnels get the full configured TTL; unauthenticated tunnels are capped at a shorter TTL (default 2 h) to limit abuse.
- **Subdomain selection** — authenticated clients may request a specific subdomain (`--domain`); unauthenticated clients receive a random 8-character subdomain.
- **Reserved subdomain list** — a built-in set of names (admin, api, www, …) plus an operator-configurable extension list.
- **Structured logging** — every significant event is a discrete slog event with typed fields; JSON mode available for log pipelines.
- **Automatic DB migrations** — schema is embedded in the binary; migrations run at startup without a separate tool.

## Quickstart

### Legacy mode (no DB, no auth)

Suitable for a single trusted operator. All tunnels are accepted; unauthed restrictions apply by default (2 h TTL, no custom subdomains) unless `--no-unauthed-restrictions` is also set.

```bash
port-server --allow-unauthed --base-domain tunnel.example.com
```

### DB mode — allow unauthenticated guests

Runs DB-backed auth but also accepts connections without a token. Authenticated clients get full capabilities; unauthenticated clients are restricted.

```bash
PORT_DB_DSN='user:pass@tcp(127.0.0.1:3306)/port?parseTime=true' \
  port-server --base-domain tunnel.example.com --allow-unauthed
```

### DB mode — strict (recommended for production)

Only clients presenting a valid API key are admitted.

```bash
PORT_DB_DSN='user:pass@tcp(127.0.0.1:3306)/port?parseTime=true' \
  port-server --base-domain tunnel.example.com
```

Provision a user and key:

```bash
PORT_DB_DSN='...' port-server admin create-user --email ops@example.com --name ops
# user_id=1
PORT_DB_DSN='...' port-server admin create-key --user-id 1 --label laptop
# key_id=1
# key=<64-hex-chars>   ← shown once; store it
```

Then pass the key from the client side:

```bash
PORT_API_KEY=<key> port expose 8080
```

## Install

```bash
go install github.com/eslusarenko/port-server@latest
```

Or download a pre-built binary from [Releases](https://github.com/eslusarenko/port-server/releases).

## Build from source

```bash
cd server
make build   # → bin/port-server
```

## Documentation

| Topic | File |
|-------|------|
| All configuration keys, priority chain, example `.conf` files | [docs/configuration.md](docs/configuration.md) |
| Auth model — strict, guest, legacy, restrictions | [docs/authentication.md](docs/authentication.md) |
| Admin subcommands (`create-user`, `create-key`, …) | [docs/admin.md](docs/admin.md) |
| Health checks, structured log events, tunnel lifecycle | [docs/operations.md](docs/operations.md) |
| Kubernetes deployment (manifests, secrets, ingress) | [docs/deployment.md](docs/deployment.md) |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)

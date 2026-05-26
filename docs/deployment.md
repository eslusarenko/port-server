# Deployment

Kubernetes manifests live in [`../../deploy/`](../../deploy/). This document covers the moving parts, required secrets, gotchas, and verification steps.

- [Architecture overview](#architecture-overview)
- [Prerequisites](#prerequisites)
- [Secrets](#secrets)
- [Apply order](#apply-order)
- [Image tags](#image-tags)
- [Ingress layout](#ingress-layout)
- [Health probes](#health-probes)
- [Resource limits](#resource-limits)
- [Gotchas](#gotchas)

## Architecture overview

```
Internet
  │
Traefik ingress (Kubernetes)
  ├── pm.tnls.lt          → port-server :8080   (tunnel control plane — clients connect here)
  └── *.tnls.lt           → port-server :8080   (tunnel proxy — public HTTP traffic)

port-server (Deployment, 1 replica, namespace: port)
  └── mysql (Deployment, 1 replica, namespace: port)
        └── mysql-data PVC (20 Gi, ReadWriteOnce)
```

All components run in the `port` namespace.

## Prerequisites

- Kubernetes cluster with Traefik ingress controller (`traefik.io/v1alpha1` CRDs).
- cert-manager in the `cert-manager` namespace.
- DNS: `*.tnls.lt` and `tnls.lt` pointing to the cluster ingress IP (managed via Cloudflare in the reference setup).
- A default StorageClass capable of `ReadWriteOnce` volumes (for the MySQL PVC).
- Image pull credentials for the private registry (`regcred` secret in the `port` namespace).

## Secrets

### 1. Cloudflare API token (`deploy/shared/cloudflare-secret.yaml`)

Used by cert-manager for DNS-01 challenge to issue a wildcard certificate for `*.tnls.lt`. Replace the placeholder `api-token` value before applying.

### 2. MySQL credentials (`deploy/mysql/secret.yaml`)

Three keys must be filled in:

| Key | Description |
|-----|-------------|
| `root-password` | MySQL root password (used only during initial DB creation) |
| `password` | Password for the `port` application user |
| `dsn` | Full Go DSN: `port:<PASSWORD>@tcp(mysql:3306)/port?parseTime=true` |

The `dsn` and `password` values must match. The secret must be in the `port` namespace — both the MySQL Deployment and port-server Deployment reference it by name (`mysql-credentials`).

### 3. Image registry pull secret

Not tracked in the manifests. Create it directly in the cluster:

```bash
kubectl create secret docker-registry regcred \
  --docker-server=<REGISTRY_HOST> \
  --docker-username=<USERNAME> \
  --docker-password=<PASSWORD_OR_TOKEN> \
  --namespace=port
```

## Apply order

```bash
# 1. Namespace, cert-manager issuer, wildcard TLS cert, HTTPS redirect middleware
kubectl apply -f deploy/shared/

# 2. Wait for TLS certificate
kubectl -n port get certificate tnls-lt-wildcard -w

# 3. MySQL (PVC, Secret, Deployment, Service)
kubectl apply -f deploy/mysql/

# 4. Wait for MySQL to be ready
kubectl -n port rollout status deployment/mysql

# 5. port-server (Deployment, Service, IngressRoutes)
kubectl apply -f deploy/server/
```

port-server's Deployment includes an init container (`busybox:1.36`) that polls `nc -z mysql 3306` every 2 seconds before starting the main container. This guards against a race where port-server starts before MySQL is ready.

## Image tags

The `port-server` Deployment references:

```
registry.k8s.rootlabs.eu/port/port-server:latest
```

`:latest` is used in the manifests. For production, pin to a specific semver tag or image digest by editing the `image:` field in `deploy/server/deployment.yaml`:

```yaml
image: registry.k8s.rootlabs.eu/port/port-server:v0.1.5
```

Releases are tagged automatically by GoReleaser when a `vX.Y.Z` git tag is pushed from the `server/` repo.

## Ingress layout

Two sets of IngressRoutes are defined (HTTP + HTTPS for each):

| IngressRoute | Match | Purpose |
|---|---|---|
| `port-server-pm` | `Host(pm.tnls.lt)` | Control plane — `port expose` WebSocket connections |
| `port-server-tunnels` | `HostRegexp(^[a-z0-9-]+\.tnls\.lt$)` | Tunnel proxy — public HTTP traffic to exposed local services |

Both target the `port-server` Service on port 8080. HTTP routes include a Traefik `https-redirect` middleware (defined in `deploy/shared/middleware.yaml`).

The TLS secret (`tnls-lt-tls`) is a wildcard certificate provisioned by cert-manager using DNS-01 (Cloudflare). It covers both `tnls.lt` and `*.tnls.lt`.

## Health probes

Both probes hit `GET /health` on port 8080:

| Probe | Initial delay | Period |
|-------|--------------|--------|
| Readiness | 3 s | 10 s |
| Liveness | 5 s | 30 s |

The `/health` endpoint returns `200 ok` as long as the HTTP server is running. It does not check DB connectivity. If the DB is unavailable after startup the server continues to run but new authenticated tunnel connections will fail at the key-lookup step.

## Resource limits

| Container | CPU request | CPU limit | Memory request | Memory limit |
|-----------|------------|-----------|---------------|-------------|
| port-server | 50 m | 500 m | 64 MiB | 256 MiB |
| mysql | 100 m | 500 m | 256 MiB | 512 MiB |

## Gotchas

**DSN format** — the Go MySQL driver requires `parseTime=true` for `DATETIME` columns to scan correctly. Missing this parameter will cause migration or query errors. The DSN in `secret.yaml` must include it.

**`mysql` hostname in DSN** — the DSN uses the short hostname `mysql`, which resolves to `mysql.port.svc.cluster.local` within the `port` namespace. If you change the MySQL Service name, update the DSN accordingly.

**Init container vs readiness probe** — the init container in port-server's pod only checks TCP reachability of MySQL port 3306. MySQL's own readiness probe (`mysqladmin ping`) gates the MySQL Deployment rollout, but by the time port-server's init container runs, MySQL may still be initializing the database. The combination of the two probes makes cold-start races very unlikely in practice.

**`:latest` in production** — the manifests use `:latest` by default. This means a `kubectl rollout restart` will pull whatever is currently tagged latest, which may not be what you expect. Pin to a digest or semver tag for reproducible deployments.

**Single replica** — both MySQL and port-server run as single replicas. Active tunnels are in-process state in port-server; there is no cross-replica session sharing. Scaling port-server to more than one replica would split tunnel state across pods. For high availability, a sticky ingress or a shared session store would be needed (not currently implemented).

---

See [configuration.md](configuration.md) for all environment-variable settings injected via the Deployment manifest.

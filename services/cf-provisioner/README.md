# CF-Provisioner

HTTP service that provisions tenant private networks (vCluster, CIDR, Cilium, kubeconfig in OpenBao)
and optional internet gateways (Gateway API + Cilium). Intended for **internal control-plane** calls
only; authenticate with `X-CF-Internal-Secret`.

## Run locally

From repo root (with `go.work`):

```bash
cd services/cf-provisioner
go run .
```

Requires ScyllaDB, OpenBao, and a valid host-cluster kubeconfig (see env vars below).

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:8082` | HTTP listen address |
| `SCYLLADB_HOSTS` | `localhost:9042` | Comma-separated ScyllaDB contact points |
| `SCYLLADB_KEYSPACE` | `cloudforge` | Scylla keyspace |
| `OPENBAO_ADDR` | `http://localhost:8200` | OpenBao API address |
| `OPENBAO_TOKEN` | `dev-root-token` | OpenBao token |
| `CF_INTERNAL_SECRET` | `dev-internal-secret` | Shared secret; must match `X-CF-Internal-Secret` header |
| `HOST_KUBECONFIG` | (empty) | Path to kubeconfig for the **host** cluster. If empty, uses `$HOME/.kube/config` |

## API

OpenAPI: `api/cf-provisioner/v1/openapi.yaml`. Regenerate server stubs:

```bash
cd services/cf-provisioner && go generate ./...
```

## Docker

Build from **repository root** (so `go.work` is available):

```bash
docker build -f services/cf-provisioner/Dockerfile .
```

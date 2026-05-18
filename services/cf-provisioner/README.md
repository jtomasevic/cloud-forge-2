# CF-Provisioner

**CF-Provisioner** is CloudForge’s **network provisioning control plane**: an HTTP service that allocates
private CIDRs, creates **vCluster** workloads on a **host** Kubernetes cluster, applies **Cilium**
policies, stores **kubeconfigs** in **OpenBao**, tracks **async jobs** in **ScyllaDB**, and optionally
exposes tenant workloads via **Gateway API** (`HTTPRoute`) + Cilium ingress policy.

It is **internal-only**: every OpenAPI route expects header **`X-CF-Internal-Secret`** (see
[`internal/rest/server.go`](internal/rest/server.go)). Typical callers are **CF-Router** (proxied
platform traffic) or other control-plane components—not end-user browsers.

---

## What it does

| Area | Behavior |
|------|-----------|
| **Networks** | Start provisioning (`POST /v1/networks`), poll status (`GET /v1/networks/{id}`), start deprovisioning (`DELETE /v1/networks/{id}`). Long work returns **202 + `Job`**; poll `GET /v1/jobs/{jobId}`. |
| **CIDRs** | Allocates non-overlapping **pod** and **service** CIDRs per network (Scylla-backed), with operator listing (`GET /v1/cidr/allocations`). |
| **vCluster** | Creates / observes / deletes vCluster via host cluster API + `vcluster` CLI (see repository package). |
| **Cilium** | Applies default-deny and optional **world → workload** ingress policies aligned with gateway exposure. |
| **Kubeconfig** | Writes tenant kubeconfig material to **OpenBao** after vCluster is ready; revokes on deprovision **before** cluster teardown. |
| **Gateway** | Async attach/detach of `HTTPRoute` for public ingress; status via `GET …/gateway`. |
| **Jobs** | Durable async state in Scylla (`provisioning_jobs` + by-network index). |
| **Subnets** | In-memory subnet API in current implementation (see OpenAPI + service). |

Contract and JSON models: [`api/cf-provisioner/v1/openapi.yaml`](../../api/cf-provisioner/v1/openapi.yaml).

---

## Why it exists

- **Separation from CF-Accounts**: account/tenant registry stays authoritative in CF-Accounts;
  CF-Provisioner owns **how** a tenant network is realized on shared infrastructure (namespaces,
  CIDRs, policies, secrets).
- **Async safety**: provisioning is slow and failure-prone; **jobs** give callers a stable handle and
  clear terminal states (`succeeded` / `failed`) without holding HTTP connections open for minutes.
- **Defense in depth**: Cilium default-deny + explicit ingress; kubeconfig never stored in Scylla;
  internal secret on every API call.

---

## How it is built (layers)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  HTTP client (CF-Router, automation, curl)                                 │
│  X-CF-Internal-Secret  +  optional X-Request-ID                           │
└───────────────────────────────────┬─────────────────────────────────────┘
                                    │ JSON
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  internal/rest                                                           │
│  Strict OpenAPI handlers → map JSON ↔ service DTOs → HTTP status/body   │
│  Middleware: RequestID, Logger, Recovery, internal-secret gate          │
└───────────────────────────────────┬─────────────────────────────────────┘
                                    │ ProvisionerService
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  internal/service                                                        │
│  Workflows: ordering (revoke kubeconfig before delete vCluster),         │
│  async goroutines (background context), job lifecycle, status fusion    │
└───────────────────────────────────┬─────────────────────────────────────┘
                                    │ repository interfaces
          ┌─────────────────────────┼─────────────────────────┐
          ▼                         ▼                         ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│  Infra clients   │      │  State repos     │      │  (no cross-     │
│  vcluster,cilium,│      │  cidr, jobs      │      │   imports       │
│  gateway,kubecfg │      │  (Scylla)        │      │   between repos)│
└────────┬─────────┘      └────────┬─────────┘      └─────────────────┘
         │                         │
         ▼                         ▼
   Host Kubernetes            ScyllaDB
   + vcluster CLI             (cloudforge keyspace)
         │
         ▼
   OpenBao (kubeconfig paths via libs/openbao)
```

Deeper dives:

- **REST layer**: [`internal/rest/README.md`](internal/rest/README.md)
- **Service layer**: [`internal/service/README.md`](internal/service/README.md)
- **Repository layer**: [`internal/repository/README.md`](internal/repository/README.md)

---

## Architecture (components)

```
                    ┌──────────────────────────────┐
                    │       CF-Provisioner         │
                    │  (single Go binary :8082)     │
                    └──────────────┬───────────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
┌─────────────────┐     ┌─────────────────┐       ┌─────────────────┐
│  rest.Handler   │     │ service.        │       │  repository.*  │
│  (OpenAPI)      │────▶│ CFProvisioner   │──────▶│  interfaces     │
│                 │     │ Service         │       │                 │
└─────────────────┘     └─────────────────┘       └────────┬────────┘
                                                           │
     ┌─────────────────────────────────────────────────────┼──────────────────────────────┐
     │                                                     │                              │
     ▼                                                     ▼                              ▼
┌─────────────┐                                    ┌─────────────┐                 ┌─────────────┐
│ Host K8s    │                                    │ ScyllaDB    │                 │ OpenBao     │
│ API + CRDs  │                                    │ jobs, cidr  │                 │ kubeconfigs │
│ vcluster CLI│                                    │ tables      │                 │             │
└─────────────┘                                    └─────────────┘                 └─────────────┘
```

---

## Infrastructure dependencies

| System | Used for | Config / notes |
|--------|-----------|------------------|
| **Host Kubernetes** | vCluster lifecycle, Cilium CRDs, Gateway API `HTTPRoute` | `HOST_KUBECONFIG` or `$HOME/.kube/config` |
| **vCluster CLI** | Create/delete/connect (invoked from repository client) | Must be available where the binary runs |
| **ScyllaDB** | `cidr_allocations`, `provisioning_jobs`, `provisioning_jobs_by_network` | `SCYLLADB_HOSTS`, `SCYLLADB_KEYSPACE`; migrations under `tools/migrations/` |
| **OpenBao** | Store/load/revoke kubeconfig bytes | `OPENBAO_ADDR`, `OPENBAO_TOKEN` |
| **Env (Gateway)** | Parent refs for `HTTPRoute` | `CF_GATEWAY_PARENT_NAME`, `CF_GATEWAY_PARENT_NAMESPACE` (defaults in repo README) |

---

## Request flow (synchronous API edge)

```
  Client                          rest                         service
    │                              │                              │
    │  POST /v1/networks         │                              │
    ├─────────────────────────────▶  decode JSON                 │
    │  X-CF-Internal-Secret        │  auth middleware             │
    │                              ├──────────────────────────────▶│
    │                              │                              │ CIDR + Jobs (sync part)
    │                              │                              │ spawn async runner
    │  202 + Job                   │◀──────────────────────────────┤
    │◀─────────────────────────────┤  map to OpenAPI              │
    │                              │                              │
    │  GET /v1/jobs/{id}           │                              │
    ├─────────────────────────────▶├──────────────────────────────▶│ Jobs.Get
    │◀─────────────────────────────┤◀──────────────────────────────┤
```

---

## Async provisioning flow (network)

After the handler returns **202**, work continues in a **background goroutine** (`context.Background()`
by design so client disconnect does not cancel infra):

```
  Async runner (service)

    pending job row (Scylla)
            │
            ▼
    running ──▶ VCluster.Create ──▶ poll until Ready
            │
            ▼
    Cilium.ApplyDefaultDeny
            │
            ▼
    wait kubeconfig from vcluster connect
            │
            ▼
    Kubeconfig.Store (OpenBao)
            │
            ▼
    job succeeded (or failed + message)
```

Deprovision ordering (high level): **revoke kubeconfig → remove Cilium → delete vCluster → release CIDR**.
See [`internal/service/README.md`](internal/service/README.md) for the full ASCII sequence.

---

## Run locally

From repo root (`go.work`):

```bash
cd services/cf-provisioner
go run .
```

Requires ScyllaDB, OpenBao, a usable **host** kubeconfig, and `vcluster` where applicable.

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:8082` | HTTP listen address |
| `SCYLLADB_HOSTS` | `localhost:9042` | Comma-separated Scylla contact points |
| `SCYLLADB_KEYSPACE` | `cloudforge` | Keyspace name |
| `OPENBAO_ADDR` | `http://localhost:8200` | OpenBao API base URL |
| `OPENBAO_TOKEN` | `dev-root-token` | OpenBao token |
| `CF_INTERNAL_SECRET` | `dev-internal-secret` | Must match `X-CF-Internal-Secret` on requests |
| `HOST_KUBECONFIG` | *(empty)* | Path to kubeconfig for **host** cluster; empty → `$HOME/.kube/config` |
| `CF_GATEWAY_PARENT_NAME` | `envoy-public` | Gateway parent for HTTPRoute (see `internal/repository/README.md`) |
| `CF_GATEWAY_PARENT_NAMESPACE` | `envoy-gateway-system` | Namespace of parent Gateway |

---

## API & codegen

- OpenAPI: [`api/cf-provisioner/v1/openapi.yaml`](../../api/cf-provisioner/v1/openapi.yaml)
- Regenerate server stubs:

```bash
cd services/cf-provisioner && go generate ./...
```

- Regenerate service mocks (after interface changes):

```bash
cd services/cf-provisioner && go generate ./internal/service/...
```

---

## Docker

Build from **repository root** (needs `go.work`):

```bash
docker build -f services/cf-provisioner/Dockerfile .
```

---

## Plans & docs

- Service layer: [`docs/plan/15.CFProvisionerServiceLayer.md`](../../docs/plan/15.CFProvisionerServiceLayer.md)
- Infra repositories: [`docs/plan/13.CFProvisionerInfraRepositoryLayer.md`](../../docs/plan/13.CFProvisionerInfraRepositoryLayer.md)
- State repositories: [`docs/plan/14.CFProvisionerStateRepositoryLayer.md`](../../docs/plan/14.CFProvisionerStateRepositoryLayer.md)

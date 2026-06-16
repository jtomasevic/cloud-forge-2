# CF-Provisioner — repository layer (`internal/repository/`)

This directory is the **data and integration access layer** for CF-Provisioner. It hides **how** the service talks to external systems (Kubernetes, Cilium CRDs, Gateway API, OpenBao, **ScyllaDB**) behind small **interfaces** and **`cferrors`-typed errors**. Higher layers (**service**, then **REST**) should depend on these interfaces, not on client-go, dynamic clients, exec details, or raw `gocql` types.

The layer is **split by dependency type** (see [`docs/plan/13.CFProvisionerInfraRepositoryLayer.md`](../../../../docs/plan/13.CFProvisionerInfraRepositoryLayer.md) and [`docs/plan/14.CFProvisionerStateRepositoryLayer.md`](../../../../docs/plan/14.CFProvisionerStateRepositoryLayer.md)):

| Track | Packages | Backing systems |
|-------|-----------|-----------------|
| **Infrastructure** (Tasks 13, 30) | [`vcluster/`](vcluster/), [`cilium/`](cilium/), [`gateway/`](gateway/), [`kubeconfig/`](kubeconfig/), [`workloads/`](workloads/) | Host cluster APIs, tenant vCluster APIs, vCluster CLI, OpenBao |
| **State** (Tasks 14, 25, 28, 29) | [`cidr/`](cidr/), [`jobs/`](jobs/), [`subnets/`](subnets/), [`appservices/`](appservices/) | ScyllaDB (`cidr_allocations`, `provisioning_jobs`, subnet tables, app-service tables, and indexes) |

---

## Purpose

- **Encapsulate integrations** — kubeconfig loading, `vcluster` CLI invocation, `CiliumNetworkPolicy` objects as unstructured CRDs, and `HTTPRoute` resources live behind explicit methods with stable parameters.
- **Typed errors at the boundary** — interface methods return (or wrap) `*cferrors.CFError` from `libs/cloudforge-core/pkg/errors`. Raw Kubernetes, exec, or Vault/OpenBao API errors are not returned as plain `error` types to callers.
- **Testability** — production types are unexported (`CF…Client` / `CF…Repository`); constructors return interfaces so the service layer can substitute fakes or partial mocks in unit tests.
- **Clear ownership** — infra repos never write CIDR rows or job rows; state repos never apply cluster CRDs. That split matches the architecture doc and keeps review boundaries obvious.

---

## Layout (one package per concern)

Each subfolder is its **own Go package**. Prefer **no imports** between sibling repository packages (e.g. avoid `vcluster` → `cilium`) so the service layer owns orchestration order (create vCluster, then default-deny policy, then store kubeconfig, etc.).

| Package | Responsibility |
|---------|----------------|
| [`vcluster/`](vcluster/) | Provision / observe / destroy vCluster workloads on the host cluster (currently via `vcluster` CLI + host API discovery). |
| [`cilium/`](cilium/) | Apply and remove `cilium.io/v2` `CiliumNetworkPolicy` objects (default deny + internet ingress). |
| [`gateway/`](gateway/) | Create, read, and delete `gateway.networking.k8s.io` `HTTPRoute` resources for Envoy Gateway. |
| [`kubeconfig/`](kubeconfig/) | Store, load, and revoke tenant kubeconfigs via `libs/openbao/pkg/client` helpers only (no direct Vault API usage here). |
| [`workloads/`](workloads/) | Apply and observe tenant vCluster `Deployment`/`Service` objects for CF App Service runtime. |
| [`cidr/`](cidr/) | Scylla-backed pod/service CIDR allocation (`cidr_allocations`), sequential auto-pool from `10.0.0.0/8` + `172.16.0.0/12`. |
| [`jobs/`](jobs/) | Async provisioning job rows (`provisioning_jobs` + `provisioning_jobs_by_network` denormalized listing), including app-service lifecycle job types. |
| [`subnets/`](subnets/) | Durable private/public subnet metadata and indexes for network-scoped listing plus duplicate CIDR detection. |
| [`appservices/`](appservices/) | Durable app-service workload intent, runtime JSON fragments, exposure metadata, and network-scoped listing. |

### Files per package (convention)

**Infrastructure packages** (`vcluster`, `cilium`, `gateway`, `kubeconfig`, `workloads`):

| File | Role |
|------|------|
| `api.go` | **Public interface** and `New(…)` constructor returning that interface. |
| `client.go` | Unexported concrete type (`CF…Client` / `CF…Repository`) implementing the interface. |
| `models.go` | **Wire / domain structs** passed across the repository boundary (e.g. `CreateVClusterParams`, `HTTPRouteParams`). |
| `errors.go` | Package **sentinel** `*cferrors.CFError` values for `errors.Is` / wrapping. |
| `*_test.go` | Unit tests (fakes, `client-go` / `gateway-api` fakes, OpenBao `mock.SecretsClient`, etc.). |

**State packages** (`cidr`, `jobs`, `subnets`, `appservices`) — same style as CF-Accounts repositories:

| File | Role |
|------|------|
| `api.go` | **Repository interface** and `New(*scylladbclient.Session) …Repository`. |
| `repository.go` | Unexported concrete type; `Query` / `Exec` / `Scan` via `libs/scylladb/pkg/client`. |
| `commands.go` | **CQL string constants** with bind-order comments. |
| `models.go` | Request/response structs (`AllocateParams`, `Job`, …). |
| `errors.go` | Sentinel `*cferrors.CFError` values. |
| `allocate.go` | *(cidr only)* Pure allocation / overlap logic and sequential index math. |
| `*_test.go` | Unit tests (pure allocator tests, `gocql.ErrNotFound` mapping, etc.). |

---

## Interface methods (summary)

### `vcluster` — [`VClusterClient`](vcluster/api.go)

| Method | Description |
|--------|-------------|
| `Create(ctx, params)` | Run `vcluster create` with namespace, optional pod/service CIDR flags; returns refreshed `VClusterInfo`. |
| `Get(ctx, name)` | Resolve namespace from host `StatefulSet`s labelled `app=vcluster`, return status and optional annotation-derived CIDRs. |
| `Delete(ctx, name)` | Run `vcluster delete` in the resolved namespace. |
| `GetKubeconfig(ctx, name)` | Run `vcluster connect … --print` after namespace resolution. |

### `cilium` — [`CiliumClient`](cilium/api.go)

| Method | Description |
|--------|-------------|
| `ApplyDefaultDenyPolicy(ctx, vclusterNamespace, networkID)` | Create default-deny egress-to-cluster policy (idempotent if already exists). |
| `ApplyIngressPolicy(ctx, params)` | Create world → port ingress policy for gateway exposure. |
| `RemovePolicy(ctx, namespace, policyName)` | Delete policy; not found is treated as success. |
| `GetPolicy(ctx, namespace, policyName)` | Return `PolicyInfo` with `Exists` false if absent. |

### `gateway` — [`GatewayClient`](gateway/api.go)

| Method | Description |
|--------|-------------|
| `CreateHTTPRoute(ctx, params)` | Create `HTTPRoute`; on success re-reads status; parent Gateway comes from env (see below). |
| `GetHTTPRoute(ctx, namespace, name)` | Read route and map parent conditions to `HTTPRouteStatus`. |
| `DeleteHTTPRoute(ctx, namespace, name)` | Delete route; not found is success. |

**Environment (Gateway parent attachment):**

- `CF_GATEWAY_PARENT_NAME` (default `envoy-public`)
- `CF_GATEWAY_PARENT_NAMESPACE` (default `envoy-gateway-system`)

### `kubeconfig` — [`KubeconfigRepository`](kubeconfig/api.go)

| Method | Description |
|--------|-------------|
| `Store(ctx, tenantID, bytes)` | Delegates to `openbao.StoreKubeconfig`. |
| `Load(ctx, tenantID)` | Delegates to `openbao.LoadKubeconfig`; maps `ErrSecretNotFound` to `ErrKubeconfigNotFound`. |
| `Revoke(ctx, tenantID)` | Delegates to `openbao.RevokeKubeconfig`. |

### `workloads` — [`WorkloadClient`](workloads/api.go)

| Method | Description |
|--------|-------------|
| `Apply(ctx, params)` | Create/update tenant vCluster Deployment and optional ClusterIP Service from app-service runtime intent. |
| `Get(ctx, namespace, name)` | Read Deployment conditions/readiness plus Service presence for status reporting. |
| `Delete(ctx, namespace, name)` | Delete Service and Deployment; missing objects are success for retry-safe cleanup. |

### `cidr` — [`CIDRRepository`](cidr/api.go)

| Method | Description |
|--------|-------------|
| `Allocate(ctx, params)` | Reserve pod/svc CIDRs (auto or explicit); rejects overlaps and duplicate network rows. |
| `Get(ctx, networkID)` | Read `cidr_allocations` by `network_id`. |
| `Release(ctx, networkID)` | Delete allocation row for deprovision. |
| `ListAll(ctx)` | Full table scan for ops/debug (`GET /v1/cidr/allocations` backing data). |

### `jobs` — [`JobsRepository`](jobs/api.go)

| Method | Description |
|--------|-------------|
| `Create(ctx, params)` | Insert primary + `provisioning_jobs_by_network` row (`pending`); `tenant_id` is stored as **all-zero UUID** until callers pass tenant context in a future API revision. |
| `Get(ctx, jobID)` | Load job by `id`. |
| `ListByNetwork(ctx, networkID)` | Walk the denormalized partition (newest first), then hydrate each job from the primary table. |
| `UpdateStatus(ctx, jobID, status, errorMsg)` | Update primary row and **best-effort** denormalized `status` (same eventual-consistency pattern as CF-Accounts `networks_by_tenant`). |

### `subnets` — [`SubnetsRepository`](subnets/api.go)

| Method | Description |
|--------|-------------|
| `Create(ctx, params)` | Insert primary + denormalized subnet rows; rejects duplicate canonical CIDR within the network. |
| `GetByID(ctx, networkID, subnetID)` | Load a subnet by id and verify it belongs to the requested network. |
| `ListByNetwork(ctx, networkID)` | Read the network-scoped listing index for `GET /v1/networks/{id}/subnets`. |

### `appservices` — [`AppServicesRepository`](appservices/api.go)

| Method | Description |
|--------|-------------|
| `Create(ctx, params)` | Insert primary desired-state row plus network-listing row; reserve public host when initial exposure is present. |
| `Get(ctx, appServiceID)` | Load and JSON-decode one app-service row by id. |
| `ListByNetwork(ctx, networkID)` | Read network-listing ids and hydrate authoritative primary rows. |
| `UpdateStatus(ctx, params)` | Update primary lifecycle status and denormalized list status. |
| `UpdateExposure(ctx, params)` | Update primary exposure JSON, list summary, and public-host lookup row. |
| `MarkDeleted(ctx, appServiceID)` | Set repository tombstone status for cleanup flows before physical deletion. |
| `Delete(ctx, appServiceID)` | Remove public host row, network listing row, and primary row after infrastructure cleanup. |

---

## Principles

1. **Interfaces at the boundary** — Service code accepts `VClusterClient`, `CiliumClient`, `CIDRRepository`, `JobsRepository`, etc., not concrete `CF…` / `*…Repository` types.
2. **Context on every call** — All methods take `context.Context` and pass it through to Kubernetes, dynamic client, exec, OpenBao, or Scylla queries.
3. **No raw downstream errors** — Map or wrap failures as `*cferrors.CFError` (optionally preserving a private `Unwrap` chain for logs).
4. **`kubeconfig` stays thin** — Only `libs/openbao/pkg/client` kubeconfig helpers; no new secret paths or SDK calls in this package.
5. **Infra vs state** — Do not add Scylla queries under `vcluster/` / `cilium/` / `gateway/` / `kubeconfig/`; do not issue Kubernetes calls from `cidr/` or `jobs/`.

---

## Build

From the service module root:

```bash
cd services/cf-provisioner
go build ./internal/repository/vcluster/ \
          ./internal/repository/cilium/ \
          ./internal/repository/gateway/ \
          ./internal/repository/kubeconfig/ \
          ./internal/repository/workloads/ \
          ./internal/repository/cidr/ \
          ./internal/repository/jobs/ \
          ./internal/repository/subnets/
```

---

## Testing

Run all repository unit tests:

```bash
cd services/cf-provisioner
go test ./internal/repository/... -count=1
```

Per-package:

```bash
go test ./internal/repository/vcluster/ -count=1
go test ./internal/repository/cilium/ -count=1
go test ./internal/repository/gateway/ -count=1
go test ./internal/repository/kubeconfig/ -count=1
go test ./internal/repository/cidr/ -count=1
go test ./internal/repository/jobs/ -count=1
go test ./internal/repository/subnets/ -count=1
```

Tests use fakes (`fake` clientsets, `dynamic/fake`, OpenBao `mock.SecretsClient`) and pure allocator tests for **cidr**; they do **not** require a live Kubernetes cluster or OpenBao by default. Scylla-backed repository methods are exercised in unit tests only through **logic helpers** and error mapping; add `//go:build integration` tests with a real session or testcontainers when you want end-to-end CQL coverage.

---

## References

- Infrastructure task: [`docs/plan/13.CFProvisionerInfraRepositoryLayer.md`](../../../../docs/plan/13.CFProvisionerInfraRepositoryLayer.md)
- State task (CIDR + jobs): [`docs/plan/14.CFProvisionerStateRepositoryLayer.md`](../../../../docs/plan/14.CFProvisionerStateRepositoryLayer.md)
- HTTP contract (for context): [`api/cf-provisioner/v1/openapi.yaml`](../../../../api/cf-provisioner/v1/openapi.yaml)
- Platform errors: `libs/cloudforge-core/pkg/errors`
- OpenBao client: `libs/openbao/pkg/client`
- Scylla client: `libs/scylladb/pkg/client`
- Migrations: [`tools/migrations/scripts/20240101007_create_cidr_allocations.cql`](../../../../tools/migrations/scripts/20240101007_create_cidr_allocations.cql), [`20240101006_create_provisioning_state.cql`](../../../../tools/migrations/scripts/20240101006_create_provisioning_state.cql)

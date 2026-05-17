# CF-Provisioner — repository layer (`internal/repository/`)

This directory is the **data and integration access layer** for CF-Provisioner. It hides **how** the service talks to external systems (Kubernetes, Cilium CRDs, Gateway API, OpenBao) behind small **interfaces** and **`cferrors`-typed errors**. Higher layers (**service**, then **REST**) should depend on these interfaces, not on client-go, dynamic clients, or exec details.

The layer is **split by dependency type** (see [`docs/plan/13.CFProvisionerInfraRepositoryLayer.md`](../../../../docs/plan/13.CFProvisionerInfraRepositoryLayer.md) and [`docs/plan/14.CFProvisionerStateRepositoryLayer.md`](../../../../docs/plan/14.CFProvisionerStateRepositoryLayer.md)):

| Track | Packages | Backing systems |
|-------|-----------|-----------------|
| **Infrastructure** (Task 13) | [`vcluster/`](vcluster/), [`cilium/`](cilium/), [`gateway/`](gateway/), [`kubeconfig/`](kubeconfig/) | Host cluster APIs, vCluster CLI, OpenBao |
| **State** (Task 14, planned) | [`cidr/`](cidr/), [`jobs/`](jobs/) | ScyllaDB (allocations, async job records) |

---

## Purpose

- **Encapsulate integrations** — kubeconfig loading, `vcluster` CLI invocation, `CiliumNetworkPolicy` objects as unstructured CRDs, and `HTTPRoute` resources live behind explicit methods with stable parameters.
- **Typed errors at the boundary** — interface methods return (or wrap) `*cferrors.CFError` from `libs/cloudforge-core/pkg/errors`. Raw Kubernetes, exec, or Vault/OpenBao API errors are not returned as plain `error` types to callers.
- **Testability** — production types are unexported (`CF…Client` / `CF…Repository`); constructors return interfaces so the service layer can substitute fakes or partial mocks in unit tests.
- **Clear ownership** — infra repos never write CIDR rows or job rows; state repos (Task 14) never apply cluster CRDs. That split matches the architecture doc and keeps review boundaries obvious.

---

## Layout (one package per concern)

Each subfolder is its **own Go package**. Prefer **no imports** between sibling repository packages (e.g. avoid `vcluster` → `cilium`) so the service layer owns orchestration order (create vCluster, then default-deny policy, then store kubeconfig, etc.).

| Package | Responsibility |
|---------|----------------|
| [`vcluster/`](vcluster/) | Provision / observe / destroy vCluster workloads on the host cluster (currently via `vcluster` CLI + host API discovery). |
| [`cilium/`](cilium/) | Apply and remove `cilium.io/v2` `CiliumNetworkPolicy` objects (default deny + internet ingress). |
| [`gateway/`](gateway/) | Create, read, and delete `gateway.networking.k8s.io` `HTTPRoute` resources for Envoy Gateway. |
| [`kubeconfig/`](kubeconfig/) | Store, load, and revoke tenant kubeconfigs via `libs/openbao/pkg/client` helpers only (no direct Vault API usage here). |
| [`cidr/`](cidr/) | *Planned (Task 14)* — Scylla-backed CIDR allocation and release. |
| [`jobs/`](jobs/) | *Planned (Task 14)* — Scylla-backed async provisioning job persistence. |

### Files per package (convention)

| File | Role |
|------|------|
| `api.go` | **Public interface** and `New(…)` constructor returning that interface. |
| `client.go` | Unexported concrete type (`CF…Client` / `CF…Repository`) implementing the interface. |
| `models.go` | **Wire / domain structs** passed across the repository boundary (e.g. `CreateVClusterParams`, `HTTPRouteParams`). |
| `errors.go` | Package **sentinel** `*cferrors.CFError` values for `errors.Is` / wrapping. |
| `*_test.go` | Unit tests (fakes, `client-go` / `gateway-api` fakes, OpenBao `mock.SecretsClient`, etc.). |

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

---

## Principles

1. **Interfaces at the boundary** — Service code accepts `VClusterClient`, `CiliumClient`, etc., not concrete `CF…` types.
2. **Context on every call** — All methods take `context.Context` and pass it through to Kubernetes, dynamic client, exec, or OpenBao helpers.
3. **No raw downstream errors** — Map or wrap failures as `*cferrors.CFError` (optionally preserving a private `Unwrap` chain for logs).
4. **`kubeconfig` stays thin** — Only `libs/openbao/pkg/client` kubeconfig helpers; no new secret paths or SDK calls in this package.
5. **Infra vs state** — Do not add Scylla queries under `vcluster/` / `cilium/` / `gateway/` / `kubeconfig/`; use `cidr/` and `jobs/` once Task 14 is implemented.

---

## Build

From the service module root:

```bash
cd services/cf-provisioner
go build ./internal/repository/vcluster/ \
          ./internal/repository/cilium/ \
          ./internal/repository/gateway/ \
          ./internal/repository/kubeconfig/
```

Note: `go build ./...` for the whole module may fail until a `main` package exists alongside `generate.go`; repository packages build independently as above.

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
```

Tests use fakes (`fake` clientsets, `dynamic/fake`, OpenBao `mock.SecretsClient`) and do **not** require a live cluster or OpenBao by default. Any future test that needs a real cluster should use `//go:build integration` and be run explicitly with tags.

---

## References

- Infrastructure task: [`docs/plan/13.CFProvisionerInfraRepositoryLayer.md`](../../../../docs/plan/13.CFProvisionerInfraRepositoryLayer.md)
- State task (CIDR + jobs): [`docs/plan/14.CFProvisionerStateRepositoryLayer.md`](../../../../docs/plan/14.CFProvisionerStateRepositoryLayer.md)
- HTTP contract (for context): [`api/cf-provisioner/v1/openapi.yaml`](../../../../api/cf-provisioner/v1/openapi.yaml)
- Platform errors: `libs/cloudforge-core/pkg/errors`
- OpenBao client: `libs/openbao/pkg/client`

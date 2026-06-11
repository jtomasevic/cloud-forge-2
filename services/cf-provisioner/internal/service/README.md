# `internal/service` - CF-Provisioner service layer

This package is the **domain orchestration** layer for CF-Provisioner: it sits between the HTTP layer
([`internal/rest`](../rest/)) and the **repository / infra clients** under [`internal/repository/`](../repository/).
It encodes **workflows**, **async jobs**, and **cross-cutting rules** (ordering, naming, error mapping)
so handlers stay thin.

## Purpose

- **Coordinate** CIDR allocation, durable subnet metadata, vCluster lifecycle, Cilium policies,
  Gateway API HTTPRoutes, kubeconfig storage (OpenBao), and Scylla job rows.
- **Expose** a stable API (`ProvisionerService`) for the rest of the service binary.
- **Return quickly** for long operations: persist a `pending` job, run work in a goroutine with
  `context.Background()`, update the job to `running` / `succeeded` / `failed`.

## Why this layer exists

- **Repositories** are narrow (Scylla CRUD, Kubernetes clients). They should not own multi-step
  sagas or “revoke kubeconfig before delete vCluster” ordering.
- **HTTP** should not embed infrastructure details (policy name rules, vCluster naming, backoff).
- **Testing**: workflows are unit-tested with Uber `gomock` (`mocks/`, `generate.go`) without a real
  cluster or Scylla.

## How callers use it

1. Construct `Deps` with real or test doubles of each repository/client interface.
2. `svc := service.New(deps)`.
3. Call `ProvisionerService` methods with `context.Context` (only the synchronous parts honor
   cancellation; async work uses a fresh background context on purpose).

## Package layout

- **`api.go`**: `ProvisionerService` interface, `Deps`, `New`.
- **`models.go`**: Request/response DTOs; see file comments for how types relate.
- **`errors.go`**: Service-level sentinel errors for HTTP mapping.
- **`service.go`**: `CFProvisionerService` implementation, naming helpers, async runners.
- **`generate.go`**: `go:generate` directives for `mockgen` (run from module root).
- **`service_test.go`**: Tests (`package service`, gomock).
- **`mocks/`**: Generated mocks; do not hand-edit.

Regenerate mocks after changing a repository interface:

```bash
cd services/cf-provisioner && go generate ./internal/service/...
```

## Components (Unicode diagram)

```
                    ┌─────────────────────────────────────┐
                    │         ProvisionerService          │
                    │      (CFProvisionerService)         │
                    └──────────────────┬──────────────────┘
                                       │
         ┌────────────────────────┬────────────────────────┬────────────────────────┐
         │                        │                        │                        │
         ▼                        ▼                        ▼                        ▼
  ┌──────────────┐         ┌──────────────┐         ┌──────────────┐         ┌──────────────┐
  │  In-memory   │         │  Jobs repo   │         │  CIDR repo   │         │ Subnets repo │
  │  hints/maps  │         │  (Scylla)    │         │  (Scylla)    │         │  (Scylla)    │
  │ tenant, ns   │         │ async status │         │ allocate /   │         │ create/list  │
  │              │         │ + list       │         │ get / release│         │ + lookup     │
  └──────────────┘         └──────────────┘         └──────────────┘         └──────────────┘
         │                        │                        │                        │
         └────────────────────────┴────────────┬───────────┴────────────────────────┘
                                               │
         ┌─────────────────────────────────────┼─────────────────────────────────────┐
         ▼                                     ▼                                     ▼
  ┌──────────────┐                     ┌──────────────┐                     ┌──────────────┐
  │  VCluster    │                     │   Cilium     │                     │   Gateway    │
  │  client      │                     │   client     │                     │   client     │
  │  (host kube) │                     │  CNPs        │                     │  HTTPRoute   │
  └──────────────┘                     └──────────────┘                     └──────────────┘
                                               │
                                               ▼
                                       ┌──────────────┐
                                       │ Kubeconfig   │
                                       │ (OpenBao)    │
                                       └──────────────┘
```

## Flow: provision network (sync + async)

```
Caller                          Service                         Async goroutine
  │                                │                                    │
  │ ProvisionNetwork(ctx, params)  │                                    │
  ├───────────────────────────────>                                    │
  │                                │ CIDR.Allocate (sync)              │
  │                                │ Jobs.Create -> pending            │
  │                                │ go runProvisionNetwork(...)        │
  │<──────────── Job (pending)─────┤                                    │
  │                                │                                    │
  │                                │                    Jobs.Update running
  │                                │                    VCluster.Create
  │                                │                    poll until Running
  │                                │                    Cilium.ApplyDefaultDeny
  │                                │                    wait kubeconfig
  │                                │                    Kubeconfig.Store
  │                                │                    Jobs.Update succeeded
  │                                │                                    │
  │ GetJob(ctx, jobID)             │                                    │
  ├───────────────────────────────>  Jobs.Get                         │
  │<──────────── status / err ──────┤                                    │
```

## Flow: deprovision network (ordering)

```
Async goroutine (runDeprovisionNetwork)

   ┌──────────────┐
   │ Job: running │
   └──────┬───────┘
          │
          ▼
   ┌──────────────────┐     (skip if tenant unknown; log warn)
   │ Kubeconfig.Revoke│  ◀── must run before cluster teardown
   └─────────┬────────┘
             │
             ▼
   ┌──────────────────┐
   │ Cilium Remove x2 │  default-deny + ingress (best-effort NotFound)
   └─────────┬────────┘
             │
             ▼
   ┌──────────────────┐
   │ VCluster.Delete  │
   └─────────┬────────┘
             │
             ▼
   ┌──────────────────┐
   │ CIDR.Release     │
   └─────────┬────────┘
             │
             ▼
   clear in-memory tenant/namespace hints ; Job: succeeded
```

## Flow: gateway provision / remove

**Provision (async)**

```
GetNetworkStatus == active ?
        │
        ▼ yes
GetHTTPRoute -> must be NotFound (else ErrGatewayExists)
        │
        ▼
Jobs.Create (provision_gateway) ; return Job
        │
        ▼ goroutine
CreateHTTPRoute  ──▶  ApplyIngressPolicy  ──▶  Job succeeded
```

**Remove (async)**

```
GetGatewayStatus (route must exist)
        │
        ▼
Jobs.Create (remove_gateway) ; return Job
        │
        ▼ goroutine
DeleteHTTPRoute  ──▶  RemovePolicy(ingress)  ──▶  Job succeeded
```

## Flow: GetNetworkStatus (data fusion)

```
                    ┌─────────────────────┐
                    │   GetNetworkStatus │
                    └──────────┬──────────┘
                               │
              ┌────────────────┴────────────────┐
              ▼                                 ▼
     Jobs.ListByNetwork                 CIDR.Get
     (newest-first rows)                (allocation or NotFound)
              │                                 │
              └────────────┬────────────────────┘
                           ▼
              inferNetworkStatusFromJobs
              (latest provision/deprovision by CreatedAt)
                           │
                           ▼
              + tenant hint map + vClusterName derivation
                           │
                           ▼
                    NetworkStatus DTO
```

## Related documentation

- Plan: `docs/plan/15.CFProvisionerServiceLayer.md`
- Repository layer overview: `internal/repository/README.md`
- Naming alignment: Cilium policy name helpers in `service.go` must stay consistent with
  `internal/repository/cilium/client.go`.

# Workload Repository

`workloads` is the CF-Provisioner repository package that materializes one CF App Service inside a tenant vCluster. It creates and reads Kubernetes objects with `client-go`; it never shells out to `kubectl`.

## Ownership

This package owns only tenant-vCluster runtime objects:

- `apps/v1 Deployment` for the app service workload
- `core/v1 Service` when `runtime.ports` is non-empty

It does not own public exposure. Gateway API routes, Cilium ingress policy, subnet validation, image builds, kubeconfig storage, and secret resolution are service-layer or sibling-repository responsibilities.

## Naming

The service layer passes `ApplyWorkloadParams.Namespace` and `Name`. This package uses `Name` as both the Deployment name and the Service name. The name must already be a Kubernetes DNS-1123 label, which matches the public app-service API name pattern.

The app service UUID is not embedded in the object name. It is applied as a required label so the object name stays human-readable while ownership remains deterministic.

## Labels

Every Deployment, pod template, and Service gets:

| Label | Meaning |
|-------|---------|
| `cloudforge.io/tenant-id` | Owning tenant. |
| `cloudforge.io/network-id` | Owning private network. |
| `cloudforge.io/subnet-id` | Placement subnet. |
| `cloudforge.io/app-service-id` | Stable app-service owner ID. |
| `cloudforge.io/visibility` | Placement visibility, `private` or `public`. |

The Kubernetes selector uses only `cloudforge.io/app-service-id`. Deployment selectors are immutable after creation; keeping the selector narrow prevents future metadata-label changes from forcing workload recreation.

## Runtime Mapping

- `runtime.image` becomes the single container image.
- `runtime.command` and `runtime.args` become container command/args.
- `runtime.env` becomes plaintext container env vars.
- `runtime.resources.cpu` and `runtime.resources.memory` are applied to both requests and limits.
- `runtime.ports` become container ports and ClusterIP Service ports.
- No ports means no Kubernetes Service; this is the worker-service MVP behavior.

HTTP, gRPC, and TCP app-service protocols are all represented as Kubernetes TCP ports here. Protocol-specific public routing is handled later by Gateway API repositories.

## Tests

Unit tests use `k8s.io/client-go/kubernetes/fake`. Keep tests focused on object shape and idempotent behavior: Deployment/Service creation, service omission for workers, resource limits, deterministic labels, readiness mapping from Deployment conditions, and retry-safe deletion.

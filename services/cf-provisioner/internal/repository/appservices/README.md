# appservices Repository

`internal/repository/appservices` owns ScyllaDB persistence for CF App Service desired state. It is a
state repository only: it does not create Kubernetes Deployments, Kubernetes Services, Gateway API
routes, Cilium policies, or tenant authorization decisions.

## Tables

The package writes the Task 28 tables:

| Table | Use |
|-------|-----|
| `app_services` | Authoritative workload intent and lifecycle row. |
| `app_services_by_network` | Denormalized network listing index for `GET /v1/networks/{networkId}/app-services`. |
| `app_service_exposures_by_host` | Public hostname lookup and Swagger/OpenAPI metadata for internet-gateway exposure. |

`app_service_jobs_by_app_service` is created by migrations for future lifecycle job correlation. Job
creation still belongs to the `jobs` repository and service orchestration layer.

## JSON Storage

The repository stores stable query fields as scalar columns (`tenant_id`, `network_id`, `subnet_id`,
`status`, `service_type`, `exposure_type`, and public host in the listing table). Runtime fragments
that do not need direct Scylla queries are serialized to JSON:

- `command_json`
- `args_json`
- `env_json`
- `ports_json`
- `exposure_json`
- `swagger_json`

The service layer validates those shapes before calling this package. The repository only verifies
required persistence fields and preserves validated data for reconstruction after restart.

## Status And Exposure Flow

`Create` inserts the primary row first, then the network listing row, then the public host row when an
initial exposure is present. Those denormalized writes are not atomic in Scylla; callers should treat
`app_services` as authoritative and repair stale index rows if future reconciliation finds them.

`UpdateStatus` updates both the primary row and the network listing row. `UpdateExposure` updates the
primary row, the network listing summary, and the host lookup table. `MarkDeleted` records a tombstone
status for cleanup flows that need to make deletion visible before physical row removal. `Delete`
removes the host row, listing row, and primary row after higher layers finish infrastructure cleanup.

## Deliberately Not Owned Here

- Tenant authorization and trusted-router header handling.
- Network/subnet placement validation.
- Public subnet versus private subnet exposure decisions.
- Kubernetes Deployment/Service reconciliation.
- Gateway API or Cilium policy creation.
- Async job creation.

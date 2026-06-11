# CF App Service — Implementation Plan

## What This Plan Is

This plan translates the CF App Service architecture proposal in [`docs/cf-app-service.md`](../cf-app-service.md) into phased implementation work.

CF App Service extends CloudForge Private Network with deployable application workloads. It should be implemented as an incremental layer over the current control plane, not as a separate platform model.

## Numbered Task Documents

Execute these after Task 24:

25. [25.CFProvisionerDurableSubnetPersistence.md](25.CFProvisionerDurableSubnetPersistence.md) — persist private/public subnet records in ScyllaDB.
26. [26.CFAppServiceOpenAPISpec.md](26.CFAppServiceOpenAPISpec.md) — define the CF App Service API contract and public Swagger requirement.
27. [27.CFAppServiceCodegenAndClient.md](27.CFAppServiceCodegenAndClient.md) — regenerate server stubs and clients.
28. [28.CFAppServiceSchemaAndMigrations.md](28.CFAppServiceSchemaAndMigrations.md) — add durable app-service schema and job types.
29. [29.CFAppServiceRepositoryLayer.md](29.CFAppServiceRepositoryLayer.md) — implement app-service state repository.
30. [30.CFAppServiceKubernetesWorkloadRepository.md](30.CFAppServiceKubernetesWorkloadRepository.md) — map app services to Kubernetes Deployment and Service.
31. [31.CFAppServiceGatewayAndCiliumRouting.md](31.CFAppServiceGatewayAndCiliumRouting.md) — replace placeholder gateway routing with app-specific routes and policies.
32. [32.CFAppServiceServiceLayer.md](32.CFAppServiceServiceLayer.md) — implement service-layer validation and orchestration.
33. [33.CFAppServiceRESTLayerAndRouter.md](33.CFAppServiceRESTLayerAndRouter.md) — expose REST handlers and route them through CF-Router.
34. [34.CFAppServicePublicSwaggerExposure.md](34.CFAppServicePublicSwaggerExposure.md) — require and expose public Swagger/OpenAPI docs for internet-exposed app services.
35. [35.CFAppServiceLocalDevLoopAndExamples.md](35.CFAppServiceLocalDevLoopAndExamples.md) — add local examples, smoke tests, and Make/Tilt support.
36. [36.CFAppServiceProtocolRoutesGrpcTcp.md](36.CFAppServiceProtocolRoutesGrpcTcp.md) — add gRPC and TCP Gateway API route support.
37. [37.CFAppServiceProductionHardeningAndCICD.md](37.CFAppServiceProductionHardeningAndCICD.md) — add production guardrails and CI/CD coverage.

---

## Phase 1 — Repository and Architecture Discovery

### Purpose

Confirm the exact current implementation boundaries before designing APIs or writing code.

### Concrete Tasks

- Inspect `docs/cf-private-network.md` and confirm current terminology.
- Inspect `libs/cloudforge-core/pkg/network`.
- Inspect `api/cf-accounts/v1/openapi.yaml`.
- Inspect `api/cf-provisioner/v1/openapi.yaml`.
- Inspect CF-Provisioner service/repository layers.
- Inspect Gateway API and Cilium clients.
- Inspect `dev/k8s/` manifests.
- Inspect the `Tiltfile` and Makefile development commands.
- Confirm whether any app-service CRDs/controllers already exist.
- Document current gaps:
  - no CF App Service API
  - no app service persistence
  - no workload deployment client
  - no durable subnet persistence
  - gateway routes currently target a placeholder backend

### Files Likely To Be Touched

- `docs/cf-app-service.md`
- `docs/plan/cf-app-service-plan.md`

### Risks

- Designing against stale assumptions from the architecture proposal instead of the current code.
- Missing local dev constraints around k3d, Tilt, and Envoy Gateway.
- Confusing private network provisioning with app workload deployment.

### Validation Criteria

- Architecture notes name the actual files and modules that exist.
- The plan explicitly states that no CF App Service code exists yet.
- The plan identifies current Gateway API, Cilium, and vCluster integration points.

---

## Phase 2 — API Design

### Purpose

Define a stable user-facing contract for app service creation, placement, runtime configuration, status, and exposure.

### Concrete Tasks

- Decide whether the first API is:
  - REST-only under CF-Provisioner
  - a new `cf-app-service` REST service
  - CRD-first
  - REST plus future CRD
- Define `CFAppService` schema.
- Define runtime schema:
  - image
  - build context for dev mode
  - command/args if needed
  - environment variables
  - CPU/memory
  - ports
  - protocol
- Define placement schema:
  - `networkId`
  - `subnetId`
  - subnet type validation
- Define exposure schema:
  - none
  - internet gateway
  - host
  - port reference
  - protocol-specific route type
- Define status schema:
  - pending
  - deploying
  - running
  - failed
  - deleting
  - deleted
- Define app service job types.
- Define validation rules:
  - network must be active
  - subnet must belong to network
  - public exposure requires public subnet
  - private subnet cannot have internet gateway exposure
  - resource values must be bounded
  - exposed port must match declared runtime port

### Files Likely To Be Touched

If extending CF-Provisioner:

- `api/cf-provisioner/v1/openapi.yaml`
- `services/cf-provisioner/internal/rest/models.go`
- `services/cf-provisioner/internal/rest/models_transform.go`
- `services/cf-provisioner/internal/service/api.go`
- `services/cf-provisioner/internal/service/models.go`
- `services/cf-provisioner/internal/service/errors.go`
- `services/cf-provisioner/internal/repository/jobs/models.go`
- `tools/migrations/migrations/`

If creating a new service:

- `api/cf-app-service/v1/openapi.yaml`
- `services/cf-app-service/`
- `libs/clients/cf-app-service/`
- root `go.work`
- root `Makefile`
- `Tiltfile`
- `dev/k8s/manifests/`
- CF-Router route table and OpenAPI aggregation

### Risks

- Making the first API too PaaS-like.
- Committing to build-service behavior before production image handling is designed.
- Modeling public exposure as a boolean instead of a route/gateway relationship.
- Not preserving future separation into a dedicated service.

### Validation Criteria

- OpenAPI spec validates.
- Generated code can be produced with `make codegen`.
- Validation rules are listed in OpenAPI descriptions and service-layer tests.
- API terminology matches `network`, `subnet`, `gateway`, and `tenant` language already used by CloudForge.

---

## Phase 3 — Local Development MVP

### Purpose

Make app services work in the local k3d development stack before production concerns are added.

### Concrete Tasks

- Define a local app service example directory under `examples/` or `dev/examples/`.
- Build local Docker images for app services.
- Push images to the k3d local registry or load images into the cluster.
- Deploy a private app service into a test private network.
- Deploy a public app service into a public subnet.
- Expose the public app through local Envoy Gateway.
- Add local hostname guidance.
- Add a smoke test for:
  - public service reachable from local machine through gateway
  - private service not reachable through gateway
  - private service reachable from inside the tenant network

### Files Likely To Be Touched

- `Tiltfile`
- `Makefile`
- `dev/README.md`
- `dev/CONNECTIONS.md`
- `dev/HOSTS.md`
- `dev/k8s/gateway/`
- `dev/k8s/manifests/`
- `dev/scripts/smoke-test.sh`
- `examples/` or `dev/examples/`

### Risks

- Letting local shortcuts become production architecture.
- Exposing private-subnet services accidentally through port-forwarding.
- Depending on wildcard DNS that is not documented for local machines.
- Making app image builds too tightly coupled to Tilt.

### Validation Criteria

- `make dev` still starts the base control plane.
- Example app image builds locally.
- Public app service is reachable through local Envoy Gateway.
- Private app service is not reachable through local Envoy Gateway.
- Smoke test documents the expected commands and outputs.

---

## Phase 4 — Kubernetes Mapping

### Purpose

Implement the mapping from CF App Service to Kubernetes, Gateway API, and Cilium primitives.

### Concrete Tasks

- Add a Kubernetes workload repository/client in CF-Provisioner or a new app service:
  - create Deployment
  - get Deployment readiness
  - delete Deployment
  - create Service
  - delete Service
- Add route repository/client support:
  - HTTPRoute for HTTP/REST
  - GRPCRoute for gRPC, if supported by selected Gateway API version
  - TCPRoute for TCP, if needed
- Extend Cilium policy client:
  - service-specific ingress policy
  - public gateway ingress policy by service/port
  - optional internal allow policy by subnet
- Add deterministic naming helpers:
  - deployment name
  - service name
  - route name
  - policy name
- Add labels:
  - tenant ID
  - network ID
  - subnet ID
  - app service ID
  - visibility
- Add status aggregation:
  - Deployment available
  - Service exists
  - Route accepted/resolved
  - Cilium policy exists

### Files Likely To Be Touched

- `services/cf-provisioner/internal/repository/workloads/`
- `services/cf-provisioner/internal/repository/gateway/`
- `services/cf-provisioner/internal/repository/cilium/`
- `services/cf-provisioner/internal/service/`
- `services/cf-provisioner/internal/rest/`
- `api/cf-provisioner/v1/openapi.yaml`
- `tools/migrations/migrations/`

### Risks

- Host-cluster and vCluster object ownership becoming unclear.
- Gateway API routes pointing to the wrong namespace.
- Cilium policy selecting too broad a set of workloads.
- Resource names exceeding Kubernetes DNS label limits.
- Deletion leaving routes or policies behind.

### Validation Criteria

- Unit tests cover name derivation and validation.
- Fake Kubernetes clients cover Deployment/Service create/delete/status.
- Fake Gateway API clients cover route create/delete/status.
- Fake dynamic client tests cover Cilium policy creation.
- Deleting an app service deletes workload, service, route, and policy resources.

---

## Phase 5 — Production Path

### Purpose

Define the production-ready path for images, gateway exposure, DNS, TLS, quotas, and hardening.

### Concrete Tasks

- Require image references for production deployments.
- Decide whether production accepts image tags or requires digests.
- Define registry authentication model.
- Define DNS model for public app services.
- Define TLS certificate flow.
- Define public hostname ownership validation.
- Define resource quotas per tenant/network.
- Define max service count per network.
- Define protocol support by route type.
- Define audit events for:
  - create app service
  - update app service
  - expose app service
  - remove exposure
  - delete app service
- Define failure and retry behavior for async jobs.

### Files Likely To Be Touched

- `docs/cf-app-service.md`
- `docs/plan/`
- `api/cf-provisioner/v1/openapi.yaml`
- `services/cf-provisioner/internal/service/`
- `services/cf-provisioner/internal/repository/`
- `tools/migrations/migrations/`
- `.github/workflows/`

### Risks

- Accepting arbitrary build contexts in production.
- Weak hostname ownership validation.
- Missing quota enforcement.
- No rollback path for partially created public exposure.
- Not capturing audit history for public exposure changes.

### Validation Criteria

- Production mode is image-reference based.
- TLS/DNS assumptions are documented.
- Public exposure cannot be created without validated internet gateway state.
- Quota validation is enforced before workload creation.
- Failure states are visible through status APIs.

---

## Phase 6 — Examples and Tests

### Purpose

Provide enough examples and validation to make the feature usable and maintainable.

### Concrete Tasks

- Add example private REST service.
- Add example public REST service.
- Add example gRPC service.
- Add example UI/frontend service.
- Add worker service example.
- Add API examples for app service create/list/get/delete.
- Add smoke tests for local public exposure.
- Add tests for validation rules.
- Add tests for Kubernetes mapping.
- Add tests for cleanup behavior.
- Add docs for local troubleshooting.

### Files Likely To Be Touched

- `examples/cf-app-service/`
- `dev/examples/`
- `dev/scripts/`
- `docs/cf-app-service.md`
- service test files under `services/cf-provisioner/internal/...`
- `api/*/openapi.yaml`

### Risks

- Examples diverging from actual API behavior.
- Smoke tests becoming dependent on local host state.
- gRPC examples requiring route support not yet implemented.
- Tests overusing real clusters when fake clients are enough.

### Validation Criteria

- `make test` passes.
- `make lint` passes.
- `make codegen` produces no drift.
- Example public REST service is reachable through the local gateway.
- Example private service is not reachable through the local gateway.
- Documentation includes copy/pasteable commands.

---

## Phase 7 — Future Enhancements

### Purpose

Extend CF App Service beyond the MVP after the core workload and exposure model is stable.

### Concrete Tasks

- Add autoscaling.
- Add health checks and readiness/liveness config.
- Add rolling deployment controls.
- Add canary and blue/green deployment strategies.
- Add logs and metrics API.
- Add OpenBao-backed secrets.
- Add config maps.
- Add private service discovery.
- Add app-to-app network policy rules.
- Add build service.
- Add CI/CD integration.
- Add custom domains.
- Add TLS policy configuration.
- Add WAF and rate limiting policy.
- Add multi-region placement.
- Add app templates.

### Files Likely To Be Touched

- New or existing service APIs.
- CF-Provisioner service and repositories.
- CF-Router route table if new public paths are added.
- OpenBao library and service integration.
- Dev examples and smoke tests.
- CI workflows.

### Risks

- Expanding into full PaaS scope before the MVP is stable.
- Adding autoscaling without reliable metrics.
- Adding secrets without clear audit and access policy.
- Adding custom domains without secure ownership validation.

### Validation Criteria

- Each enhancement has its own plan task.
- Enhancements preserve tenant isolation.
- Public exposure remains internet-gateway mediated.
- Backwards compatibility is maintained for existing app service specs.

---

## Recommended Next Implementation Step

Start with Phase 2 as a concrete task:

1. Extend the CF-Provisioner OpenAPI spec with app service schemas and endpoints.
2. Add service-layer models and validation only.
3. Add ScyllaDB schema for durable app service records and subnet records.
4. Do not create Kubernetes workloads yet.

This gives CloudForge a stable contract and persistence model before adding the more error-prone Kubernetes reconciliation path.

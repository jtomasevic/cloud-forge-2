# CloudForge Private Network — Implementation Plan

## What This Plan Is

This plan translates the architecture proposal in [`docs/cf-private-network.md`](../cf-private-network.md) into an ordered sequence of executable implementation tasks. It now also includes the CF App Service expansion described in [`docs/cf-app-service.md`](../cf-app-service.md). Each task file is a self-contained prompt written for implementation work. Tasks must be executed in order unless stated otherwise, because later tasks depend on artifacts produced by earlier ones.

The plan is iterative by design: infrastructure libraries are built first, then each CF service is built layer by layer (OpenAPI spec → codegen → repository → service → REST), and finally the developer environment is assembled to make everything runnable locally.

---

## What Is Covered

### Phase 1 — Repository Foundation (Tasks 1–4)
Establishes the Go workspace, module layout, and shared infrastructure libraries that every CF service depends on.

- Go module skeleton with `go.work`
- `libs/cloudforge-core` — shared domain types, platform errors, HTTP middleware
- `libs/scylladb` — ScyllaDB session management and schema migration tooling
- `libs/openbao` — OpenBao (Vault-compatible) secrets client

### Phase 2 — CF-Accounts Service (Tasks 5–10)
Implements the authoritative registry for accounts, tenants, private networks, and API credentials. This is the first full CF service, built strictly layer by layer following the 3-layer REST architecture.

- OpenAPI 3.0.3 spec (`api/cf-accounts/v1/openapi.yaml`)
- oapi-codegen server stubs and generated client library
- ScyllaDB schema and migration scripts for all CF-Accounts tables
- Repository layer: `accounts`, `tenants`, `networks`, `credentials`
- Service layer: `CFAccountsService` with full business logic
- REST layer: handlers, middleware chain, error mapping
- `main.go`, Dockerfile, and basic configuration

### Phase 3 — CF-Provisioner Service (Tasks 11–16)
Implements the infrastructure provisioning and lifecycle engine. This service creates vCluster instances, allocates CIDRs, applies Cilium policies, stores kubeconfigs in OpenBao, and manages internet gateways.

- OpenAPI 3.0.3 spec (`api/cf-provisioner/v1/openapi.yaml`)
- oapi-codegen server stubs and generated client library
- Repository layer — infra clients: vCluster (host Kubernetes API), Cilium, Envoy Gateway HTTPRoute, OpenBao kubeconfig
- Repository layer — state: CIDR allocation (ScyllaDB), provisioning jobs (ScyllaDB)
- Service layer: `CFProvisionerService` orchestration logic with async job support
- REST layer, `main.go`, and Dockerfile

### Phase 4 — CF-Router Service (Tasks 17–18)
Implements the tenant-aware platform routing layer that sits behind Envoy Gateway. CF-Router validates credentials, resolves tenant context via CF-Accounts, and routes enriched requests to the correct CF service.

- OpenAPI 3.0.3 spec (`api/cf-router/v1/openapi.yaml`)
- All layers: repository (API key lookup), service (JWT/key validation + tenant resolution), REST, `main.go`, Dockerfile

### Phase 5 — Developer Environment (Tasks 19–22)
Assembles a fully functional local development environment where all three CF services run together and are accessible via `localhost`.

- k3d cluster with Cilium and vCluster CRDs installed
- Docker Compose for ScyllaDB, OpenBao (dev mode), and Keycloak (seeded dev realm)
- Tiltfile for live-reload local development loop
- Envoy Gateway configuration and GitHub Actions CI/CD workflows

### Phase 5 Follow-up — Signup And Login (Tasks 23–24)
Completes the first usable account flow on top of the local development environment.

- Signup creates a Keycloak identity and active default tenant
- Login returns a Keycloak access token usable with CF-Router and Swagger Authorize

### Phase 6 — CF App Service (Tasks 25–37)
Extends CloudForge Private Network with Docker-based application workloads placed into private or public subnets.

- Durable subnet persistence in ScyllaDB
- CF App Service OpenAPI contract and generated clients
- App service persistence and repository layer
- Kubernetes Deployment/Service workload mapping
- Service-specific Envoy Gateway and Cilium routing
- Service-layer and REST-layer orchestration
- Public Swagger/OpenAPI page for every app service exposed through Internet Gateway
- Local k3d/Tilt examples and smoke tests
- gRPC/TCP route support after HTTP MVP
- Production hardening and CI/CD validation

---

## End Result

After all private-network foundation tasks are complete, the following is true:

1. **All three CF services compile and pass tests** — CF-Accounts, CF-Provisioner, CF-Router
2. **The full tenant onboarding flow works locally** — a developer can create an account, provision a private network (a real vCluster on k3d), and verify Cilium isolation policies are applied
3. **The internet gateway flow works locally** — a tenant can provision a gateway and an Envoy Gateway HTTPRoute is created
4. **API credentials work end-to-end** — an API key can be created, its hash verified against ScyllaDB, and the request routed to the correct service via CF-Router
5. **All services are accessible via localhost** — see the Local Access section below
6. **CI/CD is operational** — GitHub Actions runs lint, unit tests, integration tests, codegen drift check, and image builds on every PR

After the CF App Service tasks are complete, the following is also true:

1. **Tenant workloads can be deployed** — users can create Docker-based app services inside active private networks
2. **Subnet placement is durable** — private/public subnets are persisted in ScyllaDB and can be used for app placement after process restart
3. **Public exposure is service-specific** — Internet Gateway routes point to the selected app service backend, not a placeholder
4. **Public app services are documented** — every app service exposed through Internet Gateway has a publicly reachable Swagger page and OpenAPI JSON route
5. **Protocol support is extensible** — HTTP/REST is the MVP path, with gRPC and TCP route support planned after the core model is stable

---

## What Is Necessary for Development Mode

### Required Local Tools

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.23+ | Building all CF services |
| Docker | 24+ | Running containers (ScyllaDB, OpenBao, Keycloak) |
| k3d | 5+ | Local Kubernetes cluster |
| kubectl | 1.29+ | Interacting with k3d cluster |
| Helm | 3.14+ | Installing Cilium, Envoy Gateway, vCluster CRDs |
| Tilt | 0.33+ | Local dev loop with live reload |
| oapi-codegen | v2 | OpenAPI → Go code generation |

Install everything with:
```bash
make dev-tools
```
(This target is created in Task 21.)

### Required Services

All backing services run via Docker Compose (Task 20):
```bash
docker compose -f dev/docker-compose.yml up -d
```

This starts:
- **ScyllaDB** on `localhost:9042` (CQL)
- **OpenBao** on `localhost:8200` (HTTP, dev mode, root token `dev-root-token`)
- **Keycloak** on `localhost:8080` (HTTP, admin: `admin` / `admin`, dev realm: `cloudforge`)

### Required Kubernetes Infrastructure

The local k3d cluster with Cilium and vCluster support (Task 19):
```bash
make k3d-up
```

This creates a k3d cluster named `cloudforge-dev` and installs:
- Cilium (eBPF network enforcement, policy audit mode on for dev)
- Envoy Gateway
- vCluster Helm chart (CRDs only, instances provisioned dynamically)
- cert-manager

---

## Local Access to CF Services

Once `tilt up` is running (Task 21), all CF services are live-reloaded on code changes and accessible at:

| Service | Local URL | Description |
|---|---|---|
| CF-Accounts | `http://localhost:8081` | Account/tenant/network registry API |
| CF-Provisioner | `http://localhost:8082` | Infrastructure provisioning API |
| CF-Router | `http://localhost:8083` | Tenant-aware platform router (use this for all API calls) |
| Envoy Gateway | `http://localhost:18080` | Edge gateway via k3d LB (routes to CF-Router; see Task 19 `CF_K3D_LB_HTTP_PORT`) |
| Keycloak | `http://localhost:8084` | Identity (console: `/auth/admin`) |
| ScyllaDB | `localhost:9042` | CQL (use `cqlsh localhost 9042`) |
| OpenBao | `http://localhost:8200` | Secrets (token: `dev-root-token`) |

All external API calls should go through CF-Router on port 8083 (or Envoy Gateway on the k3d LB HTTP port, default 18080, which proxies to CF-Router).

---

## Schema Migrations

ScyllaDB schema migrations are managed by the `tools/migrations` module. Run them with:
```bash
make migrate-up
```

Migrations live in `tools/migrations/scripts/` and are idempotent (use `IF NOT EXISTS`).

---

## Codegen

When an OpenAPI spec changes, regenerate all affected artifacts with:
```bash
make codegen
```

This runs `go generate ./...` across all service modules. CI enforces that generated files are always up to date (`codegen-check` job in `lint.yml`).

---

## Task List

Tasks must be executed in order. Each task file contains a complete Claude Sonnet 4.6 prompt with full context, requirements, file paths, and acceptance criteria.

### Phase 1 — Repository Foundation

1. [01.RepositorySkeletonAndGoWork.md](01.RepositorySkeletonAndGoWork.md) — Go workspace, all module `go.mod` files, directory structure
2. [02.CloudforgeCoreLibrary.md](02.CloudforgeCoreLibrary.md) — `libs/cloudforge-core`: shared types, platform errors, HTTP middleware
3. [03.ScyllaDBClientLibrary.md](03.ScyllaDBClientLibrary.md) — `libs/scylladb`: session management and migration tooling
4. [04.OpenBaoClientLibrary.md](04.OpenBaoClientLibrary.md) — `libs/openbao`: secrets read/write/delete/revoke client

### Phase 2 — CF-Accounts Service

5. [05.CFAccountsOpenAPISpec.md](05.CFAccountsOpenAPISpec.md) — Full OpenAPI 3.0.3 spec for CF-Accounts
6. [06.CFAccountsCodegen.md](06.CFAccountsCodegen.md) — oapi-codegen configs, server stubs, generated client library
7. [07.CFAccountsSchemaAndMigrations.md](07.CFAccountsSchemaAndMigrations.md) — ScyllaDB keyspace, tables, and migration scripts
8. [08.CFAccountsRepositoryLayer.md](08.CFAccountsRepositoryLayer.md) — All four repository subfolders
9. [09.CFAccountsServiceLayer.md](09.CFAccountsServiceLayer.md) — `AccountsService` interface and `CFAccountsService` implementation
10. [10.CFAccountsRESTLayerAndWiring.md](10.CFAccountsRESTLayerAndWiring.md) — Handlers, server, error mapping, `main.go`, Dockerfile

### Phase 3 — CF-Provisioner Service

11. [11.CFProvisionerOpenAPISpec.md](11.CFProvisionerOpenAPISpec.md) — Full OpenAPI 3.0.3 spec for CF-Provisioner
12. [12.CFProvisionerCodegen.md](12.CFProvisionerCodegen.md) — oapi-codegen configs, server stubs, generated client library
13. [13.CFProvisionerInfraRepositoryLayer.md](13.CFProvisionerInfraRepositoryLayer.md) — vCluster, Cilium, gateway, kubeconfig clients
14. [14.CFProvisionerStateRepositoryLayer.md](14.CFProvisionerStateRepositoryLayer.md) — CIDR allocation and provisioning jobs (ScyllaDB)
15. [15.CFProvisionerServiceLayer.md](15.CFProvisionerServiceLayer.md) — `ProvisionerService` interface and `CFProvisionerService` orchestration
16. [16.CFProvisionerRESTLayerAndWiring.md](16.CFProvisionerRESTLayerAndWiring.md) — Handlers, server, `main.go`, Dockerfile

### Phase 4 — CF-Router Service

17. [17.CFRouterOpenAPISpec.md](17.CFRouterOpenAPISpec.md) — Full OpenAPI 3.0.3 spec for CF-Router
18. [18.CFRouterImplementation.md](18.CFRouterImplementation.md) — All layers, codegen, `main.go`, Dockerfile

### Phase 5 — Developer Environment

19. [19.DevEnvironmentK3dCiliumVCluster.md](19.DevEnvironmentK3dCiliumVCluster.md) — k3d cluster, Cilium, Envoy Gateway, vCluster CRDs, cert-manager
20. [20.DevEnvironmentDockerCompose.md](20.DevEnvironmentDockerCompose.md) — Docker Compose for ScyllaDB, OpenBao, Keycloak
21. [21.TiltfileAndLocalDevLoop.md](21.TiltfileAndLocalDevLoop.md) — Tiltfile, Makefile, local access documentation
22. [22.EnvoyGatewayAndCICD.md](22.EnvoyGatewayAndCICD.md) — Envoy Gateway HTTPRoute manifests and GitHub Actions CI/CD workflows

### Phase 5 Follow-up — Signup And Login

23. [23.SignUpFollowUp.md](23.SignUpFollowUp.md) — Signup identity creation and default tenant activation
24. [24.CFAccountsLoginToken.md](24.CFAccountsLoginToken.md) — Login endpoint returns Keycloak access token

### Phase 6 — CF App Service

25. [25.CFProvisionerDurableSubnetPersistence.md](25.CFProvisionerDurableSubnetPersistence.md) — Persist private/public subnet records in ScyllaDB
26. [26.CFAppServiceOpenAPISpec.md](26.CFAppServiceOpenAPISpec.md) — Define CF App Service API contract and public Swagger requirement
27. [27.CFAppServiceCodegenAndClient.md](27.CFAppServiceCodegenAndClient.md) — Regenerate server stubs and clients for app-service APIs
28. [28.CFAppServiceSchemaAndMigrations.md](28.CFAppServiceSchemaAndMigrations.md) — Add app service tables and lifecycle job types
29. [29.CFAppServiceRepositoryLayer.md](29.CFAppServiceRepositoryLayer.md) — Add durable app service state repository
30. [30.CFAppServiceKubernetesWorkloadRepository.md](30.CFAppServiceKubernetesWorkloadRepository.md) — Map app services to Kubernetes Deployment and Service
31. [31.CFAppServiceGatewayAndCiliumRouting.md](31.CFAppServiceGatewayAndCiliumRouting.md) — Route Internet Gateway traffic to specific app services and policies
32. [32.CFAppServiceServiceLayer.md](32.CFAppServiceServiceLayer.md) — Implement service-layer validation and orchestration
33. [33.CFAppServiceRESTLayerAndRouter.md](33.CFAppServiceRESTLayerAndRouter.md) — Expose REST handlers and CF-Router forwarding
34. [34.CFAppServicePublicSwaggerExposure.md](34.CFAppServicePublicSwaggerExposure.md) — Require and expose public Swagger/OpenAPI docs for internet-exposed app services
35. [35.CFAppServiceLocalDevLoopAndExamples.md](35.CFAppServiceLocalDevLoopAndExamples.md) — Add local examples, smoke tests, and Make/Tilt support
36. [36.CFAppServiceProtocolRoutesGrpcTcp.md](36.CFAppServiceProtocolRoutesGrpcTcp.md) — Add gRPC and TCP Gateway API route support
37. [37.CFAppServiceProductionHardeningAndCICD.md](37.CFAppServiceProductionHardeningAndCICD.md) — Add production guardrails and CI/CD coverage

# CF App Service

**Status:** Draft  
**Audience:** Architects and Engineering Leadership  
**Module root:** `github.com/jtomasevic/cloud-forge-2`

---

## Table of Contents

1. [Purpose](#1-purpose)
2. [Problem Statement](#2-problem-statement)
3. [Goals](#3-goals)
4. [Non-Goals](#4-non-goals)
5. [Current Architecture Analysis](#5-current-architecture-analysis)
6. [Proposed Concepts](#6-proposed-concepts)
7. [Example User-Facing Spec](#7-example-user-facing-spec)
8. [Architecture](#8-architecture)
9. [Development Mode](#9-development-mode)
10. [Production Mode](#10-production-mode)
11. [Private vs Public Subnet Behavior](#11-private-vs-public-subnet-behavior)
12. [Security Model](#12-security-model)
13. [Open Source Comparison](#13-open-source-comparison)
14. [Design Alternatives](#14-design-alternatives)
15. [Recommended MVP](#15-recommended-mvp)
16. [Future Extensions](#16-future-extensions)
17. [Open Questions](#17-open-questions)
18. [Summary](#18-summary)

---

## 1. Purpose

CF App Service is the CloudForge application workload abstraction. It lets a tenant deploy a Docker-based application service into a CloudForge private network without directly managing Kubernetes Deployments, Services, Gateway API routes, or Cilium policies.

CloudForge already models the network foundation: tenant accounts, private networks, public and private subnets, vCluster boundaries, Envoy Gateway, Cilium enforcement, OpenBao-stored kubeconfigs, and control-plane routing through CF-Router. CF App Service is the next layer above that foundation. It gives users a simple way to place real workloads inside those isolated networks.

An app service can represent:

- REST service
- gRPC service
- UI/frontend service
- backend worker
- generic long-running containerized service
- any Docker-based application workload that can run in Kubernetes

The private network remains the structural isolation boundary. CF App Service does not replace the private network model; it consumes it.

---

## 2. Problem Statement

CloudForge can currently model and provision tenant private networks, but a tenant still needs a first-class way to run application workloads inside those networks.

Without CF App Service, users would need to understand and manage low-level Kubernetes primitives directly:

- Deployment
- Service
- container image or build pipeline
- resource requests and limits
- port and protocol mapping
- Gateway API routes
- Cilium policies
- public vs private subnet placement
- dev-vs-production ingress behavior

That leaks too much of CloudForge's implementation detail into the user experience. It also risks bypassing the tenant isolation model if users create Kubernetes objects manually or inconsistently.

The platform needs a workload abstraction that:

- fits into private networks and subnets
- preserves tenant isolation
- supports public exposure only through explicit internet gateway configuration
- maps predictably to Kubernetes and Gateway API primitives
- works locally with k3d, Tilt, Envoy Gateway, and local Docker image builds
- can evolve toward production registry, DNS, TLS, rollout, and observability capabilities

---

## 3. Goals

- Deploy Docker-based application services into a CloudForge private network.
- Support private subnet and public subnet placement.
- Support CPU and memory allocation.
- Support REST, gRPC, UI/frontend, backend worker, TCP, and generic containerized service shapes.
- Support local development mode.
- Support production deployment mode.
- Keep tenant isolation explicit and auditable.
- Integrate with the existing CloudForge networking model.
- Expose public services only through an internet gateway.
- Map to Kubernetes primitives in a way that remains understandable to operators.
- Keep the first version small enough to implement safely.
- Provide a standardized, discoverable API documentation page for every publicly exposed application service via network adapter, using Swagger/OpenAPI for HTTP/REST services and protocol-appropriate documentation for other service types.
---

## 4. Non-Goals

- CF App Service is not a replacement for Kubernetes.
- The first iteration should not become a full PaaS.
- The first iteration should not solve CI/CD end to end.
- Autoscaling should not be designed in depth until the runtime and deployment model are stable.
- The first iteration should not support every cloud-provider-specific load balancer.
- The first iteration should not expose private subnet workloads directly to the internet.
- The first iteration should not give tenants the control-plane management kubeconfig.
- The first iteration should not require a new networking model separate from CloudForge Private Network.

---

## 5. Current Architecture Analysis

### 5.1 Existing CloudForge Private Network Model

The current architecture proposal is in [`docs/cf-private-network.md`](cf-private-network.md). Its core model is:

- A CloudForge tenant owns one or more private networks.
- A private network is implemented as a per-tenant vCluster inside the host Kubernetes cluster.
- Each private network has isolated pod and service CIDRs.
- Cilium enforces default-deny behavior across tenant boundaries.
- Envoy Gateway is the public edge.
- CF-Router sits behind Envoy Gateway and performs tenant-aware platform routing.
- CF-Accounts owns account, tenant, network, credential, and provisioning state.
- CF-Provisioner owns infrastructure lifecycle: vClusters, CIDR allocation, Cilium policies, kubeconfigs, subnets, and internet gateway routing.
- OpenBao stores tenant management kubeconfigs and secrets.
- ScyllaDB stores operational control-plane state.
- Keycloak issues user identity tokens.

The relevant network behavior is:

| Communication | Current Model |
|---|---|
| Tenant to tenant | Blocked by structural isolation and Cilium default-deny |
| Control plane to tenant network | Via tenant vCluster Kubernetes API using management kubeconfig |
| Internet to tenant private subnet | Not allowed |
| Internet to tenant public subnet | Allowed only through explicit internet gateway routing |
| User to CloudForge API | Envoy Gateway -> CF-Router -> internal CF services |

### 5.2 Existing Repository Shape

The repository is a Go workspace with per-module `go.mod` files and a root `go.work`.

Relevant directories:

| Path | Current Role |
|---|---|
| `api/` | OpenAPI specs and codegen config for CloudForge services |
| `services/cf-accounts/` | Account, tenant, private-network, credential API and persistence |
| `services/cf-provisioner/` | vCluster, CIDR, Cilium, Gateway API, kubeconfig, subnet, and job orchestration |
| `services/cf-router/` | Tenant-aware reverse proxy and API aggregation surface |
| `libs/cloudforge-core/pkg/network/` | Shared network and subnet domain types |
| `dev/k8s/` | Local k3d, Cilium, Envoy Gateway, vCluster, cert-manager, and service manifests |
| `Tiltfile` | Local and k3d development loop |

There is no CF App Service API, CRD, controller, or worker today.

### 5.3 Current Service Boundaries

CloudForge services follow the repository's three-layer service shape:

- `internal/rest/` owns HTTP handlers, generated OpenAPI types, middleware, and DTO mapping.
- `internal/service/` owns business workflows and orchestration.
- `internal/repository/` owns ScyllaDB or infrastructure clients.

CF App Service should follow this same shape if implemented as a CloudForge service capability.

### 5.4 Current Network and Subnet Types

`libs/cloudforge-core/pkg/network/network.go` defines:

- `NetworkStatus`
- `SubnetType`
- `Network`
- `Subnet`

Subnets distinguish:

- `private`: resources are not reachable from the internet
- `public`: resources may be exposed via an internet gateway

CF App Service should reuse this vocabulary. It should not introduce a second placement model with different names.

### 5.5 Current CF-Provisioner Capabilities

CF-Provisioner already models the infrastructure pieces CF App Service needs to consume:

- `ProvisionNetwork`
- `DeprovisionNetwork`
- `ProvisionGateway`
- `RemoveGateway`
- `ProvisionSubnet`
- `ListSubnets`
- Cilium default-deny policy creation
- Cilium ingress policy creation
- Gateway API `HTTPRoute` creation
- vCluster kubeconfig retrieval and OpenBao storage
- async provisioning jobs

Important current limitation:

- Subnets are currently stored in process memory inside CF-Provisioner, not durably in ScyllaDB.
- Internet gateway routing currently creates an `HTTPRoute` to a placeholder backend service, not to a specific tenant app service.
- Gateway API support is currently HTTPRoute only.

CF App Service should be designed with these limitations visible. The MVP should either close them or explicitly constrain what can be built first.

### 5.6 Current Local Development Model

Local development has two modes:

| Mode | Command | Behavior |
|---|---|---|
| k3d mode | `make dev` | Docker Compose backing services, k3d control-plane services, Envoy Gateway edge, Tilt live loop |
| local mode | `make dev-local` | Docker Compose backing services and local Go service processes via Tilt |

The k3d development path uses:

- Docker Compose for ScyllaDB, OpenBao, and Keycloak
- k3d host cluster named `cloudforge-dev`
- Cilium
- Envoy Gateway
- vCluster CRDs/operator support
- cert-manager
- Tilt Docker image builds for CloudForge control-plane services
- Envoy Gateway hostnames such as `api.cloudforge.local`

CF App Service local development should fit this model:

- build the tenant app image locally
- load or push it to the local registry used by k3d
- deploy into the selected tenant vCluster or host namespace representing the vCluster
- expose public-subnet services through the local Envoy Gateway on the k3d load-balancer port
- keep private-subnet services reachable only inside the private network

---

## 6. Proposed Concepts

### 6.1 CF App Service

CF App Service is a user-facing CloudForge resource for deploying an application workload into a private network.

It captures:

- tenant network placement
- subnet placement
- container image or build definition
- CPU and memory allocation
- environment variables
- exposed ports
- protocol type
- worker vs networked service behavior
- optional internet gateway exposure

CF App Service is a CloudForge abstraction. It is not itself a Kubernetes Deployment, but the implementation may create one.

### 6.2 Service Runtime

Service Runtime defines how the workload runs:

- image reference, or Docker build context and Dockerfile
- command and args, if needed
- environment variables
- CPU and memory requests/limits
- ports and protocol metadata
- replica count, initially fixed

The MVP should support either:

- prebuilt image reference, or
- local Dockerfile build in development mode

Production should prefer immutable image references from a registry.

### 6.3 Network Placement

Network placement identifies where the app service lives:

- `networkRef`: which private network owns the service
- `subnetRef`: which subnet inside the network receives the service
- subnet type: private or public

Placement must be validated by CloudForge:

- the network must exist
- the network must be active
- the subnet must belong to that network
- public exposure is valid only for public subnets
- private subnet services must not get public routes

### 6.4 Public Exposure

Public exposure is an explicit decision. A service placed in a public subnet is not automatically internet reachable.

Public exposure requires:

- `exposure.type: InternetGateway`
- a valid `gatewayRef`
- a host name
- a port mapping
- a route type compatible with the service protocol

This preserves a clear distinction:

| Placement | Internet Gateway | Result |
|---|---|---|
| private subnet | no | internal only |
| private subnet | yes | invalid |
| public subnet | no | reachable only inside private network |
| public subnet | yes | publicly reachable through Envoy Gateway |

### 6.5 Internet Gateway

Internet Gateway is the CloudForge component that exposes public-subnet app services outside the private network.

In development mode:

- public means reachable from the developer's local machine
- routing goes through local Envoy Gateway and k3d load-balancer ports
- hostnames can use `*.local.cloudforge.dev` or `/etc/hosts` entries

In production mode:

- public means reachable from the internet
- routing goes through production Envoy Gateway or equivalent Gateway API infrastructure
- DNS and TLS are managed by production platform automation

Internet Gateway should own exposure. CF App Service should request exposure; it should not directly create public routes without gateway validation.

### 6.6 Communication Types

CF App Service must distinguish three types of communication:

| Type | Meaning |
|---|---|
| Internal service-to-service communication | Traffic between services inside the same private network |
| Private network communication | Traffic within the tenant vCluster and its subnets |
| Public internet exposure | Traffic entering from outside the tenant network through an internet gateway |

This distinction matters because a service can be networked internally but not public.

---

## 7. Example User-Facing Spec

The exact API shape should be decided during implementation. The following YAML is the proposed user-facing resource shape, not a committed CRD.

### 7.1 Public REST Service

```yaml
apiVersion: forge.cloud/v1alpha1
kind: CFAppService
metadata:
  name: orders-api
spec:
  networkRef:
    name: customer-network

  subnetRef:
    name: public-subnet

  runtime:
    build:
      dockerfile: ./Dockerfile
      context: .
    resources:
      cpu: "500m"
      memory: "512Mi"
    env:
      - name: LOG_LEVEL
        value: info
    ports:
      - name: http
        containerPort: 8080
        protocol: HTTP

  exposure:
    type: InternetGateway
    gatewayRef:
      name: main-internet-gateway
    host: orders.local.cloudforge.dev
    portRef: http
```

### 7.2 Private Backend Service

```yaml
apiVersion: forge.cloud/v1alpha1
kind: CFAppService
metadata:
  name: invoice-worker
spec:
  networkRef:
    name: customer-network

  subnetRef:
    name: private-subnet

  runtime:
    image: registry.local/cloudforge/invoice-worker:dev
    resources:
      cpu: "250m"
      memory: "256Mi"
    env:
      - name: QUEUE_NAME
        value: invoices
    ports: []

  exposure:
    type: None
```

### 7.3 Public gRPC Service

```yaml
apiVersion: forge.cloud/v1alpha1
kind: CFAppService
metadata:
  name: pricing-grpc
spec:
  networkRef:
    name: customer-network

  subnetRef:
    name: public-subnet

  runtime:
    image: registry.local/cloudforge/pricing-grpc:dev
    resources:
      cpu: "500m"
      memory: "512Mi"
    ports:
      - name: grpc
        containerPort: 9090
        protocol: GRPC

  exposure:
    type: InternetGateway
    gatewayRef:
      name: main-internet-gateway
    host: pricing.local.cloudforge.dev
    portRef: grpc
```

### 7.4 Minimal REST API Shape

If this is exposed first as a CloudForge REST API rather than a Kubernetes CRD, the equivalent endpoint shape could be:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/networks/{networkId}/app-services` | Create app service in a private network |
| `GET` | `/v1/networks/{networkId}/app-services` | List app services for a network |
| `GET` | `/v1/app-services/{serviceId}` | Get app service status |
| `DELETE` | `/v1/app-services/{serviceId}` | Delete app service |
| `POST` | `/v1/app-services/{serviceId}/exposure` | Expose through internet gateway |
| `DELETE` | `/v1/app-services/{serviceId}/exposure` | Remove public exposure |

The REST API should still map to the same domain model as the YAML example.

---

## 8. Architecture

### 8.1 Layered Model

```
User / CLI / Console
        │
        ▼
Envoy Gateway
        │
        ▼
CF-Router
  - validate JWT or API key
  - resolve tenant context through CF-Accounts
  - inject X-CF-Tenant-ID, X-CF-Network-ID, X-CF-Region
        │
        ▼
CF App Service API
  - validate network and subnet placement
  - validate runtime and exposure rules
  - create async job
        │
        ▼
CF-Provisioner / App Service Reconciler
  - fetch tenant vCluster kubeconfig from OpenBao
  - create Kubernetes workload resources
  - create internal Service
  - create Gateway API route when exposed
  - apply or update Cilium policies
        │
        ▼
Tenant vCluster / Host Kubernetes
  - Deployment
  - Service
  - HTTPRoute / GRPCRoute / TCPRoute
  - NetworkPolicy / CiliumNetworkPolicy
```

### 8.2 Recommended Ownership

There are two implementation options:

1. Add CF App Service operations to CF-Provisioner.
2. Add a new `cf-app-service` control-plane service.

For the MVP, CF-Provisioner is the pragmatic owner because it already owns:

- vCluster access
- Gateway API clients
- Cilium clients
- kubeconfig retrieval/storage integration
- async jobs
- subnet and gateway operations

As CF App Service grows, it may justify a dedicated service. The API boundary should be designed so this extraction is possible later.

### 8.3 Kubernetes Mapping

| CF App Service Concept | Kubernetes / Gateway Mapping |
|---|---|
| App service | Deployment or Stateful workload, initially Deployment |
| Service runtime image | `containers[].image` |
| Dockerfile build | Local dev build or CI-produced image |
| CPU/memory | container `resources.requests` and `resources.limits` |
| Environment variables | container `env` |
| Port | container port and Kubernetes Service port |
| Private placement | labels/namespace/policy inside tenant vCluster |
| Public placement | same as private placement plus public-subnet label |
| Internet Gateway exposure | Gateway API route to the service |
| HTTP service | `HTTPRoute` |
| gRPC service | `GRPCRoute` or HTTPRoute with protocol-specific backend policy, depending Gateway API support |
| TCP service | `TCPRoute` |
| Default isolation | Cilium policy |

### 8.4 CloudForge Abstractions vs Implementation Details

| User Sees | CloudForge Internally Creates |
|---|---|
| `CFAppService` | Deployment |
| `runtime.ports` | Service ports and container ports |
| `resources.cpu` / `resources.memory` | Kubernetes resource requests/limits |
| `subnetRef` | labels, selectors, policy inputs, placement metadata |
| `exposure.type: InternetGateway` | Gateway API route and Cilium ingress allowance |
| app service status | Deployment readiness, Service existence, route status, policy status |

The user should not need to author Kubernetes manifests for the MVP.

### 8.5 Deployment Flow

```
User                 CF-Router        CF-Provisioner        OpenBao        Tenant vCluster
 │                       │                  │                 │                 │
 │ POST /app-services    │                  │                 │                 │
 │──────────────────────►│                  │                 │                 │
 │                       │ validate tenant  │                 │                 │
 │                       │ context          │                 │                 │
 │                       │─────────────────►│                 │                 │
 │                       │                  │ validate network/subnet           │
 │                       │                  │ create job                        │
 │                       │                  │ fetch kubeconfig │                 │
 │                       │                  │────────────────►│                 │
 │                       │                  │◄────────────────│                 │
 │                       │                  │ create Deployment/Service          │
 │                       │                  │──────────────────────────────────►│
 │                       │                  │ apply policy labels/policy         │
 │                       │                  │──────────────────────────────────►│
 │◄──────────────────────│ 202 Accepted     │                 │                 │
 │                       │                  │                 │                 │
 │ GET /app-services/{id}│                  │                 │                 │
 │──────────────────────►│─────────────────►│ read readiness/status             │
 │◄──────────────────────│ 200 status       │                 │                 │
```

### 8.6 Public Exposure Flow

```
User                 CF-Router        CF-Provisioner        Envoy Gateway       Tenant vCluster
 │                       │                  │                    │                    │
 │ POST exposure          │                  │                    │                    │
 │──────────────────────►│                  │                    │                    │
 │                       │ validate tenant  │                    │                    │
 │                       │─────────────────►│                    │                    │
 │                       │                  │ verify public subnet                    │
 │                       │                  │ verify internet gateway                  │
 │                       │                  │ create HTTPRoute/GRPCRoute/TCPRoute ───►│
 │                       │                  │ apply Cilium ingress policy ───────────►│
 │◄──────────────────────│ 202 Accepted     │                    │                    │
```

### 8.7 Worker Services

Worker services may have no exposed ports.

For workers:

- Deployment is created.
- No Kubernetes Service is required unless internal discovery is needed.
- No Gateway API route is allowed.
- Placement still matters because network policies and allowed egress may differ by subnet.

---

## 9. Development Mode

Development mode should work with the existing `make dev` stack:

- Docker Compose backing services
- k3d host cluster
- Cilium
- Envoy Gateway
- vCluster
- Tilt
- local hostnames

### 9.1 Image Build

For local development, CF App Service can support:

- Dockerfile build from local context
- image tagged into the local k3d registry
- direct image loading into k3d if a registry is not available

The MVP should prefer the existing local registry path if available, because it matches the k3d model and avoids special image-loading behavior per app service.

### 9.2 Deployment Target

The app should be deployed into the tenant private network boundary:

- preferred: tenant vCluster using the tenant management kubeconfig stored in OpenBao
- fallback for early local MVP: host namespace that represents the tenant vCluster only if true vCluster deployment is not yet reliable

The fallback must be explicitly marked as a development-only shortcut. It must not become the production model.

### 9.3 Local Public Exposure

In development mode, public exposure means reachable from the local machine.

Expected behavior:

- A public app service with internet gateway exposure receives a local hostname.
- Envoy Gateway routes traffic from the k3d load-balancer port to the app service.
- The app is reachable through a URL such as `http://orders.local.cloudforge.dev:18080` or a configured `/etc/hosts` alias.

Private services must not receive a local public route.

### 9.4 Local Private Reachability

Private services are reachable only from inside the tenant private network.

Useful validation options:

- run a temporary debug pod inside the same tenant vCluster
- call the private service DNS name from that pod
- verify the same service is not reachable through Envoy Gateway
- verify another tenant network cannot reach it

---

## 10. Production Mode

Production mode should preserve the same CloudForge model with production-grade implementations:

- images are built by CI/CD and pushed to a registry
- CF App Service references immutable image tags or digests
- tenant workload resources are created inside the tenant vCluster
- public exposure is created only by internet gateway workflow
- DNS is created or verified by platform automation
- TLS is issued by cert-manager or an equivalent certificate authority integration
- Gateway API routes are reconciled by production Envoy Gateway infrastructure
- Cilium policies enforce tenant isolation and public-ingress boundaries

Production should avoid accepting arbitrary Dockerfile contexts directly through the control-plane API unless there is a secure build service. The MVP can support Dockerfile build in local development, but production should start with registry image references.

---

## 11. Private vs Public Subnet Behavior

| Service Placement | Internet Gateway | Reachability | Routing | Security | Typical Use Case |
|---|---|---|---|---|---|
| Private subnet | None | Same private network only | Kubernetes Service DNS inside tenant vCluster | No public route; default-deny outside network | internal APIs, workers, databases, queues |
| Private subnet | Requested | Invalid | No route created | Request rejected by CloudForge validation | prevented misconfiguration |
| Public subnet | None | Same private network only | Kubernetes Service DNS inside tenant vCluster | Eligible for future exposure, but not public yet | frontend staged before release, public API not yet opened |
| Public subnet | Internet Gateway | Public through Envoy Gateway plus internal private-network reachability | Gateway API route to Kubernetes Service | Cilium ingress policy allows specific gateway path/port | public REST API, UI frontend, public gRPC endpoint |

Important distinction:

Public subnet placement is not the same as public exposure. Placement only means the service is eligible for internet gateway routing. Exposure is separate and explicit.

---

## 12. Security Model

### 12.1 Default-Deny Networking

CloudForge Private Network already assumes Cilium default-deny across tenant boundaries. CF App Service must preserve that.

App service creation must not weaken:

- tenant-to-tenant isolation
- private subnet isolation from internet traffic
- control-plane access boundaries

### 12.2 Service-to-Service Access Inside a Private Network

The MVP can allow service-to-service communication inside the same private network, but this should be a conscious default.

Future policy can narrow this to:

- same subnet only
- explicit app-to-app allow rules
- protocol and port restrictions
- service account identity-based access

### 12.3 Public Exposure Only Through Internet Gateway

No app service should create a public `LoadBalancer`, `NodePort`, ingress, or route independently.

All public ingress must go through:

1. public subnet placement validation
2. internet gateway validation
3. Gateway API route creation
4. Cilium ingress policy update

### 12.4 Resource Limits

CPU and memory limits are required for predictable multi-tenant behavior.

The MVP should require at least:

- memory request/limit
- CPU request/limit
- platform defaults when omitted
- maximum bounds per tenant or plan

### 12.5 Tenant Isolation

CF App Service must use the already-resolved tenant context from CF-Router and CF-Accounts.

Tenant identity should not be accepted from user-supplied headers or request bodies alone. The service should derive ownership from trusted router context:

- `X-CF-Tenant-ID`
- `X-CF-Account-ID`
- `X-CF-Network-ID`
- `X-CF-Region`

### 12.6 Public Service Risk

Public services introduce risk:

- internet-originated traffic
- abuse and rate-limit pressure
- accidental exposure of administrative routes
- weak application authentication
- TLS and host ownership issues

Future controls should include:

- TLS policies
- WAF integration
- rate limiting
- request body limits
- auth policy integration
- audit logs
- per-service public exposure approval

---

## 13. Open Source Comparison

| Solution | Useful Idea | What CloudForge Should Avoid |
|---|---|---|
| Kubernetes Deployment / Service | Mature workload and internal service primitives | Exposing raw Kubernetes complexity directly to CloudForge users |
| Kubernetes Gateway API | Standard route model for HTTP, gRPC, and TCP | Letting users bypass CloudForge internet gateway validation |
| Knative Service | Simple service abstraction and scale-to-zero ideas | Pulling in serverless semantics before basic services are stable |
| Dapr app model | Sidecar-based service invocation, pub/sub, bindings | Requiring sidecars or Dapr runtime for MVP workloads |
| Docker Compose service model | Simple developer-friendly service definition | Treating Compose networking as production architecture |
| Heroku app model | Developer-friendly app abstraction | Hiding too much operational state from platform operators |
| Cloud Foundry app model | Buildpack/app lifecycle and route mapping | Rebuilding a full PaaS too early |
| AWS ECS/Fargate service | Task definition, service placement, resources | Cloud-provider-specific assumptions in the core model |
| Kubernetes NetworkPolicy / Cilium | Default-deny and explicit allow policies | Relying on ad hoc per-service policies without structural tenant isolation |

Useful ideas for CloudForge:

- Kubernetes should remain the substrate.
- Gateway API should remain the routing substrate.
- Cilium should remain the network enforcement substrate.
- CloudForge should expose a smaller user-facing abstraction that validates placement and exposure.
- Production image handling should use immutable registry references.
- Local development can accept Dockerfile build convenience.

---

## 14. Design Alternatives

### 14.1 Alternative 1 — Direct Kubernetes-Native API Only

Users deploy their own Deployments, Services, and Gateway API routes into their tenant vCluster.

Benefits:

- minimal CloudForge implementation
- high flexibility for Kubernetes users
- no custom app abstraction

Problems:

- too much Kubernetes knowledge required
- public exposure can bypass CloudForge policy if not heavily constrained
- hard to provide a consistent developer experience
- difficult to enforce private/public subnet semantics
- difficult to support non-Kubernetes users

This should not be the primary MVP path.

### 14.2 Alternative 2 — CloudForge CFAppService Abstraction Over Kubernetes

Users define an app service. CloudForge validates it and maps it to Kubernetes objects.

Benefits:

- simple user model
- preserves CloudForge network semantics
- keeps Kubernetes as implementation detail
- compatible with existing CF-Provisioner responsibilities
- can evolve gradually
- suitable for REST, gRPC, UI, worker, and generic service workloads

Problems:

- CloudForge must design and own a stable service API
- advanced Kubernetes features need later escape hatches or extensions
- controller/reconciliation behavior must be implemented carefully

This is the recommended approach.

### 14.3 Alternative 3 — Full PaaS-Style Abstraction

Users push source code or Dockerfiles. CloudForge builds, deploys, routes, scales, logs, rolls back, and manages releases.

Benefits:

- very simple user experience
- strong product direction
- can hide almost all infrastructure

Problems:

- too large for the first version
- requires build service, registry, logs, releases, rollbacks, secrets, health checks, and autoscaling
- risks hiding important networking boundaries
- can distract from the private-network foundation

This should be a future direction, not the MVP.

### 14.4 Recommendation

Use Alternative 2: a CloudForge `CFAppService` abstraction over Kubernetes.

This aligns with the existing architecture:

- CF-Router remains tenant-aware request entry.
- CF-Accounts remains authoritative tenant/network registry.
- CF-Provisioner remains infrastructure lifecycle owner.
- vCluster remains the tenant boundary.
- Cilium remains network enforcement.
- Envoy Gateway remains public ingress.

---

## 15. Recommended MVP

The smallest useful first version should include:

- CFAppService domain model.
- REST API or YAML-driven API for creating app services.
- Deployment of a containerized app into a selected private network.
- Private subnet placement.
- Public subnet placement.
- Public exposure through internet gateway in development mode.
- Mapping to Kubernetes Deployment and Service.
- HTTP support first through Gateway API `HTTPRoute`.
- gRPC support next.
- Simple resource requests and limits.
- Basic app service status.
- Basic examples and smoke tests.

MVP should not include:

- autoscaling
- multi-container pods
- sidecars
- rollout strategies beyond Kubernetes default rolling update
- production build service
- advanced secrets/config management
- full observability stack
- service mesh

---

## 16. Future Extensions

- Autoscaling.
- Health checks.
- Rolling deployment controls.
- Blue/green and canary strategies.
- Service mesh integration.
- TLS automation.
- DNS automation.
- Secrets integration through OpenBao.
- Config maps.
- Logs and metrics.
- Tracing.
- Private service discovery.
- CI/CD integration.
- Build service.
- Image vulnerability scanning.
- Multi-region placement.
- Per-service egress rules.
- Internal-only service auth.
- Rate limiting and WAF policy.
- Custom domains.
- App templates.

---

## 17. Open Questions

1. Should CF App Service be implemented inside CF-Provisioner first, or should it be a new `cf-app-service` control-plane service?
2. Should the user-facing API be REST-first, CRD-first, or both?
3. How should subnet records be persisted before app placement depends on them?
4. What is the durable schema for app service records and deployment jobs?
5. Should app services be deployed directly into the tenant vCluster, or should the host cluster hold routing objects that point into vCluster services?
6. How should service DNS names be formed inside a private network?
7. What hostname convention should local public services use?
8. Should public exposure be part of the app service spec or a separate route/exposure resource?
9. What protocols are required in the MVP: HTTP only, HTTP + gRPC, or HTTP + gRPC + TCP?
10. How are image builds handled in production?
11. What per-tenant resource quotas exist at launch?
12. How are secrets and environment variables stored and audited?
13. How should CloudForge model app service deletion when public routes and Cilium policies exist?
14. How should failed deployments be retried or rolled back?
15. What minimum observability should be required before production?

---

## 18. Summary

CF App Service should be the CloudForge workload abstraction that lets users deploy Docker-based services into private networks.

The design should not bypass the existing CloudForge architecture. Private networks remain the isolation boundary. CF-Router remains the tenant-aware routing layer. CF-Accounts remains the tenant and network authority. CF-Provisioner remains the owner of infrastructure reconciliation. Envoy Gateway and Gateway API remain the public edge. Cilium remains the network enforcement layer.

The recommended path is a CloudForge `CFAppService` abstraction over Kubernetes primitives. The MVP should deploy one-container services with CPU/memory limits, private or public subnet placement, HTTP exposure through internet gateway, and local development support through the existing k3d + Envoy Gateway stack.

This keeps the first version narrow while establishing the core model for future CloudForge managed services.

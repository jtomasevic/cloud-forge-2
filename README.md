# CloudForge

CloudForge is an open-source, multi-tenant cloud platform built on Kubernetes.

The current implementation focuses on the **CloudForge Private Network**: the tenant isolation foundation that future CloudForge cloud services will run on. A private network gives each tenant a structurally isolated Kubernetes environment with separate pod/service CIDRs, DNS, network policy enforcement, and control-plane access boundaries.

This is the platform base layer, not the final product surface. Compute, database, messaging, storage, and other managed cloud services are expected to build on top of this private-network foundation.

## Architecture

CloudForge separates public edge concerns, tenant-aware platform routing, account state, infrastructure provisioning, identity, secrets, and network isolation:

- **Envoy Gateway** is the public edge. It terminates TLS, applies Gateway API routing, and sends CloudForge API traffic into the platform.
- **CF-Router** sits behind Envoy. It validates JWTs or API keys, resolves tenant context through CF-Accounts, injects trusted `X-CF-*` headers, and dispatches requests to internal services.
- **CF-Accounts** owns account, tenant, private-network, credential, and provisioning state.
- **CF-Provisioner** creates and manages tenant infrastructure: vClusters, CIDR allocations, Cilium policies, kubeconfigs, subnets, and internet gateways.
- **ScyllaDB** stores control-plane state.
- **OpenBao** stores secrets, kubeconfigs, and credentials.
- **Keycloak** provides user identity, OIDC, and JWT issuance.
- **vCluster** provides the per-tenant Kubernetes isolation boundary.
- **Cilium** enforces default-deny networking between tenant boundaries with eBPF.

Tenants do not communicate directly with other tenants or with control-plane pods. CloudForge services operate inside tenant environments through the tenant vCluster Kubernetes API using management kubeconfigs stored in OpenBao.

See [docs/cf-private-network.md](docs/cf-private-network.md) for the full architecture proposal.

## Repository Structure

- `api/` - OpenAPI 3.0.3 specifications. These are the source of truth for HTTP contracts.
- `services/` - CloudForge service binaries: `cf-router`, `cf-accounts`, and `cf-provisioner`.
- `libs/` - Shared libraries and generated client SDKs.
- `tools/` - CLI tools and the ScyllaDB migration runner.
- `dev/` - Docker Compose, k3d, Gateway API, and local development scripts.
- `docs/` - Architecture docs and implementation plans.

## Prerequisites

Install:

- Go 1.26+
- Docker
- k3d
- kubectl
- Helm
- Tilt
- `oapi-codegen` v2
- `mockgen`

You can install Go codegen tools and print links for the external tools with:

```bash
make dev-tools
```

For the k3d + Envoy Gateway loop, add local hostnames to `/etc/hosts`:

```text
127.0.0.1  api.cloudforge.local
127.0.0.1  auth.cloudforge.local
127.0.0.1  gateway.cloudforge.local
```

`make dev` warns if the required host entries are missing, but it does not edit `/etc/hosts`.

## Start Development From Zero

The default development path runs backing services in Docker Compose, CloudForge services in k3d, and Envoy Gateway as the local API edge:

```bash
make dev
```

`make dev` runs the full setup:

1. Checks required tools: Docker, k3d, kubectl, Helm, and Tilt.
2. Starts ScyllaDB, OpenBao, and Keycloak with Docker Compose.
3. Initializes Keycloak and OpenBao.
4. Applies ScyllaDB migrations.
5. Creates the `cloudforge-dev` k3d cluster if needed.
6. Installs Cilium, Envoy Gateway, vCluster, cert-manager, and the CloudForge namespace.
7. Applies local Envoy Gateway routes.
8. Starts Tilt in `CF_DEV_MODE=k8s`.

Useful local URLs:

- Tilt UI: `http://localhost:10350`
- CloudForge API through Envoy Gateway: `http://api.cloudforge.local:18080`
- Swagger UI: `http://api.cloudforge.local:18080/swagger/`

For a faster loop without k3d or Envoy Gateway, run local Go services through Tilt:

```bash
make dev-local
```

`make dev-local` starts and initializes Docker backing services, then starts Tilt in `CF_DEV_MODE=local`.

## Common Development Commands

```bash
make help            # list documented targets
make build           # build all Go modules
make test            # run unit tests for all modules
make lint            # run go vet for all modules
make verify          # run lint and test
make codegen         # run go generate across modules
make gateway-apply   # reapply local Envoy Gateway routes
```

Backing-service and Kubernetes setup commands:

```bash
make dev-init          # start Docker backing services, seed Keycloak/OpenBao, run migrations
make dev-setup         # dev-init + k3d dependencies + gateway routes
make k3d-up            # create the cloudforge-dev cluster
make k3d-install-deps  # install Cilium, Envoy Gateway, vCluster, cert-manager
make k3d-kubeconfig    # merge cloudforge-dev kubeconfig into your default kubeconfig
```

## Stop, Kill, and Reset

Use the least destructive command that matches what you need:

```bash
make dev-down
```

Stops Tilt resources and Docker backing services. It keeps Docker volumes and keeps the `cloudforge-dev` k3d cluster.

```bash
make dev-kill
```

Runs `dev-down` and deletes the `cloudforge-dev` k3d cluster. It keeps Docker Compose volumes, so ScyllaDB/OpenBao/Keycloak data remains.

```bash
make dev-reset
```

Stops Tilt/backing services and deletes Docker Compose volumes. This removes local ScyllaDB, OpenBao, and Keycloak data. Use this when you want a clean backing-service state.

```bash
make k3d-down
```

Deletes only the `cloudforge-dev` k3d cluster.

After a reset, start again with:

```bash
make dev
```

## API Contracts

OpenAPI specs under `api/` are the source of truth. After changing an API spec, regenerate code:

```bash
make codegen
```

Do not hand-edit generated server stubs, generated clients, or generated mocks.

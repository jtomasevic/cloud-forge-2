# CloudForge Private Network — Architecture Proposal

**Status:** Draft  
**Audience:** Architects and Engineering Leadership  
**Module root:** `github.com/jtomasevic/cloud-forge-2`

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Why Structural Tenant Isolation Is Required](#3-why-structural-tenant-isolation-is-required)
4. [What CloudForge Private Network Is](#4-what-cloudforge-private-network-is)
5. [Proposed Architecture](#5-proposed-architecture)
6. [Network Diagram](#6-network-diagram)
7. [Component Diagram](#7-component-diagram)
8. [Tenant Onboarding and Provisioning Flow](#8-tenant-onboarding-and-provisioning-flow)
9. [Internet Gateway and Public Exposure Flow](#9-internet-gateway-and-public-exposure-flow)
10. [Control Plane Communication Model](#10-control-plane-communication-model)
11. [Technology Validation and Alternatives](#11-technology-validation-and-alternatives)
12. [Developer Environment Proposal](#12-developer-environment-proposal)
13. [REST Service Architecture and OpenAPI Contract](#13-rest-service-architecture-and-openapi-contract)
14. [Golang Project Structure with go.work](#14-golang-project-structure-with-gowork)
15. [CI/CD Proposal](#15-cicd-proposal)
16. [Risks, Tradeoffs, and Open Questions](#16-risks-tradeoffs-and-open-questions)
17. [Final Recommendation](#17-final-recommendation)

---

## 1. Executive Summary

CloudForge is a multi-tenant cloud platform. Multiple customers share the same underlying Kubernetes infrastructure. The fundamental risk in this model is that one tenant, by accident or attack, can reach another tenant's workloads. That risk must be structurally eliminated — not just mitigated by policy.

CloudForge Private Network is the mechanism that provides each tenant with a fully isolated virtual network boundary. It is not a routing rule. It is not a firewall policy. It is a structural separation enforced at the Kubernetes virtual cluster level, at the network enforcement layer, and at the control plane's trust model.

The core building blocks are:
- **vCluster** (per-tenant virtual Kubernetes cluster) — structural isolation boundary
- **Cilium** — network enforcement with eBPF, default-deny between tenant boundaries
- **Envoy Gateway** — public-facing edge API gateway
- **CF-Router** — internal tenant-aware platform routing layer
- **CF-Accounts** — authoritative account, tenant, and private-network registry
- **CF-Provisioner** — infrastructure provisioning and lifecycle engine
- **ScyllaDB** — control-plane operational state store
- **OpenBao** — secrets and credential store
- **Keycloak** — user identity and SSO

This document proposes the architecture, technology choices, onboarding flows, control-plane interaction model, developer environment, Go project structure, and CI/CD strategy for the CloudForge Private Network capability.

---

## 2. Problem Statement

A multi-tenant platform concentrates multiple customers on shared infrastructure. This is economically necessary and operationally efficient. It also creates a structural risk: without deliberate isolation, any tenant can inadvertently — or deliberately — reach another tenant's services.

Specific failure modes that must be prevented:

| Failure Mode | Impact |
|---|---|
| Pod A (Tenant X) initiates TCP connection to Pod B (Tenant Y) | Cross-tenant data access |
| DNS resolution from Tenant X resolves a service owned by Tenant Y | Credential or traffic leakage |
| Control-plane credential compromise gives access to all tenants | Full platform breach |
| Misconfigured network policy allows broad tenant-to-tenant traffic | Regulatory and compliance violation |
| Internet-facing endpoint bypasses tenant subnet restriction | Exposure of private services |

Naive policy-based solutions (e.g., NetworkPolicy per namespace) are insufficient because:
- They depend on correct, ongoing configuration
- A single misconfiguration breaks the isolation guarantee
- They do not prevent DNS leakage or CIDR overlap in a flat cluster
- They do not provide compliance-grade evidence of separation

The platform requires a solution that makes cross-tenant communication structurally impossible, not merely forbidden by policy.

---

## 3. Why Structural Isolation Is Required

### 3.1 Policy-Only Isolation Is Not a Guarantee

Kubernetes NetworkPolicy is additive and opt-in. It depends on every team correctly configuring every service, every time. A single omitted label selector, a wildcard namespace, or an overly broad egress rule breaks the model. Policy drift is not a hypothetical — it is operationally inevitable at scale.

Structural isolation removes the human and configuration surface from the critical path. The isolation is enforced at the infrastructure level, not by policy managed by service owners.

### 3.2 Security

With structural isolation:
- A compromised pod inside Tenant A's environment cannot reach Tenant B's network, regardless of what code it runs
- There is no route between tenant networks at the IP level
- Cilium enforces default-deny at the eBPF layer before any packet leaves the virtual cluster boundary
  - Cilium blocks all traffic by default, using eBPF programs inside the Linux kernel, before that traffic can escape the tenant/virtual-cluster network boundary.
  - Cilium supports ingress and egress policy enforcement, meaning it can check both incoming and outgoing packets and decide whether they are allowed.
  - eBPF is a Linux kernel technology that lets Cilium attach small, safe programs directly into the kernel networking path.
  - instead of traffic being filtered later by an application proxy or userspace process, Cilium can make the allow/deny decision very early, inside the kernel datapath.

With policy-only isolation:
- A compromised workload can attempt lateral movement; policy misconfiguration may let it succeed
- Security depends on the correctness and completeness of every NetworkPolicy object, managed over the lifetime of the platform

### 3.3 Compliance

For tenants operating under SOC 2 Type II, PCI-DSS, HIPAA, or ISO 27001:
- Structural separation provides clear, auditable evidence of network boundary enforcement
- Policy-only solutions require auditors to evaluate the correctness of many individual policies — a brittle, audit-unfriendly model
- vCluster + Cilium produces an architecture where a single audit finding (the model itself is isolated) covers every tenant

### 3.4 Blast Radius Reduction

If a tenant's workload is compromised or behaves unexpectedly:
- With structural isolation: the blast radius is bounded to that tenant's virtual cluster
- With policy-only isolation: the blast radius depends on whether every adjacent policy was correctly written and has not drifted

An outage or security event in Tenant A's environment must never propagate to Tenant B. Structural boundaries make this a physical property of the system.

### 3.5 Customer Trust

Enterprise customers who evaluate multi-tenant platforms ask: "How do you ensure our data cannot be reached by another tenant?" The answer "we have network policies" is weak. The answer "each tenant runs in a structurally isolated virtual cluster with its own pod CIDR, service CIDR, DNS namespace, and network enforcement enforced at the eBPF layer" is auditable, credible, and defensible.

---

## 4. What CloudForge Private Network Is

### 4.1 Simple Explanation

When a customer creates an account on CloudForge, they can create a **private network** in a chosen region. That private network is their dedicated, isolated environment on the platform. Nothing outside their private network can reach it. Nothing inside their private network can reach another tenant's environment.

Inside a private network, the tenant can:
- Create **private subnets** — for services that should never be exposed to the internet
- Create **public subnets** — for services they choose to expose
- Attach an **internet gateway** — to allow inbound internet traffic to services in public subnets
- Deploy workloads into the environment using the CloudForge APIs

### 4.2 Technical Definition

A CloudForge Private Network is a **vCluster instance** running inside the CloudForge host Kubernetes cluster. It provides:

- A dedicated Kubernetes API server (per tenant), scoped to that tenant
- An isolated pod CIDR range, not overlapping with any other tenant
- An isolated service CIDR range, not overlapping with any other tenant
- An isolated DNS namespace (CoreDNS scoped to the vCluster)
- Cilium network enforcement policies that deny all traffic between vClusters by default
- A kubeconfig stored in OpenBao, used exclusively by the control plane for management access
- Subnet objects within the virtual cluster representing logical network segments (public/private)
- An optional internet gateway, provisioned by CF-Provisioner when requested

### 4.3 What It Is Not

- It is not a separate physical server or cloud VPC
- It is not enforced solely by Kubernetes RBAC
- It is not a firewall rule managed manually by an operator
- It is not optional or configurable by tenants — isolation is always enforced

---

## 5. Proposed Architecture

### 5.1 Layers of the Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Public Internet                          │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│              Envoy Gateway (edge)                           │
│   TLS termination · Gateway API routing · edge policies     │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                   CF-Router                                 │
│   Tenant identity resolution · platform routing             │
│   Context enrichment · internal service dispatch            │
└──────┬──────────────┬─────────────────┬────────────────────┘
       │              │                 │
┌──────▼───────┐ ┌────▼──────┐  ┌──────▼──────────┐
│  CF-Accounts │ │CF-Provisioner│  │ (other CF svcs) │
│  tenant/acct │ │infra lifecycle│  │                │
│  registry    │ │  engine   │  │                 │
└──────┬───────┘ └────┬──────┘  └─────────────────┘
       │              │
┌──────▼──────────────▼────────────────────────────────────────┐
│                  ScyllaDB (control-plane store)              │
│   tenant records · account metadata · network state         │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│                  OpenBao (secrets store)                     │
│   kubeconfigs · credentials · API keys · revocation         │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│                  Keycloak (identity)                         │
│   console login · OIDC · JWT issuance                       │
└──────────────────────────────────────────────────────────────┘

Host Kubernetes Cluster
┌──────────────────────────────────────────────────────────────┐
│  ┌──────────────────────┐   ┌──────────────────────────┐    │
│  │  vCluster: Tenant A  │   │  vCluster: Tenant B      │    │
│  │  pod CIDR: 10.1.0/16 │   │  pod CIDR: 10.2.0/16    │    │
│  │  svc CIDR: 10.11.0/16│   │  svc CIDR: 10.12.0/16   │    │
│  │  DNS: tenant-a.local │   │  DNS: tenant-b.local     │    │
│  │  ┌──────────────┐    │   │  ┌──────────────┐        │    │
│  │  │private subnet│    │   │  │private subnet│        │    │
│  │  ├──────────────┤    │   │  ├──────────────┤        │    │
│  │  │public subnet │    │   │  │public subnet │        │    │
│  │  └──────────────┘    │   │  └──────────────┘        │    │
│  └──────────────────────┘   └──────────────────────────┘    │
│                                                              │
│  Cilium: default-deny between vClusters (eBPF enforcement)  │
└──────────────────────────────────────────────────────────────┘
```

### 5.2 Responsibility Split

| Layer | Component | Responsibility |
|---|---|---|
| Public edge | Envoy Gateway | TLS termination, Gateway API routing, rate limiting, edge policy enforcement |
| Platform routing | CF-Router | Tenant identity resolution, internal routing to correct CF service, context enrichment |
| Account registry | CF-Accounts | Authoritative store of account ↔ tenant ↔ private network ↔ service relationships |
| Provisioning | CF-Provisioner | vCluster lifecycle, CIDR allocation, Cilium policies, kubeconfig management, internet gateway |
| Operational state | ScyllaDB | Tenant records, network state, provisioning jobs, slug resolution |
| Secrets | OpenBao | kubeconfigs, credentials, API keys, revocation of management access |
| Identity | Keycloak | Console login, OIDC flows, JWT issuance |
| Isolation boundary | vCluster | Per-tenant virtual Kubernetes cluster, scoped API, isolated CIDR, DNS namespace |
| Network enforcement | Cilium | eBPF-level default-deny between tenant boundaries, control-plane access protection |

### 5.3 CF-Router vs Envoy Gateway — A Clear Boundary

**Envoy Gateway** is the platform edge. It handles:
- TLS termination for all inbound traffic
- Routing rules expressed via Kubernetes Gateway API (`HTTPRoute`, `GatewayClass`)
- Rate limiting and edge-level traffic policy
- Exposure of tenant public endpoints (when an internet gateway is provisioned)
- Traffic entry toward CloudForge control-plane services

**CF-Router** is not an API gateway. It is a tenant-aware platform routing layer behind Envoy Gateway. It handles:
- Extracting and validating tenant identity from JWT (issued by Keycloak) or API key (verified via BLAKE2b-256 hash against ScyllaDB)
- Calling CF-Accounts to resolve the tenant context for the request
- Enriching request context with tenant metadata (tenant ID, private network ID, region)
- Routing the enriched request to the correct internal CF service (CF-Accounts, CF-Provisioner, or others)
- Enforcing CloudForge platform policies that are too specific for a general-purpose gateway

Envoy Gateway does not know about tenants. CF-Router does not terminate TLS. These are complementary, non-overlapping concerns.

### 5.4 CF-Accounts as a First-Class Control-Plane Service

CF-Accounts is the authoritative registry for the CloudForge tenant model. It is not a thin database wrapper. It owns:
- The lifecycle of a customer account (creation, suspension, deletion)
- The binding of a customer account to a tenant identity
- The binding of a tenant to its private network(s), region, and CIDR allocations
- The relationship between a tenant and its provisioned services
- The state of API credentials (creation, rotation, revocation)
- The provisioning state transitions (provisioning, active, suspended, deprovisioned)

CF-Accounts exposes a service API consumed by CF-Router (for tenant resolution), CF-Provisioner (for state transitions during provisioning), and future CF services that need tenant context.

ScyllaDB is the persistence layer for CF-Accounts. CF-Accounts owns its schema and query model.

---

## 6. Network Diagram

```
                         ┌──────────────────────────────────────────────────────────┐
                         │                   CLOUDFORGE PLATFORM                    │
                         │                                                          │
Internet ──────────────► │  ┌─────────────────┐                                    │
                         │  │  Envoy Gateway  │ ◄── TLS termination                │
                         │  │  (edge)         │ ◄── HTTPRoute / GatewayClass        │
                         │  └────────┬────────┘                                    │
                         │           │                                              │
                         │  ┌────────▼────────┐                                    │
                         │  │   CF-Router     │ ◄── JWT/API key identity resolution│
                         │  │ (platform layer)│ ◄── tenant context enrichment      │
                         │  └─┬───────┬───┬───┘                                    │
                         │    │       │   │                                         │
                         │  ┌─▼──┐ ┌──▼─┐ ┌▼────────┐                             │
                         │  │CF- │ │CF- │ │CF-      │                             │
                         │  │Acct│ │Prov│ │(others) │                             │
                         │  └─┬──┘ └──┬─┘ └─────────┘                             │
                         │    │       │                                             │
                         │  ┌─▼───────▼──────────────┐                             │
                         │  │        ScyllaDB         │                             │
                         │  │  (control-plane state)  │                             │
                         │  └─────────────────────────┘                             │
                         │  ┌─────────────────────────┐                             │
                         │  │        OpenBao          │                             │
                         │  │  kubeconfigs · secrets  │                             │
                         │  └─────────────────────────┘                             │
                         │  ┌─────────────────────────┐                             │
                         │  │        Keycloak          │                             │
                         │  │   identity / OIDC / JWT  │                             │
                         │  └─────────────────────────┘                             │
                         │                                                          │
                         │  ─ ─ ─ ─ ─ ─ Kubernetes host cluster ─ ─ ─ ─ ─ ─ ─    │
                         │                                                          │
                         │  ┌──────────────────────────────────────────────────┐   │
                         │  │  ┌──────────────────┐   ┌──────────────────┐    │   │
                         │  │  │  vCluster A      │   │  vCluster B      │    │   │
                         │  │  │ ┌──────────────┐ │   │ ┌──────────────┐ │    │   │
                         │  │  │ │ private snet │ │   │ │ private snet │ │    │   │
                         │  │  │ ├──────────────┤ │   │ ├──────────────┤ │    │   │
                         │  │  │ │ public snet  │─┼─► │ │ public snet  │ │    │   │
                         │  │  │ └──────────────┘ │X  │ └──────────────┘ │    │   │
                         │  │  └──────────────────┘   └──────────────────┘    │   │
                         │  │                                                  │   │
                         │  │   Cilium eBPF: default-deny across vCluster      │   │
                         │  │   boundaries. Only control-plane Kube API        │   │
                         │  │   (mTLS) is permitted to cross the boundary.     │   │
                         │  └──────────────────────────────────────────────────┘   │
                         └──────────────────────────────────────────────────────────┘

Control-plane to tenant: Kubernetes API over mTLS (kubeconfig stored in OpenBao)
Tenant-to-tenant:        BLOCKED at Cilium eBPF layer (no route exists)
Internet-to-tenant:      Only via Envoy Gateway → public subnet endpoints
```

---

## 7. Component Diagram

```
┌────────────────────────────────────────────────────────────────────────┐
│  CF CONTROL PLANE                                                      │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │  Envoy Gateway                                                  │  │
│  │  · GatewayClass / HTTPRoute (Kubernetes Gateway API)           │  │
│  │  · TLS termination (cert-manager issued certs)                 │  │
│  │  · Rate limiting, edge auth (ext_authz hook to CF-Router)      │  │
│  │  · Routes: /api/** → CF-Router, /console/** → Keycloak         │  │
│  └──────────────────────────────┬──────────────────────────────────┘  │
│                                 │                                      │
│  ┌──────────────────────────────▼──────────────────────────────────┐  │
│  │  CF-Router (internal service, not public)                       │  │
│  │  · Validates JWT (Keycloak) or API key (BLAKE2b-256 / ScyllaDB) │  │
│  │  · Calls CF-Accounts to resolve tenant context                  │  │
│  │  · Enriches request context (tenant_id, network_id, region)     │  │
│  │  · Routes to: CF-Accounts, CF-Provisioner, other CF services    │  │
│  └───────┬──────────────────────┬──────────────────────────────────┘  │
│          │                      │                                      │
│  ┌───────▼──────┐     ┌─────────▼────────────────┐                   │
│  │  CF-Accounts │     │  CF-Provisioner           │                   │
│  │              │     │                           │                   │
│  │  Owns:       │     │  Owns:                    │                   │
│  │  - account   │     │  - vCluster creation      │                   │
│  │  - tenant    │     │  - CIDR allocation        │                   │
│  │  - network   │     │  - Cilium policy apply    │                   │
│  │  - service   │     │  - kubeconfig gen/store   │                   │
│  │    bindings  │     │  - internet gateway setup │                   │
│  │  - API creds │     │  - subnet provisioning    │                   │
│  └───────┬──────┘     └──────────┬────────────────┘                   │
│          │                       │                                      │
│  ┌───────▼───────────────────────▼─────────┐                          │
│  │  ScyllaDB                               │                          │
│  │  Tables: accounts, tenants, networks,   │                          │
│  │  subnets, services, api_keys,           │                          │
│  │  provisioning_jobs, cidr_allocations    │                          │
│  └─────────────────────────────────────────┘                          │
│                                                                        │
│  ┌─────────────────┐   ┌──────────────────────┐                       │
│  │  OpenBao        │   │  Keycloak             │                       │
│  │  - kubeconfigs  │   │  - user accounts      │                       │
│  │  - API secrets  │   │  - OIDC provider      │                       │
│  │  - revocation   │   │  - tenant-aware realms│                       │
│  └─────────────────┘   └──────────────────────┘                       │
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │  cert-manager                                                    │  │
│  │  - certificates for Envoy Gateway TLS                           │  │
│  │  - certificates for internal mTLS service mesh                  │  │
│  │  - certificates for vCluster API server endpoints               │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────────┐
│  TENANT ISOLATION LAYER (Host Kubernetes Cluster)                      │
│                                                                        │
│  ┌────────────────────────────┐  ┌────────────────────────────────┐  │
│  │  vCluster: tenant-abc      │  │  vCluster: tenant-xyz          │  │
│  │  pod CIDR:  10.10.0.0/16   │  │  pod CIDR:  10.20.0.0/16      │  │
│  │  svc CIDR:  10.110.0.0/16  │  │  svc CIDR:  10.120.0.0/16     │  │
│  │  DNS: cluster.tenant-abc   │  │  DNS: cluster.tenant-xyz       │  │
│  │                            │  │                                │  │
│  │  Subnet: private-1         │  │  Subnet: private-1             │  │
│  │  Subnet: public-1          │  │  Subnet: public-1              │  │
│  │  Internet GW: optional     │  │  Internet GW: optional         │  │
│  └────────────────────────────┘  └────────────────────────────────┘  │
│                                                                        │
│  Cilium (eBPF):                                                        │
│  - CiliumNetworkPolicy: default-deny all cross-vCluster traffic       │
│  - Allows: control-plane Kubernetes API (mTLS) to each vCluster       │
│  - Allows: internet → public subnet endpoints (when GW provisioned)   │
│  - Denies: all vCluster-to-vCluster pod/service traffic               │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Tenant Onboarding and Provisioning Flow

### 8.1 Prerequisites

- Customer has registered via the CloudForge console
- Keycloak has issued an OIDC session and short-lived JWT for the user
- CF-Accounts has created an account record

### 8.2 Private Network Provisioning Flow

```
User (Console)         Envoy GW      CF-Router    CF-Accounts   CF-Provisioner   ScyllaDB    OpenBao
     │                    │               │              │              │              │           │
     │ POST /networks      │               │              │              │              │           │
     │ {region, name}      │               │              │              │              │           │
     │────────────────────►│               │              │              │              │           │
     │                     │ route /api/** │              │              │              │           │
     │                     │───────────────►              │              │              │           │
     │                     │               │ validate JWT  │              │              │           │
     │                     │               │ resolve tenant│              │              │           │
     │                     │               │──────────────►│              │              │           │
     │                     │               │               │ return tenant│              │           │
     │                     │               │               │ + account ctx│              │           │
     │                     │               │◄──────────────│              │              │           │
     │                     │               │               │              │              │           │
     │                     │               │ dispatch to CF-Provisioner   │              │           │
     │                     │               │────────────────────────────►│              │           │
     │                     │               │                              │ allocate CIDR│           │
     │                     │               │                              │──────────────►           │
     │                     │               │                              │ write job     │           │
     │                     │               │                              │──────────────►           │
     │                     │               │                              │              │           │
     │                     │               │                              │ create vCluster          │
     │                     │               │                              │ (host k8s API)           │
     │                     │               │                              │              │           │
     │                     │               │                              │ apply Cilium  │           │
     │                     │               │                              │ default-deny  │           │
     │                     │               │                              │              │           │
     │                     │               │                              │ generate kubeconfig      │
     │                     │               │                              │──────────────────────────►
     │                     │               │                              │ store kubeconfig          │
     │                     │               │                              │ (path: tenants/{id}/kube) │
     │                     │               │                              │──────────────────────────►
     │                     │               │                              │              │           │
     │                     │               │                              │ write network record      │
     │                     │               │                              │ (status: active)          │
     │                     │               │                              │──────────────►           │
     │                     │               │                              │ notify CF-Accounts        │
     │                     │               │                              │─────────────►│            │
     │                     │               │                              │ update account│            │
     │                     │               │                              │ bind network  │            │
     │                     │               │                              │ to tenant     │            │
     ◄──────────────────────────────────────────────────────────────────── 202 Accepted  │            │
     │                     │               │                              │              │           │
     │ GET /networks/{id}  │               │                              │              │           │
     │ (poll status)       │               │ ...                                                     │
```

### 8.3 What Is Created

| Resource | Owner | Stored In |
|---|---|---|
| Account record | CF-Accounts | ScyllaDB `accounts` table |
| Tenant record | CF-Accounts | ScyllaDB `tenants` table |
| Network record | CF-Accounts | ScyllaDB `networks` table |
| CIDR allocation | CF-Provisioner | ScyllaDB `cidr_allocations` table |
| vCluster instance | CF-Provisioner | Host Kubernetes cluster |
| Cilium default-deny policy | CF-Provisioner | Kubernetes (CiliumNetworkPolicy) |
| kubeconfig (management) | CF-Provisioner | OpenBao `tenants/{id}/kubeconfig` |
| Provisioning job | CF-Provisioner | ScyllaDB `provisioning_jobs` table |
| API credentials hash | CF-Accounts | ScyllaDB `api_keys` table |
| API credentials (raw, once) | CF-Accounts | Returned to user at creation time only |

### 8.4 Tenant Access to Its Own Environment

After provisioning, the tenant can interact with their environment via:
- The CloudForge console or API — all requests authenticated via JWT or API key
- CF-Router validates identity, CF-Accounts resolves the tenant context
- CF-Provisioner or other CF services execute operations inside the vCluster using the management kubeconfig

Tenants do not receive the management kubeconfig. Their access is mediated through CloudForge APIs.

If a future CloudForge capability offers direct `kubectl` access to the tenant vCluster, a separate, limited-scope kubeconfig should be issued and stored per-tenant in OpenBao with explicit RBAC restrictions. This is distinct from the management kubeconfig used by the control plane.

### 8.5 Revoking Management Access

To revoke control-plane management access to a tenant environment:
1. Delete the kubeconfig from OpenBao at `tenants/{id}/kubeconfig`
2. Rotate the service account token in the vCluster

After step 1, no CF service can issue Kubernetes API calls to that tenant environment. The vCluster remains running but is unreachable from the control plane.

---

## 9. Internet Gateway and Public Exposure Flow

### 9.1 Model

An internet gateway is optional. It is provisioned by CF-Provisioner when the tenant requests it. It allows inbound internet traffic to reach services in **public subnets only**.

Services in private subnets cannot be exposed through the internet gateway, regardless of configuration.

### 9.2 Exposure Flow

```
Tenant (API)          CF-Router    CF-Provisioner      vCluster         Envoy Gateway
     │                    │               │                │                  │
     │ POST /gateways      │               │                │                  │
     │ {network_id}        │               │                │                  │
     │────────────────────►│               │                │                  │
     │                     │ resolve tenant│                │                  │
     │                     │──────────────►               (validates subnet    │
     │                     │                               exists + is public) │
     │                     │ dispatch      │                │                  │
     │                     │───────────────►                │                  │
     │                     │               │ create         │                  │
     │                     │               │ GatewayClass / │                  │
     │                     │               │ HTTPRoute in   │                  │
     │                     │               │ host cluster   │                  │
     │                     │               │────────────────────────────────► │
     │                     │               │ provision cert │                  │
     │                     │               │ (cert-manager) │                  │
     │                     │               │────────────────────────────────► │
     │                     │               │                │                  │
     │                     │               │ write gateway  │                  │
     │                     │               │ record + public│                  │
     │                     │               │ endpoint DNS   │                  │
     │ ◄───────────────────────────────────                 │                  │
     │ 201 Created + endpoint               │                │                  │
```

### 9.3 What Is Created

| Resource | Owner | Where |
|---|---|---|
| GatewayClass / HTTPRoute | CF-Provisioner | Host Kubernetes, scoped to Envoy Gateway |
| TLS certificate | cert-manager | Envoy Gateway |
| Gateway record | CF-Provisioner | ScyllaDB `gateways` table |
| Public endpoint DNS entry | CF-Provisioner | Platform DNS (e.g., `{tenant}.gateway.cloudforge.io`) |
| Cilium ingress policy | CF-Provisioner | Allows internet → public subnet pods for this gateway |

### 9.4 Security Constraints

- Only services with a `subnet: public` label in the vCluster are eligible for exposure
- CF-Provisioner validates subnet assignment before creating any gateway or HTTPRoute
- Envoy Gateway enforces TLS — no plaintext public endpoints
- Each tenant's public gateway endpoint is namespaced to that tenant (`{tenant-slug}.gateway.cloudforge.io`)
- Rate limiting and DDoS protection policies are applied at Envoy Gateway level
- Private subnet services produce a rejected HTTPRoute if a tenant attempts to expose them

### 9.5 Timing

Internet gateway provisioning is a separate lifecycle event from private network provisioning. A tenant can:
- Create a private network without an internet gateway
- Add an internet gateway to an existing network
- Remove the internet gateway without destroying the network

---

## 10. Control Plane Communication Model

### 10.1 Model

The control plane (CF-Provisioner and other CF services) communicates with each tenant vCluster using the **Kubernetes API server** of that vCluster. This communication:
- Uses **mTLS** — both the client (CF service) and the server (vCluster API) present certificates
- Is authenticated using a **service account token** stored in the vCluster's API server and referenced in the management kubeconfig stored in OpenBao
- Never involves direct pod-to-pod communication between the control plane and tenant workloads

### 10.2 What Is Allowed

| Communication | Allowed | Method |
|---|---|---|
| Control plane → tenant vCluster API | Yes | Kubernetes API over mTLS |
| Control plane → tenant pod (direct) | No | Blocked by Cilium |
| Tenant vCluster A → Tenant vCluster B | No | Blocked by Cilium |
| Tenant pod → control-plane services | No (default) | Blocked by Cilium |
| Internet → tenant public subnet | Yes (if GW provisioned) | Via Envoy Gateway HTTPRoute |
| Internet → tenant private subnet | No | No route; Envoy Gateway blocks |

### 10.3 Request Flow Through the Platform

```
Inbound API request
        │
        ▼
[ Envoy Gateway ]
  - TLS termination
  - HTTPRoute matching (/api/**, /console/**)
  - Optional: ext_authz to CF-Router for early reject
        │
        ▼
[ CF-Router ]
  - Extract JWT or API key from Authorization header
  - Validate JWT signature against Keycloak JWKS
  - OR: hash API key with BLAKE2b-256, lookup in ScyllaDB api_keys table
  - Call CF-Accounts: GET /internal/tenant?account_id={id}
  - Receive: tenant_id, network_id, region, status
  - Inject headers: X-CF-Tenant-ID, X-CF-Network-ID, X-CF-Region
  - Route to appropriate CF service based on path
        │
        ├──► CF-Accounts  (account/tenant CRUD, credential management)
        ├──► CF-Provisioner (network/gateway/service lifecycle)
        └──► (other CF services)
```

### 10.4 Trust Establishment

Control-plane trust is established at provisioning time:
1. CF-Provisioner creates the vCluster
2. The vCluster's API server generates a service account and token
3. CF-Provisioner wraps these into a kubeconfig
4. The kubeconfig is stored in OpenBao at `tenants/{tenant_id}/kubeconfig`
5. Any CF service that needs to operate in a tenant environment retrieves this kubeconfig from OpenBao at runtime

cert-manager issues and renews:
- The TLS certificate presented by Envoy Gateway to internet clients
- The mTLS certificates used for internal service-to-service communication
- The certificates presented by vCluster API servers

### 10.5 Access Revocation

Revoking control-plane access to a tenant is a two-step operation:

1. **Soft revocation** (immediate): Delete the kubeconfig from OpenBao. All subsequent API calls that need to retrieve the kubeconfig will fail. No CF service can reach the tenant environment.

2. **Hard revocation**: Rotate the service account token inside the vCluster (via the host cluster's Kubernetes API directly). Any cached or in-flight requests using the old token will be rejected by the vCluster API server.

Revocation of API access (tenant calling CloudForge APIs):
1. Delete or expire the API key record in ScyllaDB `api_keys` table
2. Keycloak session expiry covers JWT-authenticated sessions

---

## 11. Technology Validation and Alternatives

### 11.1 vCluster (Loft Labs)

**Assessment: Confirmed, with caveats.**

vCluster is the correct solution for per-tenant virtual Kubernetes isolation. It provides:
- A dedicated Kubernetes API server per tenant
- Isolated pod/service CIDR ranges
- Scoped CoreDNS (DNS namespace isolation)
- RBAC isolation at the virtual cluster level

**Doubts and cautions:**
- vCluster shares the underlying host node resources. A noisy tenant can still impact host-level CPU/memory. This must be addressed with resource quotas and LimitRanges applied at the host namespace level wrapping each vCluster.
- vCluster's networking depends on syncing pod/service objects to the host. The sync mechanism must be carefully audited to ensure no cross-tenant object leakage at the host level.
- Consider vCluster Pro (Loft Labs commercial) if SLA-backed support and advanced isolation features (isolated node pools per tenant) are required.

**No alternative is recommended** for this use case. Competing approaches (e.g., namespace-per-tenant with hard RBAC) provide weaker structural guarantees.

### 11.2 Cilium

**Assessment: Confirmed, strongly.**

Cilium is the correct network enforcement layer. Key reasons:
- eBPF enforcement is kernel-level; bypassing it from userspace is not possible
- `CiliumNetworkPolicy` provides cross-namespace and cross-cluster enforcement that standard Kubernetes NetworkPolicy cannot
- L7-aware policies are available if needed in the future
- Native integration with Hubble for network observability

**Recommendation:** Cilium should be configured with `policyAuditMode: false` in production. Audit mode only logs violations without enforcing — never use it in production for tenant isolation.

**Alternative considered:** Calico. Calico provides comparable functionality and is more widely deployed in on-prem environments. It is a valid alternative if operational expertise favors it. Cilium is recommended for its eBPF-first design and better Kubernetes-native ergonomics.

### 11.3 ScyllaDB

**Assessment: Confirmed, with scope clarification.**

ScyllaDB is appropriate for the control-plane operational store. Its strengths here are:
- High-throughput, low-latency reads for the API request path (tenant resolution, API key lookup)
- Cassandra-compatible CQL — mature tooling
- Multi-region replication when CloudForge expands geographically

**Caution:** ScyllaDB is not a relational database. The schema must be modeled around read patterns. Avoid normalization-first design. Each read-heavy operation (e.g., tenant lookup by slug, API key lookup by hash) needs a dedicated table or materialized view.

**Alternative considered:** PostgreSQL (with CockroachDB for multi-region). Postgres is more familiar to most engineers, offers richer query capabilities, and is easier to reason about schema-first. If the team is not experienced with Cassandra-family data modeling, the operational risk of ScyllaDB is real. This is an open question worth revisiting before the first production deployment.

### 11.4 OpenBao

**Assessment: Confirmed.**

OpenBao (the community fork of HashiCorp Vault post-BSL) is the correct choice for secrets management. It provides:
- Audit logging for all secret access (critical for compliance)
- Lease-based access with TTL (important for kubeconfig expiry)
- Dynamic secrets generation (future use)
- Path-based access policies (per-tenant kubeconfig isolation)

**No alternative recommended** for this use case. AWS Secrets Manager or GCP Secret Manager are cloud-specific and not appropriate for an open-source platform. Kubernetes Secrets are insufficient for the compliance and audit requirements here.

### 11.5 Keycloak

**Assessment: Confirmed.**

Keycloak is the correct choice for user identity and SSO. It supports:
- Multi-tenant realm configuration
- OIDC / OAuth2 standard flows
- Short-lived JWT issuance
- Integration with enterprise identity providers (SAML, LDAP) — important for future enterprise customers

**Caution:** Keycloak has a high operational burden (JVM-based, memory-intensive, complex configuration). Consider Keycloak Operator for Kubernetes-native deployment and lifecycle management.

**Alternative considered:** Ory Hydra + Kratos. More lightweight and cloud-native, but requires more assembly. Keycloak is recommended for the breadth of protocol support and community ecosystem.

### 11.6 BLAKE2b-256

**Assessment: Confirmed.**

BLAKE2b-256 is the correct choice for API key hashing. It is:
- Faster than bcrypt/scrypt/argon2 (important for per-request verification in the API path)
- Cryptographically secure for hash storage (not password hashing — API keys are high-entropy random tokens)
- Widely available in Go standard and ecosystem libraries

**Note:** BLAKE2b-256 is appropriate for high-entropy tokens (API keys ≥ 32 random bytes). It must not be used for password hashing, where argon2id remains the correct choice.

### 11.7 cert-manager

**Assessment: Confirmed.**

cert-manager is the standard for certificate automation in Kubernetes. No alternative needed.

**Ensure:** The internal CA used for mTLS between CF services is managed by cert-manager with a self-signed ClusterIssuer or a dedicated intermediate CA. Do not use ad hoc certificate generation.

### 11.8 Envoy Gateway

**Assessment: Confirmed, with important note on version maturity.**

Envoy Gateway (the CNCF project, built on Envoy Proxy) with Kubernetes Gateway API is the correct edge gateway choice. It provides:
- Kubernetes-native configuration via Gateway API (`GatewayClass`, `HTTPRoute`, `TLSRoute`)
- TLS termination
- Rate limiting and traffic policy
- Active CNCF backing and alignment with Kubernetes API direction

**Caution:** Envoy Gateway is relatively newer than Istio's ingress or nginx-based solutions. Verify that the required features (ext_authz, rate limiting, TLS passthrough if needed) are stable in the version chosen.

**Alternative considered:** nginx Ingress Controller. More mature operationally, but Gateway API support is less complete. Envoy Gateway is the recommended direction.

---

## 12. Developer Environment Proposal

### 12.1 Local Kubernetes: k3d

**Recommendation: k3d** (k3s in Docker).

Rationale:
- k3d creates lightweight multi-node clusters inside Docker — no VM required
- Faster startup than kind for most workflows
- Supports multiple clusters on a single machine, enabling simulation of the host cluster + control plane services
- Works well with Tilt and Skaffold for local development loops

**Rejected: kind** — viable, but slower to start and less ergonomic for multi-cluster scenarios.
**Rejected: minikube** — single-node, limited multi-cluster support, slower than k3d.

### 12.2 Simulating Tenant Isolation in Development

```
k3d cluster: cloudforge-dev (host cluster)
  └── namespace: cloudforge-control-plane
      ├── envoy-gateway
      ├── cf-router
      ├── cf-accounts
      ├── cf-provisioner
      ├── scylladb (single-node dev instance)
      ├── openbao (dev mode, no TLS)
      └── keycloak (dev realm, seeded)
  └── vCluster: tenant-dev-a (created by CF-Provisioner during test)
  └── vCluster: tenant-dev-b (created by CF-Provisioner during test)
  └── cilium (installed on k3d cluster)
```

In development:
- Cilium is installed but `policyAuditMode: true` — policies are validated but not hard-enforced
- Switch to `policyAuditMode: false` for isolation integration tests
- vClusters are provisioned and destroyed as part of integration test runs
- ScyllaDB and OpenBao run in single-node dev mode (no replication, no TLS)
- Keycloak runs with a seeded dev realm (no need for external IdP)

### 12.3 What Should Be Mocked vs Real

| Component | Dev Environment |
|---|---|
| Envoy Gateway | Real — run via Helm in k3d |
| CF-Router | Real — binary from local build |
| CF-Accounts | Real — binary from local build |
| CF-Provisioner | Real — binary from local build |
| vCluster (tenant instances) | Real — provisioned dynamically |
| Cilium | Real — installed on k3d cluster |
| ScyllaDB | Real (single node, no replication) |
| OpenBao | Real (dev mode, in-memory) |
| Keycloak | Real (seeded dev realm) |
| cert-manager | Real — installed via Helm |
| DNS (public tenant endpoints) | Mocked via `/etc/hosts` or CoreDNS override |
| CIDR allocator | Real — but uses a test CIDR pool |

### 12.4 Local Development Loop

**Tooling: [Tilt](https://tilt.dev/)**

Tilt provides:
- Live reloading of Go binaries into Kubernetes pods
- Declarative Tiltfile for dependency orchestration
- UI for watching build/deploy/log state

Recommended Tiltfile structure:
```
Tiltfile
  ├── deps: ScyllaDB, OpenBao, Keycloak, Envoy Gateway, Cilium
  ├── services: cf-router, cf-accounts, cf-provisioner
  └── integration-tests: triggered on demand
```

### 12.5 Testing Provisioning Flows Locally

- Integration tests in each CF service module spin up vClusters in the local k3d cluster
- Tests call CF-Provisioner via its HTTP API (not mocked)
- Assertions check: vCluster created, Cilium policies applied, kubeconfig stored in OpenBao
- Cleanup: CF-Provisioner deprovision API destroys the vCluster after the test

---

## 13. REST Service Architecture and OpenAPI Contract

All CloudForge REST services (CF-Router, CF-Accounts, CF-Provisioner, and any future CF service) must follow a uniform 3-layer architecture enforced at the package level. This section defines the rules, the required tooling, and the repository conventions that make those rules structural rather than advisory.

### 13.1 Mandatory 3-Layer Architecture

Every REST service is divided into exactly three layers. Each layer has its own models, its own typed errors, and its own transformation methods. No layer's models may appear in another layer.

```
┌─────────────────────────────────────────────────────┐
│  REST API layer                                     │
│  handler.go · server.go · models.go                 │
│  models_transform.go · errors.go · generated/       │
└─────────────────────┬───────────────────────────────┘
                      │  explicit transform only
┌─────────────────────▼───────────────────────────────┐
│  Service layer                                      │
│  api.go · service.go · models.go                    │
│  models_transform.go · errors.go                    │
└─────────────────────┬───────────────────────────────┘
                      │  explicit transform only
┌─────────────────────▼───────────────────────────────┐
│  DB / External API layer                            │
│  (one subfolder per external system)                │
│  api.go · repository.go · models.go · errors.go     │
└─────────────────────────────────────────────────────┘
```

**Non-negotiable rules:**
- Each layer defines all its errors in `errors.go`. Most errors are static typed errors; use dynamic typed errors only when runtime context is required. Error cause and stack must always be preserved when wrapping.
- Crossing a layer boundary must go through an explicit transform method. Handlers call transform methods; they never construct service-layer params inline.
- REST models must never go below the REST layer. DB models must never surface above the DB layer.
- The concrete struct implementing a service interface must be named `CF<InterfaceName>` (e.g., interface `AccountsService` → concrete type `CFAccountsService`). It must never be exported directly — the `New()` constructor returns the interface type.
- Everything is built using **only the Go standard library**. No third-party web frameworks, routers, middleware libraries, validation libraries, or error libraries.

### 13.2 OpenAPI Contract — Step Zero

**An OpenAPI 3.0.3 specification must exist before any handler code is written.** The spec is the contract. Server stubs, client SDKs, and request validation are all generated from it. No hand-written HTTP types should duplicate what the spec already defines.

This is not optional and not retroactive. The workflow is:

```
1. Write openapi.yaml
2. Run oapi-codegen to generate server stubs and client SDK
3. Write handler.go that implements the generated StrictServerInterface
4. Write service.go, repository.go, etc.
```

If the HTTP contract changes, `openapi.yaml` is updated first. The generated files are then regenerated. Generated files are never hand-edited.

### 13.3 OpenAPI Tooling: oapi-codegen

**Tool:** [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen) — the standard code generator for OpenAPI 3.x → Go.

**Why oapi-codegen over alternatives:**
- Generates idiomatic Go — no runtime reflection, no bloated base types
- Supports `StrictServerInterface` mode: generated handler stubs enforce that every operation is implemented at compile time
- Client SDK generation produces a typed HTTP client from the same spec with zero duplication
- Generates request/response validation wrappers — no manual validation boilerplate
- Actively maintained, used widely in production Go services
- Does not force a framework — generated code works with `net/http` directly, consistent with the stdlib-only rule

**Alternative evaluated: ogen.** ogen generates more opinionated, zero-allocation code and has excellent validation. It is a valid alternative for new services if the team prefers its model. oapi-codegen is recommended here for its wider adoption and more flexible output modes.

**Not considered:** openapi-generator (Java-based, generates non-idiomatic Go), go-swagger (Swagger 2.0 only, not OpenAPI 3.x).

### 13.4 Codegen Configuration and Conventions

Codegen config files live alongside the spec:

```
api/
  cf-accounts/
    v1/
      openapi.yaml           ← source of truth for the HTTP contract
      oapi-server.cfg.yaml   ← server stub generator config
      oapi-client.cfg.yaml   ← client SDK generator config
```

**`oapi-server.cfg.yaml` (example for cf-accounts):**
```yaml
package: generated
generate:
  strict-server: true
  models: true
  embedded-spec: true
output: ../../services/cf-accounts/generated/server.gen.go
```

**`oapi-client.cfg.yaml` (example for cf-accounts):**
```yaml
package: cfaccountsclient
generate:
  client: true
  models: true
output: ../../libs/clients/cf-accounts/v1/client.gen.go
```

**Run codegen:**
```bash
oapi-codegen -config api/cf-accounts/v1/oapi-server.cfg.yaml api/cf-accounts/v1/openapi.yaml
oapi-codegen -config api/cf-accounts/v1/oapi-client.cfg.yaml api/cf-accounts/v1/openapi.yaml
```

Codegen is run as part of `go generate`. Each service's `main.go` or a dedicated `generate.go` file at the package root includes the directive:
```go
//go:generate oapi-codegen -config ../../api/cf-accounts/v1/oapi-server.cfg.yaml ../../api/cf-accounts/v1/openapi.yaml
```

CI validates that generated files are up to date by running `go generate ./...` and checking for a clean `git diff`.

### 13.5 Generated Client Libraries

Each service's OpenAPI spec also produces a **typed Go client library** stored in `libs/clients/<service>/`. Other CF services that call CF-Accounts or CF-Provisioner use these generated clients — never raw HTTP calls with hand-written types.

```
libs/
  clients/
    cf-accounts/
      go.mod    (module github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts)
      v1/
        client.gen.go   ← generated, never hand-edited
    cf-provisioner/
      go.mod    (module github.com/jtomasevic/cloud-forge-2/libs/clients/cf-provisioner)
      v1/
        client.gen.go
```

CF-Router imports `libs/clients/cf-accounts` to call CF-Accounts for tenant resolution. This ensures CF-Router's calls to CF-Accounts are always type-safe and always in sync with the CF-Accounts contract.

### 13.6 HTTP / Routing Rules

Because the stdlib-only rule is absolute:
- Use `net/http` for all HTTP handling
- Use Go 1.22+ method-aware routing patterns: `mux.HandleFunc("GET /accounts/{id}", handler)`
- Implement middleware using standard `http.Handler` composition — no external middleware libraries
- Use `http.ServeMux` as the router — no gorilla/mux, chi, fiber, gin, or echo

### 13.7 Per-Layer File Conventions

**REST API layer** (`internal/rest/`):

| File | Purpose |
|---|---|
| `handler.go` | Implements the generated `StrictServerInterface`; calls transform methods before passing to service layer |
| `server.go` | `NewRouter()` — builds `http.ServeMux` and wires middleware chain |
| `models.go` | HTTP-only request/response types not already in generated code |
| `models_transform.go` | `ToService*()` and `ToRest*()` mapping functions |
| `errors.go` | Typed REST errors; maps service errors to HTTP status codes |
| `generated/` | oapi-codegen output — never hand-edited |

**Service layer** (`internal/service/`):

| File | Purpose |
|---|---|
| `api.go` | Service interface definition + `New(deps Deps) <Interface>` constructor |
| `service.go` | `CF<ServiceName>` concrete struct — business logic only |
| `models.go` | Service-layer domain models |
| `models_transform.go` | Mapping to/from repository/client models when needed |
| `errors.go` | Typed service errors |

**DB / External API layer** (`internal/repository/<name>/`):

| File | Purpose |
|---|---|
| `api.go` | Repository interface definition + `New()` constructor |
| `repository.go` | Concrete implementation against the DB or external API |
| `models.go` | Layer-specific persistence/wire models |
| `errors.go` | Typed repository errors |

### 13.8 Naming Transformation Rules

Transform method names follow a predictable convention so the direction of the transform is always unambiguous:

```go
// REST → Service (call on the REST model, going down)
func (r *CreateAccountRequest) ToServiceCreateAccountParams() service.CreateAccountParams

// Service → REST (free function, coming back up)
func ToAccountResponseFromServiceAccount(a service.Account) AccountResponse

// Service → Repository (call on the service model, going down)
func (a *CreateAccountParams) ToRepositoryInsertAccountRow() repository.InsertAccountRow

// Repository → Service (free function, coming back up)
func ToServiceAccountFromRepositoryRow(row repository.AccountRow) service.Account
```

---

## 14. Golang Project Structure with go.work

### 14.1 go.work Design Principles

- Each CloudForge service is an independent Go module — it can be built, tested, and versioned independently
- Shared libraries live in their own modules — consumed by `require` in each service module's `go.mod`
- `go.work` at the repository root wires all modules together for local development without requiring `replace` directives
- Generated client libraries for each service are independent modules so consumers can depend on them without pulling in the full service codebase

### 14.2 Module Layout

The layout reflects the 3-layer architecture from section 13. Each service module is internally structured by layer (`rest/`, `service/`, `repository/`), not by domain noun. OpenAPI specs live at the repository root under `api/`, separated from the service code. Generated client libraries live in `libs/clients/`.

```
cloud-forge-2/
├── go.work
├── go.work.sum
│
├── api/                                        ← OpenAPI contracts (source of truth)
│   ├── cf-router/
│   │   └── v1/
│   │       ├── openapi.yaml
│   │       ├── oapi-server.cfg.yaml
│   │       └── oapi-client.cfg.yaml
│   ├── cf-accounts/
│   │   └── v1/
│   │       ├── openapi.yaml
│   │       ├── oapi-server.cfg.yaml
│   │       └── oapi-client.cfg.yaml
│   └── cf-provisioner/
│       └── v1/
│           ├── openapi.yaml
│           ├── oapi-server.cfg.yaml
│           └── oapi-client.cfg.yaml
│
├── services/
│   ├── cf-router/
│   │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/services/cf-router)
│   │   ├── main.go
│   │   └── internal/
│   │       ├── rest/
│   │       │   ├── generated/         # oapi-codegen output — never hand-edited
│   │       │   ├── handler.go         # implements generated StrictServerInterface
│   │       │   ├── server.go          # NewRouter() + middleware chain
│   │       │   ├── models.go
│   │       │   ├── models_transform.go
│   │       │   └── errors.go
│   │       ├── service/
│   │       │   ├── api.go             # RouterService interface + New()
│   │       │   ├── service.go         # CFRouterService — JWT/key validation, routing logic
│   │       │   ├── models.go
│   │       │   ├── models_transform.go
│   │       │   └── errors.go
│   │       └── repository/
│   │           └── apikeys/           # API key hash lookup (ScyllaDB)
│   │               ├── api.go
│   │               ├── repository.go
│   │               ├── models.go
│   │               └── errors.go
│   │
│   ├── cf-accounts/
│   │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/services/cf-accounts)
│   │   ├── main.go
│   │   └── internal/
│   │       ├── rest/
│   │       │   ├── generated/
│   │       │   ├── handler.go
│   │       │   ├── server.go
│   │       │   ├── models.go
│   │       │   ├── models_transform.go
│   │       │   └── errors.go
│   │       ├── service/
│   │       │   ├── api.go             # AccountsService interface + New()
│   │       │   ├── service.go         # CFAccountsService — account/tenant/network/credential logic
│   │       │   ├── models.go
│   │       │   ├── models_transform.go
│   │       │   └── errors.go
│   │       └── repository/
│   │           ├── accounts/          # ScyllaDB accounts table
│   │           │   ├── api.go
│   │           │   ├── repository.go
│   │           │   ├── models.go
│   │           │   └── errors.go
│   │           ├── tenants/           # ScyllaDB tenants table
│   │           │   ├── api.go
│   │           │   ├── repository.go
│   │           │   ├── models.go
│   │           │   └── errors.go
│   │           ├── networks/          # ScyllaDB networks table
│   │           │   ├── api.go
│   │           │   ├── repository.go
│   │           │   ├── models.go
│   │           │   └── errors.go
│   │           └── credentials/       # ScyllaDB api_keys table
│   │               ├── api.go
│   │               ├── repository.go
│   │               ├── models.go
│   │               └── errors.go
│   │
│   └── cf-provisioner/
│       ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/services/cf-provisioner)
│       ├── main.go
│       └── internal/
│           ├── rest/
│           │   ├── generated/
│           │   ├── handler.go
│           │   ├── server.go
│           │   ├── models.go
│           │   ├── models_transform.go
│           │   └── errors.go
│           ├── service/
│           │   ├── api.go             # ProvisionerService interface + New()
│           │   ├── service.go         # CFProvisionerService — orchestration logic
│           │   ├── models.go
│           │   ├── models_transform.go
│           │   └── errors.go
│           └── repository/
│               ├── vcluster/          # vCluster lifecycle (host k8s API)
│               │   ├── api.go
│               │   ├── client.go
│               │   ├── models.go
│               │   └── errors.go
│               ├── cidr/              # CIDR allocation (ScyllaDB)
│               │   ├── api.go
│               │   ├── repository.go
│               │   ├── models.go
│               │   └── errors.go
│               ├── cilium/            # CiliumNetworkPolicy management
│               │   ├── api.go
│               │   ├── client.go
│               │   ├── models.go
│               │   └── errors.go
│               ├── gateway/           # Envoy Gateway HTTPRoute management
│               │   ├── api.go
│               │   ├── client.go
│               │   ├── models.go
│               │   └── errors.go
│               ├── kubeconfig/        # kubeconfig store (OpenBao)
│               │   ├── api.go
│               │   ├── client.go
│               │   ├── models.go
│               │   └── errors.go
│               └── jobs/              # Provisioning job queue (ScyllaDB)
│                   ├── api.go
│                   ├── repository.go
│                   ├── models.go
│                   └── errors.go
│
├── libs/
│   ├── cloudforge-core/
│   │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core)
│   │   └── pkg/
│   │       ├── tenant/              # Shared tenant model types
│   │       ├── network/             # Shared network model types
│   │       ├── errors/              # CloudForge platform error types
│   │       └── middleware/          # stdlib HTTP middleware (logging, tracing, request ID)
│   │
│   ├── scylladb/
│   │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/libs/scylladb)
│   │   └── pkg/
│   │       ├── client/              # ScyllaDB session management
│   │       └── migrate/             # Schema migration tooling
│   │
│   ├── openbao/
│   │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/libs/openbao)
│   │   └── pkg/
│   │       └── client/              # OpenBao read/write/delete wrapper
│   │
│   └── clients/                     ← generated typed clients for inter-service calls
│       ├── cf-accounts/
│       │   ├── go.mod    (module github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts)
│       │   └── v1/
│       │       └── client.gen.go    # generated — never hand-edited
│       └── cf-provisioner/
│           ├── go.mod    (module github.com/jtomasevic/cloud-forge-2/libs/clients/cf-provisioner)
│           └── v1/
│               └── client.gen.go
│
└── tools/
    ├── cf-cli/
    │   ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/tools/cf-cli)
    │   └── main.go
    └── migrations/
        ├── go.mod        (module github.com/jtomasevic/cloud-forge-2/tools/migrations)
        └── main.go
```

### 14.3 go.work File

```go
go 1.23

use (
    ./services/cf-router
    ./services/cf-accounts
    ./services/cf-provisioner
    ./libs/cloudforge-core
    ./libs/scylladb
    ./libs/openbao
    ./libs/clients/cf-accounts
    ./libs/clients/cf-provisioner
    ./tools/cf-cli
    ./tools/migrations
)
```

### 14.4 Dependency Strategy

- Each service module's `go.mod` lists its direct dependencies
- Shared types and utilities live in `libs/cloudforge-core` — referenced by all services
- Database client wrappers live in `libs/scylladb` and `libs/openbao` — referenced by services that need them
- CF-Router depends on `libs/clients/cf-accounts` to call CF-Accounts for tenant resolution — never raw HTTP
- `go.work` ensures that during local development, changes to any `libs/` module are immediately reflected in all dependent service builds without a publish/update cycle
- The `api/` directory is not a Go module — it contains only YAML specs consumed by codegen tooling

### 14.5 Initial Modules to Create

For the initial CloudForge Private Network milestone, create in this order:

1. `libs/cloudforge-core` — shared types, errors, middleware
2. `libs/scylladb` — ScyllaDB client wrapper
3. `libs/openbao` — OpenBao client wrapper
4. `api/cf-accounts/v1/openapi.yaml` — CF-Accounts HTTP contract (before any handler code)
5. `services/cf-accounts` — account/tenant/network registry (after spec is written)
6. `libs/clients/cf-accounts` — generated client (from cf-accounts spec)
7. `api/cf-provisioner/v1/openapi.yaml` — CF-Provisioner HTTP contract
8. `services/cf-provisioner` — provisioning engine (after spec is written)
9. `libs/clients/cf-provisioner` — generated client
10. `api/cf-router/v1/openapi.yaml` — CF-Router HTTP contract
11. `services/cf-router` — platform routing layer (after spec is written)

---

## 15. CI/CD Proposal

### 15.1 Platform: GitHub Actions

GitHub Actions is the recommended CI platform for an open-source project. All workflows live in `.github/workflows/`.

### 15.2 Workflow Structure

```
.github/workflows/
├── lint.yml          # Linting (all modules)
├── test.yml          # Unit + integration tests (per-module)
├── build.yml         # OCI image build + push
├── infra-validate.yml # Infrastructure change validation
└── release.yml       # Versioned release workflow
```

### 15.3 Linting (`lint.yml`)

```yaml
# Triggers: push, PR to main
# Strategy: matrix over all modules
jobs:
  codegen-check:
    # Validates that generated files are up to date with their OpenAPI specs.
    # Fails if any generated file has drifted from the spec.
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
      - run: go generate ./...
      - run: git diff --exit-code -- '**/generated/*.gen.go' '**/clients/**/*.gen.go'

  lint:
    strategy:
      matrix:
        module:
          - services/cf-router
          - services/cf-accounts
          - services/cf-provisioner
          - libs/cloudforge-core
          - libs/scylladb
          - libs/openbao
          - libs/clients/cf-accounts
          - libs/clients/cf-provisioner
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go vet ./...
        working-directory: ${{ matrix.module }}
      - uses: golangci/golangci-lint-action@v6
        with:
          working-directory: ${{ matrix.module }}
```

### 15.4 Testing (`test.yml`)

```yaml
# Unit tests: run per module, no external dependencies
# Integration tests: spin up k3d cluster, run against real services
jobs:
  unit-test:
    strategy:
      matrix:
        module: [services/cf-router, services/cf-accounts, services/cf-provisioner, ...]
    steps:
      - run: go test -race -count=1 ./...
        working-directory: ${{ matrix.module }}

  integration-test:
    # Only on PR to main and scheduled nightly
    services:
      # Docker Compose services for ScyllaDB, OpenBao, Keycloak
    steps:
      - name: Setup k3d
        run: k3d cluster create cloudforge-ci
      - name: Install Cilium
        run: helm install cilium cilium/cilium --namespace kube-system
      - name: Install vCluster CRDs
        run: ...
      - name: Run integration tests
        run: go test -tags=integration -timeout=10m ./...
        working-directory: services/cf-provisioner
```

### 15.5 Image Build (`build.yml`)

- One OCI image per service (`cf-router`, `cf-accounts`, `cf-provisioner`)
- Multi-stage Dockerfile per service: build stage (Go binary) + minimal runtime stage (`gcr.io/distroless/static`)
- Images pushed to a container registry (GitHub Container Registry `ghcr.io/jtomasevic/cloud-forge-2/*`)
- Tags: `sha-{git-sha}` on every push, `v{semver}` on tagged releases

### 15.6 Infrastructure Validation (`infra-validate.yml`)

For changes to Cilium policies, vCluster templates, or Kubernetes manifests:
- `kubeconform` for Kubernetes manifest validation
- `helm lint` for Helm chart changes
- Integration test run against a fresh k3d cluster in CI

### 15.7 Testing Provisioning Flows in CI

A dedicated integration test suite in `services/cf-provisioner`:
- Creates a real k3d cluster in CI
- Runs the full provisioning flow: create network → verify vCluster → verify Cilium policy → verify kubeconfig in OpenBao
- Runs the deprovision flow: destroy network → verify vCluster deleted → verify kubeconfig deleted
- Runs the internet gateway flow: create gateway → verify HTTPRoute in Envoy Gateway → verify Cilium ingress policy

These tests run as part of PR validation for any change to `services/cf-provisioner` or `libs/cloudforge-core`.

---

## 16. Risks, Tradeoffs, and Open Questions

### 16.1 Risks

| Risk | Severity | Mitigation |
|---|---|---|
| vCluster sync leaks host objects across tenant boundaries | High | Audit vCluster sync configuration; disable unnecessary sync categories; test cross-tenant namespace isolation |
| Cilium policy audit mode accidentally left on in production | High | Environment-specific Helm values; CI gate that validates `policyAuditMode: false` in production manifests |
| ScyllaDB data model does not scale to high tenant count | Medium | Model schema with read patterns first; benchmark at 10k tenants before GA |
| OpenBao becomes a single point of failure for management access | High | HA OpenBao cluster with Raft consensus; regular snapshot backups; operational runbooks for recovery |
| Keycloak operational burden exceeds team capacity | Medium | Keycloak Operator for lifecycle management; consider migration to Ory stack post-MVP if burden is high |
| CIDR exhaustion in tenant pod/service ranges | Medium | Allocate a large supernet at platform level; design CIDR allocator with reclamation support from day one |
| mTLS certificate rotation causes control-plane downtime | Medium | cert-manager handles automated rotation; monitor certificate expiry with alerting; test rotation in staging |

### 16.2 Tradeoffs

| Decision | Tradeoff |
|---|---|
| vCluster for isolation (vs. node-per-tenant) | Lower cost at the expense of shared host resources; stronger than namespace-based but weaker than full node isolation |
| ScyllaDB (vs. PostgreSQL) | Better throughput; worse developer ergonomics and more complex data modeling |
| Go modules with go.work (vs. monorepo single module) | Better modularity and independent versioning; slightly more complex dependency management |
| OpenBao (vs. cloud-native secrets manager) | Platform-agnostic; requires operational investment in running Vault/OpenBao infrastructure |
| Envoy Gateway (vs. nginx) | Gateway API-native; less battle-tested in production than nginx; aligned with Kubernetes ecosystem direction |

### 16.3 Open Questions

1. **Node isolation:** At what tenant scale or tier should CloudForge offer dedicated node pools per tenant (stronger isolation) vs. shared nodes with vCluster (current model)?

2. **ScyllaDB vs PostgreSQL:** Should the team evaluate this more carefully before the first production deployment? The Cassandra data modeling expertise required for ScyllaDB is non-trivial.

3. **Tenant kubectl access:** Will CloudForge offer direct `kubectl` access to the tenant vCluster as a product feature? If yes, the kubeconfig issuance and RBAC model for tenant-side access needs to be designed now.

4. **Multi-region:** The current model is single-region per private network. What is the roadmap for cross-region private networks or VPC peering equivalents?

5. **Service mesh:** Should an internal service mesh (Cilium's built-in mTLS, or Istio) be used for service-to-service mTLS between CF services? This is worth deciding before the first service is deployed.

6. **Rate limiting model:** Who owns rate limiting? Envoy Gateway handles edge rate limits. Does CF-Router enforce per-tenant API quotas? This boundary needs to be defined.

---

## 17. Final Recommendation

CloudForge Private Network, as proposed in this document, is architecturally sound and buildable with the described stack.

The following decisions are confirmed:
- **vCluster** for per-tenant structural isolation — no alternative provides equivalent guarantees at this level
- **Cilium** for network enforcement — eBPF-level default-deny is the correct enforcement mechanism
- **Envoy Gateway** at the edge, **CF-Router** as the tenant-aware platform layer behind it — these are complementary, not competing
- **CF-Accounts** as a first-class control-plane service owning the account ↔ tenant ↔ network ↔ service relationship
- **CF-Provisioner** as the authoritative provisioning engine for all tenant infrastructure
- **OpenBao** for secrets and management access material — audit logging is non-negotiable for compliance
- **Keycloak** for user identity — breadth of protocol support justifies the operational cost
- **ScyllaDB** provisionally confirmed — revisit before production if Cassandra data modeling expertise is not available on the team

The following should be addressed before the first production deployment:
1. Confirm ScyllaDB vs PostgreSQL with the team's data modeling capability in mind
2. Decide on internal service mesh for CF service-to-service mTLS
3. Define the CIDR allocation supernet and reclamation design
4. Establish OpenBao HA topology and backup/recovery runbooks
5. Define the tenant kubectl access model (or explicitly exclude it from v1 scope)

The architecture described here provides CloudForge with a credible, auditable, and structurally sound multi-tenant isolation model. It is appropriate to present to enterprise customers and regulators as evidence that cross-tenant isolation is a property of the system, not a hope expressed in policy.

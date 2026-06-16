# CF-Router

**CF-Router** is CloudForge’s **tenant-aware edge and reverse proxy**. It sits in front of **CF-Accounts**
and **CF-Provisioner**: clients send **Bearer JWTs** or **X-CF-API-Key** credentials to the router; the
router validates them, resolves **tenant context** via **CF-Accounts**, adds trusted **X-CF-***
headers plus the mesh **X-CF-Internal-Secret**, and **forwards** the original HTTP request (method,
path, query, body) to the correct upstream.

It also exposes a small **native OpenAPI surface** (`/health`, `/ready`, `/internal/...`) for probes
and operator introspection.

---

## What it does

| Area | Behavior |
|------|-----------|
| **Reverse proxy** | Paths under `/v1/auth`, `/v1/accounts`, `/v1/tenants` → **CF-Accounts**; `/v1/networks`, `/v1/app-services`, `/v1/gateways`, `/v1/jobs` → **CF-Provisioner** (see [`internal/rest/proxy.go`](internal/rest/proxy.go) `DefaultRouteTable`). |
| **Authentication** | **JWT**: RS256 verify using **JWKS** from `KEYCLOAK_JWKS_URL` (no third-party JWT lib). **API key**: BLAKE2b-256 hash → Scylla **`api_keys_by_hash`**. |
| **Tenant resolution** | Calls CF-Accounts **`GET /internal/v1/resolve`** with **`X-CF-Internal-Secret`** (`CF_INTERNAL_SECRET`). |
| **Header injection** | Sets `X-CF-Tenant-ID`, `X-CF-Account-ID`, `X-CF-Network-ID`, `X-CF-Region`, `X-CF-Internal-Secret` on upstream requests. **Strips** `X-CF-API-Key` so raw keys never reach CF services. |
| **Native API** | `/health`, `/ready`, `/internal/routes` (internal secret), `/internal/tenant-context` (Bearer **or** API key for debugging). |

Contract: [`api/cf-router/v1/openapi.yaml`](../../api/cf-router/v1/openapi.yaml).

---

## Why it exists

- **Single front door**: End-user and automation traffic can target one base URL; routing and auth
  policy live in one place.
- **Trusted context on the wire**: Upstreams (especially CF-Provisioner) rely on **router-injected**
  headers instead of re-parsing end-user credentials.
- **Separation of concerns**: CF-Accounts remains the registry of truth; CF-Router does not embed
  tenant CRUD—only **resolution** and **transport**.
- **Defense in depth**: API keys are verified against Scylla but **not forwarded**; internal secret
  is **replaced** on the upstream hop so clients cannot spoof mesh auth.

---

## How it is built (layers)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Clients (SDK, browser, CF-CLI)                                             │
│  Authorization: Bearer …  and/or  X-CF-API-Key                                │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ HTTP
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  internal/rest                                                              │
│  • Specific routes: strict OpenAPI (health, ready, internal…)               │
│  • Catch-all "/": ProxyHandler → httputil.ReverseProxy                       │
│  Middleware: RequestID, Logger, Recovery                                   │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ RouterService
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  internal/service                                                            │
│  JWT verify (JWKS) · API key hash · CF-Accounts resolve · optional region  │
└───────────────────────────────────┬─────────────────────────────────────────┘
          ┌─────────────────────────┼─────────────────────────┐
          ▼                         ▼                         ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│ apikeys repo     │      │ cf-accounts      │      │ (optional)       │
│ Scylla read      │      │ HTTP client      │      │ GetNetwork for   │
│ api_keys_by_hash │      │ resolve +       │      │ region (Bearer)  │
└──────────────────┘      └────────┬─────────┘      └──────────────────┘
                                   │
                                   ▼
                          CF-Accounts :8081
```

Deeper dives:

- **REST + proxy**: [`internal/rest/README.md`](internal/rest/README.md) — includes step-by-step proxy narrative in [`proxy.go`](internal/rest/proxy.go).
- **Auth + resolution**: [`internal/service/README.md`](internal/service/README.md).
- **API key lookup**: [`internal/repository/apikeys/README.md`](internal/repository/apikeys/README.md).

---

## Architecture (components)

```
                         ┌─────────────────────────┐
                         │       CF-Router         │
                         │   (Go binary :8083)    │
                         └───────────┬─────────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
              ▼                      ▼                      ▼
     ┌────────────────┐    ┌────────────────┐    ┌────────────────┐
     │  ServeMux      │    │ RouterService  │    │ Scylla session │
     │  native + "/"  │───▶│  + JWKS cache  │───▶│  api_keys_by_  │
     │  proxy         │    │                │    │  hash reads    │
     └────────────────┘    └────────┬───────┘    └────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    ▼                               ▼
           ┌───────────────┐               ┌───────────────┐
           │  CF-Accounts   │               │ CF-Provisioner│
           │  (resolve,    │               │ (proxied /v1/ │
           │   networks)   │               │  networks…)   │
           └───────────────┘               └───────────────┘
                    ▲
                    │ HTTPS/HTTP (operator JWKS)
                    │
           ┌───────────────┐
           │  IdP JWKS     │
           │  (Keycloak)   │
           └───────────────┘
```

---

## Infrastructure dependencies

| System | Role |
|--------|------|
| **ScyllaDB** | Hot-path lookup of API key hashes (`api_keys_by_hash`); same keyspace as CF-Accounts migrations. |
| **CF-Accounts** | Internal tenant resolve + optional `GET /v1/networks/{id}` for region when caller used Bearer. |
| **CF-Provisioner** | Upstream for proxied provisioning/network APIs. |
| **Keycloak (or compatible)** | JWKS URL for JWT signature verification (`KEYCLOAK_JWKS_URL`). |
| **Mesh secret** | `CF_INTERNAL_SECRET` must match CF-Accounts expectations for `/internal/v1/resolve` and CF-Provisioner `X-CF-Internal-Secret`. |

---

## Request flow — proxied `/v1/...` call

```
  Client                    CF-Router                         Upstream
    │                           │                                 │
    │  GET /v1/networks/{id}    │                                 │
    ├──────────────────────────▶│ RouteTable.Match → provisioner  │
    │  Bearer or X-CF-API-Key   │ ValidateAndResolve              │
    │                           ├────────────────────────────────▶│ CF-Accounts
    │                           │   /internal/v1/resolve + secret   │ (resolve only)
    │                           │◀────────────────────────────────┤
    │                           │ ReverseProxy: set Host,         │
    │                           │ inject X-CF-*, strip API key     │
    │                           ├────────────────────────────────▶│ CF-Provisioner
    │◀──────────────────────────┤◀────────────────────────────────┤ response stream
```

Native routes (`/health`, `/ready`, `/internal/...`) **short-circuit** in the mux and never enter this
proxy path.

---

## Run locally

From repo root (`go.work`):

```bash
cd services/cf-router
go run .
```

Requires **ScyllaDB** (with migrations applied), reachable **CF-Accounts**, **JWKS** (for JWT tests),
and upstream URLs consistent with your route table.

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:8083` | HTTP listen address |
| `SWAGGER_ADDR` | `:8090` | Public, unauthenticated Swagger/OpenAPI docs listen address. Set to `off` to disable. |
| `SWAGGER_API_BASE_URL` | `http://localhost:8083` | API server URL written into the public OpenAPI docs. Use `/` when serving docs through the local Envoy Gateway so Swagger calls the same gateway origin. |
| `SCYLLADB_HOSTS` | `localhost:9042` | Scylla contact points (comma / space / semicolon separated) |
| `SCYLLADB_KEYSPACE` | `cloudforge` | Keyspace name |
| `CF_ACCOUNTS_URL` | `http://localhost:8081` | CF-Accounts base URL (routing + HTTP client) |
| `CF_PROVISIONER_URL` | `http://localhost:8082` | CF-Provisioner base URL (routing) |
| `CF_INTERNAL_SECRET` | `dev-internal-secret` | Sent to CF-Accounts resolve; injected on proxied upstream requests; validates `/internal/routes` |
| `KEYCLOAK_JWKS_URL` | `http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs` | JWKS document for RS256 JWT verification |
| `KEYCLOAK_ISSUER` | *(empty)* | If set, JWT `iss` claim must match exactly |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:8090,http://127.0.0.1:8090` | Origins allowed to call the API port from the public Swagger UI |

---

## API & codegen

```bash
cd services/cf-router && go generate ./...
```

Regenerates:

- `internal/rest/generated/server.gen.go` (strict HTTP server + models + embedded spec)
- `libs/clients/cf-router/v1/client.gen.go` (optional HTTP client for downstream tools)

Source spec: [`api/cf-router/v1/openapi.yaml`](../../api/cf-router/v1/openapi.yaml).

When CF-Router is running directly, public aggregated docs are available without
credentials at `http://localhost:8090/swagger/`. In k3d with Envoy Gateway,
the same docs are exposed at `http://api.cloudforge.local:18080/swagger/`.
The Swagger UI contains specs for CF-Router native endpoints plus CF-Accounts
and CF-Provisioner routes as they are exposed through CF-Router. The k3d
manifest sets `SWAGGER_API_BASE_URL=/` so Swagger "try it out" requests stay on
the same Envoy origin for both HTTP and HTTPS.

Raw OpenAPI documents:

- `http://localhost:8090/openapi/cf-router.json`
- `http://localhost:8090/openapi/cf-accounts.json`
- `http://localhost:8090/openapi/cf-provisioner.json`

---

## Docker

Build from **repository root**:

```bash
docker build -f services/cf-router/Dockerfile .
```

The Dockerfile copies `go.work`, `libs/`, and `services/cf-router/` (see [`Dockerfile`](Dockerfile)).

---

## Testing

```bash
cd services/cf-router
go test ./...
```

---

## Plans & docs

- OpenAPI task: [`docs/plan/17.CFRouterOpenAPISpec.md`](../../docs/plan/17.CFRouterOpenAPISpec.md)
- Implementation task: [`docs/plan/18.CFRouterImplementation.md`](../../docs/plan/18.CFRouterImplementation.md)
- Private network / platform context: [`docs/cf-private-network.md`](../../docs/cf-private-network.md) (sections on router position and request flow, if present)

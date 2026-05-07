# libs/openbao

Typed Go client for [OpenBao](https://openbao.org) (the open-source Vault fork), used by all CloudForge control-plane services that need to store and retrieve secrets.

## Overview

`libs/openbao` wraps the official OpenBao Go SDK (`github.com/openbao/openbao/api/v2`) and exposes a narrow, mockable `SecretsClient` interface. All errors are mapped to CloudForge `CFError` values (from `libs/cloudforge-core`) so that callers work with a single, consistent error hierarchy across every service boundary.

The library is intentionally small:

| Package              | Purpose                                                        |
|----------------------|----------------------------------------------------------------|
| `pkg/client`         | `SecretsClient` interface, `CFSecretsClient` implementation, kubeconfig helpers |
| `pkg/client/mock`    | Generated gomock mocks (do not edit by hand)                   |

## SecretsClient interface

```go
type SecretsClient interface {
    Write(ctx context.Context, path string, data map[string]interface{}) error
    Read(ctx context.Context, path string) (map[string]interface{}, error)
    Delete(ctx context.Context, path string) error
    List(ctx context.Context, pathPrefix string) ([]string, error)
}
```

### Creating a client

```go
import "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"

c, err := client.New(client.Config{
    Address: "http://localhost:8200",
    Token:   os.Getenv("OPENBAO_TOKEN"),
})
if err != nil {
    // err is a *CFError with CodeInternal
}
```

### Reading and writing secrets

```go
// Write
err = c.Write(ctx, "secret/app/db-password", map[string]interface{}{
    "value": "s3cr3t",
})

// Read
data, err := c.Read(ctx, "secret/app/db-password")
if errors.Is(err, client.ErrSecretNotFound) {
    // path does not exist or was deleted
}

// List
keys, err := c.List(ctx, "secret/app/")

// Delete
err = c.Delete(ctx, "secret/app/db-password")
```

### Error mapping

| Vault / network condition | Returned `CFError` code |
|---------------------------|-------------------------|
| HTTP 404 / nil secret     | `CodeNotFound` — matches `client.ErrSecretNotFound` |
| HTTP 403                  | `CodeForbidden` — matches `cferrors.ErrForbidden` |
| Other 4xx / 5xx           | `CodeInternal` — matches `cferrors.ErrInternal` |
| Network / timeout         | `CodeUnavailable` — matches `cferrors.ErrUnavailable` |
| Empty `Config.Address`    | `CodeInternal` — matches `client.ErrClientInit` |

Use `errors.Is` to test error codes across all layer boundaries:

```go
if errors.Is(err, client.ErrSecretNotFound) { ... }
if errors.Is(err, cferrors.ErrForbidden)    { ... }
```

## Kubeconfig helpers

Convenience functions used by CF-Provisioner to manage tenant kubeconfigs. All data is stored on a **KV v1** mount at `secret/tenants/{tenantID}/kubeconfig`.

```go
// Store a kubeconfig (base64-encodes bytes automatically)
err = client.StoreKubeconfig(ctx, c, tenantID, kubeconfigBytes)

// Load it back
kubeconfigBytes, err := client.LoadKubeconfig(ctx, c, tenantID)
if errors.Is(err, client.ErrSecretNotFound) {
    // kubeconfig was revoked or never stored
}

// Revoke — immediately cuts off control-plane access to that tenant
err = client.RevokeKubeconfig(ctx, c, tenantID)
```

## Testing with mocks

Mocks are generated with [uber-go/mock](https://github.com/uber-go/mock) and live in `pkg/client/mock`. Import the mock package to replace the real client in your service tests:

```go
import (
    "go.uber.org/mock/gomock"
    "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client/mock"
)

func TestMyService(t *testing.T) {
    ctrl := gomock.NewController(t)
    m := mock.NewMockSecretsClient(ctrl)

    m.EXPECT().
        Read(gomock.Any(), "secret/tenants/t1/kubeconfig").
        Return(map[string]interface{}{"data": "..."}, nil)

    // inject m into the service under test
}
```

To regenerate the mocks after changing the `SecretsClient` interface, run:

```bash
make generate
```

## Testing

### Unit tests (no live OpenBao required)

The full test suite runs without any infrastructure using gomock:

```bash
make test            # unit tests with -race
make coverage        # per-function coverage report
```

### Integration tests

Integration tests use [testcontainers-go](https://testcontainers.com/guides/getting-started-with-testcontainers-for-go/)
to spin up a `hashicorp/vault:1.15.6` dev-mode container automatically.
The Vault API is wire-compatible with the OpenBao SDK. Docker must be running.

```bash
make integration-test
```

The container is started **once for the package** (`TestMain`) with:
- root token fixed to `root`
- KV v1 mount at `secret/` (matches `kubeconfigPathPrefix`)
- dev mode — no TLS, no persistence, unsealed automatically

#### Using an existing OpenBao/Vault instance

Set `OPENBAO_ADDR` and `OPENBAO_TOKEN` to bypass container creation (useful
in CI with a dedicated service container):

```bash
OPENBAO_ADDR=http://localhost:8200 OPENBAO_TOKEN=root make integration-test
```

#### What the integration tests cover

| File | Tests |
|------|-------|
| `client_integration_test.go` | `New` connects; `Write/Read` round-trip; overwrite; read missing path → `ErrSecretNotFound`; `Delete` removes secret; delete non-existent succeeds; `List` returns keys; list empty prefix returns `[]` |
| `kubeconfig_integration_test.go` | `StoreKubeconfig/LoadKubeconfig` round-trip; overwrite; load missing → `ErrSecretNotFound`; `RevokeKubeconfig` removes; revoke non-existent succeeds; full store→load→revoke lifecycle |

## Development

```bash
make build              # compile all packages
make test               # run unit tests with -race
make coverage           # per-function coverage report
make coverage-html      # open HTML coverage in browser
make integration-test   # run integration tests (Docker required)
make generate           # regenerate gomock mocks
make lint               # go vet (+ golangci-lint if available)
make tidy               # go mod tidy
make clean              # remove coverage artefacts
make help               # list all targets
```

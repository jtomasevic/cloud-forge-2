# CF-Provisioner Go Client

This module contains the generated Go client for the CF-Provisioner OpenAPI contract.

Generated source lives in [`v1/client.gen.go`](v1/client.gen.go). Do not hand-edit that file; update
`api/cf-provisioner/v1/openapi.yaml` and regenerate instead:

```bash
cd services/cf-provisioner && go generate ./...
```

The client includes typed request builders, response parsers, and models for the network, gateway,
subnet, job, and CF App Service routes. The app-service methods are available so control-plane
callers can compile against the contract while the server still returns typed `501` responses until
the later app-service persistence and reconciliation tasks land.

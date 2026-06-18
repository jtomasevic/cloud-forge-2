# Gateway Repository

`gateway` manages Gateway API `HTTPRoute` objects for CF-Provisioner. It uses the Gateway API clientset directly and keeps Envoy Gateway attachment details behind a small repository interface.

## Network Gateway Routes

`CreateHTTPRoute` remains compatible with the original network-level internet gateway flow. When no explicit rules are supplied, it creates one backend rule with no path matches, letting Gateway API default to `PathPrefix /`.

That path is still used by existing private-network gateway provisioning and may target the current placeholder backend until the service layer replaces that flow.

## App Service Routes

`CreateAppServiceHTTPRoute` is for CF App Service public exposure. It derives the `HTTPRoute` object name from the app service ID, not the network ID, because one public network can expose multiple app services under different hostnames.

The default app-service route contains rules for:

- service traffic at `PathPrefix /`
- Swagger UI at `Exact /swagger` and `PathPrefix /swagger/`
- OpenAPI JSON at `Exact /openapi.json`

By default documentation paths target the same backend Service as the app. Callers can provide `DocsBackend` to route those paths to a CloudForge documentation adapter when Task 34 wires that component.

## Cleanup

`DeleteAppServiceHTTPRoute` deletes by namespace and app service ID, using the same deterministic name helper as creation. Missing routes are treated as success by `DeleteHTTPRoute`, so exposure-removal jobs can retry safely.

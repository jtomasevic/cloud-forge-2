// Package service contains CF-Router's business logic: credential validation, tenant resolution via
// CF-Accounts, optional region enrichment, and readiness checks.
//
// # Responsibilities
//
//   - **JWT (Bearer) path**: fetch JWKS from the IdP, verify RS256 locally, read account id from
//     claims, then call CF-Accounts internal resolve with X-CF-Internal-Secret.
//
//   - **API key path**: BLAKE2b-256 hash the raw key, look up api_keys_by_hash in ScyllaDB, then
//     resolve tenant by account id from the row (again via CF-Accounts internal resolve).
//
//   - **Region**: CF-Accounts TenantContext does not include region. When the caller used Bearer auth
//     and a network id exists, we optionally call GET /v1/networks/{id} with the same Bearer to read
//     region from the network record. API-key-only callers may get an empty region.
//
// The REST proxy (package rest) calls [RouterService.ValidateAndResolve] on every proxied request.
package service

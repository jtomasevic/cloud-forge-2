package service

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

// Sentinel errors returned by [RouterService] and mapped to HTTP by the REST layer.
var (
	// ErrUnauthenticated means neither Bearer nor API key was usable (missing or wrong shape).
	ErrUnauthenticated = cferrors.New(cferrors.CodeUnauthorized, "missing or invalid credentials")
	// ErrJWTInvalid means the JWT failed structural, cryptographic, expiry, or issuer checks.
	ErrJWTInvalid = cferrors.New(cferrors.CodeUnauthorized, "invalid or expired JWT")
	// ErrTenantResolution means credentials did not map to a resolvable tenant in CF-Accounts.
	ErrTenantResolution = cferrors.New(cferrors.CodeUnauthorized, "cannot resolve tenant from credentials")
	// ErrAccountsUnreachable means the router could not complete an HTTP call to CF-Accounts (dial/TLS/5xx).
	ErrAccountsUnreachable = cferrors.New(cferrors.CodeUnavailable, "cf-accounts is unreachable")
)

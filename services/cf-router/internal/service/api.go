package service

import (
	"context"
	"net/http"

	cfaccountsclient "github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts/v1"

	apikeysrepo "github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/repository/apikeys"
)

// RouterService is the façade used by the REST layer (native + proxy) for auth and resolution.
type RouterService interface {
	// ValidateAndResolve validates credentials from HTTP headers and returns tenant metadata
	// suitable for X-CF-* injection on proxied requests.
	ValidateAndResolve(ctx context.Context, params ValidateParams) (TenantContext, error)

	// Ready probes connectivity to CF-Accounts (lightweight internal call). Nil means the router
	// considers the dependency reachable enough to serve traffic.
	Ready(ctx context.Context) error
}

// Deps are runtime dependencies for [New]. All fields except HTTPClient are required for full behavior.
type Deps struct {
	// APIKeys resolves BLAKE2b-256 key hashes to account ids (ScyllaDB).
	APIKeys apikeysrepo.APIKeyRepository

	// AccountsClient is the generated CF-Accounts HTTP client (internal resolve + optional GetNetwork).
	AccountsClient cfaccountsclient.ClientWithResponsesInterface

	// JWTPublicKeyURL is typically the Keycloak OIDC JWKS endpoint (JSON with RSA keys by "kid").
	JWTPublicKeyURL string

	// JWTExpectedIssuer, when non-empty, must match the JWT "iss" claim after successful signature verify.
	JWTExpectedIssuer string

	// InternalSecret is sent as X-CF-Internal-Secret on CF-Accounts internal resolve calls.
	InternalSecret string

	// HTTPClient overrides the client used for JWKS GETs (defaults to [http.DefaultClient]).
	HTTPClient *http.Client
}

// New constructs the production [RouterService] implementation ([cfRouterService]).
func New(d Deps) RouterService {
	return &cfRouterService{deps: d}
}

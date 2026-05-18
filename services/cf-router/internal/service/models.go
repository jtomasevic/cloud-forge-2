package service

// AuthMethod records which credential path produced a [TenantContext].
type AuthMethod string

const (
	// AuthMethodJWT means a Bearer JWT was verified and claims yielded the account id.
	AuthMethodJWT AuthMethod = "jwt"
	// AuthMethodAPIKey means X-CF-API-Key was hashed and resolved via ScyllaDB + CF-Accounts.
	AuthMethodAPIKey AuthMethod = "api_key"
)

// ValidateParams is extracted from incoming HTTP headers by the REST layer (proxy or debug endpoint).
type ValidateParams struct {
	// AuthorizationHeader is the full Authorization header value, e.g. "Bearer eyJ...".
	AuthorizationHeader string
	// APIKeyHeader is the raw X-CF-API-Key value (never logged, never forwarded upstream).
	APIKeyHeader string
}

// TenantContext is the trusted tenant view attached to proxied requests as X-CF-* headers.
type TenantContext struct {
	TenantID    string
	AccountID   string
	NetworkID   string // may be empty when no active network on the tenant
	Region      string // may be empty when not derivable (see package doc)
	Status      string // tenant status string from CF-Accounts
	ResolvedVia AuthMethod
}

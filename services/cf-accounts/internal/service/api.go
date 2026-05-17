// Package service implements CF-Accounts domain logic between the HTTP layer and
// ScyllaDB repositories. Callers use [AccountsService]; persistence stays behind
// repository interfaces in internal/repository.
package service

import (
	"context"

	accountsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts"
	credentialsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials"
	networksrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks"
	tenantsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants"
)

// AccountsService is the CF-Accounts business-logic API consumed by the REST layer
// (and any other in-process callers). Each method should map to at most one HTTP
// operation once the REST layer is wired.
type AccountsService interface {
	// CreateAccount registers a new customer: validates email and password, ensures
	// the email is unused, stores a bcrypt password hash, inserts the account row,
	// then creates a default tenant in "provisioning" with a URL-safe slug derived
	// from the email local-part (with collision handling). Returns both the account
	// and that default tenant so clients need not list tenants immediately.
	CreateAccount(ctx context.Context, params CreateAccountParams) (CreateAccountResult, error)

	// LoginWithPassword loads the account by email and verifies the password against
	// the stored bcrypt hash. Malformed passwords (e.g. shorter than signup minimum)
	// return the same validation errors as [CreateAccount]. Wrong password, unknown
	// email, empty hash, or non-active account returns [ErrInvalidCredentials] so callers
	// cannot infer whether the email was unknown or the password was wrong. On success returns [Account] without password material.
	LoginWithPassword(ctx context.Context, params LoginWithPasswordParams) (Account, error)

	// GetAccount returns an account by primary key UUID string.
	GetAccount(ctx context.Context, id string) (Account, error)

	// GetAccountByEmail resolves an account via the accounts_by_email lookup table
	// then loads the full accounts row.
	GetAccountByEmail(ctx context.Context, email string) (Account, error)

	// ListAccounts returns a slice of accounts for admin-style listing. total is the
	// repository total when supported; otherwise it may be a sentinel (see repository).
	ListAccounts(ctx context.Context, limit, offset int) ([]Account, int, error)

	// SuspendAccount marks the account suspended if it exists (repository also updates
	// denormalized email lookup status where applicable).
	SuspendAccount(ctx context.Context, id string) error

	// GetTenant returns a tenant by id.
	GetTenant(ctx context.Context, id string) (Tenant, error)

	// ListTenants lists tenants for the given account id with simple limit/offset paging.
	ListTenants(ctx context.Context, accountID string, limit, offset int) ([]Tenant, error)

	// CreateNetwork ensures the tenant exists, then inserts a new network row for that
	// tenant in region from params with empty CIDRs and status "provisioning" (real
	// CIDR allocation is expected to be filled by a later provisioning pipeline).
	CreateNetwork(ctx context.Context, params CreateNetworkParams) (Network, error)

	// GetNetwork returns a network by id.
	GetNetwork(ctx context.Context, id string) (Network, error)

	// ListNetworks returns all networks for a tenant id (order defined by repository).
	ListNetworks(ctx context.Context, tenantID string) ([]Network, error)

	// DeprovisionNetwork sets the network status to "deprovisioning" if the network
	// exists; asynchronous teardown is out of scope for this call.
	DeprovisionNetwork(ctx context.Context, id string) error

	// CreateCredential verifies the account exists, generates a random 32-byte API key,
	// returns the raw secret once, and persists only a BLAKE2b-256 hex hash of the secret
	// plus an 8-character prefix for listing. The full secret cannot be read back later.
	CreateCredential(ctx context.Context, params CreateCredentialParams) (CredentialCreated, error)

	// ListCredentials returns metadata for all keys on an account (no secrets).
	ListCredentials(ctx context.Context, accountID string) ([]CredentialMeta, error)

	// RevokeCredential marks a key revoked at the current time if it exists.
	RevokeCredential(ctx context.Context, id string) error

	// ResolveTenantContext is used by internal callers (e.g. CF-Router): supply either
	// APIKeyHash (BLAKE2b-256 hex of the raw key) or AccountID. It walks credentials
	// and/or tenants and networks to produce a single routing context; see implementation
	// for v1 heuristics when multiple tenants or networks exist.
	ResolveTenantContext(ctx context.Context, params ResolveTenantParams) (TenantContext, error)
}

// Deps wires repository interfaces into the service.
type Deps struct {
	Accounts    accountsrepo.AccountsRepository
	Tenants     tenantsrepo.TenantsRepository
	Networks    networksrepo.NetworksRepository
	Credentials credentialsrepo.CredentialsRepository
}

// New constructs an AccountsService backed by the given repositories.
func New(d Deps) AccountsService {
	return &CFAccountsService{deps: d}
}

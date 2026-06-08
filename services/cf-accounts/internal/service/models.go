package service

import "time"

// Account is the service-layer view of an account (IDs as strings for API use).
type Account struct {
	ID        string
	Email     string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Tenant is the service-layer view of a tenant.
type Tenant struct {
	ID        string
	AccountID string
	Slug      string
	Region    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Network is the service-layer view of a network.
type Network struct {
	ID        string
	TenantID  string
	Region    string
	PodCIDR   string
	SvcCIDR   string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CredentialMeta is a credential without secret material.
type CredentialMeta struct {
	ID        string
	AccountID string
	KeyPrefix string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// CredentialCreated is returned once from CreateCredential; Secret is never persisted.
type CredentialCreated struct {
	CredentialMeta
	Secret string
}

// TenantContext is returned for CF-Router tenant resolution.
type TenantContext struct {
	TenantID  string
	AccountID string
	NetworkID string
	Region    string
	Status    string
}

// CreateAccountResult is returned from CreateAccount: the new account plus the
// default tenant created at signup (id and slug) so clients avoid an extra list call.
type CreateAccountResult struct {
	Account       Account
	DefaultTenant Tenant
}

// CreateAccountParams holds input for CreateAccount.
// Password is hashed with bcrypt before persistence; it is never stored in plaintext.
type CreateAccountParams struct {
	Email    string
	Password string
}

// LoginWithPasswordParams holds credentials for password-based login.
type LoginWithPasswordParams struct {
	Email    string
	Password string
}

// LoginResult is returned from password login: token material plus the active account.
type LoginResult struct {
	AccessToken      string
	RefreshToken     string
	IDToken          string
	TokenType        string
	Scope            string
	ExpiresIn        int
	RefreshExpiresIn int
	Account          Account
}

// CreateNetworkParams holds input for CreateNetwork.
type CreateNetworkParams struct {
	TenantID string
	Region   string
}

// CreateCredentialParams holds input for CreateCredential.
type CreateCredentialParams struct {
	AccountID string
}

// ResolveTenantParams selects resolution strategy (exactly one path should be used).
type ResolveTenantParams struct {
	AccountID  string
	APIKeyHash string
}

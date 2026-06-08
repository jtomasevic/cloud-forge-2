package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"golang.org/x/crypto/blake2b"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	accountsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts"
	credentialsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials"
	identityrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/identity"
	networksrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks"
	tenantsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants"
)

// CFAccountsService implements [AccountsService] using the repositories in [Deps].
type CFAccountsService struct {
	deps Deps
}

// CreateAccount implements the signup flow described on [AccountsService.CreateAccount].
// Order matters: email uniqueness is checked before writes; identity creation happens before
// database inserts so a user cannot log in to an account that failed to persist.
func (s *CFAccountsService) CreateAccount(ctx context.Context, params CreateAccountParams) (CreateAccountResult, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		return CreateAccountResult{}, cferrors.Wrap(cferrors.CodeInvalidInput, "email is required", cferrors.ErrInvalidInput)
	}
	if err := validateNewAccountPassword(params.Password); err != nil {
		return CreateAccountResult{}, err
	}

	_, err := s.deps.Accounts.GetByEmail(ctx, email)
	if err == nil {
		return CreateAccountResult{}, ErrAccountEmailTaken
	}
	if err != nil && !errors.Is(err, accountsrepo.ErrAccountNotFound) {
		return CreateAccountResult{}, err
	}

	passwordHash, err := hashPasswordBcrypt(params.Password)
	if err != nil {
		return CreateAccountResult{}, err
	}

	now := time.Now().UTC()
	accountUUID := uuid.New()
	accountID := accountUUID.String()

	accRow, err := params.ToRepositoryInsertAccountRow(accountID, now, passwordHash)
	if err != nil {
		return CreateAccountResult{}, cferrors.Wrap(cferrors.CodeInternal, "invalid generated account id", err)
	}

	baseSlug := emailLocalPartToSlug(email)
	slug, err := s.pickUniqueTenantSlug(ctx, baseSlug)
	if err != nil {
		return CreateAccountResult{}, err
	}

	tenantUUID := uuid.New()
	tenantID := gocql.UUID(tenantUUID)
	accountGocql := gocql.UUID(accountUUID)
	tenantRow := ToRepositoryInsertTenantRow(accountGocql, tenantID, slug, "", "active", now)

	identityID := ""
	if s.deps.Identity != nil {
		identityUser, err := s.deps.Identity.CreateUser(ctx, identityrepo.CreateUserParams{
			ID:        accountID,
			AccountID: accountID,
			Email:     email,
			Password:  params.Password,
		})
		if err != nil {
			if errors.Is(err, identityrepo.ErrUserExists) {
				return CreateAccountResult{}, ErrAccountEmailTaken
			}
			return CreateAccountResult{}, err
		}
		identityID = identityUser.ID
	}

	if err := s.deps.Accounts.Insert(ctx, accRow); err != nil {
		return CreateAccountResult{}, s.rollbackIdentityUser(ctx, identityID, err)
	}
	if err := s.deps.Tenants.Insert(ctx, tenantRow); err != nil {
		return CreateAccountResult{}, s.rollbackIdentityUser(ctx, identityID, err)
	}

	return CreateAccountResult{
		Account:       ToServiceAccountFromRepository(accRow),
		DefaultTenant: ToServiceTenantFromRepository(tenantRow),
	}, nil
}

func (s *CFAccountsService) rollbackIdentityUser(ctx context.Context, identityID string, cause error) error {
	if strings.TrimSpace(identityID) == "" || s.deps.Identity == nil {
		return cause
	}
	if err := s.deps.Identity.DeleteUser(context.WithoutCancel(ctx), identityID); err != nil {
		slog.WarnContext(ctx, "failed to roll back identity user after signup failure", "identity_id", identityID, "error", err)
	}
	return cause
}

// LoginWithPassword implements [AccountsService.LoginWithPassword].
func (s *CFAccountsService) LoginWithPassword(ctx context.Context, params LoginWithPasswordParams) (LoginResult, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err := validateNewAccountPassword(params.Password); err != nil {
		return LoginResult{}, err
	}
	if s.deps.Identity == nil {
		return LoginResult{}, ErrIdentityProviderNotConfigured
	}
	token, err := s.deps.Identity.AuthenticatePassword(ctx, identityrepo.AuthenticatePasswordParams{
		Email:    email,
		Password: params.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, identityrepo.ErrAuthenticationFailed):
			return LoginResult{}, ErrInvalidCredentials
		case errors.Is(err, identityrepo.ErrClientConfig):
			return LoginResult{}, cferrors.Wrap(cferrors.CodeInternal, "identity provider login configuration failed", err)
		case errors.Is(err, identityrepo.ErrIdentityService):
			return LoginResult{}, cferrors.Wrap(cferrors.CodeUnavailable, "identity provider login failed", err)
		default:
			return LoginResult{}, cferrors.Wrap(cferrors.CodeUnavailable, "identity provider login failed", err)
		}
	}
	if strings.TrimSpace(token.AccountID) == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	row, err := s.deps.Accounts.GetByID(ctx, token.AccountID)
	if err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}
	account := ToServiceAccountFromRepository(row)
	if account.Status != "active" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if account.ID != token.AccountID {
		return LoginResult{}, ErrInvalidCredentials
	}
	if token.Email != "" && !strings.EqualFold(token.Email, account.Email) {
		return LoginResult{}, ErrInvalidCredentials
	}
	return LoginResult{
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		IDToken:          token.IDToken,
		TokenType:        token.TokenType,
		Scope:            token.Scope,
		ExpiresIn:        token.ExpiresIn,
		RefreshExpiresIn: token.RefreshExpiresIn,
		Account:          account,
	}, nil
}

// pickUniqueTenantSlug returns a slug at most 63 chars based on baseSlug that does
// not collide with an existing global slug (tenants_by_slug). On collision it appends
// a short random hex suffix; after 12 attempts it returns [ErrSlugConflict].
func (s *CFAccountsService) pickUniqueTenantSlug(ctx context.Context, baseSlug string) (string, error) {
	slug := trimSlugLen(baseSlug, 63)
	for attempt := 0; attempt < 12; attempt++ {
		_, err := s.deps.Tenants.GetBySlug(ctx, slug)
		if err != nil && errors.Is(err, tenantsrepo.ErrTenantNotFound) {
			return slug, nil
		}
		if err == nil {
			suf := randomHex4()
			candidate := trimSlugLen(baseSlug+"-"+suf, 63)
			slug = candidate
			continue
		}
		return "", err
	}
	return "", ErrSlugConflict
}

// GetAccount maps repository errors to [ErrAccountNotFound] where appropriate.
func (s *CFAccountsService) GetAccount(ctx context.Context, id string) (Account, error) {
	row, err := s.deps.Accounts.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, err
	}
	return ToServiceAccountFromRepository(row), nil
}

// GetAccountByEmail loads via lookup table then full account row.
func (s *CFAccountsService) GetAccountByEmail(ctx context.Context, email string) (Account, error) {
	row, err := s.deps.Accounts.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, err
	}
	return ToServiceAccountFromRepository(row), nil
}

// ListAccounts delegates to the repository list (v1 may use ALLOW FILTERING; see repository docs).
func (s *CFAccountsService) ListAccounts(ctx context.Context, limit, offset int) ([]Account, int, error) {
	rows, total, err := s.deps.Accounts.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Account, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToServiceAccountFromRepository(row))
	}
	return out, total, nil
}

// SuspendAccount verifies the account exists then delegates status update to the repository.
func (s *CFAccountsService) SuspendAccount(ctx context.Context, id string) error {
	if _, err := s.deps.Accounts.GetByID(ctx, id); err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return ErrAccountNotFound
		}
		return err
	}
	return s.deps.Accounts.UpdateStatus(ctx, id, "suspended")
}

// GetTenant loads a tenant by primary key.
func (s *CFAccountsService) GetTenant(ctx context.Context, id string) (Tenant, error) {
	row, err := s.deps.Tenants.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, tenantsrepo.ErrTenantNotFound) {
			return Tenant{}, ErrTenantNotFound
		}
		return Tenant{}, err
	}
	return ToServiceTenantFromRepository(row), nil
}

// ListTenants lists tenants for an account with limit/offset paging.
func (s *CFAccountsService) ListTenants(ctx context.Context, accountID string, limit, offset int) ([]Tenant, error) {
	rows, err := s.deps.Tenants.ListByAccount(ctx, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]Tenant, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToServiceTenantFromRepository(row))
	}
	return out, nil
}

// CreateNetwork validates the tenant, allocates a new network id, and inserts a row
// with status "provisioning". Pod/service CIDRs are left empty here for a provisioner
// to fill later.
func (s *CFAccountsService) CreateNetwork(ctx context.Context, params CreateNetworkParams) (Network, error) {
	if _, err := s.deps.Tenants.GetByID(ctx, params.TenantID); err != nil {
		if errors.Is(err, tenantsrepo.ErrTenantNotFound) {
			return Network{}, ErrTenantNotFound
		}
		return Network{}, err
	}

	now := time.Now().UTC()
	netUUID := uuid.New()
	netID := gocql.UUID(netUUID)

	row, err := params.ToRepositoryInsertNetworkRow(netID, now)
	if err != nil {
		return Network{}, cferrors.Wrap(cferrors.CodeInvalidInput, "invalid tenant id", cferrors.ErrInvalidInput)
	}
	if err := s.deps.Networks.Insert(ctx, row); err != nil {
		return Network{}, err
	}
	return ToServiceNetworkFromRepository(row), nil
}

// GetNetwork loads a network by id.
func (s *CFAccountsService) GetNetwork(ctx context.Context, id string) (Network, error) {
	row, err := s.deps.Networks.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, networksrepo.ErrNetworkNotFound) {
			return Network{}, ErrNetworkNotFound
		}
		return Network{}, err
	}
	return ToServiceNetworkFromRepository(row), nil
}

// ListNetworks returns all networks for the tenant.
func (s *CFAccountsService) ListNetworks(ctx context.Context, tenantID string) ([]Network, error) {
	rows, err := s.deps.Networks.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Network, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToServiceNetworkFromRepository(row))
	}
	return out, nil
}

// DeprovisionNetwork transitions an existing network to "deprovisioning".
func (s *CFAccountsService) DeprovisionNetwork(ctx context.Context, id string) error {
	if _, err := s.deps.Networks.GetByID(ctx, id); err != nil {
		if errors.Is(err, networksrepo.ErrNetworkNotFound) {
			return ErrNetworkNotFound
		}
		return err
	}
	return s.deps.Networks.UpdateStatus(ctx, id, "deprovisioning")
}

// CreateCredential implements [AccountsService.CreateCredential]: random 32-byte key,
// BLAKE2b-256 hex hash stored for lookup by hash, 8-byte prefix for listing UX.
func (s *CFAccountsService) CreateCredential(ctx context.Context, params CreateCredentialParams) (CredentialCreated, error) {
	if _, err := s.deps.Accounts.GetByID(ctx, params.AccountID); err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return CredentialCreated{}, ErrAccountNotFound
		}
		return CredentialCreated{}, err
	}

	var secretBytes [32]byte
	if _, err := rand.Read(secretBytes[:]); err != nil {
		return CredentialCreated{}, cferrors.Wrap(cferrors.CodeInternal, "failed to generate API key", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes[:])
	sum := blake2b.Sum256(secretBytes[:])
	keyHash := hex.EncodeToString(sum[:])
	prefix := secret
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	now := time.Now().UTC()
	keyUUID := uuid.New()
	keyID := gocql.UUID(keyUUID)
	aid, err := gocql.ParseUUID(params.AccountID)
	if err != nil {
		return CredentialCreated{}, cferrors.Wrap(cferrors.CodeInvalidInput, "invalid account id", cferrors.ErrInvalidInput)
	}

	row := ToRepositoryInsertAPIKeyRow(aid, keyID, keyHash, prefix, now)
	if err := s.deps.Credentials.Insert(ctx, row); err != nil {
		return CredentialCreated{}, err
	}

	meta := ToCredentialMetaFromRepository(row)
	return CredentialCreated{CredentialMeta: meta, Secret: secret}, nil
}

// ListCredentials lists API key metadata for an account (no raw secrets).
func (s *CFAccountsService) ListCredentials(ctx context.Context, accountID string) ([]CredentialMeta, error) {
	rows, err := s.deps.Credentials.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialMeta, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToCredentialMetaFromRepository(row))
	}
	return out, nil
}

// RevokeCredential ensures the key exists then sets revoked_at.
func (s *CFAccountsService) RevokeCredential(ctx context.Context, id string) error {
	if _, err := s.deps.Credentials.GetByID(ctx, id); err != nil {
		if errors.Is(err, credentialsrepo.ErrCredentialNotFound) {
			return ErrCredentialNotFound
		}
		return err
	}
	return s.deps.Credentials.Revoke(ctx, id, time.Now().UTC())
}

// ResolveTenantContext dispatches to hash- or account-based resolution. Exactly one
// of APIKeyHash or AccountID should be set; both empty yields [ErrResolutionFailed].
func (s *CFAccountsService) ResolveTenantContext(ctx context.Context, params ResolveTenantParams) (TenantContext, error) {
	switch {
	case params.APIKeyHash != "":
		return s.resolveByAPIKeyHash(ctx, params.APIKeyHash)
	case params.AccountID != "":
		return s.resolveByAccountID(ctx, params.AccountID)
	default:
		return TenantContext{}, ErrResolutionFailed
	}
}

// resolveByAPIKeyHash loads the API key row by BLAKE2b-256 hex hash, maps revoked /
// missing keys to [ErrResolutionFailed], then continues as account resolution using
// the key's account id.
func (s *CFAccountsService) resolveByAPIKeyHash(ctx context.Context, keyHash string) (TenantContext, error) {
	row, err := s.deps.Credentials.GetByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, credentialsrepo.ErrCredentialNotFound) {
			return TenantContext{}, ErrResolutionFailed
		}
		if errors.Is(err, credentialsrepo.ErrCredentialRevoked) {
			return TenantContext{}, ErrResolutionFailed
		}
		return TenantContext{}, err
	}
	return s.resolveByAccountID(ctx, row.AccountID.String())
}

// resolveByAccountID is the v1 tenant-resolution heuristic for router use: verify the
// account exists, take the first tenant row from ListByAccount(limit=1), then choose
// a network via [pickActiveNetwork] when one exists. A signed-up account may have an
// active tenant before any network is provisioned, so the resolved network is optional.
// If there is no tenant or the account is missing, returns [ErrResolutionFailed].
// Region on the result is taken from the tenant row as stored (may be empty when
// networks carry authoritative region).
func (s *CFAccountsService) resolveByAccountID(ctx context.Context, accountID string) (TenantContext, error) {
	if _, err := s.deps.Accounts.GetByID(ctx, accountID); err != nil {
		if errors.Is(err, accountsrepo.ErrAccountNotFound) {
			return TenantContext{}, ErrResolutionFailed
		}
		return TenantContext{}, err
	}

	tenants, err := s.deps.Tenants.ListByAccount(ctx, accountID, 1, 0)
	if err != nil {
		return TenantContext{}, err
	}
	if len(tenants) == 0 {
		return TenantContext{}, ErrResolutionFailed
	}
	t := tenants[0]
	out := TenantContext{
		TenantID:  t.ID.String(),
		AccountID: t.AccountID.String(),
		Region:    t.Region,
		Status:    t.Status,
	}

	nets, err := s.deps.Networks.ListByTenant(ctx, t.ID.String())
	if err != nil {
		return TenantContext{}, err
	}
	net := pickActiveNetwork(nets)
	if net.ID == (gocql.UUID{}) {
		return out, nil
	}
	out.NetworkID = net.ID.String()
	return out, nil
}

// pickActiveNetwork prefers the first row with status "active", else the first
// "provisioning", else the first row in the slice, else a zero row (caller treats
// zero id as failure). This keeps router behavior deterministic without ordering guarantees from Scylla.
func pickActiveNetwork(rows []networksrepo.NetworkRow) networksrepo.NetworkRow {
	for _, n := range rows {
		if n.Status == "active" {
			return n
		}
	}
	for _, n := range rows {
		if n.Status == "provisioning" {
			return n
		}
	}
	if len(rows) > 0 {
		return rows[0]
	}
	return networksrepo.NetworkRow{}
}

func emailLocalPartToSlug(email string) string {
	at := strings.LastIndex(email, "@")
	local := email
	if at > 0 {
		local = email[:at]
	}
	local = strings.ToLower(strings.TrimSpace(local))
	var b strings.Builder
	for _, r := range local {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	s := collapseDashes(strings.Trim(b.String(), "-"))
	if s == "" {
		return "tenant"
	}
	return s
}

func collapseDashes(s string) string {
	var out strings.Builder
	prevDash := false
	for _, r := range s {
		if r == '-' {
			if !prevDash {
				out.WriteByte('-')
				prevDash = true
			}
		} else {
			prevDash = false
			out.WriteRune(r)
		}
	}
	return out.String()
}

func trimSlugLen(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func randomHex4() string {
	var buf [2]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

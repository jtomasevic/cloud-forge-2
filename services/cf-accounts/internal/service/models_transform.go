package service

import (
	"time"

	"github.com/gocql/gocql"

	accountsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts"
	credentialsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials"
	networksrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks"
	tenantsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants"
)

// ToRepositoryInsertAccountRow converts create parameters into a repository row for insertion.
// passwordHash must be a bcrypt hash of p.Password (caller responsibility).
func (p *CreateAccountParams) ToRepositoryInsertAccountRow(id string, now time.Time, passwordHash string) (accountsrepo.AccountRow, error) {
	uid, err := gocql.ParseUUID(id)
	if err != nil {
		return accountsrepo.AccountRow{}, err
	}
	return accountsrepo.AccountRow{
		ID:           uid,
		Email:        p.Email,
		Status:       "active",
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// ToServiceAccountFromRepository maps a persistence row to a service Account.
func ToServiceAccountFromRepository(row accountsrepo.AccountRow) Account {
	return Account{
		ID:        row.ID.String(),
		Email:     row.Email,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ToRepositoryInsertTenantRow builds a tenant row for insertion (default tenant on signup).
func ToRepositoryInsertTenantRow(accountID, tenantID gocql.UUID, slug, region, status string, now time.Time) tenantsrepo.TenantRow {
	return tenantsrepo.TenantRow{
		ID:        tenantID,
		AccountID: accountID,
		Slug:      slug,
		Region:    region,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ToServiceTenantFromRepository maps a persistence row to a service Tenant.
func ToServiceTenantFromRepository(row tenantsrepo.TenantRow) Tenant {
	return Tenant{
		ID:        row.ID.String(),
		AccountID: row.AccountID.String(),
		Slug:      row.Slug,
		Region:    row.Region,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ToRepositoryInsertNetworkRow converts CreateNetworkParams into a repository row for insertion.
func (p *CreateNetworkParams) ToRepositoryInsertNetworkRow(networkID gocql.UUID, now time.Time) (networksrepo.NetworkRow, error) {
	tid, err := gocql.ParseUUID(p.TenantID)
	if err != nil {
		return networksrepo.NetworkRow{}, err
	}
	return networksrepo.NetworkRow{
		ID:        networkID,
		TenantID:  tid,
		Region:    p.Region,
		PodCIDR:   "",
		SvcCIDR:   "",
		Status:    "provisioning",
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ToServiceNetworkFromRepository maps a persistence row to a service Network.
func ToServiceNetworkFromRepository(row networksrepo.NetworkRow) Network {
	return Network{
		ID:        row.ID.String(),
		TenantID:  row.TenantID.String(),
		Region:    row.Region,
		PodCIDR:   row.PodCIDR,
		SvcCIDR:   row.SvcCIDR,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ToRepositoryInsertAPIKeyRow builds an API key row for insertion (hash + prefix only; no raw secret).
func ToRepositoryInsertAPIKeyRow(accountID, keyID gocql.UUID, keyHash, keyPrefix string, createdAt time.Time) credentialsrepo.APIKeyRow {
	return credentialsrepo.APIKeyRow{
		ID:        keyID,
		AccountID: accountID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		CreatedAt: createdAt,
		RevokedAt: nil,
	}
}

// ToCredentialMetaFromRepository maps a persistence row to metadata (no hash field).
func ToCredentialMetaFromRepository(row credentialsrepo.APIKeyRow) CredentialMeta {
	var revoked *time.Time
	if row.RevokedAt != nil {
		t := *row.RevokedAt
		revoked = &t
	}
	return CredentialMeta{
		ID:        row.ID.String(),
		AccountID: row.AccountID.String(),
		KeyPrefix: row.KeyPrefix,
		CreatedAt: row.CreatedAt,
		RevokedAt: revoked,
	}
}

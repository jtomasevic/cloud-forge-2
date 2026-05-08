// Package tenant defines the canonical shared tenant model used across all
// CloudForge platform services. It is a domain type — not a persistence model
// or a REST model.
package tenant

import "time"

// TenantStatus represents the lifecycle state of a tenant.
type TenantStatus string

const (
	// TenantStatusProvisioning indicates the tenant infrastructure is being created.
	TenantStatusProvisioning TenantStatus = "provisioning"
	// TenantStatusActive indicates the tenant is fully operational.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusSuspended indicates the tenant has been temporarily suspended.
	TenantStatusSuspended TenantStatus = "suspended"
	// TenantStatusDeprovisioning indicates the tenant infrastructure is being torn down.
	TenantStatusDeprovisioning TenantStatus = "deprovisioning"
	// TenantStatusDeprovisioned indicates the tenant has been fully removed.
	TenantStatusDeprovisioned TenantStatus = "deprovisioned"
)

// Tenant is the canonical shared tenant model.
// Region is intentionally absent: a tenant may own networks in multiple
// regions, so region is a property of Network, not of Tenant.
type Tenant struct {
	ID        string
	AccountID string
	Slug      string
	Status    TenantStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

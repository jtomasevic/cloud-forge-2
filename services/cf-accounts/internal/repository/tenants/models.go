package tenants

import (
	"time"

	"github.com/gocql/gocql"
)

// TenantRow maps to CQL rows in tenants / tenants_by_account / tenants_by_slug.
type TenantRow struct {
	ID        gocql.UUID
	AccountID gocql.UUID
	Slug      string
	Region    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

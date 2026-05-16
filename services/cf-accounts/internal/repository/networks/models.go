package networks

import (
	"time"

	"github.com/gocql/gocql"
)

// NetworkRow maps to CQL rows in networks / networks_by_tenant.
type NetworkRow struct {
	ID        gocql.UUID
	TenantID  gocql.UUID
	Region    string
	PodCIDR   string
	SvcCIDR   string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

package tenants

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// TenantsRepository persists tenants in ScyllaDB.
type TenantsRepository interface {
	Insert(ctx context.Context, row TenantRow) error
	GetByID(ctx context.Context, id string) (TenantRow, error)
	GetBySlug(ctx context.Context, slug string) (TenantRow, error)
	ListByAccount(ctx context.Context, accountID string, limit, offset int) ([]TenantRow, error)
	UpdateStatus(ctx context.Context, id string, status string) error
}

// New returns a TenantsRepository backed by the given ScyllaDB session.
func New(session *scylladbclient.Session) TenantsRepository {
	return &tenantsRepository{session: session}
}

package networks

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// NetworksRepository persists networks in ScyllaDB.
type NetworksRepository interface {
	Insert(ctx context.Context, row NetworkRow) error
	GetByID(ctx context.Context, id string) (NetworkRow, error)
	ListByTenant(ctx context.Context, tenantID string) ([]NetworkRow, error)
	UpdateStatus(ctx context.Context, id string, status string) error
}

// New returns a NetworksRepository backed by the given ScyllaDB session.
func New(session *scylladbclient.Session) NetworksRepository {
	return &networksRepository{session: session}
}

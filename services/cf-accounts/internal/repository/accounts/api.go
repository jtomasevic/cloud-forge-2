package accounts

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// AccountsRepository persists accounts in ScyllaDB.
type AccountsRepository interface {
	Insert(ctx context.Context, row AccountRow) error
	GetByID(ctx context.Context, id string) (AccountRow, error)
	GetByEmail(ctx context.Context, email string) (AccountRow, error)
	// List returns a page of accounts. The int is a total hint: -1 means unknown
	// (v1 uses ALLOW FILTERING with a scan cap; not suitable for large clusters).
	List(ctx context.Context, limit, offset int) ([]AccountRow, int, error)
	UpdateStatus(ctx context.Context, id string, status string) error
}

// New returns an AccountsRepository backed by the given ScyllaDB session.
func New(session *scylladbclient.Session) AccountsRepository {
	return &accountsRepository{session: session}
}

package credentials

import (
	"context"
	"time"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// CredentialsRepository persists API keys in ScyllaDB.
type CredentialsRepository interface {
	Insert(ctx context.Context, row APIKeyRow) error
	GetByID(ctx context.Context, id string) (APIKeyRow, error)
	GetByHash(ctx context.Context, keyHash string) (APIKeyRow, error)
	ListByAccount(ctx context.Context, accountID string) ([]APIKeyRow, error)
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
}

// New returns a CredentialsRepository backed by the given ScyllaDB session.
func New(session *scylladbclient.Session) CredentialsRepository {
	return &credentialsRepository{session: session}
}

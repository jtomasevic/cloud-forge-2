package apikeys

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// APIKeyRepository performs read-only lookups on api_keys_by_hash (partition key = BLAKE2b-256 hex).
type APIKeyRepository interface {
	// GetByHash returns key metadata for a hash. ErrKeyNotFound if absent; ErrKeyRevoked if revoked_at set.
	GetByHash(ctx context.Context, keyHash string) (APIKeyRecord, error)
}

// New constructs the repository using the shared ScyllaDB session (same keyspace as CF-Accounts).
func New(session *scylladbclient.Session) APIKeyRepository {
	return &cfRouterAPIKeyRepository{session: session}
}

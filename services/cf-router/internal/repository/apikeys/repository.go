package apikeys

import (
	"context"
	"time"

	"github.com/gocql/gocql"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// cqlSelectAPIKeyByHashLookup reads the denormalized hash index row (fast partition-key lookup).
// Columns match tools/migrations: key_hash PK, key_id, account_id, revoked_at.
const cqlSelectAPIKeyByHashLookup = `SELECT key_id, account_id, revoked_at FROM api_keys_by_hash WHERE key_hash = ?`

// cfRouterAPIKeyRepository implements [APIKeyRepository] with a thin gocql wrapper.
type cfRouterAPIKeyRepository struct {
	session *scylladbclient.Session
}

// GetByHash implements [APIKeyRepository.GetByHash].
//
// Scylla represents unset timestamps as zero time; we treat non-zero revokedAt as revoked.
func (r *cfRouterAPIKeyRepository) GetByHash(ctx context.Context, keyHash string) (APIKeyRecord, error) {
	var keyID gocql.UUID
	var accountID gocql.UUID
	var revokedAt time.Time
	if err := r.session.Query(cqlSelectAPIKeyByHashLookup, keyHash).WithContext(ctx).Scan(&keyID, &accountID, &revokedAt); err != nil {
		if err == gocql.ErrNotFound {
			return APIKeyRecord{}, ErrKeyNotFound
		}
		return APIKeyRecord{}, err
	}
	if !revokedAt.IsZero() {
		return APIKeyRecord{}, ErrKeyRevoked
	}
	return APIKeyRecord{
		KeyID:     keyID.String(),
		AccountID: accountID.String(),
		RevokedAt: nil,
	}, nil
}

package credentials

import (
	"time"

	"github.com/gocql/gocql"
)

// APIKeyRow maps to CQL rows in api_keys / api_keys_by_account / api_keys_by_hash.
type APIKeyRow struct {
	ID        gocql.UUID
	AccountID gocql.UUID
	KeyHash   string
	KeyPrefix string
	CreatedAt time.Time
	RevokedAt *time.Time
}

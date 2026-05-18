package apikeys

import "time"

// APIKeyRecord is the minimal projection needed to continue authentication in the service layer.
type APIKeyRecord struct {
	KeyID     string     // api_keys.id (UUID string)
	AccountID string     // owning account UUID string
	RevokedAt *time.Time // nil when active (GetByHash returns ErrKeyRevoked instead when revoked)
}

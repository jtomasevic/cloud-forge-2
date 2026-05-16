package accounts

import (
	"time"

	"github.com/gocql/gocql"
)

// AccountRow maps to CQL rows in the accounts / accounts_by_email tables.
// PasswordHash is a bcrypt hash (never plaintext). Empty when not set (legacy rows).
type AccountRow struct {
	ID           gocql.UUID
	Email        string
	Status       string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

package accounts

import (
	"time"

	"github.com/gocql/gocql"
)

// AccountRow maps to CQL rows in the accounts / accounts_by_email tables.
type AccountRow struct {
	ID        gocql.UUID
	Email     string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

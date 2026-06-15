// Package subnets persists logical private/public subnet placement records for CF-Provisioner.
//
// Subnets are durable control-plane metadata used by later app-service placement flows to verify
// that a requested subnet exists, belongs to a network, and has the expected private/public type.
package subnets

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// SubnetsRepository owns durable subnet rows and lookup indexes in ScyllaDB.
type SubnetsRepository interface {
	// Create inserts a subnet and its denormalized lookup rows.
	// Returns ErrSubnetCIDRExists when the same canonical CIDR already exists in the network.
	Create(ctx context.Context, params CreateSubnetParams) (Subnet, error)

	// GetByID returns a subnet by id and verifies that it belongs to networkID.
	GetByID(ctx context.Context, networkID, subnetID string) (Subnet, error)

	// ListByNetwork returns all subnets in a network.
	ListByNetwork(ctx context.Context, networkID string) ([]Subnet, error)
}

// New returns a SubnetsRepository backed by the given ScyllaDB session (keyspace cloudforge).
func New(session *scylladbclient.Session) SubnetsRepository {
	return &subnetsRepository{session: session}
}

package cidr

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// CIDRRepository manages pod/service CIDR allocations for tenant networks in ScyllaDB.
type CIDRRepository interface {
	// Allocate reserves pod and service CIDRs for a network.
	// If podCIDR and svcCIDR are empty strings, auto-allocate from the platform supernet.
	// Returns ErrCIDRExhausted if no CIDRs are available.
	// Returns ErrCIDRAlreadyAllocated if the network already has an allocation.
	Allocate(ctx context.Context, params AllocateParams) (CIDRAllocation, error)

	// Get returns the CIDR allocation for a network.
	Get(ctx context.Context, networkID string) (CIDRAllocation, error)

	// Release frees the CIDR allocation for a network (called on deprovision).
	Release(ctx context.Context, networkID string) error

	// ListAll returns all current CIDR allocations (for ops/debugging).
	ListAll(ctx context.Context) ([]CIDRAllocation, error)
}

// New returns a CIDRRepository backed by the given ScyllaDB session (keyspace cloudforge).
func New(session *scylladbclient.Session) CIDRRepository {
	return &cidrRepository{session: session}
}

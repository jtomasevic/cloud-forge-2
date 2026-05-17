package cidr

// CQL for cidr_allocations (keyspace cloudforge). Bind order in each comment.

const (
	// cqlInsertAllocation inserts one allocation row.
	// Parameters: (1) network_id UUID, (2) pod_cidr text, (3) svc_cidr text, (4) allocated_at timestamp.
	cqlInsertAllocation = `INSERT INTO cidr_allocations (network_id, pod_cidr, svc_cidr, allocated_at) VALUES (?, ?, ?, ?)`

	// cqlSelectAllocationByNetwork loads a single allocation by network_id.
	// Parameters: (1) network_id UUID.
	cqlSelectAllocationByNetwork = `SELECT network_id, pod_cidr, svc_cidr, allocated_at FROM cidr_allocations WHERE network_id = ?`

	// cqlSelectAllAllocations returns every allocation row (unbounded; ops/debug only).
	cqlSelectAllAllocations = `SELECT network_id, pod_cidr, svc_cidr, allocated_at FROM cidr_allocations`

	// cqlDeleteAllocation removes an allocation for a network.
	// Parameters: (1) network_id UUID.
	cqlDeleteAllocation = `DELETE FROM cidr_allocations WHERE network_id = ?`
)

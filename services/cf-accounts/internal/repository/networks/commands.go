package networks

// CQL statements for the networks repository (default keyspace: cloudforge).
// Bind order matches each constant’s comment; types follow Scylla / gocql conventions.

const (
	// cqlInsertNetwork inserts one row into networks.
	// Parameters: (1) id UUID, (2) tenant_id UUID, (3) region text, (4) pod_cidr text, (5) svc_cidr text,
	// (6) status text, (7) created_at timestamp, (8) updated_at timestamp.
	cqlInsertNetwork = `INSERT INTO networks (id, tenant_id, region, pod_cidr, svc_cidr, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	// cqlInsertNetworkByTenant inserts the denormalized row in networks_by_tenant.
	// Parameters: (1) tenant_id UUID, (2) network_id UUID, (3) region text, (4) status text, (5) created_at timestamp.
	cqlInsertNetworkByTenant = `INSERT INTO networks_by_tenant (tenant_id, network_id, region, status, created_at) VALUES (?, ?, ?, ?, ?)`

	// cqlSelectNetworkByID loads a full network row by primary key.
	// Parameters: (1) id UUID.
	cqlSelectNetworkByID = `SELECT id, tenant_id, region, pod_cidr, svc_cidr, status, created_at, updated_at FROM networks WHERE id = ?`

	// cqlSelectNetworkIDsByTenant lists network ids for a tenant partition (newest first, capped by LIMIT).
	// Parameters: (1) tenant_id UUID, (2) limit int.
	cqlSelectNetworkIDsByTenant = `SELECT network_id, region, status, created_at FROM networks_by_tenant WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ?`

	// cqlSelectNetworksByIDInPrefix is the SELECT … WHERE id IN ( fragment. The repository appends
	// N comma-separated "?" placeholders and a closing ")"; bind values are N UUIDs in the same order.
	cqlSelectNetworksByIDInPrefix = `SELECT id, tenant_id, region, pod_cidr, svc_cidr, status, created_at, updated_at FROM networks WHERE id IN (`

	// cqlUpdateNetworkStatus updates status and updated_at on the primary networks row.
	// Parameters: (1) status text, (2) updated_at timestamp, (3) id UUID.
	cqlUpdateNetworkStatus = `UPDATE networks SET status = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateNetworkByTenantStatus updates denormalized status on networks_by_tenant (full primary key).
	// Parameters: (1) status text, (2) tenant_id UUID, (3) created_at timestamp, (4) network_id UUID.
	cqlUpdateNetworkByTenantStatus = `UPDATE networks_by_tenant SET status = ? WHERE tenant_id = ? AND created_at = ? AND network_id = ?`
)

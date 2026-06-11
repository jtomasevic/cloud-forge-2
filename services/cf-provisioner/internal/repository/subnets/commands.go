package subnets

// CQL for subnets and denormalized subnet lookup tables (keyspace cloudforge).
// Bind order matches each constant's comment.

const (
	// cqlInsertSubnet inserts the primary subnet row.
	// Parameters: (1) id UUID, (2) network_id UUID, (3) type text, (4) cidr text,
	// (5) zone text, (6) created_at timestamp.
	cqlInsertSubnet = `INSERT INTO subnets (id, network_id, type, cidr, zone, created_at) VALUES (?, ?, ?, ?, ?, ?)`

	// cqlInsertSubnetByNetwork inserts the network listing row.
	// Parameters: (1) network_id UUID, (2) subnet_id UUID, (3) type text, (4) cidr text,
	// (5) zone text, (6) created_at timestamp.
	cqlInsertSubnetByNetwork = `INSERT INTO subnets_by_network (network_id, subnet_id, type, cidr, zone, created_at) VALUES (?, ?, ?, ?, ?, ?)`

	// cqlInsertSubnetByNetworkCIDR inserts the duplicate-detection lookup row with LWT semantics.
	// Parameters: (1) network_id UUID, (2) cidr text, (3) subnet_id UUID, (4) type text,
	// (5) zone text, (6) created_at timestamp.
	cqlInsertSubnetByNetworkCIDR = `INSERT INTO subnets_by_network_cidr (network_id, cidr, subnet_id, type, zone, created_at) VALUES (?, ?, ?, ?, ?, ?) IF NOT EXISTS`

	// cqlSelectSubnetByID loads a primary subnet row by id.
	// Parameters: (1) id UUID.
	cqlSelectSubnetByID = `SELECT id, network_id, type, cidr, zone, created_at FROM subnets WHERE id = ?`

	// cqlSelectSubnetsByNetwork lists all subnets in a network partition.
	// Parameters: (1) network_id UUID.
	cqlSelectSubnetsByNetwork = `SELECT subnet_id, type, cidr, zone, created_at FROM subnets_by_network WHERE network_id = ?`

	// cqlSelectSubnetByNetworkCIDR resolves whether a network already owns a canonical CIDR.
	// Parameters: (1) network_id UUID, (2) cidr text.
	// cqlSelectSubnetByNetworkCIDR = `SELECT subnet_id, type, zone, created_at FROM subnets_by_network_cidr WHERE network_id = ? AND cidr = ?`
)

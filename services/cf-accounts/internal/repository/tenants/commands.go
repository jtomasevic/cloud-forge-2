package tenants

// CQL statements for the tenants repository (default keyspace: cloudforge).
// Bind order matches each constant’s comment; types follow Scylla / gocql conventions.

const (
	// cqlInsertTenant inserts one row into tenants.
	// Parameters: (1) id UUID, (2) account_id UUID, (3) slug text, (4) region text, (5) status text,
	// (6) created_at timestamp, (7) updated_at timestamp.
	cqlInsertTenant = `INSERT INTO tenants (id, account_id, slug, region, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`

	// cqlInsertTenantByAccount inserts the denormalized row in tenants_by_account.
	// Parameters: (1) account_id UUID, (2) tenant_id UUID, (3) slug text, (4) status text, (5) created_at timestamp.
	cqlInsertTenantByAccount = `INSERT INTO tenants_by_account (account_id, tenant_id, slug, status, created_at) VALUES (?, ?, ?, ?, ?)`

	// cqlInsertTenantBySlug inserts the slug lookup row in tenants_by_slug.
	// Parameters: (1) slug text, (2) tenant_id UUID, (3) account_id UUID, (4) status text.
	cqlInsertTenantBySlug = `INSERT INTO tenants_by_slug (slug, tenant_id, account_id, status) VALUES (?, ?, ?, ?)`

	// cqlSelectTenantByID loads a full tenant row by primary key.
	// Parameters: (1) id UUID.
	cqlSelectTenantByID = `SELECT id, account_id, slug, region, status, created_at, updated_at FROM tenants WHERE id = ?`

	// cqlSelectTenantBySlugLookup resolves slug to tenant_id, account_id, and denormalized status.
	// Parameters: (1) slug text.
	cqlSelectTenantBySlugLookup = `SELECT tenant_id, account_id, status FROM tenants_by_slug WHERE slug = ?`

	// cqlSelectTenantKeysByAccount lists tenant keys for an account partition (newest first, capped by LIMIT).
	// Parameters: (1) account_id UUID, (2) limit int — fetch window (offset+limit) before client-side slicing.
	cqlSelectTenantKeysByAccount = `SELECT tenant_id, slug, status, created_at FROM tenants_by_account WHERE account_id = ? ORDER BY created_at DESC LIMIT ?`

	// cqlSelectTenantsByIDInPrefix is the SELECT … WHERE id IN ( fragment. The repository appends
	// N comma-separated "?" placeholders and a closing ")"; bind values are N UUIDs in the same order.
	cqlSelectTenantsByIDInPrefix = `SELECT id, account_id, slug, region, status, created_at, updated_at FROM tenants WHERE id IN (`

	// cqlUpdateTenantStatus updates status and updated_at on the primary tenants row.
	// Parameters: (1) status text, (2) updated_at timestamp, (3) id UUID.
	cqlUpdateTenantStatus = `UPDATE tenants SET status = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateTenantBySlugStatus updates denormalized status on tenants_by_slug (partition key is slug).
	// Parameters: (1) status text, (2) slug text.
	cqlUpdateTenantBySlugStatus = `UPDATE tenants_by_slug SET status = ? WHERE slug = ?`

	// cqlUpdateTenantByAccountStatus updates denormalized status on tenants_by_account (full primary key).
	// Parameters: (1) status text, (2) account_id UUID, (3) created_at timestamp, (4) tenant_id UUID.
	cqlUpdateTenantByAccountStatus = `UPDATE tenants_by_account SET status = ? WHERE account_id = ? AND created_at = ? AND tenant_id = ?`
)

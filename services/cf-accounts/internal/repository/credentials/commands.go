package credentials

// CQL statements for the credentials repository (default keyspace: cloudforge).
// Bind order matches each constant’s comment; types follow Scylla / gocql conventions.

const (
	// cqlInsertAPIKey inserts one row into api_keys.
	// Parameters: (1) id UUID, (2) account_id UUID, (3) key_hash text, (4) key_prefix text,
	// (5) created_at timestamp, (6) revoked_at timestamp or null.
	cqlInsertAPIKey = `INSERT INTO api_keys (id, account_id, key_hash, key_prefix, created_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?)`

	// cqlInsertAPIKeyByAccount inserts the denormalized row in api_keys_by_account.
	// Parameters: (1) account_id UUID, (2) key_id UUID, (3) key_prefix text,
	// (4) created_at timestamp, (5) revoked_at timestamp or null.
	cqlInsertAPIKeyByAccount = `INSERT INTO api_keys_by_account (account_id, key_id, key_prefix, created_at, revoked_at) VALUES (?, ?, ?, ?, ?)`

	// cqlInsertAPIKeyByHash inserts the hash lookup row in api_keys_by_hash.
	// Parameters: (1) key_hash text, (2) key_id UUID, (3) account_id UUID, (4) revoked_at timestamp or null.
	cqlInsertAPIKeyByHash = `INSERT INTO api_keys_by_hash (key_hash, key_id, account_id, revoked_at) VALUES (?, ?, ?, ?)`

	// cqlSelectAPIKeyByID loads a full API key row by primary key.
	// Parameters: (1) id UUID.
	cqlSelectAPIKeyByID = `SELECT id, account_id, key_hash, key_prefix, created_at, revoked_at FROM api_keys WHERE id = ?`

	// cqlSelectAPIKeyByHashLookup resolves key_hash to key_id, account_id, and revoked_at (auth path).
	// Parameters: (1) key_hash text.
	cqlSelectAPIKeyByHashLookup = `SELECT key_id, account_id, revoked_at FROM api_keys_by_hash WHERE key_hash = ?`

	// cqlSelectAPIKeyIDsByAccount lists key metadata for an account partition (newest first, capped by LIMIT).
	// Parameters: (1) account_id UUID, (2) limit int.
	cqlSelectAPIKeyIDsByAccount = `SELECT key_id, key_prefix, created_at, revoked_at FROM api_keys_by_account WHERE account_id = ? ORDER BY created_at DESC LIMIT ?`

	// cqlSelectAPIKeysByIDInPrefix is the SELECT … WHERE id IN ( fragment. The repository appends
	// N comma-separated "?" placeholders and a closing ")"; bind values are N UUIDs in the same order.
	cqlSelectAPIKeysByIDInPrefix = `SELECT id, account_id, key_hash, key_prefix, created_at, revoked_at FROM api_keys WHERE id IN (`

	// cqlUpdateAPIKeyRevoke sets revoked_at on the primary api_keys row.
	// Parameters: (1) revoked_at timestamp, (2) id UUID.
	cqlUpdateAPIKeyRevoke = `UPDATE api_keys SET revoked_at = ? WHERE id = ?`

	// cqlUpdateAPIKeyByHashRevoke sets revoked_at on api_keys_by_hash (partition key is key_hash).
	// Parameters: (1) revoked_at timestamp, (2) key_hash text.
	cqlUpdateAPIKeyByHashRevoke = `UPDATE api_keys_by_hash SET revoked_at = ? WHERE key_hash = ?`

	// cqlUpdateAPIKeyByAccountRevoke sets revoked_at on the denormalized row (full primary key).
	// Parameters: (1) revoked_at timestamp, (2) account_id UUID, (3) created_at timestamp, (4) key_id UUID.
	cqlUpdateAPIKeyByAccountRevoke = `UPDATE api_keys_by_account SET revoked_at = ? WHERE account_id = ? AND created_at = ? AND key_id = ?`
)

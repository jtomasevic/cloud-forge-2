package accounts

// CQL statements for the accounts repository (default keyspace: cloudforge).
// Bind order matches each constant’s comment; types follow Scylla / gocql conventions.

const (
	// cqlInsertAccount inserts one row into accounts.
	// Parameters: (1) id UUID, (2) email text, (3) status text, (4) password_hash text (bcrypt),
	// (5) created_at timestamp, (6) updated_at timestamp.
	cqlInsertAccount = `INSERT INTO accounts (id, email, status, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`

	// cqlInsertAccountByEmail inserts the denormalized lookup row in accounts_by_email.
	// Parameters: (1) email text, (2) account_id UUID, (3) status text.
	cqlInsertAccountByEmail = `INSERT INTO accounts_by_email (email, account_id, status) VALUES (?, ?, ?)`

	// cqlSelectAccountByID loads a full account row by primary key.
	// Parameters: (1) id UUID.
	cqlSelectAccountByID = `SELECT id, email, status, password_hash, created_at, updated_at FROM accounts WHERE id = ?`

	// cqlSelectAccountByEmailLookup resolves email to account_id and denormalized status.
	// Parameters: (1) email text.
	cqlSelectAccountByEmailLookup = `SELECT account_id, status FROM accounts_by_email WHERE email = ?`

	// cqlSelectAccountsList scans accounts with a row cap (used with ALLOW FILTERING in v1).
	// Parameters: (1) limit int — maximum rows to read from the coordinator before client-side offset slicing.
	cqlSelectAccountsList = `SELECT id, email, status, password_hash, created_at, updated_at FROM accounts LIMIT ? ALLOW FILTERING`

	// cqlUpdateAccountStatus updates status and updated_at on the primary accounts row.
	// Parameters: (1) status text, (2) updated_at timestamp, (3) id UUID.
	cqlUpdateAccountStatus = `UPDATE accounts SET status = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateAccountByEmailStatus updates denormalized status on accounts_by_email.
	// Parameters: (1) status text, (2) email text (partition key).
	cqlUpdateAccountByEmailStatus = `UPDATE accounts_by_email SET status = ? WHERE email = ?`
)

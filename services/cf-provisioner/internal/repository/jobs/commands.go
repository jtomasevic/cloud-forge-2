package jobs

// CQL for provisioning_jobs and provisioning_jobs_by_network (keyspace cloudforge).

const (
	// cqlInsertJob inserts the primary job row. tenant_id is reserved for future use; v1 stores all-zero UUID.
	// Parameters: (1) id UUID, (2) tenant_id UUID, (3) network_id UUID, (4) job_type text, (5) status text,
	// (6) error_message text, (7) created_at timestamp, (8) updated_at timestamp.
	cqlInsertJob = `INSERT INTO provisioning_jobs (id, tenant_id, network_id, job_type, status, error_message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	// cqlInsertJobByNetwork inserts the denormalized listing row (clustering newest-first on created_at).
	// Parameters: (1) network_id UUID, (2) job_id UUID, (3) job_type text, (4) status text, (5) created_at timestamp.
	cqlInsertJobByNetwork = `INSERT INTO provisioning_jobs_by_network (network_id, job_id, job_type, status, created_at) VALUES (?, ?, ?, ?, ?)`

	// cqlSelectJobByID loads a job by primary key.
	// Parameters: (1) id UUID.
	cqlSelectJobByID = `SELECT id, tenant_id, network_id, job_type, status, error_message, created_at, updated_at FROM provisioning_jobs WHERE id = ?`

	// cqlSelectJobsByNetwork lists denormalized rows for a network partition (newest first via clustering order).
	// Parameters: (1) network_id UUID.
	cqlSelectJobsByNetwork = `SELECT job_id, job_type, status, created_at FROM provisioning_jobs_by_network WHERE network_id = ?`

	// cqlUpdateJobStatus updates status, optional error text, and updated_at on the primary row.
	// Parameters: (1) status text, (2) error_message text, (3) updated_at timestamp, (4) id UUID.
	cqlUpdateJobStatus = `UPDATE provisioning_jobs SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateJobByNetworkStatus updates denormalized status (full partition key).
	// Parameters: (1) status text, (2) network_id UUID, (3) created_at timestamp, (4) job_id UUID.
	cqlUpdateJobByNetworkStatus = `UPDATE provisioning_jobs_by_network SET status = ? WHERE network_id = ? AND created_at = ? AND job_id = ?`
)

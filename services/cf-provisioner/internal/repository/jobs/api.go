package jobs

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

// JobsRepository persists async provisioning jobs and their per-network index rows.
type JobsRepository interface {
	// Create inserts a new provisioning job with status "pending".
	Create(ctx context.Context, params CreateJobParams) (Job, error)

	// Get returns a job by ID.
	Get(ctx context.Context, jobID string) (Job, error)

	// ListByNetwork returns all jobs for a network, ordered by creation time (newest first).
	ListByNetwork(ctx context.Context, networkID string) ([]Job, error)

	// UpdateStatus updates a job's status and optionally sets an error message.
	UpdateStatus(ctx context.Context, jobID string, status JobStatus, errorMsg string) error
}

// New returns a JobsRepository backed by the given ScyllaDB session (keyspace cloudforge).
func New(session *scylladbclient.Session) JobsRepository {
	return &jobsRepository{session: session}
}
